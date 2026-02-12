package bootc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Note: os is still needed for findBootc()

// Driver defines the interface for bootc operations
type Driver interface {
	// Upgrade upgrades the system to a new image
	Upgrade(ctx context.Context, opts UpgradeOptions) error
	// Switch switches to a different image
	Switch(ctx context.Context, image string, opts SwitchOptions) error
	// Rollback performs a rollback to the previous deployment
	Rollback(ctx context.Context, opts RollbackOptions) error
	// Status returns the current bootc status
	Status(ctx context.Context) (*Status, error)
}

// UpgradeOptions contains options for upgrading
type UpgradeOptions struct {
	Check bool
	Apply bool
	Quiet bool
}

// SwitchOptions contains options for switching images
type SwitchOptions struct {
	Transport string // registry, oci, oci-archive
	Apply     bool
	Retain    bool
}

// RollbackOptions contains options for rollback
type RollbackOptions struct {
	Apply bool
}

// Status represents bootc system status
type Status struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   Metadata   `json:"metadata"`
	Spec       Spec       `json:"spec"`
	Status     HostStatus `json:"status"`
}

// Metadata contains status metadata
type Metadata struct {
	Name string `json:"name"`
}

// Spec contains the desired state
type Spec struct {
	Image *ImageReference `json:"image,omitempty"`
}

// ImageReference contains image reference information
type ImageReference struct {
	Image     string `json:"image"`
	Transport string `json:"transport,omitempty"`
}

// HostStatus contains the current status
type HostStatus struct {
	Staged   *BootEntry `json:"staged,omitempty"`
	Booted   *BootEntry `json:"booted,omitempty"`
	Rollback *BootEntry `json:"rollback,omitempty"`
	Type     string     `json:"type,omitempty"`
}

// BootEntry represents a boot entry
type BootEntry struct {
	Image        *ImageStatus `json:"image,omitempty"`
	CachedUpdate *ImageStatus `json:"cachedUpdate,omitempty"`
	Incompatible bool         `json:"incompatible"`
	Pinned       bool         `json:"pinned"`
}

// ImageStatus contains image status information
type ImageStatus struct {
	Image       ImageDetails `json:"image"`
	Version     string       `json:"version,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"`
	ImageDigest string       `json:"imageDigest,omitempty"`
}

// ImageDetails contains image reference details
type ImageDetails struct {
	Image     string `json:"image"`
	Transport string `json:"transport,omitempty"`
}

// HostDriver implements Driver for direct host operations
type HostDriver struct {
	binary string
}

// NewHostDriver creates a new host driver
func NewHostDriver() (*HostDriver, error) {
	binary, err := findBootc()
	if err != nil {
		return nil, err
	}
	return &HostDriver{binary: binary}, nil
}

func findBootc() (string, error) {
	paths := []string{
		"/usr/bin/bootc",
		"/usr/local/bin/bootc",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	path, err := exec.LookPath("bootc")
	if err != nil {
		return "", fmt.Errorf("bootc not found: %w", err)
	}
	return path, nil
}

func (d *HostDriver) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, d.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("bootc %s failed: %w\nstderr: %s",
			strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// Upgrade upgrades the system
func (d *HostDriver) Upgrade(ctx context.Context, opts UpgradeOptions) error {
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

	_, err := d.run(ctx, args...)
	return err
}

// Switch switches to a different image
func (d *HostDriver) Switch(ctx context.Context, image string, opts SwitchOptions) error {
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

	_, err := d.run(ctx, args...)
	return err
}

// Rollback performs a rollback
func (d *HostDriver) Rollback(ctx context.Context, opts RollbackOptions) error {
	args := []string{"rollback"}
	if opts.Apply {
		args = append(args, "--apply")
	}
	_, err := d.run(ctx, args...)
	return err
}

// Status returns the current status
func (d *HostDriver) Status(ctx context.Context) (*Status, error) {
	output, err := d.run(ctx, "status", "--format", "json")
	if err != nil {
		return nil, err
	}

	var status Status
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}

	return &status, nil
}
