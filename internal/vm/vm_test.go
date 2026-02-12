package vm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestVMInfoSerialization(t *testing.T) {
	// Test that VMInfo can be serialized and deserialized correctly
	original := &VMInfo{
		Name:         "test-vm",
		PipelineName: "Test Pipeline",
		PipelineFile: "/path/to/bootc-ci.yaml",
		ImageTag:     "localhost/bootc-man-test:latest",
		DiskImage:    "/path/to/disk.raw",
		Created:      time.Date(2026, 2, 5, 12, 0, 0, 0, time.UTC),
		SSHHost:      "localhost",
		SSHPort:      2222,
		SSHUser:      "user",
		SSHKeyPath:   "/home/user/.ssh/id_ed25519",
		LogFile:      "/tmp/test.log",
		State:        "Running",
		VMType:       "vfkit",
		ProcessID:    12345,
		GvproxyPID:   12346,
	}

	// Serialize
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal VMInfo: %v", err)
	}

	// Deserialize
	var restored VMInfo
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("failed to unmarshal VMInfo: %v", err)
	}

	// Verify fields
	if restored.Name != original.Name {
		t.Errorf("Name = %q, want %q", restored.Name, original.Name)
	}
	if restored.PipelineName != original.PipelineName {
		t.Errorf("PipelineName = %q, want %q", restored.PipelineName, original.PipelineName)
	}
	if restored.SSHPort != original.SSHPort {
		t.Errorf("SSHPort = %d, want %d", restored.SSHPort, original.SSHPort)
	}
	if restored.ProcessID != original.ProcessID {
		t.Errorf("ProcessID = %d, want %d", restored.ProcessID, original.ProcessID)
	}
	if restored.VMType != original.VMType {
		t.Errorf("VMType = %q, want %q", restored.VMType, original.VMType)
	}
}

func TestSanitizeVMName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "test-vm",
			expected: "test-vm",
		},
		{
			name:     "name with spaces",
			input:    "my test vm",
			expected: "my-test-vm",
		},
		{
			name:     "name with special characters",
			input:    "test_vm@123!",
			expected: "test-vm-123-",
		},
		{
			name:     "name with slashes",
			input:    "my/test/vm",
			expected: "my-test-vm",
		},
		{
			name:     "uppercase name",
			input:    "MyTestVM",
			expected: "MyTestVM",
		},
		{
			name:     "name longer than 30 characters",
			input:    "this-is-a-very-long-vm-name-that-exceeds-limit",
			expected: "this-is-a-very-long-vm-name-th",
		},
		{
			name:     "exactly 30 characters",
			input:    "123456789012345678901234567890",
			expected: "123456789012345678901234567890",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only special characters",
			input:    "!!!@@@###",
			expected: "---------",
		},
		{
			name:     "mixed alphanumeric and special",
			input:    "fedora-bootc_v1.0",
			expected: "fedora-bootc-v1-0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeVMName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeVMName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateImageTag(t *testing.T) {
	tests := []struct {
		name         string
		pipelineName string
		customTag    string
		expected     string
	}{
		{
			name:         "with custom tag",
			pipelineName: "my-pipeline",
			customTag:    "quay.io/myuser/myimage:v1.0",
			expected:     "quay.io/myuser/myimage:v1.0",
		},
		{
			name:         "without custom tag",
			pipelineName: "my-pipeline",
			customTag:    "",
			expected:     "localhost/bootc-man-my-pipeline:latest",
		},
		{
			name:         "pipeline name with spaces",
			pipelineName: "My Test Pipeline",
			customTag:    "",
			expected:     "localhost/bootc-man-my-test-pipeline:latest",
		},
		{
			name:         "pipeline name with uppercase",
			pipelineName: "MyPipeline",
			customTag:    "",
			expected:     "localhost/bootc-man-mypipeline:latest",
		},
		{
			name:         "empty custom tag uses pipeline name",
			pipelineName: "fedora-bootc",
			customTag:    "",
			expected:     "localhost/bootc-man-fedora-bootc:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateImageTag(tt.pipelineName, tt.customTag)
			if result != tt.expected {
				t.Errorf("GenerateImageTag(%q, %q) = %q, want %q",
					tt.pipelineName, tt.customTag, result, tt.expected)
			}
		})
	}
}

