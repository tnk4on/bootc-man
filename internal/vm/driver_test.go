package vm

import (
	"runtime"
	"testing"
)

func TestVMTypeString(t *testing.T) {
	tests := []struct {
		vmType   VMType
		expected string
	}{
		{VfkitVM, "vfkit"},
		{QemuVM, "qemu"},
		{HyperVVM, "hyperv"},
		{UnknownVM, "unknown"},
		{VMType(100), "unknown"}, // Out of range value
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.vmType.String()
			if result != tt.expected {
				t.Errorf("VMType(%d).String() = %q, want %q", tt.vmType, result, tt.expected)
			}
		})
	}
}

func TestVMTypeImageFormat(t *testing.T) {
	// All VM types should use raw format
	vmTypes := []VMType{VfkitVM, QemuVM, HyperVVM, UnknownVM}

	for _, vmType := range vmTypes {
		t.Run(vmType.String(), func(t *testing.T) {
			format := vmType.ImageFormat()
			if format != "raw" {
				t.Errorf("VMType(%s).ImageFormat() = %q, want %q", vmType, format, "raw")
			}
		})
	}
}

func TestVMTypeHostGatewayIP(t *testing.T) {
	// All VM types should use gvproxy gateway IP
	vmTypes := []VMType{VfkitVM, QemuVM, HyperVVM, UnknownVM}
	expectedIP := "192.168.127.1"

	for _, vmType := range vmTypes {
		t.Run(vmType.String(), func(t *testing.T) {
			ip := vmType.HostGatewayIP()
			if ip != expectedIP {
				t.Errorf("VMType(%s).HostGatewayIP() = %q, want %q", vmType, ip, expectedIP)
			}
		})
	}
}

func TestGetDefaultVMType(t *testing.T) {
	vmType := GetDefaultVMType()

	switch runtime.GOOS {
	case "darwin":
		if vmType != VfkitVM {
			t.Errorf("GetDefaultVMType() on darwin = %v, want VfkitVM", vmType)
		}
	case "linux":
		if vmType != QemuVM {
			t.Errorf("GetDefaultVMType() on linux = %v, want QemuVM", vmType)
		}
	case "windows":
		if vmType != HyperVVM {
			t.Errorf("GetDefaultVMType() on windows = %v, want HyperVVM", vmType)
		}
	default:
		if vmType != UnknownVM {
			t.Errorf("GetDefaultVMType() on %s = %v, want UnknownVM", runtime.GOOS, vmType)
		}
	}
}

func TestVMStateConstants(t *testing.T) {
	// Verify state string values
	tests := []struct {
		state    VMState
		expected string
	}{
		{VMStateRunning, "Running"},
		{VMStateStopped, "Stopped"},
		{VMStateStarting, "Starting"},
		{VMStateError, "Error"},
		{VMStateUnknown, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if string(tt.state) != tt.expected {
				t.Errorf("VMState = %q, want %q", string(tt.state), tt.expected)
			}
		})
	}
}

func TestVMOptionsDefaults(t *testing.T) {
	// Test that VMOptions has expected zero values
	opts := VMOptions{}

	if opts.Name != "" {
		t.Errorf("VMOptions.Name default = %q, want empty string", opts.Name)
	}
	if opts.CPUs != 0 {
		t.Errorf("VMOptions.CPUs default = %d, want 0", opts.CPUs)
	}
	if opts.Memory != 0 {
		t.Errorf("VMOptions.Memory default = %d, want 0", opts.Memory)
	}
	if opts.SSHPort != 0 {
		t.Errorf("VMOptions.SSHPort default = %d, want 0", opts.SSHPort)
	}
	if opts.GUI != false {
		t.Errorf("VMOptions.GUI default = %v, want false", opts.GUI)
	}
}

func TestSSHConfig(t *testing.T) {
	// Test SSHConfig struct
	cfg := SSHConfig{
		Host:        "localhost",
		Port:        2222,
		User:        "testuser",
		KeyPath:     "/path/to/key",
		HostGateway: "192.168.127.1",
	}

	if cfg.Host != "localhost" {
		t.Errorf("SSHConfig.Host = %q, want %q", cfg.Host, "localhost")
	}
	if cfg.Port != 2222 {
		t.Errorf("SSHConfig.Port = %d, want %d", cfg.Port, 2222)
	}
	if cfg.User != "testuser" {
		t.Errorf("SSHConfig.User = %q, want %q", cfg.User, "testuser")
	}
	if cfg.KeyPath != "/path/to/key" {
		t.Errorf("SSHConfig.KeyPath = %q, want %q", cfg.KeyPath, "/path/to/key")
	}
	if cfg.HostGateway != "192.168.127.1" {
		t.Errorf("SSHConfig.HostGateway = %q, want %q", cfg.HostGateway, "192.168.127.1")
	}
}
