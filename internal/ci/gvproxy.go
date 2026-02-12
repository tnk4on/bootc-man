package ci

import (
	"bytes"
	"context"
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
	"github.com/tnk4on/bootc-man/internal/vm"
)

// GvproxyClient manages gvproxy for VM networking
type GvproxyClient struct {
	binary            string
	socketPath        string
	pidFile           string
	logFile           string
	sshPort           int
	verbose           bool
	cmd               *exec.Cmd
	serviceSocketPath string // Path to service socket for HTTP API
}

// LogFile returns the path to the gvproxy log file
func (g *GvproxyClient) LogFile() string {
	return g.logFile
}

// ServiceSocketPath returns the path to the gvproxy service socket
func (g *GvproxyClient) ServiceSocketPath() string {
	return g.serviceSocketPath
}

// NewGvproxyClient creates a new gvproxy client with VM-specific socket paths
func NewGvproxyClient(vmName string, verbose bool) (*GvproxyClient, error) {
	binary := config.FindGvproxyBinary()

	// Check if gvproxy is available
	if _, err := exec.LookPath(binary); err != nil {
		return nil, fmt.Errorf("gvproxy is not installed. Install it: brew install bootc-man")
	}

	// Sanitize VM name for use in file paths
	safeName := sanitizeForPath(vmName)
	if safeName == "" {
		safeName = "default"
	}

	// Create temporary directory for gvproxy files with VM-specific names
	vmDir := config.RuntimeDir()
	serviceSocketPath := filepath.Join(vmDir, fmt.Sprintf("bootc-man-%s-gvproxy-service.sock", safeName))

	// Get an available port for SSH forwarding to avoid conflicts with other VMs
	sshPort, err := getAvailableSSHPort()
	if err != nil {
		return nil, fmt.Errorf("failed to get available SSH port: %w", err)
	}

	return &GvproxyClient{
		binary:            binary,
		socketPath:        filepath.Join(vmDir, fmt.Sprintf("bootc-man-%s-gvproxy.sock", safeName)),
		pidFile:           filepath.Join(vmDir, fmt.Sprintf("bootc-man-%s-gvproxy.pid", safeName)),
		logFile:           filepath.Join(vmDir, fmt.Sprintf("bootc-man-%s-gvproxy.log", safeName)),
		sshPort:           sshPort,
		verbose:           verbose,
		serviceSocketPath: serviceSocketPath,
	}, nil
}

// getAvailableSSHPort finds an available TCP port for SSH forwarding
// Uses podman machine's port allocation system to avoid conflicts
func getAvailableSSHPort() (int, error) {
	// Use the shared port allocation with podman machine (via vm package)
	port, err := vm.AllocateMachinePort()
	if err != nil {
		// Fallback to OS-assigned port if allocation fails
		listener, err := net.Listen("tcp", config.DefaultLocalhostIP+":0")
		if err != nil {
			return 0, err
		}
		defer listener.Close()
		return listener.Addr().(*net.TCPAddr).Port, nil
	}
	return port, nil
}

// sanitizeForPath sanitizes a string for use in file paths
func sanitizeForPath(name string) string {
	// Replace invalid characters with hyphens
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	return result.String()
}

