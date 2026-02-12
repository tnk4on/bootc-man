// Package config provides configuration management for bootc-man.
// This file contains helpers for finding bundled and system binaries.
package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	// MinGvproxyVersion is the minimum required gvproxy version.
	// v0.8.3 added the -services flag for HTTP API port forwarding.
	MinGvproxyVersion = "v0.8.3"
	// MinVfkitVersion is the minimum required vfkit version.
	// v0.6.1 supports EFI bootloader and RESTful API.
	MinVfkitVersion = "v0.6.1"
)

// FindGvproxyBinary searches for the gvproxy binary in priority order:
// 1. Homebrew libexec (derived from own binary path): .../libexec/bootc-man/gvproxy
// 2. PATH: gvproxy
// 3. System locations (Linux): /usr/libexec/podman/gvproxy, /usr/lib/podman/gvproxy
// Returns the absolute path to the binary, or the bare name as fallback.
func FindGvproxyBinary() string {
	// 1. Check Homebrew libexec path (derived from own binary location)
	// If bootc-man is at /opt/homebrew/bin/bootc-man,
	// gvproxy should be at /opt/homebrew/libexec/bootc-man/gvproxy
	if selfPath, err := os.Executable(); err == nil {
		selfPath, _ = filepath.EvalSymlinks(selfPath)
		binDir := filepath.Dir(selfPath)
		prefix := filepath.Dir(binDir) // e.g. /opt/homebrew or /usr/local
		libexecPath := filepath.Join(prefix, "libexec", "bootc-man", BinaryGvproxy)
		if _, err := os.Stat(libexecPath); err == nil {
			return libexecPath
		}
	}

	// 2. Check PATH
	if path, err := exec.LookPath(BinaryGvproxy); err == nil {
		return path
	}

	// 3. Check system locations (Linux)
	systemLocations := []string{
		"/usr/libexec/podman/gvproxy", // Fedora/RHEL
		"/usr/lib/podman/gvproxy",     // Alternative location
	}
	for _, loc := range systemLocations {
		if _, err := os.Stat(loc); err == nil {
			return loc
		}
	}

	return BinaryGvproxy // fallback to bare name (will fail with helpful error)
}

// FindVfkitBinary searches for the vfkit binary in priority order:
// 1. Homebrew libexec (derived from own binary path)
// 2. PATH
func FindVfkitBinary() string {
	// 1. Check Homebrew libexec path
	if selfPath, err := os.Executable(); err == nil {
		selfPath, _ = filepath.EvalSymlinks(selfPath)
		binDir := filepath.Dir(selfPath)
		prefix := filepath.Dir(binDir)
		libexecPath := filepath.Join(prefix, "libexec", "bootc-man", BinaryVfkit)
		if _, err := os.Stat(libexecPath); err == nil {
			return libexecPath
		}
	}

	// 2. Check PATH
	if path, err := exec.LookPath(BinaryVfkit); err == nil {
		return path
	}

	return BinaryVfkit // fallback
}

// GetGvproxyVersion returns the installed gvproxy version string (e.g. "v0.8.7").
// Returns empty string if gvproxy is not found or version cannot be determined.
func GetGvproxyVersion() string {
	binary := FindGvproxyBinary()
	cmd := exec.Command(binary, "-version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Parse: "gvproxy version v0.8.7"
	return extractSemver(strings.TrimSpace(string(output)))
}

// GetVfkitVersion returns the installed vfkit version string (e.g. "v0.6.1").
func GetVfkitVersion() string {
	binary := FindVfkitBinary()
	cmd := exec.Command(binary, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Parse: "vfkit version: v0.6.3" or "vfkit v0.6.3"
	return extractSemver(strings.TrimSpace(string(output)))
}

// extractSemver extracts a semantic version (vN.N.N) from a string.
// Looks for a token starting with "v" followed by a digit (e.g. "v0.8.7").
func extractSemver(s string) string {
	for _, part := range strings.Fields(s) {
		// Match "vN..." pattern (v followed by digit), not words like "version"
		clean := strings.TrimSuffix(part, ",")
		clean = strings.TrimSuffix(clean, ":")
		if len(clean) >= 2 && clean[0] == 'v' && unicode.IsDigit(rune(clean[1])) {
			return clean
		}
	}
	return ""
}

// CompareVersions compares two semantic version strings (e.g. "v0.8.3", "v0.7.3").
// Returns: -1 if a < b, 0 if a == b, 1 if a > b.
func CompareVersions(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	for i := 0; i < 3; i++ {
		var aNum, bNum int
		if i < len(aParts) {
			fmt.Sscanf(aParts[i], "%d", &aNum)
		}
		if i < len(bParts) {
			fmt.Sscanf(bParts[i], "%d", &bNum)
		}
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
	}
	return 0
}

// CheckGvproxyVersion validates that the installed gvproxy meets the minimum version.
// Returns nil if OK, or an error with instructions if version is too old.
func CheckGvproxyVersion() error {
	version := GetGvproxyVersion()
	if version == "" {
		return fmt.Errorf("gvproxy is not installed or version cannot be determined")
	}
	if CompareVersions(version, MinGvproxyVersion) < 0 {
		return fmt.Errorf("gvproxy %s is too old (required: >=%s). The -services flag was added in %s.\n"+
			"  Update: brew reinstall bootc-man\n"+
			"  Or install from: https://github.com/containers/gvisor-tap-vsock/releases",
			version, MinGvproxyVersion, MinGvproxyVersion)
	}
	return nil
}

// CheckVfkitVersion validates that the installed vfkit meets the minimum version.
func CheckVfkitVersion() error {
	version := GetVfkitVersion()
	if version == "" {
		return fmt.Errorf("vfkit is not installed or version cannot be determined")
	}
	if CompareVersions(version, MinVfkitVersion) < 0 {
		return fmt.Errorf("vfkit %s is too old (required: >=%s).\n"+
			"  Update: brew reinstall bootc-man\n"+
			"  Or install from: https://github.com/crc-org/vfkit/releases",
			version, MinVfkitVersion)
	}
	return nil
}
