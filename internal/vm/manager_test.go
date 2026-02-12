package vm

import (
	"os"
	"testing"
)

func TestIsProcessRunning(t *testing.T) {
	tests := []struct {
		name     string
		pid      int
		expected bool
	}{
		{
			name:     "invalid pid zero",
			pid:      0,
			expected: false,
		},
		{
			name:     "invalid pid negative",
			pid:      -1,
			expected: false,
		},
		{
			name:     "current process is running",
			pid:      os.Getpid(),
			expected: true,
		},
		{
			name:     "non-existent pid",
			pid:      99999999, // Very unlikely to exist
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsProcessRunning(tt.pid)
			if result != tt.expected {
				t.Errorf("IsProcessRunning(%d) = %v, want %v", tt.pid, result, tt.expected)
			}
		})
	}
}

func TestIsVMRunning(t *testing.T) {
	currentPID := os.Getpid()

	tests := []struct {
		name     string
		info     *VMInfo
		expected bool
	}{
		{
			name: "running with ProcessID",
			info: &VMInfo{
				ProcessID: currentPID,
			},
			expected: true,
		},
		{
			name: "running with VfkitPID (legacy)",
			info: &VMInfo{
				VfkitPID: currentPID,
			},
			expected: true,
		},
		{
			name: "ProcessID takes precedence",
			info: &VMInfo{
				ProcessID: currentPID,
				VfkitPID:  99999999, // Invalid but should not be checked
			},
			expected: true,
		},
		{
			name: "not running with invalid ProcessID",
			info: &VMInfo{
				ProcessID: 99999999,
			},
			expected: false,
		},
		{
			name: "not running with zero PID",
			info: &VMInfo{
				ProcessID: 0,
				VfkitPID:  0,
			},
			expected: false,
		},
		{
			name: "fallback to VfkitPID when ProcessID is zero",
			info: &VMInfo{
				ProcessID: 0,
				VfkitPID:  currentPID,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsVMRunning(tt.info)
			if result != tt.expected {
				t.Errorf("IsVMRunning(%+v) = %v, want %v", tt.info, result, tt.expected)
			}
		})
	}
}

func TestStopProcess(t *testing.T) {
	tests := []struct {
		name      string
		pid       int
		wantError bool
	}{
		{
			name:      "zero pid returns nil",
			pid:       0,
			wantError: false,
		},
		{
			name:      "negative pid returns nil",
			pid:       -1,
			wantError: false,
		},
		{
			name:      "non-existent process returns nil",
			pid:       99999999,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := StopProcess(tt.pid)
			if tt.wantError && err == nil {
				t.Errorf("StopProcess(%d) = nil, want error", tt.pid)
			}
			if !tt.wantError && err != nil {
				t.Errorf("StopProcess(%d) = %v, want nil", tt.pid, err)
			}
		})
	}
}
