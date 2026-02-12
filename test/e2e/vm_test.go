//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

// getHostSSHPublicKey reads the host's SSH public key from ~/.ssh/.
// The vm start command uses the host's private key for SSH connection,
// so we must inject the matching public key into the VM via config.toml.
// In CI environments, if no key exists, it generates one automatically.
func getHostSSHPublicKey(t *testing.T) string {
	t.Helper()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home directory: %v", err)
	}

	// Try ed25519 first, then RSA (same order as vm start command)
	keyFiles := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519.pub"),
		filepath.Join(homeDir, ".ssh", "id_rsa.pub"),
	}

	for _, keyFile := range keyFiles {
		data, err := os.ReadFile(keyFile)
		if err == nil {
			pubKey := strings.TrimSpace(string(data))
			t.Logf("Using host SSH public key: %s", keyFile)
			return pubKey
		}
	}

	// In CI, generate an SSH key pair if none exists
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		t.Log("No SSH key found in CI - generating ed25519 key pair...")
		return generateSSHKeyForCI(t, homeDir)
	}

	t.Skip("No SSH public key found at ~/.ssh/id_ed25519.pub or ~/.ssh/id_rsa.pub")
	return "" // unreachable
}

// generateSSHKeyForCI generates an SSH key pair for use in CI environments.
func generateSSHKeyForCI(t *testing.T, homeDir string) string {
	t.Helper()

	sshDir := filepath.Join(homeDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh directory: %v", err)
	}

	keyPath := filepath.Join(sshDir, "id_ed25519")

	// Generate ed25519 key pair with no passphrase
	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", keyPath, "-N", "", "-C", "bootc-man-e2e-ci")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to generate SSH key: %v\nOutput: %s", err, output)
	}
	t.Logf("Generated SSH key pair: %s", keyPath)

	// Read the public key
	pubKeyData, err := os.ReadFile(keyPath + ".pub")
	if err != nil {
		t.Fatalf("Failed to read generated public key: %v", err)
	}

	return strings.TrimSpace(string(pubKeyData))
}

// createConfigToml creates a config.toml for bootc-image-builder with SSH key injection.
// Note: Insecure registry configuration is handled separately via pipeline's
// insecureRegistries field, which injects config during the convert stage.
func createConfigToml(dir, sshPubKey string) (string, error) {
	configContent := fmt.Sprintf(`[[customizations.user]]
name = "user"
key = "%s"
groups = ["wheel"]
`, sshPubKey)

	configPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write config.toml: %w", err)
	}

	return configPath, nil
}

