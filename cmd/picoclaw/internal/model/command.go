package model

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	"github.com/sipeed/picoclaw/pkg/config"
)

// orAutoAlias is the special name users pass to switch back to OpenRouter auto-bootstrap.
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
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := internal.GetConfigPath()
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			if len(args) == 0 {
				printCurrentModel(cfg)
				return nil
			}
			return useModel(configPath, cfg, args[0])
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
			printModelList(cfg)
			return nil
		},
	}
}

func newUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <model_name>",
		Short: "Set the default model",
		Long: `Set the default model by its model_name from model_list.

Use 'openrouter' to clear the default and let OpenRouter auto-bootstrap
pick the best free model on next run.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := internal.GetConfigPath()
			cfg, err := config.LoadConfig(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}
			return useModel(configPath, cfg, args[0])
		},
	}
}

// printCurrentModel shows the active default and the full model list.
func printCurrentModel(cfg *config.Config) {
	current := cfg.Agents.Defaults.GetModelName()
	if current == "" {
		fmt.Println("Default model: (none — OpenRouter auto-bootstrap will run on next start)")
	} else {
		fmt.Printf("Default model: %s\n", current)
		// Show the underlying model string if available
		for _, m := range cfg.ModelList {
			if m.ModelName == current {
				fmt.Printf("  → %s\n", m.Model)
				break
			}
		}
	}
	fmt.Println()
	printModelList(cfg)
}

// printModelList prints all models from model_list, marking the active default.
func printModelList(cfg *config.Config) {
	current := cfg.Agents.Defaults.GetModelName()
	hasOR := cfg.Providers.OpenRouter.APIKey != ""

	// Collect user-defined models (exclude bootstrap-injected entries)
	type row struct {
		name   string
		model  string
		hasKey bool
	}
	var rows []row
	seen := map[string]bool{}
	for _, m := range cfg.ModelList {
		if seen[m.ModelName] {
			continue
		}
		seen[m.ModelName] = true
		rows = append(rows, row{m.ModelName, m.Model, m.APIKey != ""})
	}

	if len(rows) == 0 && !hasOR {
		fmt.Println("No models configured.")
		fmt.Println("Add entries to model_list in config.json, or set providers.openrouter.api_key")
		fmt.Println("for automatic free model selection.")
		return
	}

	fmt.Println("Configured models:")
	for _, r := range rows {
		marker := "  "
		if r.name == current {
			marker = "▶ "
		}
		keyStatus := ""
		if !r.hasKey {
			keyStatus = "  (no api_key)"
		}
		fmt.Printf("%s%-30s  %s%s\n", marker, r.name, r.model, keyStatus)
	}

	if hasOR {
		fmt.Println()
		if current == "" {
			fmt.Println("▶ openrouter  (auto-bootstrap active — best free model selected at startup)")
		} else {
			fmt.Println("  openrouter  (available — run 'model use openrouter' to switch)")
		}
	}

	fmt.Println()
	fmt.Println("Use 'picoclawx model use <name>' to change the default.")
}

// useModel validates and persists the new default model.
func useModel(configPath string, cfg *config.Config, name string) error {
	// Strip bootstrap entries before saving so they don't get persisted.
	config.StripBootstrappedModels(cfg)

	old := cfg.Agents.Defaults.GetModelName()
	if old == "" {
		old = "(openrouter auto)"
	}

	if strings.ToLower(name) == orAutoAlias {
		// Clear model_name → bootstrap will run on next start
		cfg.Agents.Defaults.ModelName = ""
		cfg.Agents.Defaults.Model = ""
		if err := save(configPath, cfg); err != nil {
			return err
		}
		fmt.Printf("✓ Default model cleared (was: %s)\n", old)
		fmt.Println("  OpenRouter auto-bootstrap will pick the best free model on next start.")
		return nil
	}

	// Validate the name exists in model_list with an api_key
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
		fmt.Printf("Model %q not found in model_list. Available models:\n\n", name)
		printModelList(cfg)
		return fmt.Errorf("unknown model %q", name)
	}

	cfg.Agents.Defaults.ModelName = name
	cfg.Agents.Defaults.Model = ""
	if err := save(configPath, cfg); err != nil {
		return err
	}
	fmt.Printf("✓ Default model: %s → %s\n", old, name)
	return nil
}

func save(configPath string, cfg *config.Config) error {
	if err := config.SaveConfig(configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	config.InjectProvidersPlaceholder(configPath)
	return nil
}
