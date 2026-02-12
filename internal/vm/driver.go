package vm

import (
	"context"
	"runtime"

	"github.com/tnk4on/bootc-man/internal/config"
)

// VMType represents the type of hypervisor
type VMType int

const (
	// VfkitVM is the vfkit hypervisor for macOS
	VfkitVM VMType = iota
	// QemuVM is the QEMU hypervisor for Linux
	QemuVM
	// HyperVVM is the Hyper-V hypervisor for Windows (future)
	HyperVVM
	// UnknownVM is an unknown hypervisor type
	UnknownVM
)

func (v VMType) String() string {
	switch v {
	case VfkitVM:
		return config.BinaryVfkit
	case QemuVM:
		return "qemu"
	case HyperVVM:
		return "hyperv"
	default:
		return "unknown"
	}
}

// ImageFormat returns the preferred disk image format for this VM type
// bootc-man uses raw format exclusively for cross-platform compatibility
func (v VMType) ImageFormat() string {
	// All platforms use raw format for simplicity and cross-platform compatibility
	// - vfkit (macOS) only supports raw
	// - QEMU (Linux) supports raw natively with good performance
	// - bootc-image-builder outputs raw by default
	return "raw"
}

// HostGatewayIP returns the IP address for accessing the host from within the VM
// All platforms use gvproxy which provides 192.168.127.1 as the gateway
func (v VMType) HostGatewayIP() string {
	// gvproxy provides a unified gateway IP across all platforms
	return "192.168.127.1"
}

// VMState represents the state of a virtual machine
type VMState string

const (
	VMStateRunning  VMState = "Running"
	VMStateStopped  VMState = "Stopped"
	VMStateStarting VMState = "Starting"
	VMStateError    VMState = "Error"
	VMStateUnknown  VMState = "Unknown"
)

// VMOptions contains options for starting a VM
type VMOptions struct {
	// Name is the VM name
	Name string
	// DiskImage is the path to the disk image file
	DiskImage string
	// CPUs is the number of virtual CPUs
	CPUs int
	// Memory is the amount of memory in MB
	Memory int
	// SSHKeyPath is the path to the SSH private key
	SSHKeyPath string
	// SSHUser is the SSH username
	SSHUser string
	// SSHPort is the SSH port on the host (for port forwarding)
	SSHPort int
	// GUI enables graphical display (if supported)
	GUI bool
	// SerialLogPath is the path for serial console output log
	SerialLogPath string
	// EFIVariableStore is the path for EFI variable store (for UEFI boot)
	EFIVariableStore string
}

// Driver is the interface for VM hypervisor drivers
// This provides a common abstraction for different hypervisors:
// - vfkit (macOS)
// - QEMU/KVM (Linux)
// - Hyper-V (Windows, future)
type Driver interface {
	// Type returns the VM type
	Type() VMType

	// Available checks if the hypervisor is available on this system
	Available() error

	// Start starts the VM with the given options
	// Returns the process or control handle for the VM
	Start(ctx context.Context, opts VMOptions) error

	// Stop stops the VM
	Stop(ctx context.Context) error

	// GetState returns the current VM state
	GetState(ctx context.Context) (VMState, error)

	// WaitForReady waits for the VM to be ready (booted)
	WaitForReady(ctx context.Context) error

	// WaitForSSH waits for SSH to be available
	WaitForSSH(ctx context.Context) error

	// SSH executes a command via SSH and returns the output
	SSH(ctx context.Context, command string) (string, error)

	// GetSSHConfig returns the SSH connection configuration
	GetSSHConfig() SSHConfig

	// ReadSerialLog reads the serial console log
	ReadSerialLog() (string, error)

	// Cleanup cleans up all resources associated with the VM
	Cleanup() error

	// GetProcessID returns the main VM process ID
	GetProcessID() int

	// GetLogFilePath returns the path to the serial console log file
	GetLogFilePath() string

	// ToVMInfo creates a VMInfo struct from the driver state
	ToVMInfo(name, pipelineName, pipelineFile, imageTag string) *VMInfo
}

// SSHConfig contains SSH connection configuration
type SSHConfig struct {
	Host        string
	Port        int
	User        string
	KeyPath     string
	HostGateway string // IP for host from VM's perspective
}

// NewDriver is defined in platform-specific files:
// - driver_darwin.go (vfkit)
// - driver_linux.go (QEMU)
// - driver_windows.go (Hyper-V, future)

// GetDefaultVMType returns the default VM type for the current platform
func GetDefaultVMType() VMType {
	switch runtime.GOOS {
	case "darwin":
		return VfkitVM
	case "linux":
		return QemuVM
	case "windows":
		return HyperVVM
	default:
		return UnknownVM
	}
}
