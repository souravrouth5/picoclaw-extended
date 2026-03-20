// Package openrouter provides a self-managing wrapper around the OpenRouter API.
// It fetches free models, categorises them, picks the best coding/large-context
// default, and handles fallback, cooldown, race-dispatch, caching, and health checks.
package openrouter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/providers/openai_compat"
)

const (
	openRouterAPIBase = "https://openrouter.ai/api/v1"

	// How often to re-fetch the free model list to detect paid transitions.
	modelRefreshInterval = 6 * time.Hour

	// Per-request timeout applied on top of any caller context.
	defaultRequestTimeout = 90 * time.Second

	// Number of top models to race in parallel for chat requests.
	raceWidth = 3
)

// Provider is the main OpenRouter wrapper.
// It satisfies providers.LLMProvider and can be dropped in wherever
// the existing picoclaw code expects one.
type Provider struct {
	apiKey string
	cache  *ResponseCache
	health *HealthTracker

	mu        sync.RWMutex
	models    []ModelInfo // sorted best-first, free only
	defaultID string

	// delegate is the single shared openai_compat provider for all calls.
	// OpenRouter accepts the model ID in the request body, so one HTTP client
	// with the same base URL and API key serves all models.
	delegate *openai_compat.Provider

	stopCh chan struct{}
}

// Config holds construction options.
type Config struct {
	APIKey         string
	CacheTTL       time.Duration // 0 = 5 min default
	RequestTimeout time.Duration // 0 = 90 s default
}

// New creates a Provider, fetches the initial free model list, and starts
// the background refresh loop.
func New(cfg Config) (*Provider, error) {
	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}

	p := &Provider{
		apiKey: cfg.APIKey,
		cache:  NewResponseCache(cfg.CacheTTL),
		health: NewHealthTracker(),
		stopCh: make(chan struct{}),
		delegate: openai_compat.NewProvider(
			cfg.APIKey,
			openRouterAPIBase,
			"",
			openai_compat.WithRequestTimeout(timeout),
		),
	}

	if err := p.refreshModels(); err != nil {
		return nil, fmt.Errorf("openrouter: initial model fetch failed: %w", err)
	}

	go p.refreshLoop()
	go p.healthCheckLoop()

	return p, nil
}

// Close stops background goroutines.
func (p *Provider) Close() {
	close(p.stopCh)
}

// GetDefaultModel returns the current best free model ID.
func (p *Provider) GetDefaultModel() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.defaultID
}

// Models returns a snapshot of the current free model list.
func (p *Provider) Models() []ModelInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]ModelInfo, len(p.models))
	copy(out, p.models)
	return out
}

// Chat implements providers.LLMProvider.
// Strategy:
//  1. Check cache — return immediately on hit.
//  2. Race the top-N available models in parallel; first success wins.
//  3. If all racers fail, fall back sequentially through the remaining list.
//  4. Record health outcomes; demote paid models permanently.
func (p *Provider) Chat(
	ctx context.Context,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
) (*providers.LLMResponse, error) {
	modelID := p.resolveModel(model)

	// 1. Cache check (only for tool-free requests).
	if len(tools) == 0 {
		if cached, ok := p.cache.Get(modelID, messages); ok {
			return cached, nil
		}
	}

	// 2. Build ordered candidate list.
	candidates := p.candidateList(modelID)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("openrouter: no available free models")
	}

	// 3. Race top-N, then sequential fallback for the rest.
	resp, usedModel, err := p.dispatchWithFallback(ctx, candidates, messages, tools, options)
	if err != nil {
		return nil, err
	}

	// 4. Cache successful response.
	if len(tools) == 0 {
		p.cache.Set(usedModel, messages, resp)
	}
	return resp, nil
}

// ─── internal ────────────────────────────────────────────────────────────────

func (p *Provider) resolveModel(requested string) string {
	if requested == "" {
		return p.GetDefaultModel()
	}
	// Full OpenRouter model ID (contains "/") — use directly.
	if strings.Contains(requested, "/") {
		return requested
	}
	// Logical alias — fall back to default.
	return p.GetDefaultModel()
}

// candidateList returns an ordered slice of model IDs to try.
// The requested model is first; remaining available models follow.
func (p *Provider) candidateList(primary string) []string {
	p.mu.RLock()
	all := ModelIDs(p.models)
	p.mu.RUnlock()

	available := p.health.FilterAvailable(all)

	// Ensure primary is first.
	ordered := make([]string, 0, len(available))
	hasPrimary := false
	for _, id := range available {
		if id == primary {
			hasPrimary = true
		} else {
			ordered = append(ordered, id)
		}
	}
	if hasPrimary {
		ordered = append([]string{primary}, ordered...)
	}
	if len(ordered) == 0 && primary != "" {
		ordered = []string{primary} // last resort
	}
	return ordered
}

