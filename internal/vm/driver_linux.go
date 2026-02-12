//go:build linux

package vm

// NewDriver creates a new VM driver for Linux (QEMU/KVM)
func NewDriver(opts VMOptions, verbose bool) (Driver, error) {
	return NewQemuDriver(opts, verbose)
}
