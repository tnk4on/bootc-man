//go:build linux

package vm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/tnk4on/bootc-man/internal/config"
)

// QemuDriver implements the Driver interface for QEMU/KVM on Linux
type QemuDriver struct {
	opts                 VMOptions
	verbose              bool
	sshConfig            SSHConfig
	logFile              string
	efiStore             string
	pidFile              string
	gvproxySocket        string
	gvproxyServiceSocket string // HTTP API socket for dynamic port forwarding
	gvproxyPidFile       string
	gvproxyPID           int
	gvproxyCmd           *exec.Cmd
	macAddress           string
}

// NewQemuDriver creates a new QEMU driver
func NewQemuDriver(opts VMOptions, verbose bool) (*QemuDriver, error) {
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
		logFile = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-qemu-%s.log", opts.Name))
	}
	efiStore := opts.EFIVariableStore
	if efiStore == "" {
		efiStore = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-qemu-%s-efi-vars.fd", opts.Name))
	}
	pidFile := filepath.Join(tmpDir, fmt.Sprintf("bootc-man-qemu-%s.pid", opts.Name))

	// Generate unique MAC address for this VM
	macAddress := generateMACAddress(opts.Name)

	return &QemuDriver{
		opts:       opts,
		verbose:    verbose,
		logFile:    logFile,
		efiStore:   efiStore,
		pidFile:    pidFile,
		macAddress: macAddress,
		sshConfig: SSHConfig{
			Host:        "localhost",
			Port:        opts.SSHPort,
			User:        opts.SSHUser,
			KeyPath:     opts.SSHKeyPath,
			HostGateway: "192.168.127.1", // gvproxy gateway (unified across platforms)
		},
	}, nil
}

// Type returns the VM type
func (d *QemuDriver) Type() VMType {
	return QemuVM
}

// Available checks if QEMU, KVM, and gvproxy are available
func (d *QemuDriver) Available() error {
	// Check for qemu-system-x86_64 (or appropriate architecture)
	binary := d.getQemuBinary()
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("%s is not installed. Install it: sudo dnf install qemu-kvm", binary)
	}

	// Check for KVM support (required for acceptable performance)
	if _, err := os.Stat("/dev/kvm"); err != nil {
		return fmt.Errorf(`KVM is not available. VM execution requires KVM for acceptable performance.

To enable KVM:

1. Check if your CPU supports virtualization:
   grep -E '(vmx|svm)' /proc/cpuinfo

2. If running in a VM (nested virtualization), enable it on the host:
   - VMware: Enable "Expose hardware assisted virtualization to the guest OS"
   - VirtualBox: Enable "Nested VT-x/AMD-V"
   - KVM/libvirt: Set <cpu mode="host-passthrough"/>

3. Load the KVM module:
   sudo modprobe kvm
   sudo modprobe kvm_intel  # For Intel CPUs
   # or
   sudo modprobe kvm_amd    # For AMD CPUs

4. Check /dev/kvm permissions:
   ls -la /dev/kvm
   sudo chmod 666 /dev/kvm  # Temporary fix
   # or add user to kvm group:
   sudo usermod -aG kvm $USER`)
	}

	// Check for gvproxy (required for networking)
	if _, err := exec.LookPath(d.getGvproxyBinary()); err != nil {
		return fmt.Errorf("gvproxy is not installed. Install it: sudo dnf install gvisor-tap-vsock")
	}
	return nil
}

// getGvproxyBinary returns the gvproxy binary path
func (d *QemuDriver) getGvproxyBinary() string {
	return config.FindGvproxyBinary()
}

// generateMACAddress generates a unique MAC address based on VM name
// Format: 52:54:00:XX:XX:XX where XX is derived from VM name hash
// This avoids conflicts with podman machine (5a:94:ef:e4:0c:ee) and allows multiple VMs
func generateMACAddress(vmName string) string {
	hash := sha256.Sum256([]byte(vmName))
	// Use 52:54:00 prefix (QEMU's locally administered MAC prefix)
	// Last 3 octets from hash
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x", hash[0], hash[1], hash[2])
}