// dispatchWithFallback races the first raceWidth candidates, then falls back
// sequentially through the rest.
func (p *Provider) dispatchWithFallback(
	ctx context.Context,
	candidates []string,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	options map[string]any,
) (*providers.LLMResponse, string, error) {
	racers := candidates
	rest := []string(nil)
	if len(candidates) > raceWidth {
		racers = candidates[:raceWidth]
		rest = candidates[raceWidth:]
	}

	// Race phase.
	resp, winner, err := p.race(ctx, racers, messages, tools, options)
	if err == nil {
		return resp, winner, nil
	}

	// Sequential fallback phase.
	for _, id := range rest {
		resp, err = p.callOne(ctx, id, messages, tools, options)
		if err == nil {
			return resp, id, nil
		}
	}

	return nil, "", fmt.Errorf("openrouter: all %d candidates failed; last error: %w", len(candidates), err)
}

// race fires up to raceWidth goroutines and returns the first success.
func (p *Provider) race(
	ctx context.Context,
	ids []string,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	options map[string]any,
) (*providers.LLMResponse, string, error) {
	type result struct {
		resp    *providers.LLMResponse
		modelID string
		err     error
	}

	raceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan result, len(ids))
	for _, id := range ids {
		go func(modelID string) {
			resp, err := p.callOne(raceCtx, modelID, messages, tools, options)
			ch <- result{resp, modelID, err}
		}(id)
	}

	var lastErr error
	for range ids {
		r := <-ch
		if r.err == nil {
			cancel() // stop remaining goroutines
			return r.resp, r.modelID, nil
		}
		lastErr = r.err
	}
	return nil, "", lastErr
}

// callOne makes a single Chat call to one model, records health, and detects
// paid-model transitions (HTTP 402 / billing keywords in error body).
func (p *Provider) callOne(
	ctx context.Context,
	modelID string,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	options map[string]any,
) (*providers.LLMResponse, error) {
	start := time.Now()
	resp, err := p.delegate.Chat(ctx, messages, tools, modelID, options)
	elapsed := time.Since(start)

	if err != nil {
		isPaid := isPaidError(err)
		p.health.RecordFailure(modelID, isPaid)
		if isPaid {
			p.cache.Invalidate(modelID)
			p.demoteModel(modelID)
		}
		return nil, err
	}

	p.health.RecordSuccess(modelID, elapsed)
	return resp, nil
}

// isPaidError returns true when the error indicates the model is no longer free.
func isPaidError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "402") ||
		strings.Contains(msg, "payment required") ||
		strings.Contains(msg, "insufficient credits") ||
		strings.Contains(msg, "billing")
}

// demoteModel removes a model from the active list when it becomes paid.
func (p *Provider) demoteModel(modelID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	filtered := make([]ModelInfo, 0, len(p.models))
	for _, m := range p.models {
		if m.ID != modelID {
			filtered = append(filtered, m)
		}
	}
	p.models = filtered
	if p.defaultID == modelID {
		p.defaultID = BestDefault(p.models)
	}
}

// refreshModels fetches the current free model list from OpenRouter.
func (p *Provider) refreshModels() error {
	models, err := FetchFreeModels(p.apiKey)
	if err != nil {
		return err
	}

	// Remove any models already known to be paid.
	live := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		if !p.health.BecamePaid(m.ID) {
			live = append(live, m)
		}
	}

	p.mu.Lock()
	p.models = live
	p.defaultID = BestDefault(live)
	p.mu.Unlock()
	return nil
}

// refreshLoop periodically re-fetches the free model list.
func (p *Provider) refreshLoop() {
	ticker := time.NewTicker(modelRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			_ = p.refreshModels() // errors are non-fatal; keep existing list
		}
	}
}

// healthCheckLoop pings each model with a minimal request every 10 minutes
// to proactively detect paid transitions and update latency estimates.
func (p *Provider) healthCheckLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.runHealthChecks()
		}
	}
}

func (p *Provider) runHealthChecks() {
	ids := ModelIDs(p.Models())
	probe := []providers.Message{{Role: "user", Content: "hi"}}

	for _, id := range ids {
		if p.health.BecamePaid(id) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		_, _ = p.callOne(ctx, id, probe, nil, map[string]any{"max_tokens": 1})
		cancel()
	}
}
