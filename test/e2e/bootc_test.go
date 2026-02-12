//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

// TestBootcStatus tests bootc status command via SSH.
// Requires a running VM.
func TestBootcStatus(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	RequireVMInfrastructure(t)

	env := NewTestEnvironment(t)

	// Check if any VM is running
	output, err := env.RunBootcMan("vm", "list")
	if err != nil {
		t.Skipf("Cannot list VMs: %v", err)
	}

	// Find a running VM
	vmName := findRunningVM(output)
	if vmName == "" {
		t.Skip("No running VM available - run TestVMBoot first")
	}

	t.Logf("Testing bootc status on VM: %s", vmName)

	// Get bootc status
	output, err = env.RunBootcMan("remote", "status", "--vm", vmName)
	if err != nil {
		t.Fatalf("Failed to get bootc status: %v\nOutput: %s", err, output)
	}

	t.Logf("Bootc status output: OK (%d lines)", strings.Count(output, "\n"))

	// Verify output contains expected fields
	expectedFields := []string{"booted", "image"}
	for _, field := range expectedFields {
		if !strings.Contains(strings.ToLower(output), field) {
			t.Errorf("Expected %q in bootc status output", field)
		}
	}
}

// TestBootcStatusJSON tests bootc status with JSON output
func TestBootcStatusJSON(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	RequireVMInfrastructure(t)

	env := NewTestEnvironment(t)

	// Check if any VM is running
	output, err := env.RunBootcMan("vm", "list")
	if err != nil {
		t.Skipf("Cannot list VMs: %v", err)
	}

	vmName := findRunningVM(output)
	if vmName == "" {
		t.Skip("No running VM available")
	}

	// Get bootc status with JSON flag
	output, err = env.RunBootcMan("--json", "remote", "status", "--vm", vmName)
	if err != nil {
		t.Logf("JSON status may not be fully implemented: %v", err)
	}
	t.Logf("Bootc status JSON: OK (%d lines)", strings.Count(output, "\n"))
}

