package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tnk4on/bootc-man/internal/config"
)

var (
	cfgFile string
	verbose bool
	jsonOut bool
	dryRun  bool

	cfg *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "bootc-man",
	Short: "bootc manager - A learning tool for managing bootc environments",
	Long: `bootc-man is a CLI tool for managing bootable container images.

It provides:
  - Local OCI registry management
  - CI pipeline for building bootc images
  - Wrappers for bootc operations (SSH-based remote execution)
  - Web GUI for easy management

🎓 LEARNING TOOL: bootc-man is designed to help you learn bootc.
   Each command wraps Podman/bootc commands transparently.
   Use --verbose to see the actual commands being executed.
   Use --dry-run to see commands without executing them.

⚠️  WARNING: This tool is designed for testing/development only.`,
	Version:       version,
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Hide flags from shell completion for commands where they don't apply
		// Following Podman pattern: https://github.com/containers/podman/blob/main/cmd/podman/root.go
		if cmd.Name() == cobra.ShellCompRequestCmd {
			hideUnsupportedFlags(cmd)
		}

		// Skip config loading for init command
		if cmd.Name() == "init" {
			return nil
		}

		// Setup logging
		if verbose {
			logrus.SetLevel(logrus.DebugLevel)
		}

		// Load configuration
		var err error
		cfg, err = loadConfig()
		if err != nil {
			// Config not found is OK for some commands
			logrus.Debugf("Config loading: %v", err)
		}

		// Register experimental commands based on config
		registerExperimentalCommands(cmd.Root())

		return nil
	},
}

func Execute() error {
	return rootCmd.ExecuteContext(context.Background())
}

func ExecuteWithContext(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

// experimentalRegistered tracks whether experimental commands have been registered
var experimentalRegistered bool

// registerExperimentalCommands adds experimental commands when experimental mode is enabled
func registerExperimentalCommands(root *cobra.Command) {
	if experimentalRegistered {
		return
	}
	if cfg != nil && cfg.Experimental {
		root.AddCommand(guiCmd)
		experimentalRegistered = true
	}
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "",
		"config file (default is ~/.config/bootc-man/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false,
		"verbose output (shows equivalent Podman/bootc commands)")

	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false,
		"output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false,
		"show equivalent Podman/bootc commands without executing")

	// Add subcommands
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(registryCmd)
	rootCmd.AddCommand(remoteCmd)
	rootCmd.AddCommand(ciCmd)
	rootCmd.AddCommand(vmCmd)
	rootCmd.AddCommand(completionCmd)
}

func loadConfig() (*config.Config, error) {
	path := cfgFile
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, ".config", "bootc-man", "config.yaml")
	}

	return config.Load(path)
}

func getConfig() *config.Config {
	if cfg == nil {
		return config.DefaultConfig()
	}
	return cfg
}

// hideUnsupportedFlags hides global flags from shell completion for commands
// where they don't apply. This follows the Podman pattern.
func hideUnsupportedFlags(cmd *cobra.Command) {
	// Commands that support --json output
	jsonCommands := map[string]bool{
		"status":  true,
		"list":    true, // vm list, container image list
		"show":    true, // config show
		"inspect": true, // container image inspect
	}

	// Commands that support --dry-run (almost all action commands)
	// We only hide --dry-run for pure display/query commands
	dryRunExcluded := map[string]bool{
		"show":       true, // config show
		"path":       true, // config path
		"logs":       true, // registry logs
		"completion": true, // shell completion
		"help":       true,
	}

	// Get the target command from os.Args
	// During completion, args are: bootc-man __complete <cmd> [subcmd] ...
	cmdName := ""
	if len(os.Args) > 2 {
		cmdName = os.Args[2]
		// Check for subcommand (e.g., "registry status" -> use "status")
		if len(os.Args) > 3 && !startsWith(os.Args[3], "-") {
			cmdName = os.Args[3]
		}
	}

	// Hide --json for commands that don't support it
	if !jsonCommands[cmdName] {
		if flag := cmd.Root().PersistentFlags().Lookup("json"); flag != nil {
			flag.Hidden = true
		}
	}

	// Hide --dry-run for pure display commands
	if dryRunExcluded[cmdName] {
		if flag := cmd.Root().PersistentFlags().Lookup("dry-run"); flag != nil {
			flag.Hidden = true
		}
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
