package ci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/tnk4on/bootc-man/internal/config"
)

// getAvailablePort finds an available TCP port by letting the OS assign one
func getAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", config.DefaultLocalhostIP+":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// VfkitClient wraps vfkit CLI commands for bootc images
type VfkitClient struct {
	binary   string
	verbose  bool
	endpoint string // RESTful endpoint URL
	logFile  string // Path to serial log file
}

// VMState represents the VM state from RESTful API
type VMState struct {
	State string `json:"state"`
}

// NewVfkitClient creates a new vfkit client
func NewVfkitClient(verbose bool) (*VfkitClient, error) {
	binary := config.BinaryVfkit

	// Check if vfkit is available
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("vfkit is not installed. Install it from https://github.com/crc-org/vfkit")
	}

	return &VfkitClient{
		binary:  binary,
		verbose: verbose,
	}, nil
}

// VfkitOptions defines options for vfkit VM
type VfkitOptions struct {
	Name          string
	DiskImage     string
	CPUs          int
	Memory        int    // in MiB
	IgnitionPath  string // Path to Ignition config file
	SSHKeyPath    string // Path to SSH private key
	GvproxySocket string // Path to gvproxy Unix socket (for networking)
	GUI           bool   // Display VM console in GUI window (macOS only)
}

// Start starts a VM using vfkit with Ignition support
func (v *VfkitClient) Start(ctx context.Context, opts VfkitOptions) (*exec.Cmd, error) {
	// Create a temporary directory for VM state (EFI variable store)
	vmDir := config.RuntimeDir()
	efiStorePath := fmt.Sprintf("%s/bootc-man-%s-efi-store", vmDir, opts.Name)

	// Build vfkit command
	// vfkit uses --device for disk images and --ignition for Ignition files
	// EFI bootloader requires variable-store path
	// Required devices: virtio-blk (disk), virtio-rng (entropy), virtio-serial (console)
	args := []string{
		"--cpus", fmt.Sprintf("%d", opts.CPUs),
		"--memory", fmt.Sprintf("%d", opts.Memory),
		"--bootloader", fmt.Sprintf("efi,variable-store=%s,create", efiStorePath),
		"--device", fmt.Sprintf("virtio-blk,path=%s", opts.DiskImage),
		"--device", "virtio-rng", // Required: entropy device
	}

	// Add virtio-net device for networking (if gvproxy socket is provided)
	if opts.GvproxySocket != "" {
		args = append(args, "--device", fmt.Sprintf("virtio-net,unixSocketPath=%s", opts.GvproxySocket))
	}

	// Add serial device for console output
	// Use a log file instead of stdio to avoid blocking (like Podman machine)
	// Note: vfkit CLI uses "logFilePath" not "log"
	logFile := fmt.Sprintf("%s/bootc-man-%s.log", vmDir, opts.Name)
	args = append(args, "--device", fmt.Sprintf("virtio-serial,logFilePath=%s", logFile))

	// Add RESTful endpoint for VM management (like Podman machine)
	// Use a random available port to avoid conflicts with other VMs
	restfulPort, err := getAvailablePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get available port for RESTful API: %w", err)
	}
	restfulURI := fmt.Sprintf("http://localhost:%d", restfulPort)
	args = append(args, "--restful-uri", restfulURI)

	// Store endpoint and log file for later use
	v.endpoint = restfulURI
	v.logFile = logFile

	// Add Ignition file if provided
	// Note: Podman machine uses vsock for Ignition, but we'll use --ignition flag for simplicity
	if opts.IgnitionPath != "" {
		args = append(args, "--ignition", opts.IgnitionPath)
	}

	// Add --gui flag to display VM console in GUI window (macOS only)
	if opts.GUI {
		args = append(args, "--gui")
	}

	cmd := exec.CommandContext(ctx, v.binary, args...)

	// Redirect vfkit output to reduce log spam
	// vfkit's RESTful API (GIN) logs all HTTP requests to stderr
	// In verbose mode, show output for debugging; otherwise, redirect to /dev/null
	if v.verbose {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	} else {
		// Redirect to /dev/null to suppress GIN access logs
		cmd.Stdout = nil // Use default (no output)
		cmd.Stderr = nil // Use default (no output)
	}

	if v.verbose {
		fmt.Printf("Running: %s %s\n", v.binary, strings.Join(args, " "))
	}

	// Start the command in background
	// vfkit will run until the VM is stopped or context is cancelled
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start vfkit: %w", err)
	}

	// Give vfkit a moment to initialize
	time.Sleep(500 * time.Millisecond)

	return cmd, nil
}

