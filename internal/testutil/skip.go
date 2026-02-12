package testutil

import (
	"os"
	"os/exec"
	"runtime"
	"testing"

	"github.com/tnk4on/bootc-man/internal/config"
)

// SkipIfPodmanUnavailable skips the test if Podman is not available or not functional.
// On macOS, the podman binary may exist but Podman Machine may not be running.
func SkipIfPodmanUnavailable(t *testing.T) {
	t.Helper()
	podmanPath, err := exec.LookPath("podman")
	if err != nil {
		t.Skip("Podman not available, skipping test")
	}
	// Verify Podman is actually functional (not just installed)
	cmd := exec.Command(podmanPath, "version")
	if err := cmd.Run(); err != nil {
		t.Skipf("Podman not functional (machine may not be running): %v", err)
	}
}

// SkipIfBootcUnavailable skips the test if bootc is not available.
func SkipIfBootcUnavailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bootc"); err != nil {
		t.Skip("bootc not available, skipping test")
	}
}

// SkipIfNotMacOS skips the test if not running on macOS.
func SkipIfNotMacOS(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("Test requires macOS")
	}
}

// SkipIfNotLinux skips the test if not running on Linux.
func SkipIfNotLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Test requires Linux")
	}
}

// SkipIfNotWindows skips the test if not running on Windows.
func SkipIfNotWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Test requires Windows")
	}
}

// SkipIfVfkitUnavailable skips if vfkit is not available (macOS only).
func SkipIfVfkitUnavailable(t *testing.T) {
	t.Helper()
	SkipIfNotMacOS(t)
	if _, err := exec.LookPath(config.BinaryVfkit); err != nil {
		t.Skip("vfkit not available, skipping test")
	}
}

// SkipIfQEMUUnavailable skips if QEMU is not available (Linux only).
func SkipIfQEMUUnavailable(t *testing.T) {
	t.Helper()
	SkipIfNotLinux(t)
	if _, err := exec.LookPath("qemu-system-x86_64"); err != nil {
		t.Skip("QEMU not available, skipping test")
	}
}

// SkipIfKVMUnavailable skips if KVM is not available (Linux only).
func SkipIfKVMUnavailable(t *testing.T) {
	t.Helper()
	SkipIfNotLinux(t)
	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("/dev/kvm not available, skipping test")
	}
}

// SkipIfGvproxyUnavailable skips if gvproxy is not available.
// gvproxy is required for VM networking (both QEMU on Linux and vfkit on macOS).
func SkipIfGvproxyUnavailable(t *testing.T) {
	t.Helper()
	// Check common locations
	locations := []string{
		config.BinaryGvproxy,          // PATH
		"/usr/libexec/podman/gvproxy", // Fedora/RHEL
		"/usr/lib/podman/gvproxy",     // Alternative location
	}
	for _, loc := range locations {
		if _, err := exec.LookPath(loc); err == nil {
			return // found
		}
	}
	t.Skip("gvproxy not available, skipping VM test")
}

// SkipIfShort skips the test if running with -short flag.
func SkipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}
}

// SkipIfCI skips the test if running in CI environment.
func SkipIfCI(t *testing.T) {
	t.Helper()
	// Common CI environment variables
	ciEnvVars := []string{"CI", "GITHUB_ACTIONS", "TRAVIS", "CIRCLECI", "JENKINS_URL"}
	for _, env := range ciEnvVars {
		if val, ok := lookupEnv(env); ok && val != "" {
			t.Skip("Skipping in CI environment")
		}
	}
}

// SkipIfNotCI skips the test if NOT running in CI environment.
func SkipIfNotCI(t *testing.T) {
	t.Helper()
	ciEnvVars := []string{"CI", "GITHUB_ACTIONS", "TRAVIS", "CIRCLECI", "JENKINS_URL"}
	for _, env := range ciEnvVars {
		if val, ok := lookupEnv(env); ok && val != "" {
			return // Running in CI, don't skip
		}
	}
	t.Skip("Skipping outside CI environment")
}

// SkipIfRoot skips the test if running as root.
func SkipIfRoot(t *testing.T) {
	t.Helper()
	if isRoot() {
		t.Skip("Test should not run as root")
	}
}

// SkipIfNotRoot skips the test if NOT running as root.
func SkipIfNotRoot(t *testing.T) {
	t.Helper()
	if !isRoot() {
		t.Skip("Test requires root privileges")
	}
}

// SkipIfSSHUnavailable skips if SSH is not available.
func SkipIfSSHUnavailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("SSH not available, skipping test")
	}
}

// SkipIfGitUnavailable skips if git is not available.
func SkipIfGitUnavailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping test")
	}
}

// SkipIfPodmanNotRootful skips if Podman is not running in rootful mode.
// This is required for operations like bootc-image-builder that need root access.
func SkipIfPodmanNotRootful(t *testing.T) {
	t.Helper()
	SkipIfPodmanUnavailable(t)

	// Check if podman is running rootful by checking the user in podman info
	cmd := exec.Command("podman", "info", "--format", "{{.Host.Security.Rootless}}")
	output, err := cmd.Output()
	if err != nil {
		t.Skipf("Failed to check Podman mode: %v", err)
	}

	// If rootless is true, skip the test
	if string(output) == "true\n" || string(output) == "true" {
		t.Skip("Test requires rootful Podman (rootless=false)")
	}
}

// SkipIfHadolintUnavailable skips if hadolint container image is not pullable.
// Note: This doesn't check if hadolint is installed locally, but if Podman can run it.
func SkipIfHadolintUnavailable(t *testing.T) {
	t.Helper()
	SkipIfPodmanUnavailable(t)
	// Hadolint runs as a container, so just check Podman is available
	// The actual image pull will happen during test execution
}

// SkipIfTrivyUnavailable skips if trivy is not available.
func SkipIfTrivyUnavailable(t *testing.T) {
	t.Helper()
	SkipIfPodmanUnavailable(t)
	// Trivy runs as a container, so just check Podman is available
}

// SkipIfSyftUnavailable skips if syft is not available.
func SkipIfSyftUnavailable(t *testing.T) {
	t.Helper()
	SkipIfPodmanUnavailable(t)
	// Syft runs as a container, so just check Podman is available
}

// SkipIfBootcImageBuilderUnavailable skips if bootc-image-builder is not available.
// This requires rootful Podman on Linux.
func SkipIfBootcImageBuilderUnavailable(t *testing.T) {
	t.Helper()
	SkipIfNotLinux(t)
	SkipIfPodmanNotRootful(t)
}

// lookupEnv is a helper to look up environment variables
func lookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}

// isRoot checks if the current process is running as root
func isRoot() bool {
	// On Unix systems, UID 0 is root
	// On Windows, this always returns false
	return os.Getuid() == 0
}
