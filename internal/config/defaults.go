// Package config provides configuration management for bootc-man.
// This file contains all default values and constants used throughout the application.
// Following containers/common pattern, all configurable defaults are centralized here.
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// =============================================================================
// Container Names
// =============================================================================

const (
	// ContainerNameRegistry is the name of the registry container
	ContainerNameRegistry = "bootc-man-registry"
	// ContainerNameCI is the name of the CI container
	ContainerNameCI = "bootc-man-ci"
	// ContainerNameGUI is the name of the GUI container
	ContainerNameGUI = "bootc-man-gui"
	// VolumeNameRegistryData is the name of the registry data volume
	VolumeNameRegistryData = "bootc-man-registry-data"
	// VolumeNameTrivyCache is the name of the Trivy cache volume
	VolumeNameTrivyCache = "bootc-man-trivy-cache"
	// VolumeNameGrypeCache is the name of the Grype cache volume
	VolumeNameGrypeCache = "bootc-man-grype-cache"
)

// =============================================================================
// Port Numbers
// =============================================================================

const (
	// DefaultRegistryPort is the default port for the container registry
	DefaultRegistryPort = 5000
	// DefaultRegistryContainerPort is the internal port of the registry container
	DefaultRegistryContainerPort = 5000
	// DefaultCIPort is the default port for the CI service
	DefaultCIPort = 8080
	// DefaultGUIPort is the default port for the GUI service
	DefaultGUIPort = 3000
	// DefaultSSHForwardPort is the default SSH forwarding port for VMs (gvproxy)
	DefaultSSHForwardPort = 2222
	// DefaultVfkitAPIPort is the default port for vfkit RESTful API
	DefaultVfkitAPIPort = 12345
	// DefaultSSHPort is the standard SSH port
	DefaultSSHPort = 22
)

// =============================================================================
// Container Images
// =============================================================================

const (
	// DefaultRegistryImage is the default container registry image
	DefaultRegistryImage = "docker.io/library/registry:2"
	// DefaultBootcImageBuilder is the default bootc-image-builder container image
	// Uses CentOS bootc image builder which is publicly available without authentication
	DefaultBootcImageBuilder = "quay.io/centos-bootc/bootc-image-builder"
	// DefaultHadolintImage is the default Hadolint image for Dockerfile linting
	DefaultHadolintImage = "docker.io/hadolint/hadolint:latest"
	// DefaultTrivyImage is the default Trivy image for vulnerability scanning
	DefaultTrivyImage = "docker.io/aquasec/trivy:latest"
	// DefaultGrypeImage is the default Grype image for vulnerability scanning
	DefaultGrypeImage = "docker.io/anchore/grype:latest"
	// DefaultSyftImage is the default Syft image for SBOM generation
	DefaultSyftImage = "docker.io/anchore/syft:latest"
	// DefaultSkopeoImage is the default Skopeo image for image operations
	DefaultSkopeoImage = "quay.io/skopeo/stable:latest"
	// DefaultGitleaksImage is the default Gitleaks image for secret scanning
	DefaultGitleaksImage = "docker.io/zricethezav/gitleaks:latest"
	// DefaultTrufflehogImage is the default Trufflehog image for secret scanning
	DefaultTrufflehogImage = "docker.io/trufflesecurity/trufflehog:latest"
	// DefaultFedoraBootcImage is the default Fedora bootc base image
	DefaultFedoraBootcImage = "quay.io/fedora/fedora-bootc:latest"
	// DefaultCentOSBootcImage is the default CentOS bootc base image
	DefaultCentOSBootcImage = "quay.io/centos-bootc/centos-bootc:stream10"
)

// =============================================================================
// Timeouts
// =============================================================================

