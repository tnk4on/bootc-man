//go:build darwin

package vm

import (
	"context"
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

// KrunkitDriver implements the Driver interface for krunkit on macOS.
// krunkit is a vfkit-compatible CLI backed by libkrun (Apple Hypervisor.framework).
// It supports SMBIOS Type 11 OEM strings via --oem-string for credential injection.
type KrunkitDriver struct {
	opts                 VMOptions
	verbose              bool
	cmd                  *exec.Cmd
	sshConfig            SSHConfig
	logFile              string
	efiStore             string
	restfulPort          int
	gvproxySocket        string
	gvproxyServiceSocket string
	gvproxyCmd           *exec.Cmd
	macAddress           string
}

// NewKrunkitDriver creates a new krunkit driver
func NewKrunkitDriver(opts VMOptions, verbose bool) (*KrunkitDriver, error) {
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
		port, err := AllocateMachinePort()
		if err != nil {
			return nil, fmt.Errorf("failed to allocate SSH port: %w", err)
		}
		opts.SSHPort = port
	}

	tmpDir := config.RuntimeDir()
	logFile := opts.SerialLogPath
	if logFile == "" {
		logFile = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-krunkit-%s.log", opts.Name))
	}
	efiStore := opts.EFIVariableStore
	if efiStore == "" {
		efiStore = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-krunkit-%s-efi-vars", opts.Name))
	}

	macAddress := generateMACAddressDarwin(opts.Name)
	restfulPort := FindAvailablePort(12345)

	return &KrunkitDriver{
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
			HostGateway: "192.168.127.1",
		},
	}, nil
}

// Type returns the VM type
func (d *KrunkitDriver) Type() VMType {
	return KrunkitVM
}

// Available checks if krunkit is available
func (d *KrunkitDriver) Available() error {
	if _, err := exec.LookPath(config.BinaryKrunkit); err != nil {
		return fmt.Errorf("krunkit is not installed. Install it: brew install krunkit")
	}
	return nil
}

// Start starts the VM using krunkit
func (d *KrunkitDriver) Start(ctx context.Context, opts VMOptions) error {
	if opts.Name != "" {
		allocatedSSHPort := d.opts.SSHPort
		d.opts = opts
		if d.opts.SSHPort == 0 && allocatedSSHPort > 0 {
			d.opts.SSHPort = allocatedSSHPort
		}
	}

	if err := d.Available(); err != nil {
		return err
	}

	if err := d.startGvproxy(ctx); err != nil {
		return fmt.Errorf("failed to start gvproxy: %w", err)
	}

	time.Sleep(500 * time.Millisecond)

	args := []string{}

	args = append(args, "--cpus", fmt.Sprintf("%d", d.opts.CPUs))
	args = append(args, "--memory", fmt.Sprintf("%d", d.opts.Memory))

	// EFI bootloader (variable-store is accepted for vfkit compat but ignored by krunkit)
	args = append(args, "--bootloader", fmt.Sprintf("efi,variable-store=%s,create", d.efiStore))

	if !strings.HasSuffix(d.opts.DiskImage, ".raw") {
		return fmt.Errorf("krunkit currently requires raw disk images. Convert with: qemu-img convert -f qcow2 -O raw input.qcow2 output.raw")
	}
	args = append(args, "--device", fmt.Sprintf("virtio-blk,path=%s", d.opts.DiskImage))

	args = append(args, "--device", fmt.Sprintf("virtio-net,unixSocketPath=%s,mac=%s", d.gvproxySocket, d.macAddress))

	args = append(args, "--device", fmt.Sprintf("virtio-serial,logFilePath=%s", d.logFile))

	// RESTful API for VM state control (krunkit uses tcp:// scheme, not http://)
	args = append(args, "--restful-uri", fmt.Sprintf("tcp://localhost:%d", d.restfulPort))

	d.cmd = exec.CommandContext(ctx, config.BinaryKrunkit, args...)

	if d.verbose {
		fmt.Printf("Running: krunkit %s\n", strings.Join(args, " "))
		d.cmd.Stdout = os.Stdout
		d.cmd.Stderr = os.Stderr
	}

	if err := d.cmd.Start(); err != nil {
		d.stopGvproxy()
		return fmt.Errorf("failed to start krunkit: %w", err)
	}

	if d.verbose {
		fmt.Printf("krunkit started with PID: %d\n", d.cmd.Process.Pid)
	}

	return nil
}