// getQemuBinary returns the QEMU binary path for the current architecture
// Searches common locations where QEMU is installed on different distributions
func (d *QemuDriver) getQemuBinary() string {
	// Common QEMU binary locations
	// - qemu-system-x86_64: Fedora, Ubuntu, standard installations
	// - /usr/libexec/qemu-kvm: RHEL, CentOS (qemu-kvm package)
	// - /usr/bin/qemu-kvm: Alternative location on some systems
	locations := []string{
		"qemu-system-x86_64",    // Standard (in PATH)
		"/usr/libexec/qemu-kvm", // RHEL/CentOS
		"/usr/bin/qemu-kvm",     // Alternative
		"qemu-kvm",              // In PATH
	}
	for _, loc := range locations {
		if path, err := exec.LookPath(loc); err == nil {
			return path
		}
	}
	// Fallback to standard name (will fail with helpful error message)
	return "qemu-system-x86_64"
}

// Start starts the VM
func (d *QemuDriver) Start(ctx context.Context, opts VMOptions) error {
	// Update options if provided
	if opts.Name != "" {
		d.opts = opts
	}

	if err := d.Available(); err != nil {
		return err
	}

	// Start gvproxy for networking
	if err := d.startGvproxy(ctx); err != nil {
		return fmt.Errorf("failed to start gvproxy: %w", err)
	}

	// Wait for gvproxy socket to be created
	// Use longer timeout for CI environments where gvproxy may take longer to initialize
	socketReady := false
	for i := 0; i < 50; i++ {
		if _, err := os.Stat(d.gvproxySocket); err == nil {
			socketReady = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !socketReady {
		// Read gvproxy log for diagnostics
		logPath := filepath.Join(config.RuntimeDir(), fmt.Sprintf("bootc-man-gvproxy-%s.log", d.opts.Name))
		gvproxyLog, _ := os.ReadFile(logPath)
		d.stopGvproxy()
		errMsg := fmt.Sprintf("gvproxy socket not created: %s", d.gvproxySocket)
		if len(gvproxyLog) > 0 {
			errMsg += fmt.Sprintf("\ngvproxy log:\n%s", string(gvproxyLog))
		}
		return fmt.Errorf("%s", errMsg)
	}
	if d.verbose {
		fmt.Printf("gvproxy socket ready: %s\n", d.gvproxySocket)
	}

	// Build QEMU command line
	args := []string{}

	// Machine type and acceleration (KVM is required, checked in Available())
	args = append(args, "-M", "accel=kvm")
	args = append(args, "-cpu", "host")

	// Resources
	args = append(args, "-smp", fmt.Sprintf("%d", d.opts.CPUs))
	args = append(args, "-m", fmt.Sprintf("%d", d.opts.Memory))

	// UEFI boot (using OVMF)
	// Check for OVMF firmware locations
	// Note: Ubuntu/Debian uses *_4M variants (4MB firmware), Fedora/RHEL uses standard names
	ovmfPaths := []string{
		"/usr/share/OVMF/OVMF_CODE.fd",          // Fedora/RHEL
		"/usr/share/OVMF/OVMF_CODE_4M.fd",       // Ubuntu/Debian (4MB variant)
		"/usr/share/edk2/ovmf/OVMF_CODE.fd",     // Fedora alternate
		"/usr/share/qemu/OVMF_CODE.fd",          // Generic
		"/usr/share/edk2-ovmf/x64/OVMF_CODE.fd", // Debian/Ubuntu alternate
	}
	var ovmfCode string
	for _, p := range ovmfPaths {
		if _, err := os.Stat(p); err == nil {
			ovmfCode = p
			break
		}
	}
	if ovmfCode == "" {
		return fmt.Errorf("OVMF firmware not found. Install it: sudo dnf install edk2-ovmf")
	}

	// EFI with variable store for persistent boot settings
	args = append(args, "-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", ovmfCode))

	// Create EFI variable store if it doesn't exist
	ovmfVarsTemplate := strings.Replace(ovmfCode, "CODE", "VARS", 1)
	if _, err := os.Stat(d.efiStore); os.IsNotExist(err) {
		// Copy template to create writable variable store
		if _, err := os.Stat(ovmfVarsTemplate); err == nil {
			if err := copyFile(ovmfVarsTemplate, d.efiStore); err != nil {
				return fmt.Errorf("failed to create EFI variable store: %w", err)
			}
		}
	}
	if _, err := os.Stat(d.efiStore); err == nil {
		args = append(args, "-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", d.efiStore))
	}

	// Disk image with boot priority (raw format only for cross-platform compatibility)
	// Use id and bootindex to ensure disk is booted first before network
	args = append(args, "-drive", fmt.Sprintf("file=%s,format=raw,if=none,id=disk0", d.opts.DiskImage))
	args = append(args, "-device", "virtio-blk-pci,drive=disk0,bootindex=0")

	// Networking via gvproxy (unified across platforms)
	// Uses stream socket to connect to gvproxy
	// Unique MAC address per VM allows multiple VMs and avoids conflict with podman machine
	// bootindex=1 ensures network device is after disk in boot order
	args = append(args, "-netdev", fmt.Sprintf("stream,id=net0,addr.type=unix,addr.path=%s,server=off", d.gvproxySocket))
	args = append(args, "-device", fmt.Sprintf("virtio-net-pci,netdev=net0,mac=%s,bootindex=1", d.macAddress))

	// Serial console output to file
	args = append(args, "-serial", fmt.Sprintf("file:%s", d.logFile))

	// Random number generator
	args = append(args, "-device", "virtio-rng-pci")

	// Display
	if d.opts.GUI {
		// Enable graphical display
		args = append(args, "-display", "gtk")
	} else {
		// No display - use VNC with no listener to avoid GTK/SDL initialization
		args = append(args, "-vnc", "none")
	}

	// Daemonize (run in background)
	args = append(args, "-daemonize")
	args = append(args, "-pidfile", d.pidFile)

	if d.verbose {
		fmt.Printf("Running: %s %s\n", d.getQemuBinary(), strings.Join(args, " "))
	}

	// Execute QEMU
	cmd := exec.CommandContext(ctx, d.getQemuBinary(), args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		d.stopGvproxy()
		return fmt.Errorf("failed to start QEMU: %w", err)
	}

	// Read PID from file
	time.Sleep(500 * time.Millisecond) // Wait for PID file to be written
	if pidData, err := os.ReadFile(d.pidFile); err == nil {
		if d.verbose {
			fmt.Printf("QEMU started with PID: %s\n", strings.TrimSpace(string(pidData)))
		}
	}

	return nil
}

