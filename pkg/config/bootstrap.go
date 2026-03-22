package config

import (
	"fmt"
	"os"
	"strings"

	ormodels "github.com/sipeed/picoclaw/pkg/openrouter/models"
)

const orFreeDefaultAlias = "or-free-default"

// StripBootstrappedModels removes any model_list entries that were injected
// at runtime by the OpenRouter bootstrap. Call this before saving config to disk
// so re-running onboard doesn't permanently write bootstrap entries.
func StripBootstrappedModels(cfg *Config) {
	// Collect the set of bootstrap-injected model names by finding or-free-default
	// and its declared fallbacks.
	bootstrap := make(map[string]bool)
	for _, m := range cfg.ModelList {
		if m.ModelName == orFreeDefaultAlias {
			bootstrap[orFreeDefaultAlias] = true
			for _, fb := range m.Fallbacks {
				bootstrap[fb] = true
			}
			break
		}
	}
	if len(bootstrap) == 0 {
		return
	}
	filtered := cfg.ModelList[:0]
	for _, m := range cfg.ModelList {
		if !bootstrap[m.ModelName] {
			filtered = append(filtered, m)
		}
	}
	cfg.ModelList = filtered
	if cfg.Agents.Defaults.ModelName == orFreeDefaultAlias {
		cfg.Agents.Defaults.ModelName = ""
	}
}

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

	// Disable media tools if no free model supports vision/image input.
	// Sending images or files to a text-only model produces an API error.
	hasVision := false
	for _, m := range models {
		if m.SupportsVision {
			hasVision = true
			break
		}
	}
	if !hasVision {
		cfg.Tools.SendFile.Enabled = false
		cfg.Tools.ReadFile.Enabled = false
		fmt.Fprintf(os.Stderr, "picoclaw: no free vision models available — send_file and read_file tools disabled\n")
	}
}

func runBootstrap(cfg *Config) {
	if err := bootstrapOpenRouter(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "picoclaw: openrouter bootstrap: %v\n", err)
	}
}

// InjectProvidersPlaceholder reads the saved config JSON and ensures the
// providers.openrouter.api_key placeholder is present so users see exactly
// where to put their key. Safe to call after every SaveConfig — it is a
// no-op when a real key is already written.
func InjectProvidersPlaceholder(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	s := string(data)
	const placeholder = `"providers": {
    "openrouter": {
      "api_key": ""
    }
  },
  `
	if strings.Contains(s, `"providers": null`) {
		s = strings.Replace(s, `"providers": null`, strings.TrimRight(placeholder, ",\n "), 1)
	} else if !strings.Contains(s, `"providers"`) {
		s = strings.Replace(s, `"model_list"`, placeholder+`"model_list"`, 1)
	}
	_ = os.WriteFile(configPath, []byte(s), 0o600)
}