// startGvproxy starts gvproxy for VM networking
func (d *KrunkitDriver) startGvproxy(ctx context.Context) error {
	gvproxyBin := config.FindGvproxyBinary()
	if _, err := exec.LookPath(gvproxyBin); err != nil {
		return fmt.Errorf("gvproxy is not installed. Install it: brew install bootc-man")
	}

	tmpDir := config.RuntimeDir()
	d.gvproxySocket = filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s.sock", d.opts.Name))
	gvproxyLogFile := filepath.Join(tmpDir, fmt.Sprintf("bootc-man-gvproxy-%s.log", d.opts.Name))

	_ = os.Remove(d.gvproxySocket)
	os.Remove(d.gvproxyServiceSocket)
	os.Remove(gvproxyLogFile)

	args := []string{
		"-listen-vfkit", fmt.Sprintf("unixgram://%s", d.gvproxySocket),
		"-ssh-port", "-1",
		"-services", fmt.Sprintf("unix://%s", d.gvproxyServiceSocket),
	}

	if d.verbose {
		fmt.Printf("Running: gvproxy %s\n", strings.Join(args, " "))
		args = append(args, "-debug")
	}

	d.gvproxyCmd = exec.CommandContext(ctx, gvproxyBin, args...)

	logFile, err := os.Create(gvproxyLogFile)
	if err != nil {
		return fmt.Errorf("failed to create gvproxy log file: %w", err)
	}

	if d.verbose {
		d.gvproxyCmd.Stdout = os.Stdout
		d.gvproxyCmd.Stderr = os.Stderr
	} else {
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

	socketReady := false
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		if _, err := os.Stat(d.gvproxySocket); err == nil {
			socketReady = true
			break
		}
		if d.gvproxyCmd.ProcessState != nil && d.gvproxyCmd.ProcessState.Exited() {
			break
		}
	}

	if !socketReady {
		logFile.Close()
		logContent, _ := os.ReadFile(gvproxyLogFile)
		if d.gvproxyCmd.ProcessState != nil && d.gvproxyCmd.ProcessState.Exited() {
			return fmt.Errorf("gvproxy exited prematurely with code %d\nLog output:\n%s",
				d.gvproxyCmd.ProcessState.ExitCode(), string(logContent))
		}
		return fmt.Errorf("gvproxy socket not created within timeout: %s\nLog output:\n%s",
			d.gvproxySocket, string(logContent))
	}

	return nil
}

// stopGvproxy stops gvproxy
func (d *KrunkitDriver) stopGvproxy() {
	if d.gvproxyCmd != nil && d.gvproxyCmd.Process != nil {
		_ = d.gvproxyCmd.Process.Kill()
		_ = d.gvproxyCmd.Wait()
	}
	_ = os.Remove(d.gvproxySocket)
	_ = os.Remove(d.gvproxyServiceSocket)
}

// Stop stops the VM
func (d *KrunkitDriver) Stop(ctx context.Context) error {
	if err := d.requestVMState(ctx, "Stopping"); err == nil {
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			state, _ := d.GetState(ctx)
			if state == VMStateStopped {
				break
			}
		}
	}

	if d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
		_ = d.cmd.Wait()
	}
	d.stopGvproxy()
	return nil
}

