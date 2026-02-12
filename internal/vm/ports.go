package vm

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	portAllocFileName = "port-alloc.dat"
	portLockFileName  = "port-alloc.lck"
)

// getPodmanMachineDataDir returns the podman machine data directory
// This is the same directory used by podman machine for port allocation
func getPodmanMachineDataDir() (string, error) {
	// Use XDG_DATA_HOME or default to ~/.local/share
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(homeDir, ".local", "share")
	}
	
	dataDir := filepath.Join(dataHome, "containers", "podman", "machine")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", err
	}
	return dataDir, nil
}

// AllocateMachinePort reserves a unique port for a VM instance
// Uses the same port-alloc.dat file as podman machine to avoid conflicts
func AllocateMachinePort() (int, error) {
	const maxRetries = 10000

	handles := []io.Closer{}
	defer func() {
		for _, handle := range handles {
			handle.Close()
		}
	}()

	lock, err := acquirePortLock()
	if err != nil {
		return 0, err
	}
	defer lock.Close()

	ports, err := loadPortAllocations()
	if err != nil {
		return 0, err
	}

	var port int
	for i := 0; ; i++ {
		var handle io.Closer

		// Ports must be held temporarily to prevent repeat search results
		handle, port, err = getRandomPortHold()
		if err != nil {
			return 0, err
		}
		handles = append(handles, handle)

		if _, exists := ports[port]; !exists {
			break
		}

		if i > maxRetries {
			return 0, errors.New("maximum number of retries exceeded searching for available port")
		}
	}

	ports[port] = struct{}{}
	if err := storePortAllocations(ports); err != nil {
		return 0, err
	}

	return port, nil
}

// ReleaseMachinePort releases a reserved port for a VM when no longer required
func ReleaseMachinePort(port int) error {
	if port <= 0 {
		return nil
	}

	lock, err := acquirePortLock()
	if err != nil {
		return err
	}
	defer lock.Close()

	ports, err := loadPortAllocations()
	if err != nil {
		return err
	}

	delete(ports, port)
	return storePortAllocations(ports)
}

// IsLocalPortAvailable checks if a port is available for use
func IsLocalPortAvailable(port int) bool {
	if port <= 0 {
		return false
	}

	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// FindAvailablePort finds an available TCP port starting from the given port
// This is for local use only and does not register in port-alloc.dat
func FindAvailablePort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err == nil {
			listener.Close()
			return port
		}
	}
	return startPort
}

func getRandomPortHold() (io.Closer, int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, 0, fmt.Errorf("unable to get free machine port: %w", err)
	}
	_, portString, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		l.Close()
		return nil, 0, fmt.Errorf("unable to determine free machine port: %w", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		l.Close()
		return nil, 0, fmt.Errorf("unable to convert port to int: %w", err)
	}
	return l, port, nil
}

// acquirePortLock acquires an exclusive lock on the port allocation file
func acquirePortLock() (*os.File, error) {
	lockDir, err := getPodmanMachineDataDir()
	if err != nil {
		return nil, err
	}

	lockPath := filepath.Join(lockDir, portLockFileName)
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	// Acquire exclusive lock
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		lock.Close()
		return nil, fmt.Errorf("failed to acquire port lock: %w", err)
	}

	return lock, nil
}

func loadPortAllocations() (map[int]struct{}, error) {
	portDir, err := getPodmanMachineDataDir()
	if err != nil {
		return nil, err
	}

	var portData []int
	exists := true
	file, err := os.OpenFile(filepath.Join(portDir, portAllocFileName), os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		exists = false
	} else if err != nil {
		return nil, err
	}
	if exists {
		defer file.Close()
	}

	// Non-existence of the file, or a corrupt file are not treated as hard
	// failures, since dynamic reassignment and continued use will eventually
	// rebuild the dataset
	if exists {
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&portData); err != nil {
			// Corrupt file - ignore and start fresh
			portData = nil
		}
	}

	ports := make(map[int]struct{})
	for _, port := range portData {
		ports[port] = struct{}{}
	}

	return ports, nil
}

func storePortAllocations(ports map[int]struct{}) error {
	portDir, err := getPodmanMachineDataDir()
	if err != nil {
		return err
	}

	portData := make([]int, 0, len(ports))
	for port := range ports {
		portData = append(portData, port)
	}

	// Write to temp file first, then rename (atomic)
	portFile := filepath.Join(portDir, portAllocFileName)
	tmpFile := portFile + ".tmp"

	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(f)
	if err := enc.Encode(portData); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return err
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpFile)
		return err
	}

	return os.Rename(tmpFile, portFile)
}