// startGvproxy starts gvproxy for VM networking
func (d *QemuDriver) startGvproxy(ctx context.Context) error {
	// Create socket paths
	tmpDir := config.RuntimeDir()
	d.gvproxySocket = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s.sock", d.opts.Name))
	d.gvproxyServiceSocket = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s-service.sock", d.opts.Name))
	d.gvproxyPidFile = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s.pid", d.opts.Name))

	// Remove existing sockets
	os.Remove(d.gvproxySocket)
	os.Remove(d.gvproxyServiceSocket)

	// gvproxy arguments for QEMU
	// -ssh-port -1: Disable fixed SSH forwarding to 192.168.127.2 (allows multiple VMs)
	// -services: Enable HTTP API for dynamic port forwarding
	args := []string{
		"-listen-qemu", fmt.Sprintf("unix://%s", d.gvproxySocket),
		"-ssh-port", "-1", // Disable fixed SSH forwarding
		"-services", fmt.Sprintf("unix://%s", d.gvproxyServiceSocket), // HTTP API for dynamic port forwarding
		"-pid-file", d.gvproxyPidFile,
	}

	gvproxyBin := d.getGvproxyBinary()
	if d.verbose {
		fmt.Printf("Running: %s %s\n", gvproxyBin, strings.Join(args, " "))
	}

	// Start gvproxy as a detached daemon process
	d.gvproxyCmd = exec.Command(gvproxyBin, args...)

	// Detach from parent process - create new process group
	d.gvproxyCmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Redirect output to log file for debugging
	logPath := filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s.log", d.opts.Name))
	logFile, err := os.Create(logPath)
	if err == nil {
		d.gvproxyCmd.Stdout = logFile
		d.gvproxyCmd.Stderr = logFile
	}

	if err := d.gvproxyCmd.Start(); err != nil {
		if logFile != nil {
			logFile.Close()
		}
		return fmt.Errorf("failed to start gvproxy: %w", err)
	}

	// Save the PID before releasing the process
	d.gvproxyPID = d.gvproxyCmd.Process.Pid

	if d.verbose {
		fmt.Printf("gvproxy started with PID: %d\n", d.gvproxyPID)
	}

	// Release the process so it continues after this process exits
	if err := d.gvproxyCmd.Process.Release(); err != nil {
		if d.verbose {
			fmt.Printf("Warning: failed to release gvproxy process: %v\n", err)
		}
	}

	return nil
}

