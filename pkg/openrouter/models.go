package openrouter

import (
	ormodels "github.com/sipeed/picoclaw/pkg/openrouter/models"
)

// Re-export types and functions from the cycle-free sub-package.
type ModelInfo = ormodels.ModelInfo

func FetchFreeModels(apiKey string) ([]ModelInfo, error) {
	return ormodels.FetchFreeModels(apiKey)
}

func BestDefault(ms []ModelInfo) string {
	return ormodels.BestDefault(ms)
}

func ModelIDs(ms []ModelInfo) []string {
	ids := make([]string, len(ms))
	for i, m := range ms {
		ids[i] = m.ID
	}
	return ids
}
