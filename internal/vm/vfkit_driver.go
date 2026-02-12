//go:build darwin

package vm

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tnk4on/bootc-man/internal/config"
)

// VfkitDriver implements the Driver interface for vfkit on macOS
type VfkitDriver struct {
	opts                 VMOptions
	verbose              bool
	cmd                  *exec.Cmd
	sshConfig            SSHConfig
	logFile              string
	efiStore             string
	restfulPort          int
	gvproxySocket        string
	gvproxyServiceSocket string // HTTP API socket for dynamic port forwarding
	gvproxyCmd           *exec.Cmd
	macAddress           string
}

// generateMACAddressDarwin generates a unique MAC address based on VM name
// Format: 52:54:00:XX:XX:XX where XX is derived from VM name hash
// This avoids conflicts with podman machine (5a:94:ef:e4:0c:ee) and allows multiple VMs
func generateMACAddressDarwin(vmName string) string {
	hash := sha256.Sum256([]byte(vmName))
	// Use 52:54:00 prefix (QEMU's locally administered MAC prefix)
	// Last 3 octets from hash
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", hash[0], hash[1], hash[2])
}

// NewVfkitDriver creates a new vfkit driver
func NewVfkitDriver(opts VMOptions, verbose bool) (*VfkitDriver, error) {
	// Set defaults
	if opts.CPUs == 0 {
		opts.CPUs = 2
	}
	if opts.Memory == 0 {
		opts.Memory = 4096
	}
	if opts.SSHUser == "" {
		opts.SSHUser = "user"
	}
	if opts.SSHPort == 0 {
		// Allocate a port using podman machine's port allocation system
		port, err := AllocateMachinePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate SSH port: %w", err)
		}
		opts.SSHPort = port
	}

	// Setup paths
	tmpDir := config.RuntimeDir()
	logFile := opts.SerialLogPath
	if logFile == "" {
		logFile = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-vfkit-%s.log", opts.Name))
	}
	efiStore := opts.EFIVariableStore
	if efiStore == "" {
		efiStore = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-vfkit-%s-efi-vars", opts.Name))
	}

	// Find available port for RESTful API
	restfulPort := FindAvailablePort(12345)

	// Generate unique MAC address for this VM
	macAddress := generateMACAddressDarwin(opts.Name)

	return &VfkitDriver{
		opts:                 opts,
		verbose:              verbose,
		logFile:              logFile,
		efiStore:             efiStore,
		restfulPort:          restfulPort,
		macAddress:           macAddress,
		gvproxyServiceSocket: filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s-services.sock", opts.Name)),
		sshConfig: SSHConfig{
			Host:        "localhost",
			Port:        opts.SSHPort,
			User:        opts.SSHUser,
			KeyPath:     opts.SSHKeyPath,
			HostGateway: "192.168.127.1", // gvproxy gateway
		},
	}, nil
}

// Type returns the VM type
func (d *VfkitDriver) Type() VMType {
	return VfkitVM
}

// Available checks if vfkit is available
func (d *VfkitDriver) Available() error {
	if _, err := exec.LookPath(config.BinaryVfkit); err != nil {
		return fmt.Errorf("vfkit is not installed. Install it: brew install vfkit")
	}
	return nil
}

