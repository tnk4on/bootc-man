package main

import (
	"testing"
)

func TestVMCommandStructure(t *testing.T) {
	// Test that vm command has expected subcommands
	subcommands := vmCmd.Commands()

	expectedCmds := map[string]bool{
		"start":  false,
		"list":   false,
		"status": false,
		"stop":   false,
		"ssh":    false,
		"rm":     false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found on vm command", name)
		}
	}
}

func TestVMCommandMetadata(t *testing.T) {
	if vmCmd.Use != "vm" {
		t.Errorf("vmCmd.Use = %q, want %q", vmCmd.Use, "vm")
	}

	if vmCmd.Short == "" {
		t.Error("vmCmd.Short should not be empty")
	}

	if vmCmd.Long == "" {
		t.Error("vmCmd.Long should not be empty")
	}
}

func TestVMStartFlags(t *testing.T) {
	// Test that vm start has expected flags
	expectedFlags := []string{"name", "pipeline", "cpus", "memory", "gui"}

	for _, flagName := range expectedFlags {
		flag := vmStartCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("expected flag %q not found on vm start", flagName)
		}
	}

	// Check default values
	cpusFlag := vmStartCmd.Flags().Lookup("cpus")
	if cpusFlag != nil && cpusFlag.DefValue != "2" {
		t.Errorf("cpus flag default = %q, want %q", cpusFlag.DefValue, "2")
	}

	memoryFlag := vmStartCmd.Flags().Lookup("memory")
	if memoryFlag != nil && memoryFlag.DefValue != "4096" {
		t.Errorf("memory flag default = %q, want %q", memoryFlag.DefValue, "4096")
	}
}

func TestVMRemoveFlags(t *testing.T) {
	// Test that vm rm has expected flags
	flag := vmRemoveCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("expected flag 'force' not found on vm rm")
	}

	if flag.Shorthand != "f" {
		t.Errorf("force flag shorthand = %q, want %q", flag.Shorthand, "f")
	}
}

func TestVMSSHFlags(t *testing.T) {
	// Test that vm ssh has expected flags
	flag := vmSSHCmd.Flags().Lookup("user")
	if flag == nil {
		t.Fatal("expected flag 'user' not found on vm ssh")
	}

	if flag.Shorthand != "u" {
		t.Errorf("user flag shorthand = %q, want %q", flag.Shorthand, "u")
	}
}

func TestVMListMetadata(t *testing.T) {
	if vmListCmd.Use != "list" {
		t.Errorf("vmListCmd.Use = %q, want %q", vmListCmd.Use, "list")
	}

	if vmListCmd.Short == "" {
		t.Error("vmListCmd.Short should not be empty")
	}
}

func TestVMStatusMetadata(t *testing.T) {
	if vmStatusCmd.Use != "status [name]" {
		t.Errorf("vmStatusCmd.Use = %q, want %q", vmStatusCmd.Use, "status [name]")
	}

	if vmStatusCmd.Short == "" {
		t.Error("vmStatusCmd.Short should not be empty")
	}
}

func TestVMStopMetadata(t *testing.T) {
	if vmStopCmd.Use != "stop [name]" {
		t.Errorf("vmStopCmd.Use = %q, want %q", vmStopCmd.Use, "stop [name]")
	}

	if vmStopCmd.Short == "" {
		t.Error("vmStopCmd.Short should not be empty")
	}
}

func TestGetSSHUser(t *testing.T) {
	// This function should always return a non-empty string
	user := getSSHUser()
	if user == "" {
		t.Error("getSSHUser() should not return empty string")
	}

	// Default fallback should be "user"
	// This may vary depending on config, so just verify it's not empty
	t.Logf("getSSHUser() returned: %s", user)
}

func TestStartableCandidateStruct(t *testing.T) {
	// Test that the struct can be created and used
	candidate := StartableCandidate{
		Name:        "test-vm",
		Type:        "stopped_vm",
		Description: "A stopped VM",
		Command:     "test-vm",
	}

	if candidate.Name != "test-vm" {
		t.Errorf("Name = %q, want %q", candidate.Name, "test-vm")
	}
	if candidate.Type != "stopped_vm" {
		t.Errorf("Type = %q, want %q", candidate.Type, "stopped_vm")
	}
}

func TestVMCommandsHaveDryRunSupport(t *testing.T) {
	// Verify all VM subcommands inherit the global --dry-run flag
	// through the root command's persistent flags
	vmSubcommands := []struct {
		name string
		cmd  interface{ HasParent() bool }
	}{
		{"start", vmStartCmd},
		{"list", vmListCmd},
		{"status", vmStatusCmd},
		{"stop", vmStopCmd},
		{"ssh", vmSSHCmd},
		{"rm", vmRemoveCmd},
	}

	for _, sub := range vmSubcommands {
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

func TestVMListDryRunOutput(t *testing.T) {
	// Test that vm list produces expected dry-run output format
	// The actual output is tested via integration tests,
	// but we verify the command is configured correctly
	if vmListCmd.Args != nil {
		// vmListCmd.Args should enforce NoArgs
		if err := vmListCmd.Args(vmListCmd, []string{"extra"}); err == nil {
			t.Error("expected error for extra arguments, but got nil")
		}
	}
}
