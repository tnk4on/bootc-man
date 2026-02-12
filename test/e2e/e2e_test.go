//go:build e2e
// +build e2e

// Package e2e contains end-to-end tests for bootc-man.
// These tests require actual VM infrastructure (vfkit on macOS, QEMU on Linux).
package e2e

import (
	"bytes"
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

// E2E test configuration
const (
	// DefaultTimeout is the default timeout for E2E operations
	DefaultTimeout = 10 * time.Minute

	// VMBootTimeout is the timeout for VM boot operations
	VMBootTimeout = 5 * time.Minute

	// SSHRetryInterval is the interval between SSH retry attempts
	SSHRetryInterval = 5 * time.Second

	// SSHMaxRetries is the maximum number of SSH connection retries
	SSHMaxRetries = 60

	// TestVMName is the default VM name for E2E tests
	TestVMName = "e2e-test-vm"
)

// TestEnvironment holds the E2E test environment state
type TestEnvironment struct {
	t             *testing.T
	ctx           context.Context
	cancel        context.CancelFunc
	workDir       string
	registryPort  int
	vmName        string
	sshKeyPath    string
	sshPort       int
	cleanupFuncs  []func()
}

// NewTestEnvironment creates a new E2E test environment
func NewTestEnvironment(t *testing.T) *TestEnvironment {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)

	env := &TestEnvironment{
		t:            t,
		ctx:          ctx,
		cancel:       cancel,
		workDir:      t.TempDir(),
		registryPort: 5000,
		vmName:       fmt.Sprintf("%s-%d", TestVMName, time.Now().UnixNano()%10000),
		cleanupFuncs: []func(){},
	}

	// Register cleanup
	t.Cleanup(func() {
		env.Cleanup()
	})

	return env
}

// Cleanup runs all registered cleanup functions
func (e *TestEnvironment) Cleanup() {
	e.cancel()

	// Run cleanup functions in reverse order
	for i := len(e.cleanupFuncs) - 1; i >= 0; i-- {
		e.cleanupFuncs[i]()
	}
}

// AddCleanup registers a cleanup function
func (e *TestEnvironment) AddCleanup(fn func()) {
	e.cleanupFuncs = append(e.cleanupFuncs, fn)
}

// RunBootcMan runs the bootc-man CLI with the given arguments
func (e *TestEnvironment) RunBootcMan(args ...string) (string, error) {
	// Find bootc-man binary
	binary := findBootcManBinary()
	if binary == "" {
		return "", fmt.Errorf("bootc-man binary not found")
	}

	cmd := exec.CommandContext(e.ctx, binary, args...)
	cmd.Dir = e.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("bootc-man %s failed: %v\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}

	return stdout.String(), nil
}

// RunCommand runs a shell command and returns output
func (e *TestEnvironment) RunCommand(name string, args ...string) (string, error) {
	cmd := exec.CommandContext(e.ctx, name, args...)
	cmd.Dir = e.workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return stdout.String(), fmt.Errorf("%s %s failed: %v\nstderr: %s", name, strings.Join(args, " "), err, stderr.String())
	}

	return stdout.String(), nil
}

// findBootcManBinary finds the bootc-man binary
func findBootcManBinary() string {
	// Check common locations
	paths := []string{
		"./bin/bootc-man",
		"../bin/bootc-man",
		"../../bin/bootc-man",
		"../../../bin/bootc-man",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			absPath, _ := filepath.Abs(p)
			return absPath
		}
	}

	// Try PATH
	if path, err := exec.LookPath("bootc-man"); err == nil {
		return path
	}

	return ""
}

// RequirePodman ensures Podman is available
func RequirePodman(t *testing.T) {
	t.Helper()
	testutil.SkipIfPodmanUnavailable(t)
}

// RequireVfkit ensures vfkit is available (macOS only)
func RequireVfkit(t *testing.T) {
	t.Helper()
	testutil.SkipIfVfkitUnavailable(t)
}

// RequireQEMU ensures QEMU is available (Linux only)
func RequireQEMU(t *testing.T) {
	t.Helper()
	testutil.SkipIfQEMUUnavailable(t)
}

// RequireGvproxy ensures gvproxy is available
func RequireGvproxy(t *testing.T) {
	t.Helper()
	testutil.SkipIfGvproxyUnavailable(t)
}

// RequireKVM ensures KVM is available (Linux only)
func RequireKVM(t *testing.T) {
	t.Helper()
	testutil.SkipIfKVMUnavailable(t)
}

// RequireVMInfrastructure ensures VM infrastructure is available
func RequireVMInfrastructure(t *testing.T) {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		RequireVfkit(t)
		RequireGvproxy(t)
	case "linux":
		RequireQEMU(t)
		RequireKVM(t)
		RequireGvproxy(t)
	default:
		t.Skipf("Unsupported OS for VM tests: %s", runtime.GOOS)
	}
}

// TestE2EEnvironmentSetup verifies that the E2E test environment can be created
func TestE2EEnvironmentSetup(t *testing.T) {
	env := NewTestEnvironment(t)

	if env.workDir == "" {
		t.Fatal("workDir should not be empty")
	}

	if env.vmName == "" {
		t.Fatal("vmName should not be empty")
	}

	t.Logf("E2E test environment created:")
	t.Logf("  workDir: %s", env.workDir)
	t.Logf("  vmName: %s", env.vmName)
	t.Logf("  OS: %s", runtime.GOOS)
}

// TestBootcManBinaryExists verifies that the bootc-man binary is available
func TestBootcManBinaryExists(t *testing.T) {
	binary := findBootcManBinary()
	if binary == "" {
		t.Skip("bootc-man binary not found - build with 'make build' first")
	}

	t.Logf("Found bootc-man at: %s", binary)

	// Verify it's executable
	cmd := exec.Command(binary, "--version")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to run bootc-man --version: %v", err)
	}

	t.Logf("bootc-man version: %s", strings.TrimSpace(string(output)))
}

// TestPodmanAvailability checks if Podman is available for E2E tests
func TestPodmanAvailability(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	output, err := exec.Command("podman", "version", "--format", "{{.Version}}").Output()
	if err != nil {
		t.Fatalf("Failed to get podman version: %v", err)
	}

	t.Logf("Podman version: %s", strings.TrimSpace(string(output)))
}

// TestVMInfrastructureAvailability checks if VM infrastructure is available
func TestVMInfrastructureAvailability(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("vfkit"); err != nil {
			t.Skip("vfkit not available - install with 'brew install vfkit'")
		}
		t.Log("vfkit is available for macOS VM tests")

	case "linux":
		if _, err := exec.LookPath("qemu-system-x86_64"); err != nil {
			t.Skip("QEMU not available - install with 'dnf install qemu-system-x86'")
		}
		t.Log("QEMU is available for Linux VM tests")

	default:
		t.Skipf("Unsupported OS for VM tests: %s", runtime.GOOS)
	}
}
