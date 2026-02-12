//go:build windows

package vm

import (
	"context"
	"fmt"
)

// HypervDriver implements the Driver interface for Hyper-V on Windows
// This is a placeholder for future implementation
type HypervDriver struct {
	opts      VMOptions
	verbose   bool
	sshConfig SSHConfig
}

// NewHypervDriver creates a new Hyper-V driver
func NewHypervDriver(opts VMOptions, verbose bool) (*HypervDriver, error) {
	return nil, fmt.Errorf("Hyper-V driver is not yet implemented")
}

// Type returns the VM type
func (d *HypervDriver) Type() VMType {
	return HyperVVM
}

// Available checks if Hyper-V is available
func (d *HypervDriver) Available() error {
	return fmt.Errorf("Hyper-V driver is not yet implemented")
}

// Start starts the VM
func (d *HypervDriver) Start(ctx context.Context, opts VMOptions) error {
	return fmt.Errorf("Hyper-V driver is not yet implemented")
}

// Stop stops the VM
func (d *HypervDriver) Stop(ctx context.Context) error {
	return fmt.Errorf("Hyper-V driver is not yet implemented")
}

// GetState returns the current VM state
func (d *HypervDriver) GetState(ctx context.Context) (VMState, error) {
	return VMStateUnknown, fmt.Errorf("Hyper-V driver is not yet implemented")
}

// WaitForReady waits for the VM to be ready
func (d *HypervDriver) WaitForReady(ctx context.Context) error {
	return fmt.Errorf("Hyper-V driver is not yet implemented")
}

// WaitForSSH waits for SSH to be available
func (d *HypervDriver) WaitForSSH(ctx context.Context) error {
	return fmt.Errorf("Hyper-V driver is not yet implemented")
}

// SSH executes a command via SSH
func (d *HypervDriver) SSH(ctx context.Context, command string) (string, error) {
	return "", fmt.Errorf("Hyper-V driver is not yet implemented")
}

// GetSSHConfig returns the SSH configuration
func (d *HypervDriver) GetSSHConfig() SSHConfig {
	return d.sshConfig
}

// ReadSerialLog reads the serial console log
func (d *HypervDriver) ReadSerialLog() (string, error) {
	return "", fmt.Errorf("Hyper-V driver is not yet implemented")
}

// Cleanup cleans up all resources
func (d *HypervDriver) Cleanup() error {
	return fmt.Errorf("Hyper-V driver is not yet implemented")
}

// GetProcessID returns the VM process ID
func (d *HypervDriver) GetProcessID() int {
	return 0
}

// GetLogFilePath returns the path to the serial console log file
func (d *HypervDriver) GetLogFilePath() string {
	return ""
}

// ToVMInfo creates a VMInfo struct from the driver state
func (d *HypervDriver) ToVMInfo(name, pipelineName, pipelineFile, imageTag string) *VMInfo {
	return nil
}
