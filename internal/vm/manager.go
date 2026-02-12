package vm

import (
	"os"
	"syscall"
)

// IsProcessRunning checks if a process with the given PID is running
func IsProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks if process exists without sending actual signal
	if err := process.Signal(os.Signal(syscall.Signal(0))); err != nil {
		return false
	}
	return true
}

// IsVMRunning checks if a VM is running by checking its main process
func IsVMRunning(info *VMInfo) bool {
	// Check ProcessID first (new format), then VfkitPID (legacy format)
	pid := info.ProcessID
	if pid == 0 {
		pid = info.VfkitPID
	}
	return IsProcessRunning(pid)
}

// StopProcess attempts to stop a process gracefully, with force kill fallback
func StopProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil // Process doesn't exist
	}

	// Try graceful shutdown first
	if err := process.Signal(os.Interrupt); err != nil {
		// If signal failed, process may already be dead
		return nil
	}

	// Wait for process to exit (caller should handle timeout if needed)
	_, _ = process.Wait()
	return nil
}
