package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage bootc-man configuration",
	Long:  `View and modify bootc-man configuration settings.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long: `Display the current configuration.

Configuration is loaded from (in order of priority):
  1. /usr/share/bootc-man/config.yaml (system default)
  2. /etc/bootc-man/config.yaml (system admin)
  3. ~/.config/bootc-man/config.yaml (user)
  4. Environment variables (BOOTCMAN_*)
  5. Command-line flags`,
	RunE: runConfigShow,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show configuration file path",
	RunE:  runConfigPath,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit configuration file in default editor",
	RunE:  runConfigEdit,
}

// Local flag for config edit command
var configEditQuiet bool

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configEditCmd)

	// Add --quiet flag to config edit (local, not global)
	configEditCmd.Flags().BoolVarP(&configEditQuiet, "quiet", "q", false, "Suppress output")
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfg := getConfig()

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	}

	// YAML output (default)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func runConfigPath(cmd *cobra.Command, args []string) error {
	path, err := config.UserConfigPath()
	if err != nil {
		return err
	}

	fmt.Println(path)

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "(file does not exist)")
	}

	return nil
}

func runConfigEdit(cmd *cobra.Command, args []string) error {
	path, err := config.UserConfigPath()
	if err != nil {
		return err
	}

	// Check if file exists, create if not
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Printf("Configuration file does not exist. Run 'bootc-man init' first.\n")
		return nil
	}

	// Find editor
	editor, err := findEditor()
	if err != nil {
		return fmt.Errorf("failed to find editor: %w\nSet EDITOR or VISUAL environment variable, or install nano, vim, or vi", err)
	}

	// Parse editor command (may contain arguments like "code --wait")
	editorParts := strings.Fields(editor)
	editorCmd := editorParts[0]
	editorArgs := append(editorParts[1:], path)

	// In dry-run mode, just show what would be executed
	if dryRun {
		fmt.Printf("Would execute: %s %s\n", editorCmd, strings.Join(editorArgs, " "))
		return nil
	}

	// Execute editor
	if !configEditQuiet {
		fmt.Printf("Opening %s with %s...\n", path, editor)
	}

	cmdObj := exec.Command(editorCmd, editorArgs...)
	cmdObj.Stdin = os.Stdin
	cmdObj.Stdout = os.Stdout
	cmdObj.Stderr = os.Stderr

	if err := cmdObj.Run(); err != nil {
		return fmt.Errorf("failed to run editor: %w", err)
	}

	return nil
}

// findEditor finds an available editor following the bootc pattern:
// 1. Check EDITOR environment variable
// 2. Check VISUAL environment variable
// 3. Try backup editors: nano, vim, vi (in that order)
//
// The backup editor order (nano → vim → vi) follows bootc's implementation,
// which is based on systemd's edit-util.c pattern:
// - nano: Beginner-friendly, default on many modern distros, shows on-screen help
// - vim: More powerful but steeper learning curve, widely available
// - vi: Minimal but always present on POSIX-compliant systems (last resort)
func findEditor() (string, error) {
	// Check environment variables first
	if editor := os.Getenv("EDITOR"); editor != "" {
		if _, err := exec.LookPath(strings.Fields(editor)[0]); err == nil {
			return editor, nil
		}
	}
	if editor := os.Getenv("VISUAL"); editor != "" {
		if _, err := exec.LookPath(strings.Fields(editor)[0]); err == nil {
			return editor, nil
		}
	}

	// Try backup editors in order (nano → vim → vi)
	// This order prioritizes user-friendliness while ensuring availability
	backupEditors := []string{"nano", "vim", "vi"}
	for _, editor := range backupEditors {
		if path, err := exec.LookPath(editor); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no editor found")
}