// Start starts the VM
func (d *VfkitDriver) Start(ctx context.Context, opts VMOptions) error {
	// Update options if provided, but preserve allocated SSH port
	if opts.Name != "" {
		allocatedSSHPort := d.opts.SSHPort
		d.opts = opts
		// Preserve SSH port if it was allocated in NewVfkitDriver and not provided in opts
		if d.opts.SSHPort == 0 && allocatedSSHPort > 0 {
			d.opts.SSHPort = allocatedSSHPort
		}
	}

	if err := d.Available(); err != nil {
		return err
	}

	// Start gvproxy for networking
	if err := d.startGvproxy(ctx); err != nil {
		return fmt.Errorf("failed to start gvproxy: %w", err)
	}

	// Wait for gvproxy to be ready
	time.Sleep(500 * time.Millisecond)

	// Build vfkit command line
	args := []string{}

	// Resources
	args = append(args, "--cpus", fmt.Sprintf("%d", d.opts.CPUs))
	args = append(args, "--memory", fmt.Sprintf("%d", d.opts.Memory))

	// EFI bootloader with variable store
	args = append(args, "--bootloader", fmt.Sprintf("efi,variable-store=%s,create", d.efiStore))

	// Disk image (vfkit only supports raw format)
	if !strings.HasSuffix(d.opts.DiskImage, ".raw") {
		return fmt.Errorf("vfkit only supports raw disk images. Convert with: qemu-img convert -f qcow2 -O raw input.qcow2 output.raw")
	}
	args = append(args, "--device", fmt.Sprintf("virtio-blk,path=%s", d.opts.DiskImage))

	// Networking via gvproxy
	// Unique MAC address per VM allows multiple VMs and avoids conflict with podman machine
	args = append(args, "--device", fmt.Sprintf("virtio-net,unixSocketPath=%s,mac=%s", d.gvproxySocket, d.macAddress))

	// Serial console output
	args = append(args, "--device", fmt.Sprintf("virtio-serial,logFilePath=%s", d.logFile))

	// Random number generator
	args = append(args, "--device", "virtio-rng")

	// RESTful API for VM control
	args = append(args, "--restful-uri", fmt.Sprintf("http://localhost:%d", d.restfulPort))

	// GUI
	if d.opts.GUI {
		args = append(args, "--gui")
	}

	// Execute vfkit in background
	d.cmd = exec.CommandContext(ctx, config.BinaryVfkit, args...)

	if d.verbose {
		fmt.Printf("Running: vfkit %s\n", strings.Join(args, " "))
		d.cmd.Stdout = os.Stdout
		d.cmd.Stderr = os.Stderr
	}

	if err := d.cmd.Start(); err != nil {
		d.stopGvproxy()
		return fmt.Errorf("failed to start vfkit: %w", err)
	}

	if d.verbose {
		fmt.Printf("vfkit started with PID: %d\n", d.cmd.Process.Pid)
	}

	return nil
}

// startGvproxy starts gvproxy for VM networking
func (d *VfkitDriver) startGvproxy(ctx context.Context) error {
	gvproxyBin := config.FindGvproxyBinary()
	if _, err := exec.LookPath(gvproxyBin); err != nil {
		return fmt.Errorf("gvproxy is not installed. Install it: brew install bootc-man")
	}

	// Create socket path
	tmpDir := config.RuntimeDir()
	d.gvproxySocket = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s.sock", d.opts.Name))
	gvproxyLogFile := filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s.log", d.opts.Name))

	// Remove existing sockets and log file
	_ = os.Remove(d.gvproxySocket)
	os.Remove(d.gvproxyServiceSocket)
	os.Remove(gvproxyLogFile)

	// gvproxy arguments for vfkit
	// Note: vfkit requires -listen-vfkit with unixgram:// prefix
	// -ssh-port -1: Disable fixed SSH forwarding to 192.168.127.2 (allows multiple VMs)
	// -services: Enable HTTP API for dynamic port forwarding
	// This allows us to forward to the VM's actual DHCP-assigned IP
	args := []string{
		"-listen-vfkit", fmt.Sprintf("unixgram://%s", d.gvproxySocket),
		"-ssh-port", "-1", // Disable fixed SSH forwarding to avoid port conflicts
		"-services", fmt.Sprintf("unix://%s", d.gvproxyServiceSocket),
	}

	if d.verbose {
		fmt.Printf("Running: gvproxy %s\n", strings.Join(args, " "))
		args = append(args, "-debug")
	}

	d.gvproxyCmd = exec.CommandContext(ctx, gvproxyBin, args...)

	// Create log file for gvproxy output (always capture for debugging)
	logFile, err := os.Create(gvproxyLogFile)
	if err != nil {
		return fmt.Errorf("failed to create gvproxy log file: %w", err)
	}

	if d.verbose {
		// In verbose mode, write to both stdout/stderr and log file
		d.gvproxyCmd.Stdout = os.Stdout
		d.gvproxyCmd.Stderr = os.Stderr
	} else {
		// In non-verbose mode, write to log file for debugging
		d.gvproxyCmd.Stdout = logFile
		d.gvproxyCmd.Stderr = logFile
	}

	if err := d.gvproxyCmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start gvproxy: %w", err)
	}

	if d.verbose {
		fmt.Printf("gvproxy started with PID: %d\n", d.gvproxyCmd.Process.Pid)
	}

	// Wait for gvproxy socket to be created (extended timeout: 5 seconds)
	socketReady := false
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(d.gvproxySocket); err == nil {
			socketReady = true
			break
		}
		// Check if gvproxy has exited
		if d.gvproxyCmd.ProcessState != nil && d.gvproxyCmd.ProcessState.Exited() {
			break
		}
	}

	if !socketReady {
		// Read log file for error details
		logFile.Close()
		logContent, _ := os.ReadFile(gvproxyLogFile)

		// Check if gvproxy process has exited
		if d.gvproxyCmd.ProcessState != nil && d.gvproxyCmd.ProcessState.Exited() {
			return fmt.Errorf("gvproxy exited prematurely with code %d\nLog output:\n%s",
				d.gvproxyCmd.ProcessState.ExitCode(), string(logContent))
		}
		return fmt.Errorf("gvproxy socket not created within timeout: %s\nLog output:\n%s",
			d.gvproxySocket, string(logContent))
	}

	if d.verbose {
		fmt.Printf("gvproxy socket ready: %s\n", d.gvproxySocket)
	}

	return nil
}