const (
	// DefaultVMBootTimeout is the default timeout for VM boot
	DefaultVMBootTimeout = 30 * time.Second
	// DefaultSSHConnectTimeout is the default timeout for SSH connection
	DefaultSSHConnectTimeout = 10 * time.Second
	// DefaultSSHRetryTimeout is the default timeout for SSH retry
	DefaultSSHRetryTimeout = 15 * time.Second
	// DefaultSSHTestTimeout is the default timeout for SSH test
	DefaultSSHTestTimeout = 2 * time.Second
	// DefaultHTTPClientTimeout is the default timeout for HTTP client
	DefaultHTTPClientTimeout = 5 * time.Second
	// DefaultSocketTimeout is the default timeout for socket creation
	DefaultSocketTimeout = 10 * time.Second
	// DefaultGitHubAPITimeout is the default timeout for GitHub API calls
	DefaultGitHubAPITimeout = 3 * time.Second
	// DefaultSoftRebootTimeout is the default timeout for soft reboot
	DefaultSoftRebootTimeout = 20 * time.Second
	// DefaultHardRebootStopTimeout is the default timeout for hard reboot stop
	DefaultHardRebootStopTimeout = 30 * time.Second
	// DefaultHardRebootRestartTimeout is the default timeout for hard reboot restart
	DefaultHardRebootRestartTimeout = 60 * time.Second
)

// =============================================================================
// Network Configuration
// =============================================================================

const (
	// DefaultLocalhost is the default localhost address
	DefaultLocalhost = "localhost"
	// DefaultLocalhostIP is the default localhost IP address
	DefaultLocalhostIP = "127.0.0.1"
	// DefaultVMIP is the default VM IP address (gvproxy)
	DefaultVMIP = "192.168.127.2"
	// DefaultGatewayIP is the default gateway IP address
	DefaultGatewayIP = "192.168.127.1"
	// AlternativeVMIP is an alternative VM IP address
	AlternativeVMIP = "192.168.127.3"
)

// =============================================================================
// SSH Configuration
// =============================================================================

const (
	// DefaultSSHUser is the default SSH user for VM connections
	DefaultSSHUser = "user"
	// DefaultSSHKeyPath is the default SSH key path (relative to home)
	DefaultSSHKeyPath = ".ssh/id_ed25519"
	// SSHOptionStrictHostKeyCheckingNo is the SSH option to disable strict host key checking
	SSHOptionStrictHostKeyCheckingNo = "StrictHostKeyChecking=no"
	// SSHOptionStrictHostKeyCheckingAcceptNew is the SSH option to accept new host keys
	SSHOptionStrictHostKeyCheckingAcceptNew = "StrictHostKeyChecking=accept-new"
	// SSHOptionConnectTimeout2 is the SSH option for 2 second connect timeout
	SSHOptionConnectTimeout2 = "ConnectTimeout=2"
	// SSHOptionConnectTimeout10 is the SSH option for 10 second connect timeout
	SSHOptionConnectTimeout10 = "ConnectTimeout=10"
	// SSHOptionUserKnownHostsFileDevNull is the SSH option to use /dev/null as known hosts file
	SSHOptionUserKnownHostsFileDevNull = "UserKnownHostsFile=/dev/null"
)

// =============================================================================
// File Paths and Patterns
// =============================================================================

const (
	// DefaultRegistryDataPath is the default path for registry data inside container
	DefaultRegistryDataPath = "/var/lib/registry"
	// DefaultKeygenTempDir is the default temp directory for keygen
	DefaultKeygenTempDir = "/var/tmp/bootc-man-keygen"
	// DefaultSignTempDir is the default temp directory for signing
	DefaultSignTempDir = "/var/tmp/bootc-man-sign"

	// GvproxySockPattern is the pattern for gvproxy socket filename
	GvproxySockPattern = "bootc-man-%s-gvproxy.sock"
	// GvproxyPIDPattern is the pattern for gvproxy PID filename
	GvproxyPIDPattern = "bootc-man-%s-gvproxy.pid"
	// GvproxyLogPattern is the pattern for gvproxy log filename
	GvproxyLogPattern = "bootc-man-%s-gvproxy.log"
	// EFIStorePattern is the pattern for EFI store directory
	EFIStorePattern = "bootc-man-%s-efi-store"
	// TestDiskImagePattern is the pattern for test disk image filename
	TestDiskImagePattern = "bootc-man-test-%s.raw"
	// ScanArchiveTempPattern is the pattern for scan archive temp filename
	ScanArchiveTempPattern = "bootc-man-scan-*.tar"
	// DigestFileTempPattern is the pattern for digest file temp filename
	DigestFileTempPattern = "bootc-man-digest-*.txt"
)

// =============================================================================
// VM Defaults
// =============================================================================