// Start starts gvproxy for VM networking
func (g *GvproxyClient) Start(ctx context.Context) error {
	// Clean up any stale socket files and processes before starting
	if err := g.cleanupStaleResources(); err != nil {
		if g.verbose {
			fmt.Printf("⚠️  Warning: Failed to cleanup stale resources: %v\n", err)
		}
		// Continue anyway - we'll try to start and see if it works
	}

	// Build gvproxy command
	// gvproxy listens on a Unix socket for vfkit connections
	// Note: -listen-vfkit requires unixgram:// prefix for the socket path
	// -services exposes HTTP API for dynamic port forwarding
	// -ssh-port sets up port forwarding from localhost:SSHPort to 192.168.127.2:22
	// We use a dynamically allocated port to avoid conflicts with other VMs and Podman machine
	args := []string{
		"-listen-vfkit", fmt.Sprintf("unixgram://%s", g.socketPath), // unixgram:// prefix required
		"-pid-file", g.pidFile,
		"-services", fmt.Sprintf("unix://%s", g.serviceSocketPath), // HTTP API for dynamic port forwarding
		"-ssh-port", fmt.Sprintf("%d", g.sshPort), // Use dynamically allocated SSH port
	}

	if g.verbose {
		args = append(args, "-debug")
	}

	// Add log file if specified
	if g.logFile != "" {
		args = append(args, "-log-file", g.logFile)
	}

	cmd := exec.CommandContext(ctx, g.binary, args...)

	// Redirect output to stderr for real-time debugging
	if g.verbose {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	} else {
		// Redirect to log file
		logFile, err := os.Create(g.logFile)
		if err != nil {
			return fmt.Errorf("failed to create gvproxy log file: %w", err)
		}
		defer logFile.Close()
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if g.verbose {
		fmt.Printf("Starting gvproxy: %s %s\n", g.binary, strings.Join(args, " "))
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start gvproxy: %w", err)
	}

	g.cmd = cmd

	// Wait for gvproxy socket to be created
	// Check process is still running and socket exists
	deadline := time.Now().Add(config.DefaultSocketTimeout) // Increased timeout
	checkInterval := 200 * time.Millisecond

	for time.Now().Before(deadline) {
		// Check if process is still running
		// Use syscall.Kill with signal 0 to check if process exists (no-op signal)
		if g.cmd.Process != nil {
			if err := syscall.Kill(g.cmd.Process.Pid, 0); err != nil {
				// Process doesn't exist or error occurred
				// Check if process state indicates it exited
				state := g.cmd.ProcessState
				if state != nil && state.Exited() {
					// Process exited, check log for errors
					logContent, _ := os.ReadFile(g.logFile)
					return fmt.Errorf("gvproxy process exited with status: %v\nLog output:\n%s", state, string(logContent))
				}
				// If we can't determine state, continue waiting
			}
		}

		// Check if socket exists
		if _, err := os.Stat(g.socketPath); err == nil {
			if g.verbose {
				fmt.Printf("✅ gvproxy started (socket: %s)\n", g.socketPath)
			}
			return nil
		}

		time.Sleep(checkInterval)
	}

	// Check if process is still running at timeout
	if g.cmd.Process != nil {
		state := g.cmd.ProcessState
		if state != nil && state.Exited() {
			// Process exited, check log for errors
			logContent, _ := os.ReadFile(g.logFile)
			return fmt.Errorf("gvproxy process exited before socket was created\nProcess state: %v\nLog output:\n%s", state, string(logContent))
		}
	}

	// Timeout reached, check log for errors
	logContent, _ := os.ReadFile(g.logFile)
	return fmt.Errorf("gvproxy socket not created within timeout\nExpected socket: %s\nLog output:\n%s", g.socketPath, string(logContent))
}

// cleanupStaleResources removes stale socket files and stops any running gvproxy processes
// This is called before starting a new gvproxy instance to avoid "address already in use" errors
func (g *GvproxyClient) cleanupStaleResources() error {
	// Check if PID file exists and if the process is still running
	if pidData, err := os.ReadFile(g.pidFile); err == nil {
		var pid int
		if _, err := fmt.Sscanf(string(pidData), "%d", &pid); err == nil && pid > 0 {
			// Check if process is still running
			if err := syscall.Kill(pid, 0); err == nil {
				// Process is running, try to stop it
				process, err := os.FindProcess(pid)
				if err == nil {
					// Try graceful shutdown first
					if err := process.Signal(os.Interrupt); err == nil {
						// Wait a bit for graceful shutdown
						time.Sleep(500 * time.Millisecond)
						// Check if still running
						if err := syscall.Kill(pid, 0); err == nil {
							// Still running, force kill
							_ = process.Kill()
							time.Sleep(200 * time.Millisecond)
						}
					} else {
						// Signal failed, try kill directly
						_ = process.Kill()
						time.Sleep(200 * time.Millisecond)
					}
				}
			}
		}
	}

	// Remove stale socket files
	_ = os.Remove(g.socketPath)
	_ = os.Remove(g.serviceSocketPath)
	_ = os.Remove(g.pidFile)

	return nil
}

// Stop stops gvproxy
func (g *GvproxyClient) Stop() error {
	if g.cmd == nil || g.cmd.Process == nil {
		return nil
	}

	// Try graceful shutdown
	if err := g.cmd.Process.Signal(os.Interrupt); err != nil {
		// If signal fails, try kill
		_ = g.cmd.Process.Kill()
	}

	// Wait for process to exit
	done := make(chan error, 1)
	go func() {
		done <- g.cmd.Wait()
	}()

	select {
	case <-done:
		// Process exited
	case <-time.After(3 * time.Second):
		// Force kill if still running
		_ = g.cmd.Process.Kill()
		_ = g.cmd.Wait()
	}

	// Clean up socket, service socket, and pid file
	_ = os.Remove(g.socketPath)
	_ = os.Remove(g.serviceSocketPath)
	_ = os.Remove(g.pidFile)

	if g.verbose {
		fmt.Println("🧹 gvproxy stopped")
	}

	return nil
}

// SocketPath returns the path to the gvproxy socket
func (g *GvproxyClient) SocketPath() string {
	return g.socketPath
}

// VMIP returns the hostname/IP to connect to the VM via SSH
// Note: gvproxy performs SSH port forwarding from localhost:SSHPort to VM:22
// Therefore, we connect to localhost (not the VM's actual IP address 192.168.127.2)
// This matches Podman machine's behavior: it uses localhost for SSH connections
func (g *GvproxyClient) VMIP() string {
	return "localhost" // Use localhost, gvproxy handles port forwarding
}

// ExtractVMIPFromLog extracts the actual VM IP address from serial console log
// Looks for patterns like "enp0s1: 192.168.127.3" or "inet 192.168.127.3"
// Prefers the last occurrence (most recent) in the log
func ExtractVMIPFromLog(logContent string) string {
	if logContent == "" {
		return ""
	}

	// Split log into lines and search from the end (most recent first)
	lines := strings.Split(logContent, "\n")

	// Pattern 1: "enp0s1: 192.168.127.3" (systemd-networkd format)
	re1 := regexp.MustCompile(`enp\d+s\d+:\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)

	// Search from the end of the log (most recent entries first)
	for i := len(lines) - 1; i >= 0; i-- {
		matches := re1.FindStringSubmatch(lines[i])
		if len(matches) > 1 {
			ip := matches[1]
			// Validate it's in the expected subnet
			if strings.HasPrefix(ip, "192.168.127.") {
				return ip
			}
		}
	}

	// Pattern 2: "inet 192.168.127.3" (ip addr format)
	re2 := regexp.MustCompile(`inet\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)

	// Search from the end of the log (most recent entries first)
	for i := len(lines) - 1; i >= 0; i-- {
		matches := re2.FindStringSubmatch(lines[i])
		if len(matches) > 1 {
			ip := matches[1]
			if strings.HasPrefix(ip, "192.168.127.") {
				return ip
			}
		}
	}

	return "" // Not found
}

// SSHPort returns the SSH port
func (g *GvproxyClient) SSHPort() int {
	return g.sshPort
}

// PID returns the gvproxy process ID
func (g *GvproxyClient) PID() int {
	if g.cmd != nil && g.cmd.Process != nil {
		return g.cmd.Process.Pid
	}
	return 0
}

// GetLeases retrieves DHCP lease information from gvproxy's HTTP API
// Returns a map of IP addresses to MAC addresses
func (g *GvproxyClient) GetLeases(ctx context.Context) (map[string]string, error) {
	if g.serviceSocketPath == "" {
		return nil, fmt.Errorf("service socket path not set")
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", g.serviceSocketPath)
			},
		},
		Timeout: config.DefaultHTTPClientTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://unix/leases", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get leases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get leases: status %d, body: %s", resp.StatusCode, string(body))
	}

	var leases map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&leases); err != nil {
		return nil, fmt.Errorf("failed to decode leases: %w", err)
	}

	return leases, nil
}

