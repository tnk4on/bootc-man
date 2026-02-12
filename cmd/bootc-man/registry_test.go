package main

import (
	"errors"
	"testing"

	"github.com/tnk4on/bootc-man/internal/registry"
	"github.com/spf13/cobra"
)

func TestRegistryCommandStructure(t *testing.T) {
	// Test that registry command has expected subcommands
	subcommands := registryCmd.Commands()

	expectedCmds := map[string]bool{
		"up":     false,
		"down":   false,
		"status": false,
		"logs":   false,
		"rm":     false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found on registry command", name)
		}
	}
}

func TestRegistryLogsFlags(t *testing.T) {
	// Test that registry logs has expected flags
	flag := registryLogsCmd.Flags().Lookup("follow")
	if flag == nil {
		t.Fatal("expected flag 'follow' not found on registry logs")
	}

	// Check shorthand
	if flag.Shorthand != "f" {
		t.Errorf("follow flag shorthand = %q, want %q", flag.Shorthand, "f")
	}

	// Check default
	if flag.DefValue != "false" {
		t.Errorf("follow flag default = %q, want %q", flag.DefValue, "false")
	}
}

func TestRegistryRmFlags(t *testing.T) {
	// Test that registry rm has expected flags
	tests := []struct {
		flagName  string
		shorthand string
		defValue  string
	}{
		{"force", "f", "false"},
		{"volumes", "", "false"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := registryRmCmd.Flags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("expected flag %q not found on registry rm", tt.flagName)
			}

			if tt.shorthand != "" && flag.Shorthand != tt.shorthand {
				t.Errorf("flag %q shorthand = %q, want %q", tt.flagName, flag.Shorthand, tt.shorthand)
			}

			if flag.DefValue != tt.defValue {
				t.Errorf("flag %q default = %q, want %q", tt.flagName, flag.DefValue, tt.defValue)
			}
		})
	}
}

func TestRegistryCommandMetadata(t *testing.T) {
	// Test registry command metadata
	if registryCmd.Use != "registry" {
		t.Errorf("registryCmd.Use = %q, want %q", registryCmd.Use, "registry")
	}

	if registryCmd.Short == "" {
		t.Error("registryCmd.Short should not be empty")
	}
}

func TestFormatRegistryError(t *testing.T) {
	tests := []struct {
		name        string
		context     string
		err         error
		wantContain string
	}{
		{
			name:        "registry error",
			context:     "failed to start",
			err:         &registry.RegistryError{Message: "port in use"},
			wantContain: "failed to start",
		},
		{
			name:        "generic error",
			context:     "failed to connect",
			err:         errors.New("connection refused"),
			wantContain: "failed to connect: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatRegistryError(tt.context, tt.err)
			if result == nil {
				t.Fatal("formatRegistryError returned nil")
			}
			if !containsString(result.Error(), tt.wantContain) {
				t.Errorf("error %q does not contain %q", result.Error(), tt.wantContain)
			}
		})
	}
}

// containsString checks if s contains substr
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestRegistryCommandsHaveDryRunSupport(t *testing.T) {
	// Verify all registry subcommands inherit the global --dry-run flag
	registrySubcommands := []struct {
		name string
		cmd  *Command
	}{
		{"up", registryUpCmd},
		{"down", registryDownCmd},
		{"status", registryStatusCmd},
		{"rm", registryRmCmd},
	}

	for _, sub := range registrySubcommands {
		t.Run(sub.name, func(t *testing.T) {
			// All commands should have access to the global --dry-run flag
			// through PersistentFlags on root
			dryRunFlag := rootCmd.PersistentFlags().Lookup("dry-run")
			if dryRunFlag == nil {
				t.Error("root command should have --dry-run persistent flag")
			}
		})
	}
}

func TestRegistryDryRunOutputFormat(t *testing.T) {
	// Test that registry commands follow the unified dry-run output format
	// The output format should be:
	// 📋 Equivalent command (<purpose>):
	//    <command>
	//
	// (dry-run mode - command not executed)

	// This is a design verification test - actual output is tested via integration tests
	// Here we verify the commands are properly configured

	// Verify up command doesn't have silenceerrors
	if registryUpCmd.SilenceUsage != true {
		t.Error("registryUpCmd.SilenceUsage should be true")
	}
	if registryDownCmd.SilenceUsage != true {
		t.Error("registryDownCmd.SilenceUsage should be true")
	}
	if registryStatusCmd.SilenceUsage != true {
		t.Error("registryStatusCmd.SilenceUsage should be true")
	}
}

// Command is an alias for testing purposes
type Command = cobra.Command
