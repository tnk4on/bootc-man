package main

import (
	"os"
	"testing"
)

func TestConfigCommandStructure(t *testing.T) {
	// Test that config command has expected subcommands
	subcommands := configCmd.Commands()

	expectedCmds := map[string]bool{
		"show": false,
		"path": false,
		"edit": false,
	}

	for _, cmd := range subcommands {
		if _, ok := expectedCmds[cmd.Name()]; ok {
			expectedCmds[cmd.Name()] = true
		}
	}

	for name, found := range expectedCmds {
		if !found {
			t.Errorf("expected subcommand %q not found on config command", name)
		}
	}
}

func TestConfigCommandMetadata(t *testing.T) {
	if configCmd.Use != "config" {
		t.Errorf("configCmd.Use = %q, want %q", configCmd.Use, "config")
	}

	if configCmd.Short == "" {
		t.Error("configCmd.Short should not be empty")
	}
}

func TestFindEditor(t *testing.T) {
	// Save original environment
	origEditor := os.Getenv("EDITOR")
	origVisual := os.Getenv("VISUAL")
	defer func() {
		os.Setenv("EDITOR", origEditor)
		os.Setenv("VISUAL", origVisual)
	}()

	tests := []struct {
		name      string
		setEditor string
		setVisual string
		wantErr   bool
	}{
		{
			name:      "EDITOR set to valid command",
			setEditor: "vi",
			setVisual: "",
			wantErr:   false,
		},
		{
			name:      "VISUAL set to valid command",
			setEditor: "",
			setVisual: "vi",
			wantErr:   false,
		},
		{
			name:      "Both unset, fallback to backup editors",
			setEditor: "",
			setVisual: "",
			wantErr:   false, // Assumes at least vi/vim/nano is available
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("EDITOR", tt.setEditor)
			os.Setenv("VISUAL", tt.setVisual)

			editor, err := findEditor()

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				// Only check for error if we expect no error
				// Note: This test may fail on systems without any editors installed
				if err != nil {
					t.Skipf("no editor available: %v", err)
				}
				if editor == "" {
					t.Error("findEditor returned empty string")
				}
			}
		})
	}
}

func TestFindEditorWithArguments(t *testing.T) {
	// Save original environment
	origEditor := os.Getenv("EDITOR")
	defer func() {
		os.Setenv("EDITOR", origEditor)
	}()

	// Test with editor that has arguments (like "code --wait")
	// We use "echo" as a test since it's always available
	os.Setenv("EDITOR", "echo --test")

	editor, err := findEditor()
	if err != nil {
		t.Skipf("echo not available in PATH: %v", err)
	}

	// Should return the full string including arguments
	if editor != "echo --test" {
		t.Errorf("findEditor() = %q, want %q", editor, "echo --test")
	}
}

func TestConfigShowCommandMetadata(t *testing.T) {
	if configShowCmd.Use != "show" {
		t.Errorf("configShowCmd.Use = %q, want %q", configShowCmd.Use, "show")
	}

	if configShowCmd.Short == "" {
		t.Error("configShowCmd.Short should not be empty")
	}

	if configShowCmd.Long == "" {
		t.Error("configShowCmd.Long should not be empty")
	}
}

func TestConfigPathCommandMetadata(t *testing.T) {
	if configPathCmd.Use != "path" {
		t.Errorf("configPathCmd.Use = %q, want %q", configPathCmd.Use, "path")
	}

	if configPathCmd.Short == "" {
		t.Error("configPathCmd.Short should not be empty")
	}
}

func TestConfigEditCommandMetadata(t *testing.T) {
	if configEditCmd.Use != "edit" {
		t.Errorf("configEditCmd.Use = %q, want %q", configEditCmd.Use, "edit")
	}

	if configEditCmd.Short == "" {
		t.Error("configEditCmd.Short should not be empty")
	}
}
