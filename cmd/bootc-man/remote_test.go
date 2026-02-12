package main

import (
	"testing"

	"github.com/spf13/cobra"
)

// Ensure cobra.Command is used in the tests
var _ *cobra.Command = remoteCmd

func TestRemoteCommandStructure(t *testing.T) {
	// Test that remote command has expected subcommands
	subcommands := remoteCmd.Commands()

	expectedCmds := map[string]bool{
		"upgrade":  false,
		"switch":   false,
		"rollback": false,
		"status":   false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found on remote command", name)
		}
	}
}

func TestRemoteCommandMetadata(t *testing.T) {
	if remoteCmd.Use != "remote" {
		t.Errorf("remoteCmd.Use = %q, want %q", remoteCmd.Use, "remote")
	}

	if remoteCmd.Short == "" {
		t.Error("remoteCmd.Short should not be empty")
	}

	if remoteCmd.Long == "" {
		t.Error("remoteCmd.Long should not be empty")
	}
}

func TestRemoteUpgradeFlags(t *testing.T) {
	// Test that remote upgrade has expected flags
	expectedFlags := []string{"check", "apply", "vm"}

	for _, flagName := range expectedFlags {
		flag := remoteUpgradeCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("expected flag %q not found on remote upgrade", flagName)
		}
	}
}

func TestRemoteSwitchFlags(t *testing.T) {
	// Test that remote switch has expected flags
	expectedFlags := []string{"apply", "transport", "retain", "vm"}

	for _, flagName := range expectedFlags {
		flag := remoteSwitchCmd.Flags().Lookup(flagName)
		if flag == nil {
			t.Errorf("expected flag %q not found on remote switch", flagName)
		}
	}
}

func TestRemoteRollbackFlags(t *testing.T) {
	// Test that remote rollback has expected flags
	flag := remoteRollbackCmd.Flags().Lookup("vm")
	if flag == nil {
		t.Error("expected flag 'vm' not found on remote rollback")
	}
}

func TestRemoteStatusFlags(t *testing.T) {
	// Test that remote status has expected flags
	flag := remoteStatusCmd.Flags().Lookup("vm")
	if flag == nil {
		t.Error("expected flag 'vm' not found on remote status")
	}
}

func TestRemoteSubcommandMetadata(t *testing.T) {
	tests := []struct {
		name string
		cmd  *cobra.Command
		use  string
	}{
		{"upgrade", remoteUpgradeCmd, "upgrade [host]"},
		{"switch", remoteSwitchCmd, "switch [host] <image>"},
		{"rollback", remoteRollbackCmd, "rollback [host]"},
		{"status", remoteStatusCmd, "status [host]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.cmd.Use != tt.use {
				t.Errorf("%s Use = %q, want %q", tt.name, tt.cmd.Use, tt.use)
			}
			if tt.cmd.Short == "" {
				t.Errorf("%s Short should not be empty", tt.name)
			}
		})
	}
}
