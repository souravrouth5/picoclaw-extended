package openrouter

import (
	"sync"
	"time"
)

// healthState tracks runtime health for a single model.
type healthState struct {
	mu           sync.Mutex
	healthy      bool
	cooldownEnd  time.Time
	errorCount   int
	latencyEWMA  float64 // milliseconds, exponential weighted moving average
	becamePaid   bool    // true once a 402 / pricing change is detected
	lastChecked  time.Time
}

// HealthTracker manages health state for all active models.
type HealthTracker struct {
	mu     sync.RWMutex
	states map[string]*healthState
}

// NewHealthTracker creates an empty tracker.
func NewHealthTracker() *HealthTracker {
	return &HealthTracker{states: make(map[string]*healthState)}
}

func (ht *HealthTracker) get(modelID string) *healthState {
	ht.mu.RLock()
	s := ht.states[modelID]
	ht.mu.RUnlock()
	if s != nil {
		return s
	}
	ht.mu.Lock()
	defer ht.mu.Unlock()
	if s = ht.states[modelID]; s == nil {
		s = &healthState{healthy: true}
		ht.states[modelID] = s
	}
	return s
}

// IsAvailable returns true when the model is healthy and not in cooldown.
func (ht *HealthTracker) IsAvailable(modelID string) bool {
	s := ht.get(modelID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.becamePaid {
		return false
	}
	if !s.cooldownEnd.IsZero() && time.Now().Before(s.cooldownEnd) {
		return false
	}
	return s.healthy
}

// RecordSuccess resets error count and updates latency EWMA.
func (ht *HealthTracker) RecordSuccess(modelID string, latency time.Duration) {
	s := ht.get(modelID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthy = true
	s.errorCount = 0
	s.cooldownEnd = time.Time{}
	s.lastChecked = time.Now()
	ms := float64(latency.Milliseconds())
	if s.latencyEWMA == 0 {
		s.latencyEWMA = ms
	} else {
		const alpha = 0.2
		s.latencyEWMA = alpha*ms + (1-alpha)*s.latencyEWMA
	}
}

// RecordFailure increments error count and sets exponential cooldown.
// If the error is a 402 (billing/paid), the model is permanently demoted.
func (ht *HealthTracker) RecordFailure(modelID string, isPaid bool) {
	s := ht.get(modelID)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastChecked = time.Now()
	if isPaid {
		s.becamePaid = true
		s.healthy = false
		return
	}
	s.errorCount++
	s.healthy = false
	// Exponential backoff: 10s, 30s, 90s, 270s, cap 600s
	backoff := time.Duration(10<<min(s.errorCount-1, 5)) * time.Second
	if backoff > 600*time.Second {
		backoff = 600 * time.Second
	}
	s.cooldownEnd = time.Now().Add(backoff)
}

// MarkPaid permanently removes a model from the free pool.
func (ht *HealthTracker) MarkPaid(modelID string) {
	ht.RecordFailure(modelID, true)
}

// AvgLatency returns the EWMA latency in milliseconds for a model.
func (ht *HealthTracker) AvgLatency(modelID string) float64 {
	s := ht.get(modelID)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.latencyEWMA
}

// BecamePaid returns true if the model was detected as no longer free.
func (ht *HealthTracker) BecamePaid(modelID string) bool {
	s := ht.get(modelID)
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.becamePaid
}

// FilterAvailable returns only the model IDs that are currently available.
func (ht *HealthTracker) FilterAvailable(ids []string) []string {
	out := ids[:0:0]
	for _, id := range ids {
		if ht.IsAvailable(id) {
			out = append(out, id)
		}
	}
	return out
}
