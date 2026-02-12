package main

import (
	"runtime"
	"testing"
)

func TestInitCommandMetadata(t *testing.T) {
	if initCmd.Use != "init" {
		t.Errorf("initCmd.Use = %q, want %q", initCmd.Use, "init")
	}

	if initCmd.Short == "" {
		t.Error("initCmd.Short should not be empty")
	}

	if initCmd.Long == "" {
		t.Error("initCmd.Long should not be empty")
	}
}

func TestFindBinary(t *testing.T) {
	tests := []struct {
		name       string
		binary     string
		wantFound  bool
	}{
		{
			name:      "find existing binary (sh)",
			binary:    "sh",
			wantFound: true,
		},
		{
			name:      "find nonexistent binary",
			binary:    "nonexistent-binary-12345",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := findBinary(tt.binary)

			if tt.wantFound {
				if err != nil {
					t.Errorf("findBinary(%q) error = %v, want nil", tt.binary, err)
				}
				if path == "" {
					t.Errorf("findBinary(%q) returned empty path", tt.binary)
				}
			} else {
				if err == nil {
					t.Errorf("findBinary(%q) should return error for nonexistent binary", tt.binary)
				}
			}
		})
	}
}

func TestFindBinaryPodman(t *testing.T) {
	// Test finding podman specifically
	path, err := findBinary("podman")
	if err != nil {
		t.Skipf("podman not installed: %v", err)
	}

	if path == "" {
		t.Error("findBinary(podman) returned empty path")
	}

	t.Logf("Found podman at: %s", path)
}

func TestShowInstallInstructions(t *testing.T) {
	// This test just verifies the function doesn't panic
	// The actual output goes to stdout which we don't capture

	tools := []string{"podman", "vfkit", "gvproxy", "qemu-kvm"}

	for _, tool := range tools {
		t.Run(tool, func(t *testing.T) {
			// Should not panic
			showInstallInstructions(tool)
		})
	}
}

func TestShowInstallInstructionsPlatformSpecific(t *testing.T) {
	// Verify that instructions are platform-specific
	switch runtime.GOOS {
	case "darwin":
		// On macOS, should show brew commands
		// We can't easily capture stdout, so just verify no panic
		showInstallInstructions("vfkit")
	case "linux":
		// On Linux, should show dnf/apt commands
		showInstallInstructions("qemu-kvm")
	default:
		// On other platforms, should handle gracefully
		showInstallInstructions("podman")
	}
}

func TestCheckDependencyWithInstall(t *testing.T) {
	// Test with a binary that definitely exists
	t.Run("existing binary", func(t *testing.T) {
		result := checkDependencyWithInstall("sh")
		if !result {
			t.Error("checkDependencyWithInstall(sh) should return true")
		}
	})

	// Test with a binary that definitely doesn't exist
	t.Run("nonexistent binary", func(t *testing.T) {
		result := checkDependencyWithInstall("nonexistent-tool-xyz-12345")
		if result {
			t.Error("checkDependencyWithInstall for nonexistent tool should return false")
		}
	})
}

func TestPrintWarning(t *testing.T) {
	// Just verify it doesn't panic
	printWarning()
}