// stopGvproxy stops gvproxy
func (d *VfkitDriver) stopGvproxy() {
	if d.gvproxyCmd != nil && d.gvproxyCmd.Process != nil {
		_ = d.gvproxyCmd.Process.Kill()
		_ = d.gvproxyCmd.Wait()
	}
	_ = os.Remove(d.gvproxySocket)
	_ = os.Remove(d.gvproxyServiceSocket)
}

// Stop stops the VM
func (d *VfkitDriver) Stop(ctx context.Context) error {
	// Try graceful shutdown via RESTful API first
	if err := d.requestVMState(ctx, "Stopping"); err == nil {
		// Wait for VM to stop
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			state, _ := d.GetState(ctx)
			if state == VMStateStopped {
				break
			}
		}
	}

	// Force kill if still running
	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}

	// Stop gvproxy
	d.stopGvproxy()

	return nil
}

// GetState returns the current VM state
func (d *VfkitDriver) GetState(ctx context.Context) (VMState, error) {
	// Query RESTful API
	url := fmt.Sprintf("http://localhost:%d/vm/state", d.restfulPort)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// If we can't connect, check if process is running
		if d.cmd != nil && d.cmd.ProcessState == nil {
			return VMStateRunning, nil
		}
		return VMStateStopped, nil
	}
	defer resp.Body.Close()

	var state struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return VMStateUnknown, err
	}

	switch state.State {
	case "VirtualMachineStateRunning":
		return VMStateRunning, nil
	case "VirtualMachineStateStopped":
		return VMStateStopped, nil
	default:
		return VMStateUnknown, nil
	}
}

// requestVMState sends a state change request to vfkit
func (d *VfkitDriver) requestVMState(ctx context.Context, newState string) error {
	url := fmt.Sprintf("http://localhost:%d/vm/state", d.restfulPort)

	body := fmt.Sprintf(`{"state": "%s"}`, newState)
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to change VM state: %s", resp.Status)
	}

	return nil
}

// WaitForReady waits for the VM to be ready
func (d *VfkitDriver) WaitForReady(ctx context.Context) error {
	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		state, err := d.GetState(ctx)
		if err == nil && state == VMStateRunning {
			return nil
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("VM did not become ready within %v", timeout)
}

// extractVMIPFromLog extracts the VM's IP address from the serial console log
func (d *VfkitDriver) extractVMIPFromLog() string {
	logContent, err := d.ReadSerialLog()
	if err != nil || logContent == "" {
		return ""
	}

	lines := strings.Split(logContent, "\n")

	// Pattern 1: "enp0s1: 192.168.127.3" (systemd-networkd format)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "192.168.127.") {
			// Extract IP address
			parts := strings.Fields(lines[i])
			for _, part := range parts {
				if strings.HasPrefix(part, "192.168.127.") {
					// Clean up the IP (remove any trailing characters)
					ip := strings.TrimRight(part, ":/,")
					if strings.Count(ip, ".") == 3 {
						return ip
					}
				}
			}
		}
	}

	return ""
}

// exposeSSHPort sets up SSH port forwarding via gvproxy HTTP API
func (d *VfkitDriver) exposeSSHPort(ctx context.Context, vmIP string) error {
	if d.gvproxyServiceSocket == "" {
		return fmt.Errorf("gvproxy service socket not configured")
	}

	// Create HTTP client that uses Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", d.gvproxyServiceSocket)
			},
		},
		Timeout: 5 * time.Second,
	}

	// Expose SSH port (22 on VM) to localhost:SSHPort
	payload := map[string]string{
		"local":    fmt.Sprintf(":%d", d.sshConfig.Port),
		"remote":   fmt.Sprintf("%s:22", vmIP),
		"protocol": "tcp",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := "http://unix/services/forwarder/expose"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to expose SSH port: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 256)
		n, _ := resp.Body.Read(body)
		return fmt.Errorf("failed to expose SSH port: %s: %s", resp.Status, string(body[:n]))
	}

	if d.verbose {
		fmt.Printf("SSH port forwarding configured: localhost:%d -> %s:22\n", d.sshConfig.Port, vmIP)
	}

	return nil
}