func TestGetVMsDir(t *testing.T) {
	vmsDir, err := GetVMsDir()
	if err != nil {
		t.Fatalf("GetVMsDir() error = %v", err)
	}

	if vmsDir == "" {
		t.Error("GetVMsDir() returned empty string")
	}

	// Verify it ends with "vms"
	if filepath.Base(vmsDir) != "vms" {
		t.Errorf("GetVMsDir() = %q, expected to end with 'vms'", vmsDir)
	}

	// Verify it contains bootc-man in the path
	if filepath.Base(filepath.Dir(vmsDir)) != "bootc-man" {
		t.Errorf("GetVMsDir() = %q, expected parent to be 'bootc-man'", vmsDir)
	}

	// Platform-specific checks
	if runtime.GOOS != "windows" {
		// Unix should use ~/.local/share
		homeDir, _ := os.UserHomeDir()
		expectedPrefix := filepath.Join(homeDir, ".local", "share", "bootc-man")
		if len(vmsDir) > len(expectedPrefix) && vmsDir[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("GetVMsDir() = %q, expected prefix %q", vmsDir, expectedPrefix)
		}
	}
	// Windows path check is skipped as it depends on APPDATA environment
}

func TestSaveAndLoadVMInfo(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create original VMInfo
	original := &VMInfo{
		Name:         "test-save-load",
		PipelineName: "Test Pipeline",
		PipelineFile: "/path/to/bootc-ci.yaml",
		ImageTag:     "localhost/test:latest",
		DiskImage:    "/path/to/disk.raw",
		Created:      time.Now().UTC().Truncate(time.Second),
		SSHHost:      "localhost",
		SSHPort:      2222,
		SSHUser:      "user",
		SSHKeyPath:   "/home/user/.ssh/id_ed25519",
		State:        "Running",
		VMType:       "vfkit",
		ProcessID:    12345,
	}

	// Save
	if err := SaveVMInfo(original); err != nil {
		t.Fatalf("SaveVMInfo() error = %v", err)
	}

	// Load
	loaded, err := LoadVMInfo("test-save-load")
	if err != nil {
		t.Fatalf("LoadVMInfo() error = %v", err)
	}

	// Verify
	if loaded.Name != original.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, original.Name)
	}
	if loaded.PipelineName != original.PipelineName {
		t.Errorf("PipelineName = %q, want %q", loaded.PipelineName, original.PipelineName)
	}
	if loaded.SSHPort != original.SSHPort {
		t.Errorf("SSHPort = %d, want %d", loaded.SSHPort, original.SSHPort)
	}
	if loaded.ProcessID != original.ProcessID {
		t.Errorf("ProcessID = %d, want %d", loaded.ProcessID, original.ProcessID)
	}
}

func TestLoadVMInfoNotFound(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Try to load non-existent VM
	_, err := LoadVMInfo("non-existent-vm")
	if err == nil {
		t.Error("LoadVMInfo() expected error for non-existent VM, got nil")
	}
}

func TestListVMInfos(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create test VMs
	vms := []*VMInfo{
		{
			Name:      "vm-1",
			SSHPort:   2222,
			Created:   time.Now().UTC(),
			VMType:    "vfkit",
			ProcessID: 1001,
		},
		{
			Name:      "vm-2",
			SSHPort:   2223,
			Created:   time.Now().UTC(),
			VMType:    "vfkit",
			ProcessID: 1002,
		},
		{
			Name:      "vm-3",
			SSHPort:   2224,
			Created:   time.Now().UTC(),
			VMType:    "qemu",
			ProcessID: 1003,
		},
	}

	// Save all VMs
	for _, vm := range vms {
		if err := SaveVMInfo(vm); err != nil {
			t.Fatalf("SaveVMInfo(%s) error = %v", vm.Name, err)
		}
	}

	// List VMs
	listed, err := ListVMInfos()
	if err != nil {
		t.Fatalf("ListVMInfos() error = %v", err)
	}

	if len(listed) != len(vms) {
		t.Errorf("ListVMInfos() returned %d VMs, want %d", len(listed), len(vms))
	}

	// Verify all VMs are present (order may vary)
	nameSet := make(map[string]bool)
	for _, vm := range listed {
		nameSet[vm.Name] = true
	}
	for _, vm := range vms {
		if !nameSet[vm.Name] {
			t.Errorf("VM %q not found in list", vm.Name)
		}
	}
}

func TestListVMInfosEmptyDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// List VMs (should return empty list, not error)
	listed, err := ListVMInfos()
	if err != nil {
		t.Fatalf("ListVMInfos() error = %v", err)
	}

	if len(listed) != 0 {
		t.Errorf("ListVMInfos() returned %d VMs, want 0", len(listed))
	}
}

