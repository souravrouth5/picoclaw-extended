package config

import (
	"fmt"
	"os"
	"strings"

	ormodels "github.com/sipeed/picoclaw/pkg/openrouter/models"
)

const orFreeDefaultAlias = "or-free-default"

func bootstrapOpenRouter(cfg *Config) error {
	apiKey := resolveOpenRouterKey(cfg)
	if apiKey == "" {
		return nil
	}
	if hasActiveModel(cfg) {
		return nil
	}

	models, err := ormodels.FetchFreeModels(apiKey)
	if err != nil {
		return fmt.Errorf("could not fetch free models: %w", err)
	}
	if len(models) == 0 {
		return fmt.Errorf("no free models returned")
	}

	injectFreeModels(cfg, models, apiKey)
	return nil
}

func resolveOpenRouterKey(cfg *Config) string {
	if cfg.Providers.OpenRouter.APIKey != "" {
		return cfg.Providers.OpenRouter.APIKey
	}
	for _, m := range cfg.ModelList {
		if strings.HasPrefix(strings.ToLower(m.Model), "openrouter/") && m.APIKey != "" {
			return m.APIKey
		}
	}
	return ""
}

func hasActiveModel(cfg *Config) bool {
	name := cfg.Agents.Defaults.GetModelName()
	if name == "" {
		return false
	}
	for _, m := range cfg.ModelList {
		if m.ModelName == name && m.APIKey != "" {
			return true
		}
	}
	return false
}

func injectFreeModels(cfg *Config, models []ormodels.ModelInfo, apiKey string) {
	entries := make([]ModelConfig, len(models))
	for i, m := range models {
		name := m.ID
		if i == 0 {
			name = orFreeDefaultAlias
		}
		entries[i] = ModelConfig{
			ModelName:      name,
			Model:          "openrouter/" + m.ID,
			APIKey:         apiKey,
			RequestTimeout: 90,
		}
	}

	if len(entries) > 1 {
		fallbacks := make([]string, len(entries)-1)
		for i, e := range entries[1:] {
			fallbacks[i] = e.ModelName
		}
		entries[0].Fallbacks = fallbacks
	}

	cfg.ModelList = append(entries, cfg.ModelList...)
	cfg.Agents.Defaults.ModelName = orFreeDefaultAlias
}

func runBootstrap(cfg *Config) {
	if err := bootstrapOpenRouter(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "picoclaw: openrouter bootstrap: %v\n", err)
	}
}
