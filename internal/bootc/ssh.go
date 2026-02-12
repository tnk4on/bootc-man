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

// SSHDriver implements Driver for remote bootc operations via SSH
// It uses the system's ssh command and ~/.ssh/config for connection settings
type SSHDriver struct {
	host    string // SSH host name (as defined in ~/.ssh/config)
	verbose bool   // Show commands being executed
	dryRun  bool   // Show commands without executing
}

// SSHDriverOptions contains options for creating an SSH driver
type SSHDriverOptions struct {
	Host    string
	Verbose bool
	DryRun  bool
}

// NewSSHDriver creates a new SSH driver for the specified host
// The host should be a name defined in ~/.ssh/config with key-based authentication
func NewSSHDriver(opts SSHDriverOptions) *SSHDriver {
	return &SSHDriver{
		host:    opts.Host,
		verbose: opts.Verbose,
		dryRun:  opts.DryRun,
	}
}

// Host returns the SSH host name
func (d *SSHDriver) Host() string {
	return d.host
}

// run executes a command on the remote host via SSH
func (d *SSHDriver) run(ctx context.Context, args ...string) ([]byte, error) {
	// Build the remote command
	remoteCmd := "sudo bootc " + strings.Join(args, " ")

	// Build equivalent command for display
	equivalentCmd := fmt.Sprintf("ssh %s %s", d.host, remoteCmd)

	// Show command in verbose mode or dry-run
	if d.verbose || d.dryRun {
		fmt.Printf("📋 Equivalent command:\n   %s\n\n", equivalentCmd)
	}

	// In dry-run mode, don't execute
	if d.dryRun {
		return []byte{}, nil
	}

	// Use ssh with BatchMode to ensure non-interactive execution
	sshArgs := []string{
		"-o", "BatchMode=yes",
		"-o", config.SSHOptionStrictHostKeyCheckingAcceptNew,
		d.host,
		remoteCmd,
	}

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ssh %s bootc %s failed: %w\nstderr: %s",
			d.host, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// IsDryRun returns whether the driver is in dry-run mode
func (d *SSHDriver) IsDryRun() bool {
	return d.dryRun
}

// CheckConnection verifies SSH connectivity to the remote host
func (d *SSHDriver) CheckConnection(ctx context.Context) error {
	sshArgs := []string{
		"-o", "BatchMode=yes",
		"-o", config.SSHOptionStrictHostKeyCheckingAcceptNew,
		"-o", config.SSHOptionConnectTimeout10,
		d.host,
		"echo ok",
	}

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("SSH connection to %s failed: %w\nstderr: %s\n\nMake sure:\n  1. Host '%s' is defined in ~/.ssh/config\n  2. SSH key authentication is configured\n  3. The remote host is reachable",
			d.host, err, stderr.String(), d.host)
	}
	return nil
}

// CheckBootc verifies that bootc is available on the remote host
func (d *SSHDriver) CheckBootc(ctx context.Context) error {
	sshArgs := []string{
		"-o", "BatchMode=yes",
		d.host,
		"which bootc || command -v bootc",
	}

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bootc not found on remote host %s", d.host)
	}
	return nil
}

// Upgrade upgrades the remote system
func (d *SSHDriver) Upgrade(ctx context.Context, opts UpgradeOptions) error {
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

// Switch switches to a different image on the remote system
func (d *SSHDriver) Switch(ctx context.Context, image string, opts SwitchOptions) error {
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

// Rollback performs a rollback on the remote system
func (d *SSHDriver) Rollback(ctx context.Context, opts RollbackOptions) error {
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

// Status returns the current status of the remote system
func (d *SSHDriver) Status(ctx context.Context) (*Status, error) {
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
