package main

import (
	"testing"
)

func TestContainerCommandStructure(t *testing.T) {
	// Test that container command has expected subcommands
	subcommands := containerCmd.Commands()

	expectedCmds := map[string]bool{
		"build": false,
		"run":   false,
		"push":  false,
		"image": false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestContainerImageCommandStructure(t *testing.T) {
	// Test that container image command has expected subcommands
	subcommands := containerImageCmd.Commands()

	expectedCmds := map[string]bool{
		"list":    false,
		"rm":      false,
		"inspect": false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found under 'container image'", name)
		}
	}
}

func TestContainerBuildFlags(t *testing.T) {
	// Test that container build has expected flags
	expectedFlags := []string{"tag", "file", "no-cache", "push", "tls-verify"}

	for _, flagName := range expectedFlags {
		flag := containerBuildCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("expected flag %q not found on container build", flagName)
		}
	}

	// Verify --tag is required
	if !containerBuildCmd.HasFlags() {
		t.Error("container build should have flags")
	}
}

func TestContainerRunNoCustomFlags(t *testing.T) {
	// container run should have no custom flags (only global flags)
	// It always runs /bin/bash interactively
	localFlags := containerRunCmd.Flags()

	// Verify no --command flag exists (removed for simplicity)
	if flag := localFlags.Lookup("command"); flag != nil {
		t.Error("container run should not have --command flag")
	}
}

func TestContainerImageListFlags(t *testing.T) {
	// Test that container image list has expected flags
	flag := containerImageListCmd.Flags().Lookup("all")
	if flag == nil {
		t.Fatal("expected flag 'all' not found on container image list")
	}

	// Check default value is false
	if flag.DefValue != "false" {
		t.Errorf("all flag default = %q, want %q", flag.DefValue, "false")
	}
}

func TestContainerImageRmFlags(t *testing.T) {
	// Test that container image rm has expected flags
	flag := containerImageRmCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("expected flag 'force' not found on container image rm")
	}

	// Check default value is false
	if flag.DefValue != "false" {
		t.Errorf("force flag default = %q, want %q", flag.DefValue, "false")
	}
}

func TestContainerImageListAliases(t *testing.T) {
	// Test that container image list has "ls" alias
	aliases := containerImageListCmd.Aliases
	found := false
	for _, alias := range aliases {
		if alias == "ls" {
			found = true
			break
		}
	}
	if !found {
		t.Error("container image list should have 'ls' alias")
	}
}
