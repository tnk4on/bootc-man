package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/vm"
)

// TestStage handles the test stage execution
type TestStage struct {
	pipeline *Pipeline
	imageTag string
	verbose  bool
}

// NewTestStage creates a new test stage executor
func NewTestStage(pipeline *Pipeline, imageTag string, verbose bool) *TestStage {
	return &TestStage{
		pipeline: pipeline,
		imageTag: imageTag,
		verbose:  verbose,
	}
}

// Execute runs the test stage
func (t *TestStage) Execute(ctx context.Context) error {
	if t.pipeline.Spec.Test == nil {
		return fmt.Errorf("test stage is not configured")
	}

	cfg := t.pipeline.Spec.Test
	if cfg.Boot == nil || !cfg.Boot.Enabled {
		return fmt.Errorf("boot test is not enabled")
	}

	// Find raw disk image file from convert stage
	// bootc-man uses raw format exclusively for cross-platform compatibility
	diskImagePath, err := t.findDiskImageFile()
	if err != nil {
		return fmt.Errorf("failed to find disk image file: %w\n   Make sure to run the convert stage first: bootc-man ci run --stage convert", err)
	}

	if t.verbose {
		fmt.Printf("Found disk image file: %s\n", diskImagePath)
	}

	// Generate VM name from pipeline name with ci-test prefix to avoid conflicts with vm start
	pipelineName := t.pipeline.Metadata.Name
	pipelineName = strings.ReplaceAll(pipelineName, "/", "-")
	pipelineName = strings.ReplaceAll(pipelineName, " ", "-")
	pipelineName = strings.ToLower(pipelineName)
	vmName := sanitizeVMName("ci-test-" + pipelineName)

	// Copy disk image to temporary location for test execution
	testDiskPath := filepath.Join(config.TempDataDir(), fmt.Sprintf("bootc-man-test-%s.raw", pipelineName))

	// Clean up any existing temporary test disk from previous failed run
	if _, err := os.Stat(testDiskPath); err == nil {
		if t.verbose {
			fmt.Printf("Removing stale test disk from previous run: %s\n", testDiskPath)
		}
		os.Remove(testDiskPath)
	}

	// Copy disk image for test execution
	if t.verbose {
		fmt.Printf("Copying disk image for test execution...\n")
		fmt.Printf("  Source: %s\n", diskImagePath)
		fmt.Printf("  Dest:   %s\n", testDiskPath)
	}
	if err := copyFile(diskImagePath, testDiskPath); err != nil {
		return fmt.Errorf("failed to copy disk image: %w", err)
	}
	if t.verbose {
		fmt.Println("✅ Disk image copied")
	}

	// Schedule cleanup of test disk after test completion
	defer func() {
		if t.verbose {
			fmt.Printf("🧹 Cleaning up test disk: %s\n", testDiskPath)
		}
		if err := os.Remove(testDiskPath); err != nil {
			fmt.Printf("⚠️  Warning: Failed to remove test disk: %v\n", err)
		}
	}()

	// Get SSH key path
	sshKeyPath, err := t.findSSHKeyPath()
	if err != nil {
		return err
	}

	// Determine if GUI should be enabled
	// GUI requires DISPLAY environment variable on Linux
	guiEnabled := cfg.Boot.GUI
	if guiEnabled && os.Getenv("DISPLAY") == "" {
		fmt.Println("⚠️  GUI requested but DISPLAY not set, running headless")
		guiEnabled = false
	}

	// Create VM driver for current platform
	// SSHPort is set to 0 for dynamic allocation via port-alloc.dat
	vmOpts := vm.VMOptions{
		Name:       vmName,
		DiskImage:  testDiskPath,
		CPUs:       2,
		Memory:     4096,
		SSHKeyPath: sshKeyPath,
		SSHUser:    "user",
		SSHPort:    0, // Dynamic allocation
		GUI:        guiEnabled,
	}

	driver, err := vm.NewDriver(vmOpts, t.verbose)
	if err != nil {
		return fmt.Errorf("failed to create VM driver: %w", err)
	}

	// Check if hypervisor is available
	if err := driver.Available(); err != nil {
		return err
	}

	// Display platform info
	vmType := driver.Type()
	fmt.Printf("🖥️  Platform: %s (%s)\n", runtime.GOOS, vmType.String())
	fmt.Printf("   Host gateway IP: %s\n", vmType.HostGatewayIP())

	// Start VM
	if t.verbose {
		fmt.Println("🚀 Starting VM...")
	}
	if err := driver.Start(ctx, vmOpts); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	// Ensure VM is cleaned up on exit
	defer func() {
		if t.verbose {
			fmt.Println("🧹 Cleaning up VM...")
		}
		_ = driver.Cleanup()
	}()

	// Wait for VM to be ready
	timeout := time.Duration(cfg.Boot.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	fmt.Printf("⏳ Waiting for VM to boot (timeout: %v)...\n", timeout)
	vmReadyStart := time.Now()
	if err := driver.WaitForReady(ctx); err != nil {
		// Try to get serial log for debugging
		logContent, _ := driver.ReadSerialLog()
		if logContent != "" {
			fmt.Printf("\n📋 VM serial console output:\n%s\n", t.truncateLog(logContent, 50))
		}
		return fmt.Errorf("VM failed to boot: %w", err)
	}
	vmReadyDuration := time.Since(vmReadyStart)

	fmt.Printf("✅ VM is running (took %v)\n", vmReadyDuration.Round(time.Millisecond))

	// Perform boot checks if configured
	if len(cfg.Boot.Checks) > 0 {
		// Wait for SSH to be available
		fmt.Println("⏳ Waiting for SSH to be available...")
		sshStart := time.Now()
		if err := driver.WaitForSSH(ctx); err != nil {
			// Show diagnostics
			t.showSSHDiagnostics(driver)
			return fmt.Errorf("SSH not available: %w", err)
		}
		sshDuration := time.Since(sshStart)
		fmt.Printf("✅ SSH connection established (took %v)\n", sshDuration.Round(time.Millisecond))

		// Execute boot checks
		fmt.Println("🔍 Running boot checks...")
		for i, check := range cfg.Boot.Checks {
			if t.verbose {
				fmt.Printf("   [%d/%d] %s\n", i+1, len(cfg.Boot.Checks), check)
			}

			output, err := driver.SSH(ctx, check)
			if err != nil {
				// Check if this is a reboot command
				if t.isRebootCommand(check) && t.isExpectedRebootError(err) {
					if output != "" {
						fmt.Printf("   Output: %s\n", strings.TrimSpace(output))
					}
					fmt.Printf("   ✅ %s\n", check)

					// Wait for VM to restart after reboot
					if err := t.waitForReboot(ctx, driver, check); err != nil {
						return err
					}
					continue
				}
				return fmt.Errorf("boot check failed: %s\nError: %w\nOutput: %s", check, err, output)
			}

			if output != "" {
				fmt.Printf("   Output: %s\n", strings.TrimSpace(output))
			}
			fmt.Printf("   ✅ %s\n", check)
		}

		fmt.Println("✅ All boot checks passed")
	} else {
		if t.verbose {
			fmt.Println("ℹ️  No boot checks configured")
		}
	}

	return nil
}

// findDiskImageFile finds the raw disk image file from convert stage artifacts
// bootc-man uses raw format exclusively for cross-platform compatibility:
// - vfkit (macOS) only supports raw
// - QEMU (Linux) supports raw natively
// - Simpler workflow without format conversion
func (t *TestStage) findDiskImageFile() (string, error) {
	artifactsDir := filepath.Join(t.pipeline.baseDir, "output", "images")

	// Generate expected filename from pipeline name
	pipelineName := t.pipeline.Metadata.Name
	pipelineName = strings.ReplaceAll(pipelineName, "/", "-")
	pipelineName = strings.ReplaceAll(pipelineName, " ", "-")
	pipelineName = strings.ToLower(pipelineName)

	// Try to find raw file with expected name
	rawFile := filepath.Join(artifactsDir, fmt.Sprintf("%s.raw", pipelineName))
	if _, err := os.Stat(rawFile); err == nil {
		return rawFile, nil
	}

	// Search for any raw file in the directory
	var foundRawFile string
	_ = filepath.Walk(artifactsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), ".raw") {
			foundRawFile = path
			return filepath.SkipAll
		}
		return nil
	})
	if foundRawFile != "" {
		return foundRawFile, nil
	}

	return "", fmt.Errorf("no raw disk image file found in %s\n   bootc-man requires raw format. Make sure convert stage outputs raw format", artifactsDir)
}

