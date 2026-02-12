package main

import (
	"testing"
)

func TestCICommandStructure(t *testing.T) {
	// Test that ci command has expected subcommands
	subcommands := ciCmd.Commands()

	expectedCmds := map[string]bool{
		"run":    false,
		"check":  false,
		"keygen": false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found on ci command", name)
		}
	}
}

func TestCICommandMetadata(t *testing.T) {
	if ciCmd.Use != "ci" {
		t.Errorf("ciCmd.Use = %q, want %q", ciCmd.Use, "ci")
	}

	if ciCmd.Short == "" {
		t.Error("ciCmd.Short should not be empty")
	}

	if ciCmd.Long == "" {
		t.Error("ciCmd.Long should not be empty")
	}
}

func TestCIRunFlags(t *testing.T) {
	// Test that ci run has expected local flags
	// Note: --dry-run is a global flag inherited from rootCmd
	expectedFlags := []string{"pipeline", "stage"}

	for _, flagName := range expectedFlags {
		flag := ciRunCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("expected flag %q not found on ci run", flagName)
		}
	}

	// Check shorthand for pipeline
	flag := ciRunCmd.Flags().Lookup("pipeline")
	if flag != nil && flag.Shorthand != "p" {
		t.Errorf("pipeline flag shorthand = %q, want %q", flag.Shorthand, "p")
	}
}

func TestCIKeygenFlags(t *testing.T) {
	// Test that ci keygen has expected flags
	flag := ciKeygenCmd.Flags().Lookup("output")
	if flag == nil {
		t.Fatal("expected flag 'output' not found on ci keygen")
	}

	if flag.Shorthand != "o" {
		t.Errorf("output flag shorthand = %q, want %q", flag.Shorthand, "o")
	}
}