func TestDeleteVMInfo(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Create and save a VM
	vm := &VMInfo{
		Name:      "vm-to-delete",
		SSHPort:   2222,
		Created:   time.Now().UTC(),
		VMType:    "vfkit",
		ProcessID: 1001,
	}
	if err := SaveVMInfo(vm); err != nil {
		t.Fatalf("SaveVMInfo() error = %v", err)
	}

	// Verify it exists
	_, err := LoadVMInfo("vm-to-delete")
	if err != nil {
		t.Fatalf("VM should exist before delete: %v", err)
	}

	// Delete
	if err := DeleteVMInfo("vm-to-delete"); err != nil {
		t.Fatalf("DeleteVMInfo() error = %v", err)
	}

	// Verify it's gone
	_, err = LoadVMInfo("vm-to-delete")
	if err == nil {
		t.Error("VM should not exist after delete")
	}
}

func TestDeleteVMInfoNotFound(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Override home directory for testing
	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", originalHome)

	// Try to delete non-existent VM
	err := DeleteVMInfo("non-existent-vm")
	if err == nil {
		t.Error("DeleteVMInfo() expected error for non-existent VM, got nil")
	}
}

func TestFindDiskImageFile(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		setup     func(baseDir string)
		imageTag  string
		wantErr   bool
		wantFile  string // relative to baseDir
	}{
		{
			name: "find raw file in expected location",
			setup: func(baseDir string) {
				imagesDir := filepath.Join(baseDir, "output", "images")
				_ = os.MkdirAll(imagesDir, 0755)
				_ = os.WriteFile(filepath.Join(imagesDir, "localhost_test_latest.raw"), []byte("test"), 0644)
			},
			imageTag: "localhost/test:latest",
			wantErr:  false,
			wantFile: "output/images/localhost_test_latest.raw",
		},
		{
			name: "find raw file in image subdirectory",
			setup: func(baseDir string) {
				imageDir := filepath.Join(baseDir, "output", "images", "image")
				_ = os.MkdirAll(imageDir, 0755)
				_ = os.WriteFile(filepath.Join(imageDir, "disk.raw"), []byte("test"), 0644)
			},
			imageTag: "localhost/test:latest",
			wantErr:  false,
			wantFile: "output/images/image/disk.raw",
		},
		{
			name: "find qcow2 fallback",
			setup: func(baseDir string) {
				imagesDir := filepath.Join(baseDir, "output", "images")
				_ = os.MkdirAll(imagesDir, 0755)
				_ = os.WriteFile(filepath.Join(imagesDir, "localhost_test_latest.qcow2"), []byte("test"), 0644)
			},
			imageTag: "localhost/test:latest",
			wantErr:  false,
			wantFile: "output/images/localhost_test_latest.qcow2",
		},
		{
			name: "no disk image found",
			setup: func(baseDir string) {
				imagesDir := filepath.Join(baseDir, "output", "images")
				_ = os.MkdirAll(imagesDir, 0755)
				// Create some other file, not a disk image
				_ = os.WriteFile(filepath.Join(imagesDir, "readme.txt"), []byte("test"), 0644)
			},
			imageTag: "localhost/test:latest",
			wantErr:  true,
		},
		{
			name: "find nested raw file",
			setup: func(baseDir string) {
				nestedDir := filepath.Join(baseDir, "output", "images", "nested", "dir")
				_ = os.MkdirAll(nestedDir, 0755)
				_ = os.WriteFile(filepath.Join(nestedDir, "custom.raw"), []byte("test"), 0644)
			},
			imageTag: "localhost/test:latest",
			wantErr:  false,
			wantFile: "output/images/nested/dir/custom.raw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test directory for this case
			testDir := filepath.Join(tmpDir, tt.name)
			_ = os.MkdirAll(testDir, 0755)

			// Setup test files
			tt.setup(testDir)

			// Run test
			result, err := FindDiskImageFile(testDir, tt.imageTag)

			if tt.wantErr {
				if err == nil {
					t.Errorf("FindDiskImageFile() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("FindDiskImageFile() error = %v", err)
			}

			expectedPath := filepath.Join(testDir, tt.wantFile)
			if result != expectedPath {
				t.Errorf("FindDiskImageFile() = %q, want %q", result, expectedPath)
			}
		})
	}
}

func TestCheckVfkitAvailable(t *testing.T) {
	err := CheckVfkitAvailable()

	// On non-macOS platforms, should always return an error
	if runtime.GOOS != "darwin" {
		if err == nil {
			t.Error("CheckVfkitAvailable() should return error on non-macOS")
		}
		return
	}

	// On macOS, it depends on whether vfkit is installed
	// Just verify the function doesn't panic
	// The actual result depends on the system state
	_ = err
}
