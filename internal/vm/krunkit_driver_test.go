//go:build darwin

package vm_test

import (
	"testing"

	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/vm"
)

func TestKrunkitVMType(t *testing.T) {
	if vm.KrunkitVM.String() != config.BinaryKrunkit {
		t.Errorf("KrunkitVM.String() = %q, want %q", vm.KrunkitVM.String(), config.BinaryKrunkit)
	}
}

func TestKrunkitVMTypeImageFormat(t *testing.T) {
	if vm.KrunkitVM.ImageFormat() != "raw" {
		t.Errorf("KrunkitVM.ImageFormat() = %q, want %q", vm.KrunkitVM.ImageFormat(), "raw")
	}
}

func TestKrunkitVMTypeHostGateway(t *testing.T) {
	if vm.KrunkitVM.HostGatewayIP() != "192.168.127.1" {
		t.Errorf("KrunkitVM.HostGatewayIP() = %q, want %q", vm.KrunkitVM.HostGatewayIP(), "192.168.127.1")
	}
}

func TestNewKrunkitDriver(t *testing.T) {
	opts := vm.VMOptions{
		Name:     "test-krunkit",
		SSHPort:  12345,
		SSHUser:  "testuser",
		Memory:   2048,
		CPUs:     2,
	}

	driver, err := vm.NewKrunkitDriver(opts, false)
	if err != nil {
		t.Fatalf("NewKrunkitDriver() error: %v", err)
	}

	if driver.Type() != vm.KrunkitVM {
		t.Errorf("driver.Type() = %v, want KrunkitVM", driver.Type())
	}

	sshCfg := driver.GetSSHConfig()
	if sshCfg.Port != 12345 {
		t.Errorf("SSHConfig.Port = %d, want 12345", sshCfg.Port)
	}
	if sshCfg.User != "testuser" {
		t.Errorf("SSHConfig.User = %q, want 'testuser'", sshCfg.User)
	}
	if sshCfg.HostGateway != "192.168.127.1" {
		t.Errorf("SSHConfig.HostGateway = %q, want '192.168.127.1'", sshCfg.HostGateway)
	}
}

func TestKrunkitDriverLogFilePath(t *testing.T) {
	opts := vm.VMOptions{
		Name:    "test-log",
		SSHPort: 12346,
	}

	driver, err := vm.NewKrunkitDriver(opts, false)
	if err != nil {
		t.Fatalf("NewKrunkitDriver() error: %v", err)
	}

	logPath := driver.GetLogFilePath()
	if logPath == "" {
		t.Error("GetLogFilePath() returned empty string")
	}
	if !contains(logPath, "krunkit") {
		t.Errorf("GetLogFilePath() = %q, expected to contain 'krunkit'", logPath)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