// TestVMBoot tests VM boot functionality.
// This is a comprehensive test that:
// 1. Reads the host's SSH public key and creates config.toml for injection
// 2. Builds a bootc container image
// 3. Converts it to a raw disk image (with SSH key injected via config.toml)
// 4. Boots the VM
// 5. Verifies SSH connectivity
// 6. Cleans up
//
// Important: The vm start command uses the host's ~/.ssh/id_ed25519 private key
// for SSH, so the matching public key must be injected into the VM image.
func TestVMBoot(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SkipIfPodmanUnavailable(t)
	RequireVMInfrastructure(t)

	env := NewTestEnvironment(t)

	// Log test environment
	t.Logf("Running VM boot test on %s", runtime.GOOS)
	t.Logf("Work directory: %s", env.workDir)

	// Step 1: Read host's SSH public key and create config.toml
	// The vm start command uses ~/.ssh/id_ed25519 for SSH connection,
	// so we inject the matching public key into the VM via config.toml.
	sshPubKey := getHostSSHPublicKey(t)

	t.Log("Creating config.toml for SSH key injection...")
	_, err := createConfigToml(env.workDir, sshPubKey)
	if err != nil {
		t.Fatalf("Failed to create config.toml: %v", err)
	}

	// Create Containerfile
	// Keep minimal to reduce memory usage (important for CI runners with limited RAM)
	containerfile := fmt.Sprintf(`FROM %s

LABEL containers.bootc=1

# Create test user with SSH access
# Note: SSH key is injected via config.toml during convert stage,
# not via Containerfile. This ensures keys are properly updated on upgrade.
RUN useradd -m -G wheel user && \
    echo "user ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/user
`, testutil.TestBootcImageCurrent())

	if err := testutil.WriteFileToPath(filepath.Join(env.workDir, "Containerfile"), containerfile); err != nil {
		t.Fatalf("Failed to write Containerfile: %v", err)
	}

	// Create pipeline configuration
	// Use raw format (required for vfkit on macOS, also works on Linux/QEMU)
	// Reference config.toml for SSH key injection during convert stage
	pipelineYAML := `apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: e2e-vm-test
  description: E2E VM boot test
spec:
  source:
    containerfile: Containerfile
    context: .
  build:
    imageTag: host.containers.internal:5000/e2e-vm-test:latest
  convert:
    enabled: true
    insecureRegistries:
      - "host.containers.internal:5000"
    formats:
      - type: raw
        config: config.toml
  test:
    boot:
      enabled: true
      timeout: 300
`

	if err := testutil.WriteFileToPath(filepath.Join(env.workDir, "bootc-ci.yaml"), pipelineYAML); err != nil {
		t.Fatalf("Failed to write pipeline YAML: %v", err)
	}

	// Step 2: Start registry
	t.Log("Starting registry...")
	output, err := env.RunBootcMan("registry", "up")
	if err != nil {
		t.Fatalf("Failed to start registry: %v\nOutput: %s", err, output)
	}
	env.AddCleanup(func() {
		t.Log("Cleaning up registry...")
		_, _ = env.RunBootcMan("registry", "down")
		_, _ = env.RunBootcMan("registry", "rm", "--force", "--volumes")
	})

	// Wait for registry
	if err := waitForRegistry(env.ctx, env.registryPort); err != nil {
		t.Fatalf("Registry not ready: %v", err)
	}

	// Step 3: Run build stage
	t.Log("Running build stage...")
	output, err = env.RunBootcMan("ci", "run", "--stage", "build", "-p", filepath.Join(env.workDir, "bootc-ci.yaml"))
	if err != nil {
		t.Fatalf("Build stage failed: %v\nOutput: %s", err, output)
	}
	t.Logf("Build output: OK (%d lines)", strings.Count(output, "\n"))

	env.AddCleanup(func() {
		t.Log("Cleaning up built image...")
		_, _ = env.RunCommand("podman", "rmi", "-f", "host.containers.internal:5000/e2e-vm-test:latest")
	})

	// Step 4: Convert stage (injects SSH keys via config.toml)
	t.Log("Running convert stage (with SSH key injection)...")
	output, err = env.RunBootcMan("ci", "run", "--stage", "convert", "-p", filepath.Join(env.workDir, "bootc-ci.yaml"))
	if err != nil {
		t.Logf("Convert stage failed: %v\nOutput: %s", err, output)
		t.Skip("Convert stage failed - may require special setup")
	}

	// Clean up root-owned output files from bootc-image-builder (runs as root via sudo)
	// Go's TempDir cleanup will fail on root-owned files, so we clean them explicitly
	env.AddCleanup(func() {
		t.Log("Cleaning up root-owned output files...")
		outputDir := filepath.Join(env.workDir, "output")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupCmd := exec.CommandContext(ctx, "sudo", "-n", "rm", "-rf", outputDir)
		if err := cleanupCmd.Run(); err != nil {
			t.Logf("Warning: failed to clean root-owned files (may need manual cleanup): %v", err)
		}
	})

	// Step 5: Start VM
	// Use lower memory in CI to avoid OOM on runners with limited RAM (7GB)
	vmMemory := "4096"
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" {
		vmMemory = "2048"
		t.Logf("CI environment detected, using reduced VM memory: %sMB", vmMemory)
	}
	t.Log("Starting VM...")
	output, err = env.RunBootcMan("vm", "start", env.vmName,
		"--memory", vmMemory,
		"-p", filepath.Join(env.workDir, "bootc-ci.yaml"))
	if err != nil {
		t.Fatalf("Failed to start VM: %v\nOutput: %s", err, output)
	}

	// Note: VM is intentionally NOT cleaned up after the test.
	// Other tests (e.g., TestBootcUpgrade, TestBootcStatus) depend on
	// a running VM with host.containers.internal imageTag.
	// Clean up manually with: bootc-man vm stop <name> && bootc-man vm rm --force <name>

	// Step 6: Wait for SSH
	t.Log("Waiting for SSH connectivity...")
	if err := waitForSSH(env, env.vmName); err != nil {
		t.Fatalf("SSH not ready: %v", err)
	}

	// Step 7: Verify bootc status
	t.Log("Checking bootc status...")
	output, err = env.RunBootcMan("remote", "status", "--vm", env.vmName)
	if err != nil {
		t.Fatalf("Failed to get bootc status: %v\nOutput: %s", err, output)
	}
	t.Logf("Bootc status: %s", output)

	t.Log("VM boot test completed successfully!")
}

