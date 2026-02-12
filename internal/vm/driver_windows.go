//go:build windows

package vm

import "fmt"

// NewDriver creates a new VM driver for Windows (Hyper-V)
// Note: Hyper-V support is not yet implemented
func NewDriver(opts VMOptions, verbose bool) (Driver, error) {
	return nil, fmt.Errorf("Windows support is not yet implemented (Hyper-V driver)")
}
