// Package models provides free model fetching from the OpenRouter API.
// It has no internal picoclaw imports so it can be used by pkg/config
// without creating an import cycle.
package models

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

const (
	OpenRouterModelsURL = "https://openrouter.ai/api/v1/models"

	TierLargeContext = 3
	TierCoding       = 2
	TierGeneral      = 1
)

// ModelInfo is the subset of the OpenRouter /models response we care about.
type ModelInfo struct {
	ID            string
	Name          string
	ContextLength int
	IsFree        bool
	Tier          int
}

type openRouterModelsResponse struct {
	Data []struct {
		ID            string `json:"id"`
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
		Pricing       struct {
			Prompt string `json:"prompt"`
		} `json:"pricing"`
	} `json:"data"`
}

// FetchFreeModels calls the OpenRouter models endpoint and returns only free
// models sorted best-first (large-context coding models first).
func FetchFreeModels(apiKey string) ([]ModelInfo, error) {
	req, err := http.NewRequest(http.MethodGet, OpenRouterModelsURL, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter models fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("openrouter models fetch: status %d: %s", resp.StatusCode, body)
	}

	var raw openRouterModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("openrouter models decode: %w", err)
	}

	var free []ModelInfo
	for _, m := range raw.Data {
		if m.Pricing.Prompt != "0" {
			continue
		}
		free = append(free, ModelInfo{
			ID:            m.ID,
			Name:          m.Name,
			ContextLength: m.ContextLength,
			IsFree:        true,
			Tier:          Classify(m.ID, m.Name, m.ContextLength),
		})
	}

	slices.SortFunc(free, func(a, b ModelInfo) int {
		if n := cmp.Compare(b.Tier, a.Tier); n != 0 {
			return n
		}
		if n := cmp.Compare(b.ContextLength, a.ContextLength); n != 0 {
			return n
		}
		return cmp.Compare(a.ID, b.ID)
	})

	return free, nil
}

// Classify assigns a tier based on model id/name and context length.
func Classify(id, name string, contextLen int) int {
	lower := strings.ToLower(id + " " + name)
	for _, kw := range []string{"coder", "code", "codex", "deepseek-coder", "qwen-coder", "starcoder", "codestral", "devstral", "coding"} {
		if strings.Contains(lower, kw) {
			if contextLen >= 64_000 {
				return TierLargeContext
			}
			return TierCoding
		}
	}
	if contextLen >= 128_000 {
		return TierLargeContext
	}
	return TierGeneral
}

// BestDefault returns the ID of the top-ranked model.
func BestDefault(ms []ModelInfo) string {
	if len(ms) == 0 {
		return ""
	}
	return ms[0].ID
}
