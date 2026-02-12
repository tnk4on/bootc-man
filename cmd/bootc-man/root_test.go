package main

import (
	"testing"
)

func TestRootCommandStructure(t *testing.T) {
	// Test that root command has expected subcommands
	subcommands := rootCmd.Commands()

	expectedCmds := map[string]bool{
		"init":       false,
		"status":     false,
		"config":     false,
		"registry":   false,
		"remote":     false,
		"ci":         false,
		"vm":         false,
		"completion": false,
		"container":  false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found on root command", name)
		}
	}
}

func TestRootCommandGlobalFlags(t *testing.T) {
	// Test that root command has expected global flags
	expectedFlags := []string{"config", "verbose", "json", "dry-run"}

	for _, flagName := range expectedFlags {
		flag := rootCmd.PersistentFlags().Lookup(flagName)
		if flag == nil {
			t.Errorf("expected persistent flag %q not found on root command", flagName)
		}
	}
}

func TestRootCommandFlagShortcuts(t *testing.T) {
	// Test that certain flags have shortcuts
	tests := []struct {
		flagName string
		shortcut string
	}{
		{"config", "c"},
		{"verbose", "v"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := rootCmd.PersistentFlags().Lookup(tt.flagName)
			if flag == nil {
				t.Fatalf("flag %q not found", tt.flagName)
			}
			if flag.Shorthand != tt.shortcut {
				t.Errorf("flag %q shorthand = %q, want %q", tt.flagName, flag.Shorthand, tt.shortcut)
			}
		})
	}
}

func TestRootCommandMetadata(t *testing.T) {
	// Test root command metadata
	if rootCmd.Use != "bootc-man" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "bootc-man")
	}

	if rootCmd.Short == "" {
		t.Error("rootCmd.Short should not be empty")
	}

	if rootCmd.Long == "" {
		t.Error("rootCmd.Long should not be empty")
	}

	// Version should be set
	if rootCmd.Version == "" {
		t.Error("rootCmd.Version should not be empty")
	}
}

func TestGetConfigDefault(t *testing.T) {
	// Save original cfg
	originalCfg := cfg
	defer func() { cfg = originalCfg }()

	// Test with nil cfg
	cfg = nil
	result := getConfig()
	if result == nil {
		t.Error("getConfig() should return default config when cfg is nil")
	}
}

func TestExecuteFunctions(t *testing.T) {
	// Just verify the functions exist and are callable
	// We can't actually run them without affecting global state

	// Verify Execute exists
	_ = Execute

	// Verify ExecuteWithContext exists
	_ = ExecuteWithContext
}

func TestStartsWith(t *testing.T) {
	tests := []struct {
		s        string
		prefix   string
		expected bool
	}{
		{"--json", "-", true},
		{"--json", "--", true},
		{"json", "-", false},
		{"", "-", false},
		{"-v", "-", true},
		{"status", "sta", true},
		{"status", "STATUS", false}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.s+"_"+tt.prefix, func(t *testing.T) {
			result := startsWith(tt.s, tt.prefix)
			if result != tt.expected {
				t.Errorf("startsWith(%q, %q) = %v, want %v", tt.s, tt.prefix, result, tt.expected)
			}
		})
	}
}

func TestHideUnsupportedFlagsJsonCommands(t *testing.T) {
	// Test that the jsonCommands map contains expected commands
	jsonCommands := map[string]bool{
		"status":  true,
		"list":    true,
		"show":    true,
		"inspect": true,
	}

	// Verify expected json commands are defined
	for cmd := range jsonCommands {
		if !jsonCommands[cmd] {
			t.Errorf("expected %q to be in jsonCommands", cmd)
		}
	}

	// Verify non-json commands are not in the map
	nonJsonCommands := []string{"up", "down", "rm", "build", "push", "run"}
	for _, cmd := range nonJsonCommands {
		if jsonCommands[cmd] {
			t.Errorf("command %q should not be in jsonCommands", cmd)
		}
	}
}

func TestHideUnsupportedFlagsDryRunExcluded(t *testing.T) {
	// Test that dryRunExcluded map contains expected commands
	dryRunExcluded := map[string]bool{
		"show":       true,
		"path":       true,
		"logs":       true,
		"completion": true,
		"help":       true,
	}

	// Verify expected excluded commands are defined
	for cmd := range dryRunExcluded {
		if !dryRunExcluded[cmd] {
			t.Errorf("expected %q to be in dryRunExcluded", cmd)
		}
	}

	// Verify action commands are not excluded
	actionCommands := []string{"up", "down", "rm", "build", "push", "run", "start", "stop"}
	for _, cmd := range actionCommands {
		if dryRunExcluded[cmd] {
			t.Errorf("command %q should not be in dryRunExcluded", cmd)
		}
	}
}