// WaitForSSH waits for SSH to be available
func (d *VfkitDriver) WaitForSSH(ctx context.Context) error {
	timeout := 120 * time.Second
	deadline := time.Now().Add(timeout)
	portForwardingConfigured := false

	for time.Now().Before(deadline) {
		// First, try to get the VM's IP address and configure port forwarding
		if !portForwardingConfigured {
			vmIP := d.extractVMIPFromLog()
			if vmIP != "" && vmIP != "192.168.127.1" { // Skip gateway IP
				if d.verbose {
					fmt.Printf("Detected VM IP: %s\n", vmIP)
				}
				if err := d.exposeSSHPort(ctx, vmIP); err != nil {
					if d.verbose {
						fmt.Printf("Failed to configure SSH port forwarding: %v (will retry)\n", err)
					}
				} else {
					portForwardingConfigured = true
					// Give the port forwarding a moment to start
					time.Sleep(500 * time.Millisecond)
				}
			}
		}

		// Try to connect to SSH port
		if portForwardingConfigured {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", d.sshConfig.Host, d.sshConfig.Port), 2*time.Second)
			if err == nil {
				conn.Close()
				// Port is open, now try actual SSH connection
				if err := d.testSSHConnection(ctx); err == nil {
					return nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}

	if !portForwardingConfigured {
		return fmt.Errorf("failed to detect VM IP and configure SSH port forwarding within %v", timeout)
	}
	return fmt.Errorf("SSH not available within %v", timeout)
}

// testSSHConnection tests if SSH connection works
func (d *VfkitDriver) testSSHConnection(ctx context.Context) error {
	args := d.buildSSHArgs("echo connected")
	cmd := exec.CommandContext(ctx, "ssh", args...)
	return cmd.Run()
}

// SSH executes a command via SSH
func (d *VfkitDriver) SSH(ctx context.Context, command string) (string, error) {
	args := d.buildSSHArgs(command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// buildSSHArgs builds SSH command arguments
func (d *VfkitDriver) buildSSHArgs(command string) []string {
	args := []string{
		"-i", d.sshConfig.KeyPath,
		"-p", fmt.Sprintf("%d", d.sshConfig.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		fmt.Sprintf("%s@%s", d.sshConfig.User, d.sshConfig.Host),
		command,
	}
	return args
}

// GetSSHConfig returns the SSH configuration
func (d *VfkitDriver) GetSSHConfig() SSHConfig {
	return d.sshConfig
}

// ReadSerialLog reads the serial console log
func (d *VfkitDriver) ReadSerialLog() (string, error) {
	data, err := os.ReadFile(d.logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// Cleanup cleans up all resources
func (d *VfkitDriver) Cleanup() error {
	// Stop the VM if running
	ctx := context.Background()
	if err := d.Stop(ctx); err != nil {
		return err
	}

	// Remove temporary files
	os.Remove(d.logFile)
	os.Remove(d.efiStore)
	os.Remove(d.gvproxySocket)
	os.Remove(d.gvproxyServiceSocket)

	return nil
}

// GetProcessID returns the vfkit process ID
func (d *VfkitDriver) GetProcessID() int {
	if d.cmd != nil && d.cmd.Process != nil {
		return d.cmd.Process.Pid
	}
	return 0
}

// GetLogFilePath returns the path to the serial console log file
func (d *VfkitDriver) GetLogFilePath() string {
	return d.logFile
}

// ToVMInfo creates a VMInfo struct from the driver state
func (d *VfkitDriver) ToVMInfo(name, pipelineName, pipelineFile, imageTag string) *VMInfo {
	gvproxyPID := 0
	if d.gvproxyCmd != nil && d.gvproxyCmd.Process != nil {
		gvproxyPID = d.gvproxyCmd.Process.Pid
	}

	return &VMInfo{
		Name:                 name,
		PipelineName:         pipelineName,
		PipelineFile:         pipelineFile,
		ImageTag:             imageTag,
		DiskImage:            d.opts.DiskImage,
		Created:              time.Now(),
		SSHHost:              d.sshConfig.Host,
		SSHPort:              d.sshConfig.Port,
		SSHUser:              d.sshConfig.User,
		SSHKeyPath:           d.sshConfig.KeyPath,
		LogFile:              d.logFile,
		State:                string(VMStateRunning),
		VMType:               VfkitVM.String(),
		ProcessID:            d.GetProcessID(),
		GvproxySocket:        d.gvproxySocket,
		GvproxyServiceSocket: d.gvproxyServiceSocket,
		GvproxyPID:           gvproxyPID,
		VfkitEndpoint:        fmt.Sprintf("http://localhost:%d", d.restfulPort),
		VfkitPID:             d.GetProcessID(),
	}
}