// stopGvproxy stops gvproxy
func (d *QemuDriver) stopGvproxy() {
	if d.gvproxyCmd != nil && d.gvproxyCmd.Process != nil {
		_ = d.gvproxyCmd.Process.Kill()
		_ = d.gvproxyCmd.Wait()
	}
	os.Remove(d.gvproxySocket)
	os.Remove(d.gvproxyServiceSocket)
}

// Stop stops the VM
func (d *QemuDriver) Stop(ctx context.Context) error {
	// Read PID and send SIGTERM
	pidData, err := os.ReadFile(d.pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			d.stopGvproxy()
			return nil // Already stopped
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	pid := strings.TrimSpace(string(pidData))
	if pid == "" {
		d.stopGvproxy()
		return nil
	}

	// Kill the process
	killCmd := exec.CommandContext(ctx, "kill", pid)
	if err := killCmd.Run(); err != nil {
		// Check if process already exited
		checkCmd := exec.CommandContext(ctx, "kill", "-0", pid)
		if checkCmd.Run() != nil {
			// Process doesn't exist anymore
			d.stopGvproxy()
			return nil
		}
		return fmt.Errorf("failed to stop QEMU: %w", err)
	}

	// Wait for process to exit
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		checkCmd := exec.CommandContext(ctx, "kill", "-0", pid)
		if checkCmd.Run() != nil {
			// Process exited
			break
		}
	}

	// Stop gvproxy
	d.stopGvproxy()

	return nil
}

// GetState returns the current VM state
func (d *QemuDriver) GetState(ctx context.Context) (VMState, error) {
	pidData, err := os.ReadFile(d.pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return VMStateStopped, nil
		}
		return VMStateUnknown, err
	}

	pid := strings.TrimSpace(string(pidData))
	if pid == "" {
		return VMStateStopped, nil
	}

	// Check if process is running
	checkCmd := exec.CommandContext(ctx, "kill", "-0", pid)
	if err := checkCmd.Run(); err != nil {
		return VMStateStopped, nil
	}

	return VMStateRunning, nil
}