// ForwarderInfo represents a port forwarding configuration
type ForwarderInfo struct {
	Local    string `json:"local"`
	Remote   string `json:"remote"`
	Protocol string `json:"protocol"`
}

// GetForwarders retrieves all active port forwarders from gvproxy's HTTP API
func (g *GvproxyClient) GetForwarders(ctx context.Context) ([]ForwarderInfo, error) {
	if g.serviceSocketPath == "" {
		return nil, fmt.Errorf("service socket path not set")
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", g.serviceSocketPath)
			},
		},
		Timeout: config.DefaultHTTPClientTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://unix/services/forwarder/all", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get forwarders: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get forwarders: status %d, body: %s", resp.StatusCode, string(body))
	}

	var forwarders []ForwarderInfo
	if err := json.NewDecoder(resp.Body).Decode(&forwarders); err != nil {
		return nil, fmt.Errorf("failed to decode forwarders: %w", err)
	}

	return forwarders, nil
}

// UnexposePort removes port forwarding for a given local port using gvproxy's HTTP API
func (g *GvproxyClient) UnexposePort(ctx context.Context) error {
	if g.serviceSocketPath == "" {
		return fmt.Errorf("service socket path not set")
	}

	url := "http://unix/services/forwarder/unexpose"

	// Create HTTP client that uses Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", g.serviceSocketPath)
			},
		},
	}

	payload := map[string]string{
		"local":    fmt.Sprintf(":%d", g.sshPort),
		"protocol": "tcp",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to unexpose port: %w", err)
	}
	defer resp.Body.Close()

	// Ignore errors if port is not already exposed (404 or proxy not found)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to unexpose port: status %d, body: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ExposePort exposes a port on the host to a VM IP address using gvproxy's HTTP API