// findSSHKeyPath finds the SSH private key path
func (t *TestStage) findSSHKeyPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try ed25519 first, then RSA
	keyPaths := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519"),
		filepath.Join(homeDir, ".ssh", "id_rsa"),
	}

	for _, keyPath := range keyPaths {
		if _, err := os.Stat(keyPath); err == nil {
			return keyPath, nil
		}
	}

	return "", fmt.Errorf("no SSH private key found. Please ensure ~/.ssh/id_ed25519 or ~/.ssh/id_rsa exists")
}

// isRebootCommand checks if the command triggers a reboot
func (t *TestStage) isRebootCommand(cmd string) bool {
	return strings.Contains(cmd, "reboot") ||
		(strings.Contains(cmd, "--apply") &&
			(strings.Contains(cmd, "bootc switch") ||
				strings.Contains(cmd, "bootc upgrade") ||
				strings.Contains(cmd, "bootc rollback")))
}

// isExpectedRebootError checks if the error is expected during reboot
func (t *TestStage) isExpectedRebootError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "exit status 255") ||
		strings.Contains(errStr, "Connection closed") ||
		strings.Contains(errStr, "Connection reset")
}

// waitForReboot waits for the VM to restart after a reboot command
func (t *TestStage) waitForReboot(ctx context.Context, driver vm.Driver, cmd string) error {
	isSoftReboot := strings.Contains(cmd, "soft-reboot") || strings.Contains(cmd, "--soft-reboot")

	rebootType := "reboot"
	if isSoftReboot {
		rebootType = "soft-reboot"
	}
	fmt.Printf("   ⚠️  Detected %s, waiting for VM to restart...\n", rebootType)

	// Wait for VM to stop (skip for soft-reboot)
	if !isSoftReboot {
		if t.verbose {
			fmt.Println("   ⏳ Waiting for VM to stop...")
		}
		stopDeadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(stopDeadline) {
			state, _ := driver.GetState(ctx)
			if state == vm.VMStateStopped {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Wait for VM to be running again
	if t.verbose {
		fmt.Println("   ⏳ Waiting for VM to restart...")
	}
	restartDeadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(restartDeadline) {
		state, _ := driver.GetState(ctx)
		if state == vm.VMStateRunning {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Wait for SSH to be available
	if t.verbose {
		fmt.Println("   ⏳ Waiting for SSH after reboot...")
	}
	if err := driver.WaitForSSH(ctx); err != nil {
		return fmt.Errorf("SSH not available after reboot: %w", err)
	}

	if t.verbose {
		fmt.Println("   ✓ SSH available after reboot")
	}
	return nil
}

// showSSHDiagnostics shows diagnostic information for SSH connection issues
func (t *TestStage) showSSHDiagnostics(driver vm.Driver) {
	sshConfig := driver.GetSSHConfig()

	fmt.Println("\n🔍 SSH connection diagnostics:")
	fmt.Printf("   - Host: %s\n", sshConfig.Host)
	fmt.Printf("   - Port: %d\n", sshConfig.Port)
	fmt.Printf("   - User: %s\n", sshConfig.User)
	fmt.Printf("   - Key: %s\n", sshConfig.KeyPath)
	fmt.Printf("   - Host gateway (from VM): %s\n", sshConfig.HostGateway)

	// Show serial console log
	logContent, err := driver.ReadSerialLog()
	if err == nil && logContent != "" {
		fmt.Printf("\n📋 VM serial console output (last 50 lines):\n")
		fmt.Println(t.truncateLog(logContent, 50))

		// Extract diagnostics
		diagnostics := extractDiagnosticsFromLog(logContent)
		if len(diagnostics) > 0 {
			fmt.Println("\n🔍 Diagnostic information:")
			for _, diag := range diagnostics {
				fmt.Printf("   %s\n", diag)
			}
		}
	}

	fmt.Println("\n💡 Troubleshooting:")
	fmt.Printf("   - Verify SSH service is enabled in Containerfile (systemctl enable sshd)\n")
	fmt.Printf("   - Check that user '%s' exists and has SSH key in ~/.ssh/authorized_keys\n", sshConfig.User)
	fmt.Printf("   - Try manual SSH: ssh -i %s -p %d %s@%s\n", sshConfig.KeyPath, sshConfig.Port, sshConfig.User, sshConfig.Host)
}

// truncateLog truncates log to last N lines
func (t *TestStage) truncateLog(logContent string, maxLines int) string {
	lines := strings.Split(logContent, "\n")
	if len(lines) <= maxLines {
		return logContent
	}
	start := len(lines) - maxLines
	return strings.Join(lines[start:], "\n")
}

// extractDiagnosticsFromLog extracts diagnostic information from serial console logs
func extractDiagnosticsFromLog(logContent string) []string {
	var diagnostics []string

	// Check for network interface
	if matches := regexp.MustCompile(`enp\d+s\d+:\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`).FindStringSubmatch(logContent); len(matches) > 1 {
		diagnostics = append(diagnostics, fmt.Sprintf("✓ Network interface detected: IP %s", matches[1]))
	}

	// Check for login prompt
	if strings.Contains(logContent, "login:") {
		diagnostics = append(diagnostics, "✓ Login prompt detected - system is fully booted")
	}

	// Check for systemd
	if strings.Contains(logContent, "systemd") {
		diagnostics = append(diagnostics, "✓ systemd detected in logs")
	}

	// Check for SSH
	if strings.Contains(logContent, "sshd") || strings.Contains(logContent, "ssh") {
		diagnostics = append(diagnostics, "⚠ SSH-related messages found")
	}

	// Check for errors
	if strings.Contains(logContent, "error") || strings.Contains(logContent, "Error") || strings.Contains(logContent, "ERROR") {
		diagnostics = append(diagnostics, "⚠ Error messages found in logs")
	}

	return diagnostics
}

// sanitizeVMName sanitizes a VM name
func sanitizeVMName(name string) string {
	maxLen := 30
	if len(name) > maxLen {
		name = name[:maxLen]
	}

	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}

	return result.String()
}