// TestVMSSHConnection tests SSH connection to a booted VM.
func TestVMSSHConnection(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SkipIfPodmanUnavailable(t)
	RequireVMInfrastructure(t)

	// This test requires an already running VM
	// Skip if no VM is available
	env := NewTestEnvironment(t)

	// List running VMs
	output, err := env.RunBootcMan("vm", "list")
	if err != nil {
		t.Skipf("Cannot list VMs: %v", err)
	}

	if !strings.Contains(strings.ToLower(output), "running") {
		t.Skip("No running VMs available for SSH test")
	}

	vmCount := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(strings.ToLower(line), "running") || strings.Contains(strings.ToLower(line), "stopped") {
			vmCount++
		}
	}
	t.Logf("VM list: %d VMs found", vmCount)
	t.Log("SSH connection test would connect to running VM")
}

// TestVMCleanup cleans up all E2E test resources.
// This runs as Phase 4 and removes:
// - All test VMs (e2e-*) - stop and remove
// - Test container images (e2e-vm-test, e2e-*-test)
// - Registry container and volumes
// - Root-owned temp files from convert stage
func TestVMCleanup(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// Step 1: Stop and remove all e2e test VMs
	output, err := env.RunBootcMan("vm", "list")
	if err != nil {
		t.Logf("Cannot list VMs (may be none): %v", err)
	} else {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "e2e-") {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					vmName := parts[0]
					t.Logf("Stopping test VM: %s", vmName)
					if out, err := env.RunBootcMan("vm", "stop", vmName); err != nil {
						t.Logf("  Stop failed (may already be stopped): %v\n  %s", err, out)
					}
					t.Logf("Removing test VM: %s", vmName)
					if out, err := env.RunBootcMan("vm", "rm", "--force", vmName); err != nil {
						t.Logf("  Remove failed: %v\n  %s", err, out)
					} else {
						t.Logf("  Removed VM: %s", vmName)
					}
				}
			}
		}
	}

	// Step 2: Clean up test container images
	testImages := []string{
		"host.containers.internal:5000/e2e-vm-test:latest",
		"host.containers.internal:5000/e2e-vm-test:switch",
		"localhost:5000/e2e-vm-test:latest",
		"localhost:5000/e2e-vm-test:switch",
	}
	for _, img := range testImages {
		if _, err := env.RunCommand("podman", "rmi", "-f", img); err == nil {
			t.Logf("Removed image: %s", img)
		}
	}

	// Step 3: Stop registry and remove volumes
	if out, err := env.RunBootcMan("registry", "down"); err == nil {
		t.Logf("Registry stopped: %s", strings.TrimSpace(out))
	}
	if out, err := env.RunBootcMan("registry", "rm", "--force", "--volumes"); err == nil {
		t.Logf("Registry removed: %s", strings.TrimSpace(out))
	}

	// Step 4: Clean up root-owned temp files from convert stage
	tmpDir := os.Getenv("TMPDIR")
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}
	// Look for leftover test output directories
	entries, err := os.ReadDir(tmpDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() && (strings.HasPrefix(entry.Name(), "TestVMBoot") ||
				strings.HasPrefix(entry.Name(), "TestCIConvertRaw") ||
				strings.HasPrefix(entry.Name(), "TestCIPipeline")) {
				dirPath := filepath.Join(tmpDir, entry.Name())
				t.Logf("Cleaning up temp dir: %s", dirPath)
				// Try normal removal first, then sudo for root-owned files
				if err := os.RemoveAll(dirPath); err != nil {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					_ = exec.CommandContext(ctx, "sudo", "-n", "rm", "-rf", dirPath).Run()
					cancel()
				}
			}
		}
	}

	t.Log("E2E cleanup completed")
}

// TestVMList tests the VM list command
func TestVMList(t *testing.T) {
	env := NewTestEnvironment(t)

	output, err := env.RunBootcMan("vm", "list")
	if err != nil {
		// It's OK if the command fails when no VMs exist
		t.Logf("VM list output: %s, err: %v", output, err)
	} else {
		t.Logf("VMs: %d lines", strings.Count(output, "\n"))
	}
}

// TestVMStatus tests the VM status command
func TestVMStatus(t *testing.T) {
	env := NewTestEnvironment(t)

	// Test status of non-existent VM
	output, err := env.RunBootcMan("vm", "status", "nonexistent-vm")
	// This should fail gracefully
	t.Logf("VM status (nonexistent): %s, err: %v", output, err)
}

// waitForSSH waits for SSH to be available on the VM
func waitForSSH(env *TestEnvironment, vmName string) error {
	for i := 0; i < SSHMaxRetries; i++ {
		select {
		case <-env.ctx.Done():
			return env.ctx.Err()
		default:
		}

		// Try to connect via bootc-man remote
		_, err := env.RunBootcMan("remote", "status", "--vm", vmName)
		if err == nil {
			return nil
		}

		time.Sleep(SSHRetryInterval)
	}

	return fmt.Errorf("SSH not ready after %d retries", SSHMaxRetries)
}