// This allows dynamic port forwarding when the VM's IP address is not 192.168.127.2
// If a port forwarding already exists, it will be removed first
func (g *GvproxyClient) ExposePort(ctx context.Context, vmIP string, vmPort int) error {
	if g.serviceSocketPath == "" {
		return fmt.Errorf("service socket path not set")
	}

	// First, try to remove any existing port forwarding to avoid "proxy already running" error
	_ = g.UnexposePort(ctx) // Ignore errors - port may not exist

	// Use gvproxy's HTTP API to expose the port
	// POST to http://unix/services/forwarder/expose
	// Body: {"local":":2222","remote":"192.168.127.3:22","protocol":"tcp"}
	url := "http://unix/services/forwarder/expose"

	// Create HTTP client that uses Unix socket
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", g.serviceSocketPath)
			},
		},
	}

	payload := map[string]string{
		"local":    fmt.Sprintf(":%d", g.sshPort),
		"remote":   fmt.Sprintf("%s:%d", vmIP, vmPort),
		"protocol": "tcp",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
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
		// Check if error is "proxy already running" - this can happen if unexpose didn't work
		bodyStr := string(body)
		if strings.Contains(bodyStr, "proxy already running") {
			// Try to unexpose and retry once more
			if err := g.UnexposePort(ctx); err == nil {
				// Retry the expose
				req2, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
				req2.Header.Set("Content-Type", "application/json")
				resp2, err2 := client.Do(req2)
				if err2 == nil {
					resp2.Body.Close()
					if resp2.StatusCode == http.StatusOK {
						if g.verbose {
							fmt.Printf("✅ Exposed port %d on host to %s:%d via gvproxy (after retry)\n", g.sshPort, vmIP, vmPort)
						}
						return nil
					}
				}
			}
		}
		return fmt.Errorf("failed to expose port: status %d, body: %s", resp.StatusCode, bodyStr)
	}

	if g.verbose {
		fmt.Printf("✅ Exposed port %d on host to %s:%d via gvproxy\n", g.sshPort, vmIP, vmPort)
	}

	return nil
}