const (
	// DefaultVMCPUs is the default number of CPUs for VMs
	DefaultVMCPUs = 2
	// DefaultVMMemoryMB is the default memory size in MB for VMs
	DefaultVMMemoryMB = 4096
)

// =============================================================================
// Labels and Metadata
// =============================================================================

const (
	// LabelBootc is the label key for bootc containers
	LabelBootc = "containers.bootc"
	// PipelineAPIVersion is the supported API version for pipeline definitions
	PipelineAPIVersion = "bootc-man/v1"
	// PipelineKind is the expected kind for pipeline definitions
	PipelineKind = "Pipeline"
	// DefaultPipelineFileName is the default pipeline definition filename
	DefaultPipelineFileName = "bootc-ci.yaml"
	// DefaultContainerfileName is the default Containerfile name
	DefaultContainerfileName = "Containerfile"
)

// =============================================================================
// Binary Names
// =============================================================================

const (
	// BinaryGvproxy is the name of the gvproxy binary
	BinaryGvproxy = "gvproxy"
	// BinaryVfkit is the name of the vfkit binary
	BinaryVfkit = "vfkit"
	// BinaryPodman is the name of the podman binary
	BinaryPodman = "podman"
	// BinarySSH is the name of the ssh binary
	BinarySSH = "ssh"
	// BinarySSHKeygen is the name of the ssh-keygen binary
	BinarySSHKeygen = "ssh-keygen"
)

// =============================================================================
// Config File Paths
// =============================================================================

const (
	// SystemDefaultConfigPath is the system default config file path
	SystemDefaultConfigPath = "/usr/share/bootc-man/config.yaml"
	// SystemAdminConfigPath is the system admin config file path
	SystemAdminConfigPath = "/etc/bootc-man/config.yaml"
	// UserConfigFileName is the user config file name (relative to ~/.config)
	UserConfigFileName = "bootc-man/config.yaml"
)

// =============================================================================
// Miscellaneous
// =============================================================================

const (
	// DefaultSSHPublicKeyPlaceholder is the placeholder for SSH public key in samples
	DefaultSSHPublicKeyPlaceholder = "ssh-ed25519 REPLACE_WITH_YOUR_SSH_PUBLIC_KEY bootc-man-placeholder"
	// StageSeparator is the separator line for CI stages
	StageSeparator = "────────────────────────────────────────────────────────────────────────────────"
)

// =============================================================================
// Temporary Directory Helpers
// =============================================================================
// On Linux (Fedora/RHEL), /tmp is typically tmpfs (backed by RAM).
// For an 8GB VM, /tmp is only ~3.9GB which is insufficient for 10GB+ disk images.
// These helpers provide disk-backed alternatives:
// - RuntimeDir(): for small files (sockets, PIDs, logs) -> /var/tmp/bootc-man/
// - TempDataDir(): for large files (disk images) -> ~/.local/share/bootc-man/tmp/

// RuntimeDir returns a disk-backed temporary directory for small runtime files
// (sockets, PID files, logs, EFI variable stores).
// On Linux, uses /var/tmp/bootc-man/ which is disk-backed (not tmpfs).
// On macOS, uses /tmp/bootc-man/ to keep socket paths short.
// macOS's default $TMPDIR (/var/folders/.../T/) is too long and causes
// Unix socket path length limit (104 bytes) errors with gvproxy.
// See: https://github.com/containers/podman/issues/22360
// The directory is created if it does not exist.
func RuntimeDir() string {
	var dir string
	if runtime.GOOS == "linux" {
		dir = "/var/tmp/bootc-man"
	} else {
		// Use /tmp/bootc-man instead of os.TempDir() to avoid
		// exceeding the 104-byte Unix socket path limit on macOS.
		dir = "/tmp/bootc-man"
	}
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// TempDataDir returns a directory for large temporary files (e.g., disk images).
// Uses ~/.local/share/bootc-man/tmp/ which is always on a real filesystem.
// The directory is created if it does not exist.
func TempDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to RuntimeDir if home directory is unavailable
		return RuntimeDir()
	}
	dir := filepath.Join(home, ".local", "share", "bootc-man", "tmp")
	_ = os.MkdirAll(dir, 0755)
	return dir
}
