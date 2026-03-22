package model

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/pkg/config"
)

const orAutoAlias = "openrouter"

func NewModelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model [model_name]",
		Short: "Show or change the default model",
		Long: `Show or change the default model.

Without arguments, shows the current default model and all configured models.
With a model name, sets it as the default (shorthand for 'model use <name>').

Subcommands:
  list          List all configured models
  use <name>    Set the default model by name

Special values for 'use':
  openrouter    Clear the default model so OpenRouter auto-bootstrap picks
                the best free model on next run (requires providers.openrouter.api_key)

Examples:
  picoclawx model                        # show current default + all models
  picoclawx model list                   # list all configured models
  picoclawx model use my-gpt4o           # set my-gpt4o as default
  picoclawx model use openrouter         # switch to OpenRouter auto-bootstrap`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("accepts at most 1 arg(s), received %d", len(args))
			}
			configPath := internal.GetConfigPath()
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			if len(args) == 0 {
				showCurrentModel(cfg)
				return nil
			}
			return setDefaultModel(configPath, cfg, args[0])
		},
	}

	cmd.AddCommand(newListCommand(), newUseCommand())
	return cmd
}

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured models",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig(internal.GetConfigPath())
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			listAvailableModels(cfg)
			return nil
		},
	}
}

func newUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <model_name>",
		Short: "Set the default model",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := internal.GetConfigPath()
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			return setDefaultModel(configPath, cfg, args[0])
		},
	}
}

func showCurrentModel(cfg *config.Config) {
	current := cfg.Agents.Defaults.GetModelName()
	if current == "" {
		fmt.Println("No default model is currently set.")
		fmt.Println("OpenRouter auto-bootstrap will pick the best free model on next start.")
	} else {
		fmt.Printf("Current default model: %s\n", current)
		for _, m := range cfg.ModelList {
			if m.ModelName == current {
				fmt.Printf("  → %s\n", m.Model)
				break
			}
		}
	}
	fmt.Println()
	fmt.Println("Available models in your config:")
	listAvailableModels(cfg)
}

func listAvailableModels(cfg *config.Config) {
	current := cfg.Agents.Defaults.GetModelName()

	type row struct {
		name  string
		model string
	}
	var rows []row
	seen := map[string]bool{}
	for _, m := range cfg.ModelList {
		if seen[m.ModelName] || m.APIKey == "" {
			continue
		}
		seen[m.ModelName] = true
		rows = append(rows, row{m.ModelName, m.Model})
	}

	if len(rows) == 0 {
		fmt.Println("No models configured in model_list.")
		fmt.Println("Add entries to model_list in config.json, or set providers.openrouter.api_key")
		fmt.Println("for automatic free model selection.")
		return
	}

	for _, r := range rows {
		marker := "  "
		if r.name == current {
			marker = "> "
		}
		fmt.Printf("%s- %s (%s)\n", marker, r.name, r.model)
	}

	if cfg.Providers.OpenRouter.APIKey != "" {
		fmt.Println()
		if current == "" {
			fmt.Println("> openrouter  (auto-bootstrap active)")
		} else {
			fmt.Println("  openrouter  (run 'model use openrouter' to switch)")
		}
	}
}

func setDefaultModel(configPath string, cfg *config.Config, name string) error {
	config.StripBootstrappedModels(cfg)

	old := formatModelName(cfg.Agents.Defaults.GetModelName())

	if strings.ToLower(name) == orAutoAlias {
		cfg.Agents.Defaults.ModelName = ""
		cfg.Agents.Defaults.Model = ""
		if err := save(configPath, cfg); err != nil {
			return err
		}
		fmt.Printf("Default model cleared (was: %s)\n", old)
		fmt.Println("OpenRouter auto-bootstrap will pick the best free model on next start.")
		return nil
	}

	found := false
	for _, m := range cfg.ModelList {
		if m.ModelName == name {
			if m.APIKey == "" {
				return fmt.Errorf("model %q has no api_key configured", name)
			}
			found = true
			break
		}
	}
	if !found {
		listAvailableModels(cfg)
		return fmt.Errorf("model %q not found in model_list", name)
	}

	cfg.Agents.Defaults.ModelName = name
	cfg.Agents.Defaults.Model = ""
	if err := save(configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("Default model changed from '%s' to '%s'\n", old, name)
	return nil
}

func formatModelName(name string) string {
	if name == "" {
		return "(none)"
	}
	return name
}

func save(configPath string, cfg *config.Config) error {
	if err := config.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	config.InjectProvidersPlaceholder(configPath)
	return nil
}
