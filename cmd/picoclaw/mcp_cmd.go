package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sipeed/picoclaw/cmd/picoclaw/internal"
	mcpmgr "github.com/sipeed/picoclaw/cmd/picoclaw/internal/mcp"
	"github.com/sipeed/picoclaw/pkg/config"
)

func NewMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage embedded MCP servers",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newMCPListCommand(),
		newMCPEnableCommand(),
		newMCPDisableCommand(),
		newMCPDepsCommand(),
		newMCPInstallDepsCommand(),
	)

	return cmd
}

func newMCPListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available MCP servers and their status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := internal.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			mcpmgr.ListServers(cfg)
			return nil
		},
	}
}

func newMCPEnableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <server>",
		Short: "Extract, check deps, and enable an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home := internal.GetPicoclawHome()
			configPath := internal.GetConfigPath()
			// Ensure config exists before patching.
			if _, err := config.LoadConfig(configPath); err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			return mcpmgr.EnableServer(home, configPath, args[0])
		},
	}
}

func newMCPDisableCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <server>",
		Short: "Disable an MCP server and remove it from config",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath := internal.GetConfigPath()
			return mcpmgr.DisableServer(configPath, args[0])
		},
	}
}

func newMCPDepsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deps <server>",
		Short: "Show dependency requirements for an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return mcpmgr.ShowDeps(args[0])
		},
	}
}

func newMCPInstallDepsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install-deps <server>",
		Short: "Run the dependency install script for an MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			home := internal.GetPicoclawHome()
			return mcpmgr.InstallDeps(home, args[0])
		},
	}
}
