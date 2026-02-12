package bootc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/tnk4on/bootc-man/internal/config"
)

// VMDriver implements Driver for bootc operations on VMs managed by bootc-man
// It connects via SSH using the VM's stored connection information
type VMDriver struct {
	vmName     string // VM name (as registered with bootc-man vm)
	sshHost    string // SSH host (usually localhost)
	sshPort    int    // SSH port (gvproxy forwarded port)
	sshUser    string // SSH user (usually "user")
	sshKeyPath string // Path to SSH private key
	verbose    bool   // Show commands being executed
	dryRun     bool   // Show commands without executing
}

// VMDriverOptions contains options for creating a VM driver
type VMDriverOptions struct {
	VMName     string
	SSHHost    string
	SSHPort    int
	SSHUser    string
	SSHKeyPath string
	Verbose    bool
	DryRun     bool
}

// NewVMDriver creates a new VM driver for the specified VM
func NewVMDriver(opts VMDriverOptions) *VMDriver {
	return &VMDriver{
		vmName:     opts.VMName,
		sshHost:    opts.SSHHost,
		sshPort:    opts.SSHPort,
		sshUser:    opts.SSHUser,
		sshKeyPath: opts.SSHKeyPath,
		verbose:    opts.Verbose,
		dryRun:     opts.DryRun,
	}
}

// VMName returns the VM name
func (d *VMDriver) VMName() string {
	return d.vmName
}

// Host returns a display name for the VM connection
func (d *VMDriver) Host() string {
	return fmt.Sprintf("vm:%s", d.vmName)
}

// run executes a command on the VM via SSH
func (d *VMDriver) run(ctx context.Context, args ...string) ([]byte, error) {
	// Build the remote command
	remoteCmd := "sudo bootc " + strings.Join(args, " ")

	// Build equivalent command for display
	equivalentCmd := fmt.Sprintf("ssh -i %s -p %d %s@%s %s",
		d.sshKeyPath, d.sshPort, d.sshUser, d.sshHost, remoteCmd)

	// Show command in verbose mode or dry-run
	if d.verbose || d.dryRun {
		fmt.Printf("📋 Equivalent command:\n   %s\n\n", equivalentCmd)
	}

	// In dry-run mode, don't execute
	if d.dryRun {
		return []byte{}, nil
	}

	// Build SSH args for VM connection
	sshArgs := []string{
		"-i", d.sshKeyPath,
		"-p", fmt.Sprintf("%d", d.sshPort),
		"-o", "BatchMode=yes",
		"-o", config.SSHOptionStrictHostKeyCheckingNo,
		"-o", config.SSHOptionUserKnownHostsFileDevNull,
		"-o", "LogLevel=ERROR",
		fmt.Sprintf("%s@%s", d.sshUser, d.sshHost),
		remoteCmd,
	}

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ssh to VM %s failed: %w\nstderr: %s",
			d.vmName, err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// IsDryRun returns whether the driver is in dry-run mode
func (d *VMDriver) IsDryRun() bool {
	return d.dryRun
}

// CheckConnection verifies SSH connectivity to the VM
func (d *VMDriver) CheckConnection(ctx context.Context) error {
	sshArgs := []string{
		"-i", d.sshKeyPath,
		"-p", fmt.Sprintf("%d", d.sshPort),
		"-o", "BatchMode=yes",
		"-o", config.SSHOptionStrictHostKeyCheckingNo,
		"-o", config.SSHOptionUserKnownHostsFileDevNull,
		"-o", "LogLevel=ERROR",
		"-o", config.SSHOptionConnectTimeout10,
		fmt.Sprintf("%s@%s", d.sshUser, d.sshHost),
		"echo ok",
	}

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("SSH connection to VM %s failed: %w\nstderr: %s\n\nMake sure the VM is running:\n  bootc-man vm status %s",
			d.vmName, err, stderr.String(), d.vmName)
	}
	return nil
}

// CheckBootc verifies that bootc is available on the VM
func (d *VMDriver) CheckBootc(ctx context.Context) error {
	sshArgs := []string{
		"-i", d.sshKeyPath,
		"-p", fmt.Sprintf("%d", d.sshPort),
		"-o", "BatchMode=yes",
		"-o", config.SSHOptionStrictHostKeyCheckingNo,
		"-o", config.SSHOptionUserKnownHostsFileDevNull,
		"-o", "LogLevel=ERROR",
		fmt.Sprintf("%s@%s", d.sshUser, d.sshHost),
		"which bootc || command -v bootc",
	}

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bootc not found on VM %s", d.vmName)
	}
	return nil
}

// Upgrade upgrades the VM system
func (d *VMDriver) Upgrade(ctx context.Context, opts UpgradeOptions) error {
	args := []string{"upgrade"}

	if opts.Check {
		args = append(args, "--check")
	}
	if opts.Apply {
		args = append(args, "--apply")
	}
	if opts.Quiet {
		args = append(args, "--quiet")
	}

	output, err := d.run(ctx, args...)
	if err != nil {
		return err
	}

	// Print output if not quiet
	if !opts.Quiet && len(output) > 0 {
		fmt.Print(string(output))
	}
	return nil
}

// Switch switches to a different image on the VM
func (d *VMDriver) Switch(ctx context.Context, image string, opts SwitchOptions) error {
	args := []string{"switch"}

	if opts.Transport != "" && opts.Transport != "registry" {
		args = append(args, "--transport", opts.Transport)
	}
	if opts.Apply {
		args = append(args, "--apply")
	}
	if opts.Retain {
		args = append(args, "--retain")
	}

	args = append(args, image)

	output, err := d.run(ctx, args...)
	if err != nil {
		return err
	}

	if len(output) > 0 {
		fmt.Print(string(output))
	}
	return nil
}

// Rollback performs a rollback on the VM
func (d *VMDriver) Rollback(ctx context.Context, opts RollbackOptions) error {
	args := []string{"rollback"}
	if opts.Apply {
		args = append(args, "--apply")
	}

	output, err := d.run(ctx, args...)
	if err != nil {
		return err
	}

	if len(output) > 0 {
		fmt.Print(string(output))
	}
	return nil
}

// Status returns the current status of the VM system
func (d *VMDriver) Status(ctx context.Context) (*Status, error) {
	output, err := d.run(ctx, "status", "--format", "json")
	if err != nil {
		return nil, err
	}

	// In dry-run mode, return a placeholder status
	if d.dryRun {
		return &Status{
			Kind: "(dry-run)",
			Status: HostStatus{
				Type: "dry-run",
			},
		}, nil
	}

	var status Status
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}

	return &status, nil
}