// GetState returns the current VM state via REST API
func (d *KrunkitDriver) GetState(ctx context.Context) (VMState, error) {
	url := fmt.Sprintf("http://localhost:%d/vm/state", d.restfulPort)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
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

// requestVMState sends a state change request to krunkit
func (d *KrunkitDriver) requestVMState(ctx context.Context, newState string) error {
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
func (d *KrunkitDriver) WaitForReady(ctx context.Context) error {
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
func (d *KrunkitDriver) extractVMIPFromLog() string {
	logContent, err := d.ReadSerialLog()
	if err != nil || logContent == "" {
		return ""
	}

	lines := strings.Split(logContent, "\n")

	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "192.168.127.") {
			parts := strings.Fields(lines[i])
			for _, part := range parts {
				if strings.HasPrefix(part, "192.168.127.") {
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
func (d *KrunkitDriver) exposeSSHPort(ctx context.Context, vmIP string) error {
	if d.gvproxyServiceSocket == "" {
		return fmt.Errorf("gvproxy service socket not configured")
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", d.gvproxyServiceSocket)
			},
		},
		Timeout: 5 * time.Second,
	}

	payload := fmt.Sprintf(`{"local":":%d","remote":"%s:22","protocol":"tcp"}`, d.sshConfig.Port, vmIP)

	url := "http://unix/services/forwarder/expose"
	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(payload))
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
func (d *KrunkitDriver) WaitForSSH(ctx context.Context) error {
	timeout := 120 * time.Second
	deadline := time.Now().Add(timeout)
	portForwardingConfigured := false

	for time.Now().Before(deadline) {
		if !portForwardingConfigured {
			vmIP := d.extractVMIPFromLog()
			if vmIP != "" && vmIP != "192.168.127.1" {
				if d.verbose {
					fmt.Printf("Detected VM IP: %s\n", vmIP)
				}
				if err := d.exposeSSHPort(ctx, vmIP); err != nil {
					if d.verbose {
						fmt.Printf("Failed to configure SSH port forwarding: %v (will retry)\n", err)
					}
				} else {
					portForwardingConfigured = true
					time.Sleep(500 * time.Millisecond)
				}
			}
		}

		if portForwardingConfigured {
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", d.sshConfig.Host, d.sshConfig.Port), 2*time.Second)
			if err == nil {
				conn.Close()
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
func (d *KrunkitDriver) testSSHConnection(ctx context.Context) error {
	args := d.buildSSHArgs("echo connected")
	cmd := exec.CommandContext(ctx, "ssh", args...)
	return cmd.Run()
}

// SSH executes a command via SSH
func (d *KrunkitDriver) SSH(ctx context.Context, command string) (string, error) {
	args := d.buildSSHArgs(command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// buildSSHArgs builds SSH command arguments
func (d *KrunkitDriver) buildSSHArgs(command string) []string {
	return []string{
		"-i", d.sshConfig.KeyPath,
		"-p", fmt.Sprintf("%d", d.sshConfig.Port),
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "BatchMode=yes",
		fmt.Sprintf("%s@%s", d.sshConfig.User, d.sshConfig.Host),
		command,
	}
}

// GetSSHConfig returns the SSH configuration
func (d *KrunkitDriver) GetSSHConfig() SSHConfig {
	return d.sshConfig
}

// ReadSerialLog reads the serial console log
func (d *KrunkitDriver) ReadSerialLog() (string, error) {
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
func (d *KrunkitDriver) Cleanup() error {
	ctx := context.Background()
	if err := d.Stop(ctx); err != nil {
		return err
	}
	os.Remove(d.logFile)
	os.Remove(d.efiStore)
	os.Remove(d.gvproxySocket)
	os.Remove(d.gvproxyServiceSocket)
	return nil
}

// GetProcessID returns the krunkit process ID
func (d *KrunkitDriver) GetProcessID() int {
	if d.cmd != nil && d.cmd.Process != nil {
		return d.cmd.Process.Pid
	}
	return 0
}

// GetLogFilePath returns the path to the serial console log file
func (d *KrunkitDriver) GetLogFilePath() string {
	return d.logFile
}

// ToVMInfo creates a VMInfo struct from the driver state
func (d *KrunkitDriver) ToVMInfo(name, pipelineName, pipelineFile, imageTag string) *VMInfo {
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
		VMType:               KrunkitVM.String(),
		ProcessID:            d.GetProcessID(),
		GvproxySocket:        d.gvproxySocket,
		GvproxyServiceSocket: d.gvproxyServiceSocket,
		GvproxyPID:           gvproxyPID,
		VfkitPID:             d.GetProcessID(),
	}
}
