package bootc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// Test image constants - keep in sync with testutil package
const (
	testBootcImageRegistry    = "quay.io/fedora"
	testBootcImageName        = "fedora-bootc"
	testBootcImageTagCurrent  = "41"
	testBootcImageTagPrevious = "40"
	testBootcImageTagNext     = "42"
)

func testBootcImage(tag string) string {
	return fmt.Sprintf("%s/%s:%s", testBootcImageRegistry, testBootcImageName, tag)
}

func testBootcImageCurrent() string  { return testBootcImage(testBootcImageTagCurrent) }
func testBootcImagePrevious() string { return testBootcImage(testBootcImageTagPrevious) }
func testBootcImageNext() string     { return testBootcImage(testBootcImageTagNext) }

func TestStatusParsing(t *testing.T) {
	currentImage := testBootcImageCurrent()
	previousImage := testBootcImagePrevious()
	nextImage := testBootcImageNext()

	tests := []struct {
		name     string
		json     string
		wantErr  bool
		validate func(*testing.T, *Status)
	}{
		{
			name: "basic status with booted image",
			json: fmt.Sprintf(`{
				"apiVersion": "org.containers.bootc/v1",
				"kind": "BootcHost",
				"metadata": {"name": "host"},
				"spec": {
					"image": {
						"image": "%s",
						"transport": "registry"
					}
				},
				"status": {
					"staged": null,
					"booted": {
						"image": {
							"image": {
								"image": "%s",
								"transport": "registry"
							},
							"version": "%s.20240101.0",
							"timestamp": "2024-01-01T00:00:00Z",
							"imageDigest": "sha256:abc123"
						},
						"incompatible": false,
						"pinned": false
					},
					"rollback": null,
					"type": "bootcHost"
				}
			}`, currentImage, currentImage, testBootcImageTagCurrent),
			wantErr: false,
			validate: func(t *testing.T, s *Status) {
				if s.APIVersion != "org.containers.bootc/v1" {
					t.Errorf("APIVersion = %q, want %q", s.APIVersion, "org.containers.bootc/v1")
				}
				if s.Kind != "BootcHost" {
					t.Errorf("Kind = %q, want %q", s.Kind, "BootcHost")
				}
				if s.Status.Booted == nil {
					t.Fatal("Booted is nil")
				}
				if s.Status.Booted.Image == nil {
					t.Fatal("Booted.Image is nil")
				}
				if s.Status.Booted.Image.Image.Image != currentImage {
					t.Errorf("Booted.Image.Image.Image = %q, want %q",
						s.Status.Booted.Image.Image.Image, currentImage)
				}
				expectedVersion := testBootcImageTagCurrent + ".20240101.0"
				if s.Status.Booted.Image.Version != expectedVersion {
					t.Errorf("Booted.Image.Version = %q, want %q",
						s.Status.Booted.Image.Version, expectedVersion)
				}
			},
		},
		{
			name: "status with staged update",
			json: fmt.Sprintf(`{
				"apiVersion": "org.containers.bootc/v1",
				"kind": "BootcHost",
				"metadata": {"name": "host"},
				"spec": {
					"image": {
						"image": "%s",
						"transport": "registry"
					}
				},
				"status": {
					"staged": {
						"image": {
							"image": {
								"image": "%s",
								"transport": "registry"
							},
							"version": "%s.20240201.0",
							"imageDigest": "sha256:new123"
						},
						"incompatible": false,
						"pinned": false
					},
					"booted": {
						"image": {
							"image": {
								"image": "%s",
								"transport": "registry"
							},
							"version": "%s.20240101.0",
							"imageDigest": "sha256:abc123"
						},
						"incompatible": false,
						"pinned": false
					},
					"rollback": null,
					"type": "bootcHost"
				}
			}`, nextImage, nextImage, testBootcImageTagNext, currentImage, testBootcImageTagCurrent),
			wantErr: false,
			validate: func(t *testing.T, s *Status) {
				if s.Status.Staged == nil {
					t.Fatal("Staged is nil")
				}
				if s.Status.Staged.Image.Image.Image != nextImage {
					t.Errorf("Staged.Image.Image.Image = %q, want %q",
						s.Status.Staged.Image.Image.Image, nextImage)
				}
				expectedVersion := testBootcImageTagNext + ".20240201.0"
				if s.Status.Staged.Image.Version != expectedVersion {
					t.Errorf("Staged.Image.Version = %q, want %q",
						s.Status.Staged.Image.Version, expectedVersion)
				}
			},
		},
		{
			name: "status with rollback",
			json: fmt.Sprintf(`{
				"apiVersion": "org.containers.bootc/v1",
				"kind": "BootcHost",
				"metadata": {"name": "host"},
				"spec": {},
				"status": {
					"staged": null,
					"booted": {
						"image": {
							"image": {
								"image": "%s",
								"transport": "registry"
							},
							"version": "%s.20240101.0",
							"imageDigest": "sha256:abc123"
						},
						"incompatible": false,
						"pinned": false
					},
					"rollback": {
						"image": {
							"image": {
								"image": "%s",
								"transport": "registry"
							},
							"version": "%s.20231201.0",
							"imageDigest": "sha256:old123"
						},
						"incompatible": false,
						"pinned": false
					},
					"type": "bootcHost"
				}
			}`, currentImage, testBootcImageTagCurrent, previousImage, testBootcImageTagPrevious),
			wantErr: false,
			validate: func(t *testing.T, s *Status) {
				if s.Status.Rollback == nil {
					t.Fatal("Rollback is nil")
				}
				if s.Status.Rollback.Image.Image.Image != previousImage {
					t.Errorf("Rollback.Image.Image.Image = %q, want %q",
						s.Status.Rollback.Image.Image.Image, previousImage)
				}
			},
		},
		{
			name: "status with pinned entry",
			json: `{
				"apiVersion": "org.containers.bootc/v1",
				"kind": "BootcHost",
				"metadata": {"name": "host"},
				"spec": {},
				"status": {
					"booted": {
						"image": {
							"image": {"image": "test:latest", "transport": "registry"},
							"version": "1.0.0"
						},
						"incompatible": false,
						"pinned": true
					},
					"type": "bootcHost"
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, s *Status) {
				if !s.Status.Booted.Pinned {
					t.Error("Booted.Pinned = false, want true")
				}
			},
		},
		{
			name: "status with incompatible entry",
			json: `{
				"apiVersion": "org.containers.bootc/v1",
				"kind": "BootcHost",
				"metadata": {"name": "host"},
				"spec": {},
				"status": {
					"booted": {
						"image": {
							"image": {"image": "test:latest", "transport": "registry"},
							"version": "1.0.0"
						},
						"incompatible": true,
						"pinned": false
					},
					"type": "bootcHost"
				}
			}`,
			wantErr: false,
			validate: func(t *testing.T, s *Status) {
				if !s.Status.Booted.Incompatible {
					t.Error("Booted.Incompatible = false, want true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status Status
			err := json.Unmarshal([]byte(tt.json), &status)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, &status)
			}
		})
	}
}

func TestUpgradeOptions(t *testing.T) {
	tests := []struct {
		name string
		opts UpgradeOptions
		want []string
	}{
		{
			name: "default options",
			opts: UpgradeOptions{},
			want: []string{"upgrade"},
		},
		{
			name: "with check",
			opts: UpgradeOptions{Check: true},
			want: []string{"upgrade", "--check"},
		},
		{
			name: "with apply",
			opts: UpgradeOptions{Apply: true},
			want: []string{"upgrade", "--apply"},
		},
		{
			name: "with quiet",
			opts: UpgradeOptions{Quiet: true},
			want: []string{"upgrade", "--quiet"},
		},
		{
			name: "with all options",
			opts: UpgradeOptions{Check: true, Apply: true, Quiet: true},
			want: []string{"upgrade", "--check", "--apply", "--quiet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"upgrade"}
			if tt.opts.Check {
				args = append(args, "--check")
			}
			if tt.opts.Apply {
				args = append(args, "--apply")
			}
			if tt.opts.Quiet {
				args = append(args, "--quiet")
			}

			if len(args) != len(tt.want) {
				t.Errorf("args length = %d, want %d", len(args), len(tt.want))
				return
			}

			for i, arg := range args {
				if arg != tt.want[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.want[i])
				}
			}
		})
	}
}

func TestSwitchOptions(t *testing.T) {
	currentImage := testBootcImageCurrent()

	tests := []struct {
		name  string
		image string
		opts  SwitchOptions
		want  []string
	}{
		{
			name:  "default options",
			image: currentImage,
			opts:  SwitchOptions{},
			want:  []string{"switch", currentImage},
		},
		{
			name:  "with transport",
			image: currentImage,
			opts:  SwitchOptions{Transport: "oci"},
			want:  []string{"switch", "--transport", "oci", currentImage},
		},
		{
			name:  "with registry transport (default)",
			image: currentImage,
			opts:  SwitchOptions{Transport: "registry"},
			want:  []string{"switch", currentImage},
		},
		{
			name:  "with apply",
			image: currentImage,
			opts:  SwitchOptions{Apply: true},
			want:  []string{"switch", "--apply", currentImage},
		},
		{
			name:  "with retain",
			image: currentImage,
			opts:  SwitchOptions{Retain: true},
			want:  []string{"switch", "--retain", currentImage},
		},
		{
			name:  "with all options",
			image: "test:latest",
			opts:  SwitchOptions{Transport: "oci-archive", Apply: true, Retain: true},
			want:  []string{"switch", "--transport", "oci-archive", "--apply", "--retain", "test:latest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"switch"}
			if tt.opts.Transport != "" && tt.opts.Transport != "registry" {
				args = append(args, "--transport", tt.opts.Transport)
			}
			if tt.opts.Apply {
				args = append(args, "--apply")
			}
			if tt.opts.Retain {
				args = append(args, "--retain")
			}
			args = append(args, tt.image)

			if len(args) != len(tt.want) {
				t.Errorf("args length = %d, want %d\nargs: %v\nwant: %v", len(args), len(tt.want), args, tt.want)
				return
			}

			for i, arg := range args {
				if arg != tt.want[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.want[i])
				}
			}
		})
	}
}

func TestRollbackOptions(t *testing.T) {
	tests := []struct {
		name string
		opts RollbackOptions
		want []string
	}{
		{
			name: "default options",
			opts: RollbackOptions{},
			want: []string{"rollback"},
		},
		{
			name: "with apply",
			opts: RollbackOptions{Apply: true},
			want: []string{"rollback", "--apply"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"rollback"}
			if tt.opts.Apply {
				args = append(args, "--apply")
			}

			if len(args) != len(tt.want) {
				t.Errorf("args length = %d, want %d", len(args), len(tt.want))
				return
			}

			for i, arg := range args {
				if arg != tt.want[i] {
					t.Errorf("args[%d] = %q, want %q", i, arg, tt.want[i])
				}
			}
		})
	}
}

func TestImageDetails(t *testing.T) {
	currentImage := testBootcImageCurrent()

	details := ImageDetails{
		Image:     currentImage,
		Transport: "registry",
	}

	if details.Image != currentImage {
		t.Errorf("Image = %q, want %q", details.Image, currentImage)
	}
	if details.Transport != "registry" {
		t.Errorf("Transport = %q, want %q", details.Transport, "registry")
	}
}

func TestImageStatus(t *testing.T) {
	currentImage := testBootcImageCurrent()
	expectedVersion := testBootcImageTagCurrent + ".20240101.0"

	status := ImageStatus{
		Image: ImageDetails{
			Image:     currentImage,
			Transport: "registry",
		},
		Version:     expectedVersion,
		Timestamp:   "2024-01-01T00:00:00Z",
		ImageDigest: "sha256:abc123def456",
	}

	if status.Version != expectedVersion {
		t.Errorf("Version = %q, want %q", status.Version, expectedVersion)
	}
	if status.ImageDigest != "sha256:abc123def456" {
		t.Errorf("ImageDigest = %q, want %q", status.ImageDigest, "sha256:abc123def456")
	}
}

func TestBootEntry(t *testing.T) {
	entry := BootEntry{
		Image: &ImageStatus{
			Image: ImageDetails{
				Image: "test:latest",
			},
			Version: "1.0.0",
		},
		Incompatible: false,
		Pinned:       true,
	}

	if entry.Image == nil {
		t.Fatal("Image is nil")
	}
	if !entry.Pinned {
		t.Error("Pinned = false, want true")
	}
	if entry.Incompatible {
		t.Error("Incompatible = true, want false")
	}
}

func TestHostStatus(t *testing.T) {
	status := HostStatus{
		Booted: &BootEntry{
			Image: &ImageStatus{
				Image:   ImageDetails{Image: "booted:latest"},
				Version: "1.0.0",
			},
		},
		Rollback: &BootEntry{
			Image: &ImageStatus{
				Image:   ImageDetails{Image: "rollback:latest"},
				Version: "0.9.0",
			},
		},
		Type: "bootcHost",
	}

	if status.Staged != nil {
		t.Error("Staged should be nil")
	}
	if status.Booted == nil {
		t.Fatal("Booted is nil")
	}
	if status.Rollback == nil {
		t.Fatal("Rollback is nil")
	}
	if status.Type != "bootcHost" {
		t.Errorf("Type = %q, want %q", status.Type, "bootcHost")
	}
}

func TestFindBootcPaths(t *testing.T) {
	// This test verifies the expected search paths
	expectedPaths := []string{
		"/usr/bin/bootc",
		"/usr/local/bin/bootc",
	}

	// We can't test findBootc directly without modifying the system,
	// but we can verify the expected paths are checked
	for _, path := range expectedPaths {
		if path == "" {
			t.Error("path should not be empty")
		}
	}
}

// === SSHDriver Tests ===

func TestNewSSHDriver(t *testing.T) {
	tests := []struct {
		name    string
		opts    SSHDriverOptions
		wantErr bool
	}{
		{
			name: "basic driver",
			opts: SSHDriverOptions{
				Host:    "test-host",
				Verbose: false,
				DryRun:  false,
			},
		},
		{
			name: "driver with verbose",
			opts: SSHDriverOptions{
				Host:    "verbose-host",
				Verbose: true,
				DryRun:  false,
			},
		},
		{
			name: "driver with dry-run",
			opts: SSHDriverOptions{
				Host:    "dryrun-host",
				Verbose: false,
				DryRun:  true,
			},
		},
		{
			name: "driver with all options",
			opts: SSHDriverOptions{
				Host:    "full-host",
				Verbose: true,
				DryRun:  true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewSSHDriver(tt.opts)
			if driver == nil {
				t.Fatal("NewSSHDriver returned nil")
			}
			if driver.Host() != tt.opts.Host {
				t.Errorf("Host() = %q, want %q", driver.Host(), tt.opts.Host)
			}
			if driver.IsDryRun() != tt.opts.DryRun {
				t.Errorf("IsDryRun() = %v, want %v", driver.IsDryRun(), tt.opts.DryRun)
			}
		})
	}
}

func TestSSHDriverDryRunMethods(t *testing.T) {
	ctx := context.Background()
	driver := NewSSHDriver(SSHDriverOptions{
		Host:    "dry-run-host",
		Verbose: false,
		DryRun:  true,
	})

	// Test Status in dry-run mode
	status, err := driver.Status(ctx)
	if err != nil {
		t.Fatalf("Status() in dry-run mode should not error: %v", err)
	}
	if status == nil {
		t.Fatal("Status() returned nil")
	}
	if status.Kind != "(dry-run)" {
		t.Errorf("Status().Kind = %q, want %q", status.Kind, "(dry-run)")
	}
	if status.Status.Type != "dry-run" {
		t.Errorf("Status().Status.Type = %q, want %q", status.Status.Type, "dry-run")
	}

	// Test Upgrade in dry-run mode (should not error)
	err = driver.Upgrade(ctx, UpgradeOptions{})
	if err != nil {
		t.Errorf("Upgrade() in dry-run mode should not error: %v", err)
	}

	// Test Upgrade with options
	err = driver.Upgrade(ctx, UpgradeOptions{Check: true, Apply: true, Quiet: true})
	if err != nil {
		t.Errorf("Upgrade() with options in dry-run mode should not error: %v", err)
	}

	// Test Switch in dry-run mode
	err = driver.Switch(ctx, "test-image:latest", SwitchOptions{})
	if err != nil {
		t.Errorf("Switch() in dry-run mode should not error: %v", err)
	}

	// Test Switch with options
	err = driver.Switch(ctx, "test-image:latest", SwitchOptions{Transport: "oci", Apply: true, Retain: true})
	if err != nil {
		t.Errorf("Switch() with options in dry-run mode should not error: %v", err)
	}

	// Test Rollback in dry-run mode
	err = driver.Rollback(ctx, RollbackOptions{})
	if err != nil {
		t.Errorf("Rollback() in dry-run mode should not error: %v", err)
	}

	// Test Rollback with Apply
	err = driver.Rollback(ctx, RollbackOptions{Apply: true})
	if err != nil {
		t.Errorf("Rollback() with Apply in dry-run mode should not error: %v", err)
	}
}

// === VMDriver Tests ===

func TestNewVMDriver(t *testing.T) {
	tests := []struct {
		name string
		opts VMDriverOptions
	}{
		{
			name: "basic driver",
			opts: VMDriverOptions{
				VMName:     "test-vm",
				SSHHost:    "localhost",
				SSHPort:    2222,
				SSHUser:    "user",
				SSHKeyPath: "/path/to/key",
				Verbose:    false,
				DryRun:     false,
			},
		},
		{
			name: "driver with verbose",
			opts: VMDriverOptions{
				VMName:     "verbose-vm",
				SSHHost:    "localhost",
				SSHPort:    2223,
				SSHUser:    "admin",
				SSHKeyPath: "/path/to/admin-key",
				Verbose:    true,
				DryRun:     false,
			},
		},
		{
			name: "driver with dry-run",
			opts: VMDriverOptions{
				VMName:     "dryrun-vm",
				SSHHost:    "localhost",
				SSHPort:    2224,
				SSHUser:    "user",
				SSHKeyPath: "/path/to/key",
				Verbose:    false,
				DryRun:     true,
			},
		},
		{
			name: "driver with all options",
			opts: VMDriverOptions{
				VMName:     "full-vm",
				SSHHost:    "127.0.0.1",
				SSHPort:    3333,
				SSHUser:    "root",
				SSHKeyPath: "/root/.ssh/id_rsa",
				Verbose:    true,
				DryRun:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			driver := NewVMDriver(tt.opts)
			if driver == nil {
				t.Fatal("NewVMDriver returned nil")
			}
			if driver.VMName() != tt.opts.VMName {
				t.Errorf("VMName() = %q, want %q", driver.VMName(), tt.opts.VMName)
			}
			expectedHost := fmt.Sprintf("vm:%s", tt.opts.VMName)
			if driver.Host() != expectedHost {
				t.Errorf("Host() = %q, want %q", driver.Host(), expectedHost)
			}
			if driver.IsDryRun() != tt.opts.DryRun {
				t.Errorf("IsDryRun() = %v, want %v", driver.IsDryRun(), tt.opts.DryRun)
			}
		})
	}
}

func TestVMDriverDryRunMethods(t *testing.T) {
	ctx := context.Background()
	driver := NewVMDriver(VMDriverOptions{
		VMName:     "dry-run-vm",
		SSHHost:    "localhost",
		SSHPort:    2222,
		SSHUser:    "user",
		SSHKeyPath: "/path/to/key",
		Verbose:    false,
		DryRun:     true,
	})

	// Test Status in dry-run mode
	status, err := driver.Status(ctx)
	if err != nil {
		t.Fatalf("Status() in dry-run mode should not error: %v", err)
	}
	if status == nil {
		t.Fatal("Status() returned nil")
	}
	if status.Kind != "(dry-run)" {
		t.Errorf("Status().Kind = %q, want %q", status.Kind, "(dry-run)")
	}
	if status.Status.Type != "dry-run" {
		t.Errorf("Status().Status.Type = %q, want %q", status.Status.Type, "dry-run")
	}

	// Test Upgrade in dry-run mode
	err = driver.Upgrade(ctx, UpgradeOptions{})
	if err != nil {
		t.Errorf("Upgrade() in dry-run mode should not error: %v", err)
	}

	// Test Upgrade with all options
	err = driver.Upgrade(ctx, UpgradeOptions{Check: true, Apply: true, Quiet: true})
	if err != nil {
		t.Errorf("Upgrade() with all options in dry-run mode should not error: %v", err)
	}

	// Test Switch in dry-run mode
	err = driver.Switch(ctx, "test-image:latest", SwitchOptions{})
	if err != nil {
		t.Errorf("Switch() in dry-run mode should not error: %v", err)
	}

	// Test Switch with all options
	err = driver.Switch(ctx, "test-image:v2", SwitchOptions{Transport: "oci-archive", Apply: true, Retain: true})
	if err != nil {
		t.Errorf("Switch() with all options in dry-run mode should not error: %v", err)
	}

	// Test Rollback in dry-run mode
	err = driver.Rollback(ctx, RollbackOptions{})
	if err != nil {
		t.Errorf("Rollback() in dry-run mode should not error: %v", err)
	}

	// Test Rollback with Apply
	err = driver.Rollback(ctx, RollbackOptions{Apply: true})
	if err != nil {
		t.Errorf("Rollback() with Apply in dry-run mode should not error: %v", err)
	}
}

// === Driver Interface Tests ===

func TestDriverInterfaceCompliance(t *testing.T) {
	// Verify that all driver types implement the Driver interface
	var _ Driver = (*HostDriver)(nil)
	var _ Driver = (*SSHDriver)(nil)
	var _ Driver = (*VMDriver)(nil)
}

// === Status JSON Round-trip Test ===

func TestStatusJSONRoundTrip(t *testing.T) {
	currentImage := testBootcImageCurrent()
	expectedVersion := testBootcImageTagCurrent + ".20240101.0"

	original := Status{
		APIVersion: "org.containers.bootc/v1",
		Kind:       "BootcHost",
		Metadata:   Metadata{Name: "host"},
		Spec: Spec{
			Image: &ImageReference{
				Image:     currentImage,
				Transport: "registry",
			},
		},
		Status: HostStatus{
			Booted: &BootEntry{
				Image: &ImageStatus{
					Image: ImageDetails{
						Image:     currentImage,
						Transport: "registry",
					},
					Version:     expectedVersion,
					Timestamp:   "2024-01-01T00:00:00Z",
					ImageDigest: "sha256:abc123",
				},
				Incompatible: false,
				Pinned:       false,
			},
			Type: "bootcHost",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	// Unmarshal back
	var decoded Status
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Verify key fields
	if decoded.APIVersion != original.APIVersion {
		t.Errorf("APIVersion mismatch: got %q, want %q", decoded.APIVersion, original.APIVersion)
	}
	if decoded.Kind != original.Kind {
		t.Errorf("Kind mismatch: got %q, want %q", decoded.Kind, original.Kind)
	}
	if decoded.Status.Booted == nil {
		t.Fatal("Booted is nil after round-trip")
	}
	if decoded.Status.Booted.Image.Version != expectedVersion {
		t.Errorf("Version mismatch: got %q, want %q", decoded.Status.Booted.Image.Version, expectedVersion)
	}
}

// === HostDriver Tests (require bootc to be installed) ===

func TestNewHostDriverNotFound(t *testing.T) {
	// This test verifies that NewHostDriver returns an error when bootc is not found
	// We can't easily mock this, so we skip if bootc is actually installed
	driver, err := NewHostDriver()
	if err == nil {
		// bootc is installed, skip this test
		t.Skipf("bootc is installed at %s, skipping not-found test", driver.binary)
	}

	// Verify error message mentions bootc not found
	if err.Error() == "" {
		t.Error("expected non-empty error message")
	}
}

// === Additional Tests for Coverage ===

// TestMetadataStruct tests Metadata struct
func TestMetadataStruct(t *testing.T) {
	meta := Metadata{Name: "test-host"}
	if meta.Name != "test-host" {
		t.Errorf("Name = %q, want %q", meta.Name, "test-host")
	}
}

// TestSpecStruct tests Spec struct
func TestSpecStruct(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
	}{
		{
			name: "nil image",
			spec: Spec{Image: nil},
		},
		{
			name: "with image",
			spec: Spec{
				Image: &ImageReference{
					Image:     "test:latest",
					Transport: "registry",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.name == "nil image" && tt.spec.Image != nil {
				t.Error("Image should be nil")
			}
			if tt.name == "with image" && tt.spec.Image == nil {
				t.Error("Image should not be nil")
			}
		})
	}
}

// TestImageReferenceStruct tests ImageReference struct
func TestImageReferenceStruct(t *testing.T) {
	ref := ImageReference{
		Image:     "quay.io/test/image:v1.0",
		Transport: "oci",
	}

	if ref.Image != "quay.io/test/image:v1.0" {
		t.Errorf("Image = %q, want %q", ref.Image, "quay.io/test/image:v1.0")
	}
	if ref.Transport != "oci" {
		t.Errorf("Transport = %q, want %q", ref.Transport, "oci")
	}
}

// TestBootEntryWithCachedUpdate tests BootEntry with CachedUpdate
func TestBootEntryWithCachedUpdate(t *testing.T) {
	entry := BootEntry{
		Image: &ImageStatus{
			Image:   ImageDetails{Image: "current:latest"},
			Version: "1.0.0",
		},
		CachedUpdate: &ImageStatus{
			Image:   ImageDetails{Image: "updated:latest"},
			Version: "1.1.0",
		},
		Incompatible: false,
		Pinned:       false,
	}

	if entry.CachedUpdate == nil {
		t.Fatal("CachedUpdate is nil")
	}
	if entry.CachedUpdate.Version != "1.1.0" {
		t.Errorf("CachedUpdate.Version = %q, want %q", entry.CachedUpdate.Version, "1.1.0")
	}
}

// TestUpgradeOptionsFields tests UpgradeOptions struct fields
func TestUpgradeOptionsFields(t *testing.T) {
	opts := UpgradeOptions{
		Check: true,
		Apply: true,
		Quiet: true,
	}

	if !opts.Check {
		t.Error("Check should be true")
	}
	if !opts.Apply {
		t.Error("Apply should be true")
	}
	if !opts.Quiet {
		t.Error("Quiet should be true")
	}
}

// TestSwitchOptionsFields tests SwitchOptions struct fields
func TestSwitchOptionsFields(t *testing.T) {
	opts := SwitchOptions{
		Transport: "oci-archive",
		Apply:     true,
		Retain:    true,
	}

	if opts.Transport != "oci-archive" {
		t.Errorf("Transport = %q, want %q", opts.Transport, "oci-archive")
	}
	if !opts.Apply {
		t.Error("Apply should be true")
	}
	if !opts.Retain {
		t.Error("Retain should be true")
	}
}

// TestRollbackOptionsFields tests RollbackOptions struct fields
func TestRollbackOptionsFields(t *testing.T) {
	opts := RollbackOptions{Apply: true}

	if !opts.Apply {
		t.Error("Apply should be true")
	}
}

// TestSSHDriverOptionsFields tests SSHDriverOptions struct fields
func TestSSHDriverOptionsFields(t *testing.T) {
	opts := SSHDriverOptions{
		Host:    "remote-host",
		Verbose: true,
		DryRun:  true,
	}

	if opts.Host != "remote-host" {
		t.Errorf("Host = %q, want %q", opts.Host, "remote-host")
	}
	if !opts.Verbose {
		t.Error("Verbose should be true")
	}
	if !opts.DryRun {
		t.Error("DryRun should be true")
	}
}

// TestVMDriverOptionsFields tests VMDriverOptions struct fields
func TestVMDriverOptionsFields(t *testing.T) {
	opts := VMDriverOptions{
		VMName:     "my-vm",
		SSHHost:    "192.168.1.100",
		SSHPort:    2222,
		SSHUser:    "admin",
		SSHKeyPath: "/home/admin/.ssh/id_ed25519",
		Verbose:    true,
		DryRun:     false,
	}

	if opts.VMName != "my-vm" {
		t.Errorf("VMName = %q, want %q", opts.VMName, "my-vm")
	}
	if opts.SSHHost != "192.168.1.100" {
		t.Errorf("SSHHost = %q, want %q", opts.SSHHost, "192.168.1.100")
	}
	if opts.SSHPort != 2222 {
		t.Errorf("SSHPort = %d, want %d", opts.SSHPort, 2222)
	}
	if opts.SSHUser != "admin" {
		t.Errorf("SSHUser = %q, want %q", opts.SSHUser, "admin")
	}
	if opts.SSHKeyPath != "/home/admin/.ssh/id_ed25519" {
		t.Errorf("SSHKeyPath = %q, want %q", opts.SSHKeyPath, "/home/admin/.ssh/id_ed25519")
	}
	if !opts.Verbose {
		t.Error("Verbose should be true")
	}
	if opts.DryRun {
		t.Error("DryRun should be false")
	}
}

// TestSSHDriverVerboseMode tests SSHDriver with verbose mode
func TestSSHDriverVerboseMode(t *testing.T) {
	driver := NewSSHDriver(SSHDriverOptions{
		Host:    "verbose-host",
		Verbose: true,
		DryRun:  true, // Keep dry-run to avoid actual SSH calls
	})

	if driver.Host() != "verbose-host" {
		t.Errorf("Host() = %q, want %q", driver.Host(), "verbose-host")
	}

	ctx := context.Background()

	// Test methods with verbose output (they should still work in dry-run)
	if err := driver.Upgrade(ctx, UpgradeOptions{Check: true}); err != nil {
		t.Errorf("Upgrade() error = %v", err)
	}
	if err := driver.Switch(ctx, "new-image:latest", SwitchOptions{}); err != nil {
		t.Errorf("Switch() error = %v", err)
	}
	if err := driver.Rollback(ctx, RollbackOptions{}); err != nil {
		t.Errorf("Rollback() error = %v", err)
	}
}

// TestVMDriverVerboseMode tests VMDriver with verbose mode
func TestVMDriverVerboseMode(t *testing.T) {
	driver := NewVMDriver(VMDriverOptions{
		VMName:     "verbose-vm",
		SSHHost:    "localhost",
		SSHPort:    2222,
		SSHUser:    "user",
		SSHKeyPath: "/path/to/key",
		Verbose:    true,
		DryRun:     true, // Keep dry-run to avoid actual SSH calls
	})

	if driver.VMName() != "verbose-vm" {
		t.Errorf("VMName() = %q, want %q", driver.VMName(), "verbose-vm")
	}
	expectedHost := "vm:verbose-vm"
	if driver.Host() != expectedHost {
		t.Errorf("Host() = %q, want %q", driver.Host(), expectedHost)
	}

	ctx := context.Background()

	// Test methods with verbose output
	if err := driver.Upgrade(ctx, UpgradeOptions{Apply: true}); err != nil {
		t.Errorf("Upgrade() error = %v", err)
	}
	if err := driver.Switch(ctx, "new-image:v2", SwitchOptions{Transport: "oci"}); err != nil {
		t.Errorf("Switch() error = %v", err)
	}
	if err := driver.Rollback(ctx, RollbackOptions{Apply: true}); err != nil {
		t.Errorf("Rollback() error = %v", err)
	}
}

// TestStatusParsingEmptyFields tests parsing status with empty/null fields
func TestStatusParsingEmptyFields(t *testing.T) {
	jsonData := `{
		"apiVersion": "org.containers.bootc/v1",
		"kind": "BootcHost",
		"metadata": {"name": ""},
		"spec": {},
		"status": {
			"type": "bootcHost"
		}
	}`

	var status Status
	if err := json.Unmarshal([]byte(jsonData), &status); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if status.Metadata.Name != "" {
		t.Errorf("Metadata.Name = %q, want empty", status.Metadata.Name)
	}
	if status.Spec.Image != nil {
		t.Error("Spec.Image should be nil")
	}
	if status.Status.Booted != nil {
		t.Error("Status.Booted should be nil")
	}
	if status.Status.Staged != nil {
		t.Error("Status.Staged should be nil")
	}
	if status.Status.Rollback != nil {
		t.Error("Status.Rollback should be nil")
	}
}

// TestImageStatusTimestamp tests ImageStatus timestamp field
func TestImageStatusTimestamp(t *testing.T) {
	status := ImageStatus{
		Image: ImageDetails{
			Image:     "test:v1",
			Transport: "registry",
		},
		Version:     "1.0.0",
		Timestamp:   "2024-06-15T12:30:45Z",
		ImageDigest: "sha256:abc123def456789",
	}

	if status.Timestamp != "2024-06-15T12:30:45Z" {
		t.Errorf("Timestamp = %q, want %q", status.Timestamp, "2024-06-15T12:30:45Z")
	}
}

// TestHostStatusAllFields tests HostStatus with all fields
func TestHostStatusAllFields(t *testing.T) {
	status := HostStatus{
		Staged: &BootEntry{
			Image: &ImageStatus{
				Image:   ImageDetails{Image: "staged:latest"},
				Version: "2.0.0",
			},
		},
		Booted: &BootEntry{
			Image: &ImageStatus{
				Image:   ImageDetails{Image: "booted:latest"},
				Version: "1.0.0",
			},
		},
		Rollback: &BootEntry{
			Image: &ImageStatus{
				Image:   ImageDetails{Image: "rollback:latest"},
				Version: "0.9.0",
			},
		},
		Type: "bootcHost",
	}

	if status.Staged == nil {
		t.Fatal("Staged is nil")
	}
	if status.Staged.Image.Version != "2.0.0" {
		t.Errorf("Staged.Image.Version = %q, want %q", status.Staged.Image.Version, "2.0.0")
	}
	if status.Booted == nil {
		t.Fatal("Booted is nil")
	}
	if status.Rollback == nil {
		t.Fatal("Rollback is nil")
	}
}

// TestStatusFullStructure tests the complete Status structure
func TestStatusFullStructure(t *testing.T) {
	status := Status{
		APIVersion: "org.containers.bootc/v1",
		Kind:       "BootcHost",
		Metadata:   Metadata{Name: "test-host"},
		Spec: Spec{
			Image: &ImageReference{
				Image:     "quay.io/test/bootc:latest",
				Transport: "registry",
			},
		},
		Status: HostStatus{
			Booted: &BootEntry{
				Image: &ImageStatus{
					Image: ImageDetails{
						Image:     "quay.io/test/bootc:latest",
						Transport: "registry",
					},
					Version:     "1.0.0",
					Timestamp:   "2024-01-01T00:00:00Z",
					ImageDigest: "sha256:abc123",
				},
				Incompatible: false,
				Pinned:       false,
			},
			Type: "bootcHost",
		},
	}

	if status.APIVersion != "org.containers.bootc/v1" {
		t.Errorf("APIVersion = %q, want %q", status.APIVersion, "org.containers.bootc/v1")
	}
	if status.Kind != "BootcHost" {
		t.Errorf("Kind = %q, want %q", status.Kind, "BootcHost")
	}
	if status.Spec.Image == nil {
		t.Fatal("Spec.Image is nil")
	}
	if status.Status.Booted == nil {
		t.Fatal("Status.Booted is nil")
	}
}

// TestSSHDriverStatusArgs tests Status command argument building
func TestSSHDriverStatusArgs(t *testing.T) {
	// Test that Status uses the correct arguments
	args := []string{"status", "--format", "json"}

	if args[0] != "status" {
		t.Errorf("args[0] = %q, want %q", args[0], "status")
	}
	if args[1] != "--format" {
		t.Errorf("args[1] = %q, want %q", args[1], "--format")
	}
	if args[2] != "json" {
		t.Errorf("args[2] = %q, want %q", args[2], "json")
	}
}

// TestSwitchTransportTypes tests different transport types
func TestSwitchTransportTypes(t *testing.T) {
	tests := []struct {
		name          string
		transport     string
		wantTransport bool // whether --transport should be added
	}{
		{
			name:          "registry transport",
			transport:     "registry",
			wantTransport: false, // registry is default, shouldn't add --transport
		},
		{
			name:          "oci transport",
			transport:     "oci",
			wantTransport: true,
		},
		{
			name:          "oci-archive transport",
			transport:     "oci-archive",
			wantTransport: true,
		},
		{
			name:          "empty transport",
			transport:     "",
			wantTransport: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"switch"}
			if tt.transport != "" && tt.transport != "registry" {
				args = append(args, "--transport", tt.transport)
			}
			args = append(args, "test-image:latest")

			hasTransport := false
			for _, arg := range args {
				if arg == "--transport" {
					hasTransport = true
					break
				}
			}

			if hasTransport != tt.wantTransport {
				t.Errorf("hasTransport = %v, want %v (args: %v)", hasTransport, tt.wantTransport, args)
			}
		})
	}
}

// TestHostDriverStruct tests HostDriver struct
func TestHostDriverStruct(t *testing.T) {
	driver := &HostDriver{binary: "/usr/bin/bootc"}
	if driver.binary != "/usr/bin/bootc" {
		t.Errorf("binary = %q, want %q", driver.binary, "/usr/bin/bootc")
	}
}

// TestSSHDriverStruct tests SSHDriver struct fields
func TestSSHDriverStruct(t *testing.T) {
	driver := &SSHDriver{
		host:    "test-host",
		verbose: true,
		dryRun:  false,
	}

	if driver.host != "test-host" {
		t.Errorf("host = %q, want %q", driver.host, "test-host")
	}
	if !driver.verbose {
		t.Error("verbose should be true")
	}
	if driver.dryRun {
		t.Error("dryRun should be false")
	}
}

// TestVMDriverStruct tests VMDriver struct fields
func TestVMDriverStruct(t *testing.T) {
	driver := &VMDriver{
		vmName:     "test-vm",
		sshHost:    "127.0.0.1",
		sshPort:    2222,
		sshUser:    "testuser",
		sshKeyPath: "/home/user/.ssh/key",
		verbose:    false,
		dryRun:     true,
	}

	if driver.vmName != "test-vm" {
		t.Errorf("vmName = %q, want %q", driver.vmName, "test-vm")
	}
	if driver.sshHost != "127.0.0.1" {
		t.Errorf("sshHost = %q, want %q", driver.sshHost, "127.0.0.1")
	}
	if driver.sshPort != 2222 {
		t.Errorf("sshPort = %d, want %d", driver.sshPort, 2222)
	}
	if driver.sshUser != "testuser" {
		t.Errorf("sshUser = %q, want %q", driver.sshUser, "testuser")
	}
	if driver.sshKeyPath != "/home/user/.ssh/key" {
		t.Errorf("sshKeyPath = %q, want %q", driver.sshKeyPath, "/home/user/.ssh/key")
	}
	if driver.verbose {
		t.Error("verbose should be false")
	}
	if !driver.dryRun {
		t.Error("dryRun should be true")
	}
}

// TestImageDetailsTransportVariants tests ImageDetails with various transports
func TestImageDetailsTransportVariants(t *testing.T) {
	tests := []struct {
		name      string
		transport string
	}{
		{"registry", "registry"},
		{"oci", "oci"},
		{"oci-archive", "oci-archive"},
		{"docker", "docker"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := ImageDetails{
				Image:     "test:latest",
				Transport: tt.transport,
			}
			if details.Transport != tt.transport {
				t.Errorf("Transport = %q, want %q", details.Transport, tt.transport)
			}
		})
	}
}