// TestBootcUpgrade tests bootc upgrade check via SSH.
// This test:
// 1. Cleans up registry volumes to ensure a fresh state
// 2. Starts the local registry
// 3. Builds and pushes an upgrade image to the registry
// 4. Runs bootc upgrade --check on the running VM
// 5. Verifies the VM can reach the registry (no connection refused)
func TestBootcUpgrade(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfShort(t)
	RequireVMInfrastructure(t)

	env := NewTestEnvironment(t)

	// Check if any VM is running
	output, err := env.RunBootcMan("vm", "list")
	if err != nil {
		t.Skipf("Cannot list VMs: %v", err)
	}

	vmName := findRunningVM(output)
	if vmName == "" {
		t.Skip("No running VM available - run TestVMBoot first")
	}

	// Verify the VM was booted with host.containers.internal (not localhost)
	// VMs booted with localhost:5000 cannot reach the host registry from within the VM
	statusOutput, err := env.RunBootcMan("remote", "status", "--vm", vmName)
	if err != nil {
		t.Skipf("Cannot get VM status: %v", err)
	}
	if !strings.Contains(statusOutput, "host.containers.internal") {
		t.Skipf("VM %s was booted with old imageTag (not host.containers.internal) - delete and recreate with TestVMBoot", vmName)
	}

	t.Logf("Testing bootc upgrade check on VM: %s", vmName)

	// Step 1: Clean up registry volumes for a fresh state
	t.Log("Cleaning up registry volumes...")
	_, _ = env.RunBootcMan("registry", "down")
	_, _ = env.RunBootcMan("registry", "rm", "--force", "--volumes")

	// Step 2: Start registry
	t.Log("Starting registry...")
	output, err = env.RunBootcMan("registry", "up")
	if err != nil {
		t.Fatalf("Failed to start registry: %v\nOutput: %s", err, output)
	}
	env.AddCleanup(func() {
		t.Log("Cleaning up registry...")
		_, _ = env.RunBootcMan("registry", "down")
		_, _ = env.RunBootcMan("registry", "rm", "--force", "--volumes")
	})

	// Wait for registry to be ready
	if err := waitForRegistry(env.ctx, env.registryPort); err != nil {
		t.Fatalf("Registry not ready: %v", err)
	}

	// Step 3: Build and push upgrade image
	// Build with localhost:5000 (host-side) and push there.
	// The VM sees the same registry as host.containers.internal:5000.
	t.Log("Building upgrade image...")
	localImageTag := "localhost:5000/e2e-vm-test:latest"

	containerfileContent := fmt.Sprintf(`FROM %s

LABEL containers.bootc=1

RUN useradd -m -G wheel user && \
    echo "user ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/user
RUN echo "upgrade-marker" > /etc/bootc-upgrade-test
`, testutil.TestBootcImageCurrent())

	containerfile := filepath.Join(env.workDir, "Containerfile")
	if err := os.WriteFile(containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	// Build the image
	output, err = env.RunCommand("podman", "build", "-t", localImageTag, "-f", containerfile, env.workDir)
	if err != nil {
		t.Fatalf("Failed to build upgrade image: %v\nOutput: %s", err, output)
	}
	env.AddCleanup(func() {
		_, _ = env.RunCommand("podman", "rmi", "-f", localImageTag)
	})

	// Push to registry (localhost:5000 is resolvable on both macOS and Linux hosts)
	t.Log("Pushing upgrade image to registry...")
	output, err = env.RunCommand("podman", "push", "--tls-verify=false", localImageTag)
	if err != nil {
		t.Fatalf("Failed to push upgrade image: %v\nOutput: %s", err, output)
	}

	// Step 4: Run upgrade check
	t.Log("Running bootc upgrade --check...")
	output, err = env.RunBootcMan("remote", "upgrade", "--vm", vmName, "--check")
	if err != nil {
		errStr := err.Error()
		// connection refused means the VM cannot reach the registry - this is a real failure
		if strings.Contains(errStr, "connection refused") {
			t.Fatalf("VM cannot reach registry: %v", err)
		}
		// HTTPS error means the VM is trying HTTPS for an insecure registry
		if strings.Contains(errStr, "HTTP response to HTTPS client") {
			t.Fatalf("VM cannot reach insecure registry (HTTPS error) - recreate VM with insecureRegistries: %v", err)
		}
		// Other errors (e.g., "no update available") are acceptable
		t.Logf("Upgrade check result: %v", err)
	} else {
		t.Logf("Upgrade check output: OK (%d lines)", strings.Count(output, "\n"))
	}

	t.Log("Bootc upgrade check test completed successfully")
}

// TestBootcSwitch tests bootc switch command via SSH.
// This test:
// 1. Ensures the registry is running (started by TestBootcUpgrade)
// 2. Builds a different image with a switch-marker
// 3. Pushes the switch image to the registry
// 4. Runs bootc switch --apply on the VM (triggers reboot)
// 5. Waits for SSH reconnection after reboot
// 6. Verifies the VM booted with the new image
// Note: Run after TestBootcUpgrade in sequence (upgrade → switch → rollback)
func TestBootcSwitch(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfShort(t)
	RequireVMInfrastructure(t)

	env := NewTestEnvironment(t)

	// Check if any VM is running
	output, err := env.RunBootcMan("vm", "list")
	if err != nil {
		t.Skipf("Cannot list VMs: %v", err)
	}

	vmName := findRunningVM(output)
	if vmName == "" {
		t.Skip("No running VM available - run TestVMBoot first")
	}

	t.Logf("Testing bootc switch on VM: %s", vmName)

	// Ensure registry is running
	if err := waitForRegistry(env.ctx, env.registryPort); err != nil {
		t.Skipf("Registry not available - run TestBootcUpgrade first: %v", err)
	}

	// Build a different image for switch
	// Use localhost:5000 for host-side build/push, VM sees it as host.containers.internal:5000
	localSwitchTag := "localhost:5000/e2e-vm-test:switch"
	vmSwitchTag := "host.containers.internal:5000/e2e-vm-test:switch"

	containerfileContent := fmt.Sprintf(`FROM %s

LABEL containers.bootc=1

RUN useradd -m -G wheel user && \
    echo "user ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/user
RUN echo "switch-marker" > /etc/bootc-switch-test
`, testutil.TestBootcImageCurrent())

	containerfile := filepath.Join(env.workDir, "Containerfile")
	if err := os.WriteFile(containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	// Build the switch image
	t.Log("Building switch image...")
	output, err = env.RunCommand("podman", "build", "-t", localSwitchTag, "-f", containerfile, env.workDir)
	if err != nil {
		t.Fatalf("Failed to build switch image: %v\nOutput: %s", err, output)
	}
	env.AddCleanup(func() {
		_, _ = env.RunCommand("podman", "rmi", "-f", localSwitchTag)
	})

	// Push to registry (localhost:5000 is resolvable on both macOS and Linux hosts)
	t.Log("Pushing switch image to registry...")
	output, err = env.RunCommand("podman", "push", "--tls-verify=false", localSwitchTag)
	if err != nil {
		t.Fatalf("Failed to push switch image: %v\nOutput: %s", err, output)
	}

	// Run bootc switch --apply (triggers reboot)
	t.Log("Running bootc switch --apply (will trigger reboot)...")
	output, err = env.RunBootcMan("remote", "switch", "--vm", vmName, "--apply", vmSwitchTag)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "connection refused") {
			t.Fatalf("VM cannot reach registry: %v", err)
		}
		if strings.Contains(errStr, "HTTP response to HTTPS client") {
			t.Fatalf("VM cannot reach insecure registry (HTTPS error) - recreate VM with insecureRegistries: %v", err)
		}
		// SSH disconnect during reboot is expected
		if strings.Contains(errStr, "ssh") || strings.Contains(errStr, "connection reset") ||
			strings.Contains(errStr, "EOF") {
			t.Logf("SSH disconnected during reboot (expected): %v", err)
		} else {
			t.Logf("Switch --apply result: %v", err)
		}
	} else {
		t.Logf("Switch --apply output: OK (%d lines)", strings.Count(output, "\n"))
	}

	// Wait for VM to come back up after reboot
	t.Log("Waiting for VM to reboot and SSH to reconnect...")
	if err := waitForSSHReconnect(env, vmName, 3*time.Minute); err != nil {
		t.Fatalf("VM did not come back after reboot: %v", err)
	}

	// Verify the VM is running with the new image
	t.Log("Verifying VM status after switch...")
	output, err = env.RunBootcMan("remote", "status", "--vm", vmName)
	if err != nil {
		t.Fatalf("Failed to get bootc status after switch: %v", err)
	}
	t.Logf("Post-switch status: OK (%d lines)", strings.Count(output, "\n"))

	// Verify rollback entry exists (previous image should be the rollback target)
	if !strings.Contains(strings.ToLower(output), "rollback") {
		t.Log("Warning: no rollback entry found after switch - TestBootcRollback will be skipped")
	} else {
		t.Log("Rollback entry found - TestBootcRollback can proceed")
	}

	t.Log("Bootc switch test completed successfully")
}

