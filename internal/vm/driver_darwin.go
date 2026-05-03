//go:build darwin

package vm

import (
	"os/exec"

	"github.com/tnk4on/bootc-man/internal/config"
)

// NewDriver creates a new VM driver for macOS.
// Prefers krunkit (libkrun/HVF) if available, falls back to vfkit.
func NewDriver(opts VMOptions, verbose bool) (Driver, error) {
	if _, err := exec.LookPath(config.BinaryKrunkit); err == nil {
		return NewKrunkitDriver(opts, verbose)
	}
	return NewVfkitDriver(opts, verbose)
}