// WaitForReady waits for the VM to be ready
func (d *QemuDriver) WaitForReady(ctx context.Context) error {
	// Wait for VM to start and begin booting
	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		state, err := d.GetState(ctx)
		if err != nil {
			return err
		}
		if state == VMStateRunning {
			return nil
		}
		if state == VMStateStopped {
			// Check if there's an error in the log
			logContent, _ := d.ReadSerialLog()
			if logContent != "" && strings.Contains(logContent, "error") {
				return fmt.Errorf("VM failed to start: check serial log")
			}
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("VM did not become ready within %v", timeout)
}

// WaitForSSH waits for SSH to be available
func (d *QemuDriver) WaitForSSH(ctx context.Context) error {
	// KVM is required, so we can use a reasonable timeout
	timeout := 2 * time.Minute
	deadline := time.Now().Add(timeout)

	portForwardingSet := false

	for time.Now().Before(deadline) {
		// Try to get VM IP from serial log and set up port forwarding
		if !portForwardingSet {
			if vmIP := d.extractVMIPFromLog(); vmIP != "" {
				if err := d.setupPortForwarding(ctx, vmIP); err != nil {
					if d.verbose {
						fmt.Printf("⚠️  Failed to set up port forwarding: %v\n", err)
					}
				} else {
					portForwardingSet = true
					if d.verbose {
						fmt.Printf("✅ Port forwarding set up: localhost:%d -> %s:22\n", d.sshConfig.Port, vmIP)
					}
				}
			}
		}

		// Try to connect to SSH port
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", d.sshConfig.Host, d.sshConfig.Port), 2*time.Second)
		if err == nil {
			conn.Close()
			// Port is open, now try actual SSH connection
			if err := d.testSSHConnection(ctx); err == nil {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("SSH not available within %v", timeout)
}

// extractVMIPFromLog extracts VM IP address from serial console log
func (d *QemuDriver) extractVMIPFromLog() string {
	logContent, err := d.ReadSerialLog()
	if err != nil || logContent == "" {
		return ""
	}

	// Look for IP address in gvproxy network range (192.168.127.x)
	// Common patterns in serial log:
	// - "ens3: 192.168.127.2"
	// - "inet 192.168.127.2/24"
	patterns := []string{
		`192\.168\.127\.\d+`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllString(logContent, -1)
		for _, match := range matches {
			// Skip gateway IP
			if match != "192.168.127.1" && match != "192.168.127.255" {
				return match
			}
		}
	}

	return ""
}

// setupPortForwarding sets up SSH port forwarding via gvproxy HTTP API
func (d *QemuDriver) setupPortForwarding(ctx context.Context, vmIP string) error {
	if d.gvproxyServiceSocket == "" {
		return fmt.Errorf("gvproxy service socket not set")
	}

	// Wait for service socket to be available
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(d.gvproxyServiceSocket); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Use gvproxy's HTTP API to expose the SSH port
	// POST to http://unix/services/forwarder/expose
	url := "http://unix/services/forwarder/expose"

	// Create HTTP client that uses Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", d.gvproxyServiceSocket)
			},
		},
		Timeout: 5 * time.Second,
	}

	// Request body: {"local":":2222","remote":"192.168.127.3:22","protocol":"tcp"}
	reqBody := map[string]string{
		"local":    fmt.Sprintf(":%d", d.sshConfig.Port),
		"remote":   fmt.Sprintf("%s:22", vmIP),
		"protocol": "tcp",
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to expose port: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to expose port: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// testSSHConnection tests if SSH connection works
func (d *QemuDriver) testSSHConnection(ctx context.Context) error {
	args := d.buildSSHArgs("echo connected")
	cmd := exec.CommandContext(ctx, "ssh", args...)
	return cmd.Run()
}

// SSH executes a command via SSH
func (d *QemuDriver) SSH(ctx context.Context, command string) (string, error) {
	args := d.buildSSHArgs(command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// buildSSHArgs builds SSH command arguments
func (d *QemuDriver) buildSSHArgs(command string) []string {
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
func (d *QemuDriver) GetSSHConfig() SSHConfig {
	return d.sshConfig
}

// ReadSerialLog reads the serial console log
func (d *QemuDriver) ReadSerialLog() (string, error) {
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
func (d *QemuDriver) Cleanup() error {
	// Stop the VM if running
	ctx := context.Background()
	if state, _ := d.GetState(ctx); state == VMStateRunning {
		if err := d.Stop(ctx); err != nil {
			return err
		}
	}

	// Remove temporary files
	os.Remove(d.pidFile)
	os.Remove(d.logFile)
	os.Remove(d.efiStore)
	os.Remove(d.gvproxySocket)

	return nil
}

// GetProcessID returns the QEMU process ID
func (d *QemuDriver) GetProcessID() int {
	pidData, err := os.ReadFile(d.pidFile)
	if err != nil {
		return 0
	}
	pid := strings.TrimSpace(string(pidData))
	if pid == "" {
		return 0
	}
	var pidInt int
	if _, err := fmt.Sscanf(pid, "%d", &pidInt); err != nil {
		return 0
	}
	return pidInt
}

// GetLogFilePath returns the path to the serial console log file
func (d *QemuDriver) GetLogFilePath() string {
	return d.logFile
}

// ToVMInfo creates a VMInfo struct from the driver state
func (d *QemuDriver) ToVMInfo(name, pipelineName, pipelineFile, imageTag string) *VMInfo {
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
		VMType:               QemuVM.String(),
		ProcessID:            d.GetProcessID(),
		PIDFile:              d.pidFile,
		GvproxySocket:        d.gvproxySocket,
		GvproxyServiceSocket: d.gvproxyServiceSocket,
		GvproxyPID:           d.gvproxyPID,
	}
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