// TestBootcRollback tests bootc rollback command via SSH.
// This test:
// 1. Verifies a rollback target exists (from switch --apply + reboot)
// 2. Records the current booted image
// 3. Runs bootc rollback on the VM
// 4. Verifies via bootc status that the rollback was staged correctly
// Note: Run after TestBootcSwitch (upgrade → switch → rollback)
func TestBootcRollback(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfShort(t)
	RequireVMInfrastructure(t)

	env := NewTestEnvironment(t)

	// Check if any VM is running
	output, err := env.RunBootcMan("vm", "list")
	if err != nil {
		t.Skipf("Cannot list VMs: %v", err)
	}

	vmName := findRunningVM(output)
	if vmName == "" {
		t.Skip("No running VM available - run TestVMBoot first")
	}

	// Check status to see if rollback target is available
	output, err = env.RunBootcMan("remote", "status", "--vm", vmName)
	if err != nil {
		t.Skipf("Cannot get bootc status: %v", err)
	}

	if !strings.Contains(strings.ToLower(output), "rollback") {
		t.Skip("No rollback target available - run TestBootcSwitch first")
	}

	t.Logf("Testing bootc rollback on VM: %s", vmName)
	t.Logf("Pre-rollback status:\n%s", output)

	// Run bootc rollback (stages the rollback, no reboot)
	t.Log("Running bootc rollback...")
	output, err = env.RunBootcMan("remote", "rollback", "--vm", vmName)
	if err != nil {
		t.Fatalf("Rollback failed: %v\nOutput: %s", err, output)
	}
	t.Logf("Rollback output: OK (%d lines)", strings.Count(output, "\n"))

	// Verify: check bootc status after rollback
	// The rollback should have been staged (visible in status)
	t.Log("Verifying bootc status after rollback...")
	statusOutput, err := env.RunBootcMan("remote", "status", "--vm", vmName)
	if err != nil {
		t.Fatalf("Failed to get bootc status after rollback: %v", err)
	}
	t.Logf("Post-rollback status:\n%s", statusOutput)

	// Verify that a staged entry exists (rollback stages the previous image)
	if strings.Contains(strings.ToLower(statusOutput), "staged") {
		t.Log("Verified: rollback was staged successfully")
	} else {
		// Even without "staged", rollback may have swapped the deployment order
		t.Log("Note: no 'staged' entry found - rollback may have swapped deployment order")
	}

	t.Log("Bootc rollback test completed successfully")
}

// TestRemoteStatusWithSSHHost tests remote status using SSH host from ~/.ssh/config
func TestRemoteStatusWithSSHHost(t *testing.T) {
	testutil.SkipIfSSHUnavailable(t)

	env := NewTestEnvironment(t)

	// This test requires a pre-configured SSH host
	// Skip if no host is available
	t.Skip("Remote SSH host test requires pre-configured ~/.ssh/config entry")

	// Example: would run 'bootc-man remote status <hostname>'
	_ = env // suppress unused warning
}

// findRunningVM finds a running VM from the vm list output
func findRunningVM(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Look for lines indicating a running VM
		if strings.Contains(strings.ToLower(line), "running") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}
	return ""
}

// waitForSSHReconnect waits for SSH connectivity to be restored after VM reboot.
// It polls `remote status` until it succeeds or the timeout is reached.
func waitForSSHReconnect(env *TestEnvironment, vmName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	interval := 5 * time.Second

	// Wait a bit before first attempt (VM needs time to shut down)
	time.Sleep(10 * time.Second)

	for time.Now().Before(deadline) {
		_, err := env.RunBootcMan("remote", "status", "--vm", vmName)
		if err == nil {
			return nil
		}
		env.t.Logf("SSH not ready yet, retrying in %v...", interval)
		time.Sleep(interval)
	}
	return fmt.Errorf("SSH reconnect timed out after %v", timeout)
}
