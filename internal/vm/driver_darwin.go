//go:build darwin

package vm

// NewDriver creates a new VM driver for macOS (vfkit)
func NewDriver(opts VMOptions, verbose bool) (Driver, error) {
	return NewVfkitDriver(opts, verbose)
}