// WaitForSSH waits for SSH to be available in the VM
func (v *VfkitClient) WaitForSSH(ctx context.Context, sshKeyPath string, host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second) // Poll every 1 second for faster debugging
	defer ticker.Stop()

	attemptCount := 0
	lastError := error(nil)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			attemptCount++
			if time.Now().After(deadline) {
				if lastError != nil && v.verbose {
					return fmt.Errorf("timeout waiting for SSH to be available after %d attempts (last error: %v)", attemptCount, lastError)
				}
				return fmt.Errorf("timeout waiting for SSH to be available after %d attempts", attemptCount)
			}

			// Try to connect via SSH
			if err := v.testSSHConnection(ctx, sshKeyPath, host, port); err == nil {
				return nil
			} else {
				lastError = err
				// Log error on every attempt in verbose mode
				if v.verbose {
					fmt.Printf("   SSH attempt %d failed: %v\n", attemptCount, err)
				}
			}
		}
	}
}

// testSSHConnection tests SSH connection to the VM
func (v *VfkitClient) testSSHConnection(ctx context.Context, sshKeyPath, host string, port int) error {
	testCtx, cancel := context.WithTimeout(ctx, config.DefaultSSHTestTimeout) // Reduced timeout for faster debugging
	defer cancel()

	// Try to execute a simple command via SSH
	// Default to "user" for bootc images, but allow override via environment
	username := os.Getenv("BOOTCMAN_SSH_USER")
	if username == "" {
		username = config.DefaultSSHUser // Default username for bootc images
	}

	// Build SSH arguments
	sshArgs := []string{
		"-T",                  // Disable pseudo-terminal allocation (prevents terminal control sequence leakage)
		"-o", "BatchMode=yes", // Disable interactive prompts
		"-i", sshKeyPath,
		"-o", config.SSHOptionStrictHostKeyCheckingNo,
		"-o", config.SSHOptionUserKnownHostsFileDevNull,
		"-o", config.SSHOptionConnectTimeout2,
		"-p", fmt.Sprintf("%d", port),
	}

	// Add verbose logging only in verbose mode
	if v.verbose {
		sshArgs = append(sshArgs, "-o", "LogLevel=DEBUG3", "-v")
	} else {
		sshArgs = append(sshArgs, "-o", "LogLevel=ERROR")
	}

	sshArgs = append(sshArgs, fmt.Sprintf("%s@%s", username, host), "echo test")

	cmd := exec.CommandContext(testCtx, "ssh", sshArgs...)
	// Capture both stdout and stderr for detailed debugging
	// Use /dev/null for stdin to completely prevent terminal control sequence issues
	var stdout, stderr bytes.Buffer
	devNull, _ := os.Open(os.DevNull)
	if devNull != nil {
		defer devNull.Close()
		cmd.Stdin = devNull
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Provide more detailed error information
		stdoutMsg := stdout.String()
		stderrMsg := stderr.String()
		if v.verbose {
			if stdoutMsg != "" {
				fmt.Printf("SSH stdout: %s\n", stdoutMsg)
			}
			if stderrMsg != "" {
				fmt.Printf("SSH stderr: %s\n", stderrMsg)
			}
		}
		if stderrMsg != "" {
			return fmt.Errorf("SSH connection failed: %w (stderr: %s)", err, stderrMsg)
		}
		if stdoutMsg != "" {
			return fmt.Errorf("SSH connection failed: %w (stdout: %s)", err, stdoutMsg)
		}
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	return nil
}

// SSH executes a command via SSH in the VM
func (v *VfkitClient) SSH(ctx context.Context, sshKeyPath, host string, port int, command string) (string, error) {
	// Default to "user" for bootc images, but allow override via environment
	username := os.Getenv("BOOTCMAN_SSH_USER")
	if username == "" {
		username = config.DefaultSSHUser // Default username for bootc images
	}

	sshArgs := []string{
		"-T",                  // Disable pseudo-terminal allocation (prevents terminal control sequence leakage)
		"-o", "BatchMode=yes", // Disable interactive prompts
		"-i", sshKeyPath,
		"-o", config.SSHOptionStrictHostKeyCheckingNo,
		"-o", config.SSHOptionUserKnownHostsFileDevNull,
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf("%s@%s", username, host),
		command,
	}

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	// Use /dev/null for stdin to completely prevent terminal control sequence issues
	devNull, _ := os.Open(os.DevNull)
	if devNull != nil {
		defer devNull.Close()
		cmd.Stdin = devNull
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("SSH command failed: %w (output: %s)", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// GetState checks the VM state using RESTful endpoint
func (v *VfkitClient) GetState(ctx context.Context) (string, error) {
	if v.endpoint == "" {
		return "", fmt.Errorf("RESTful endpoint not configured")
	}

	url := v.endpoint + "/vm/state"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{Timeout: config.DefaultSSHTestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get VM state: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var state VMState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return state.State, nil
}

// ReadLogFile reads the serial log file to see VM console output
func (v *VfkitClient) ReadLogFile() (string, error) {
	if v.logFile == "" {
		return "", fmt.Errorf("log file path not configured")
	}

	data, err := os.ReadFile(v.logFile)
	if err != nil {
		return "", fmt.Errorf("failed to read log file: %w", err)
	}

	return string(data), nil
}

// Endpoint returns the RESTful endpoint URL
func (v *VfkitClient) Endpoint() string {
	return v.endpoint
}

// LogFile returns the path to the serial log file
func (v *VfkitClient) LogFilePath() string {
	return v.logFile
}

// CheckVfkitAvailable checks if vfkit is available
func CheckVfkitAvailable() error {
	if _, err := exec.LookPath(config.BinaryVfkit); err != nil {
		return fmt.Errorf(`vfkit is not installed. To install on macOS:

  Using Homebrew:
  brew tap cfergeau/crc
  brew install vfkit

  Or download from:
  https://github.com/crc-org/vfkit/releases

For more information, see: https://github.com/crc-org/vfkit`)
	}
	return nil
}

// CheckGvproxyAvailable checks if gvproxy is available
func CheckGvproxyAvailable() error {
	binary := config.FindGvproxyBinary()
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf(`gvproxy is not installed. To install:

  On macOS (with Homebrew):
  brew install bootc-man

  On Fedora/RHEL:
  sudo dnf install gvisor-tap-vsock

  gvproxy %s+ is required for VM networking.

For more information, see: https://github.com/containers/gvisor-tap-vsock`, config.MinGvproxyVersion)
	}
	return nil
}

// GetVfkitVersion gets the installed vfkit version
func GetVfkitVersion() (string, error) {
	cmd := exec.Command(config.BinaryVfkit, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get vfkit version: %w", err)
	}

	version := strings.TrimSpace(string(output))
	// Remove "vfkit version " prefix if present
	version = strings.TrimPrefix(version, "vfkit version ")
	version = strings.TrimPrefix(version, "vfkit ")

	return version, nil
}

// CheckVfkitAndGvproxy checks if both vfkit and gvproxy are available
func CheckVfkitAndGvproxy() error {
	if err := CheckVfkitAvailable(); err != nil {
		return err
	}
	if err := CheckGvproxyAvailable(); err != nil {
		return err
	}
	return nil
}
