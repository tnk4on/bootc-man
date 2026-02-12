// Package config provides configuration management for bootc-man.
// The design follows containers/common's pattern with hierarchical config files
// and environment variable overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// Environment variable names for configuration overrides
const (
	EnvConfig            = "BOOTCMAN_CONFIG"
	EnvDataDir           = "BOOTCMAN_DATA_DIR"
	EnvRegistryPort      = "BOOTCMAN_REGISTRY_PORT"
	EnvCIPort            = "BOOTCMAN_CI_PORT"
	EnvGUIPort           = "BOOTCMAN_GUI_PORT"
	EnvPodmanPath        = "BOOTCMAN_PODMAN"
	EnvBootcImageBuilder = "BOOTCMAN_BOOTC_IMAGE_BUILDER"
	EnvExperimental      = "BOOTCMAN_EXPERIMENTAL"
)

// Config represents the bootc-man configuration
type Config struct {
	Runtime      RuntimeConfig    `yaml:"runtime"`
	Paths        PathsConfig      `yaml:"paths"`
	Registry     RegistryConfig   `yaml:"registry"`
	CI           CIConfig         `yaml:"ci"`
	GUI          GUIConfig        `yaml:"gui"`
	VM           VMConfig         `yaml:"vm"`
	Containers   ContainersConfig `yaml:"containers"`
	Images       ImagesConfig     `yaml:"images"`
	Network      NetworkConfig    `yaml:"network"`
	Timeouts     TimeoutsConfig   `yaml:"timeouts"`
	SSH          SSHConfig        `yaml:"ssh"`
	Experimental bool             `yaml:"experimental"`
}

// RuntimeConfig contains runtime settings
type RuntimeConfig struct {
	// Podman binary to use: "auto", "podman", or full path
	Podman string `yaml:"podman"`
}

// PathsConfig contains path settings
type PathsConfig struct {
	// Data directory for bootc-man state
	Data string `yaml:"data"`
}

// RegistryConfig contains registry service settings
type RegistryConfig struct {
	// Port to expose the registry on
	Port int `yaml:"port"`
	// Container image to use for the registry
	Image string `yaml:"image"`
}

// CIConfig contains CI service settings
type CIConfig struct {
	// Remote execution target for Linux-only stages (e.g., "ssh://host", "podman-machine")
	Remote string `yaml:"remote,omitempty"`
	// Port for CI web interface (if any)
	Port int `yaml:"port"`
	// BootcImageBuilder is the container image for bootc-image-builder
	BootcImageBuilder string `yaml:"bootc_image_builder,omitempty"`
}

// GUIConfig contains GUI service settings
type GUIConfig struct {
	// Port to expose the GUI on
	Port int `yaml:"port"`
}

// VMConfig contains VM settings
type VMConfig struct {
	// Default SSH user for VM connections
	SSHUser string `yaml:"ssh_user"`
	// Default number of CPUs for VMs
	CPUs int `yaml:"cpus"`
	// Default memory size in MB for VMs
	Memory int `yaml:"memory"`
}

// ContainersConfig contains container naming settings
type ContainersConfig struct {
	// Name of the registry container
	RegistryName string `yaml:"registry_name"`
	// Name of the CI container
	CIName string `yaml:"ci_name"`
	// Name of the GUI container
	GUIName string `yaml:"gui_name"`
	// Name of the registry data volume
	RegistryDataVolume string `yaml:"registry_data_volume"`
	// Name of the Trivy cache volume
	TrivyCacheVolume string `yaml:"trivy_cache_volume"`
	// Name of the Grype cache volume
	GrypeCacheVolume string `yaml:"grype_cache_volume"`
}

// ImagesConfig contains container image settings
type ImagesConfig struct {
	// Hadolint image for Dockerfile linting
	Hadolint string `yaml:"hadolint"`
	// Trivy image for vulnerability scanning
	Trivy string `yaml:"trivy"`
	// Grype image for vulnerability scanning
	Grype string `yaml:"grype"`
	// Syft image for SBOM generation
	Syft string `yaml:"syft"`
	// Skopeo image for image operations
	Skopeo string `yaml:"skopeo"`
	// Gitleaks image for secret scanning
	Gitleaks string `yaml:"gitleaks"`
	// Trufflehog image for secret scanning
	Trufflehog string `yaml:"trufflehog"`
}

// NetworkConfig contains network settings
type NetworkConfig struct {
	// Default VM IP address (gvproxy)
	VMIP string `yaml:"vm_ip"`
	// Default gateway IP address
	GatewayIP string `yaml:"gateway_ip"`
	// Default SSH forwarding port for VMs
	SSHForwardPort int `yaml:"ssh_forward_port"`
	// Default vfkit API port
	VfkitAPIPort int `yaml:"vfkit_api_port"`
}

// TimeoutsConfig contains timeout settings (in seconds)
type TimeoutsConfig struct {
	// VM boot timeout in seconds
	VMBoot int `yaml:"vm_boot"`
	// SSH connection timeout in seconds
	SSHConnect int `yaml:"ssh_connect"`
	// SSH retry timeout in seconds
	SSHRetry int `yaml:"ssh_retry"`
	// HTTP client timeout in seconds
	HTTPClient int `yaml:"http_client"`
	// Socket creation timeout in seconds
	Socket int `yaml:"socket"`
}

// SSHConfig contains SSH settings
type SSHConfig struct {
	// Default SSH user for VM connections
	User string `yaml:"user"`
	// Default SSH key path (relative to home)
	KeyPath string `yaml:"key_path"`
	// SSH option for strict host key checking
	StrictHostKeyChecking string `yaml:"strict_host_key_checking"`
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".local", "share", "bootc-man")

	return &Config{
		Runtime: RuntimeConfig{
			Podman: "auto",
		},
		Paths: PathsConfig{
			Data: dataDir,
		},
		Registry: RegistryConfig{
			Port:  DefaultRegistryPort,
			Image: DefaultRegistryImage,
		},
		CI: CIConfig{
			Port:              DefaultCIPort,
			BootcImageBuilder: DefaultBootcImageBuilder,
		},
		GUI: GUIConfig{
			Port: DefaultGUIPort,
		},
		VM: VMConfig{
			SSHUser: DefaultSSHUser,
			CPUs:    DefaultVMCPUs,
			Memory:  DefaultVMMemoryMB,
		},
		Containers: ContainersConfig{
			RegistryName:       ContainerNameRegistry,
			CIName:             ContainerNameCI,
			GUIName:            ContainerNameGUI,
			RegistryDataVolume: VolumeNameRegistryData,
			TrivyCacheVolume:   VolumeNameTrivyCache,
			GrypeCacheVolume:   VolumeNameGrypeCache,
		},
		Images: ImagesConfig{
			Hadolint:   DefaultHadolintImage,
			Trivy:      DefaultTrivyImage,
			Grype:      DefaultGrypeImage,
			Syft:       DefaultSyftImage,
			Skopeo:     DefaultSkopeoImage,
			Gitleaks:   DefaultGitleaksImage,
			Trufflehog: DefaultTrufflehogImage,
		},
		Network: NetworkConfig{
			VMIP:           DefaultVMIP,
			GatewayIP:      DefaultGatewayIP,
			SSHForwardPort: DefaultSSHForwardPort,
			VfkitAPIPort:   DefaultVfkitAPIPort,
		},
		Timeouts: TimeoutsConfig{
			VMBoot:     int(DefaultVMBootTimeout.Seconds()),
			SSHConnect: int(DefaultSSHConnectTimeout.Seconds()),
			SSHRetry:   int(DefaultSSHRetryTimeout.Seconds()),
			HTTPClient: int(DefaultHTTPClientTimeout.Seconds()),
			Socket:     int(DefaultSocketTimeout.Seconds()),
		},
		SSH: SSHConfig{
			User:                  DefaultSSHUser,
			KeyPath:               DefaultSSHKeyPath,
			StrictHostKeyChecking: "accept-new",
		},
	}
}

// configPaths returns the list of config file paths to check, in order of priority
// (later files override earlier ones)
func configPaths() []string {
	var paths []string

	// System default (lowest priority)
	paths = append(paths, "/usr/share/bootc-man/config.yaml")

	// System admin config
	paths = append(paths, "/etc/bootc-man/config.yaml")

	// User config (highest priority for files)
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".config", "bootc-man", "config.yaml"))
	}

	return paths
}

// Load reads configuration from files and applies environment overrides.
// It follows the containers/common pattern:
// 1. Start with default values
// 2. Load system default config
// 3. Load system admin config
// 4. Load user config
// 5. Apply environment variable overrides
func Load(explicitPath string) (*Config, error) {
	cfg := DefaultConfig()

	// If explicit path is provided, only load that file
	if explicitPath != "" {
		if err := loadFile(cfg, explicitPath); err != nil {
			return nil, err
		}
		applyEnvOverrides(cfg)
		return cfg, nil
	}

	// Check for environment variable override for config path
	if envPath := os.Getenv(EnvConfig); envPath != "" {
		if err := loadFile(cfg, envPath); err != nil {
			return nil, err
		}
		applyEnvOverrides(cfg)
		return cfg, nil
	}

	// Load config files in order (later files override earlier ones)
	var loadedAny bool
	for _, path := range configPaths() {
		if _, err := os.Stat(path); err == nil {
			logrus.Debugf("Loading config from %s", path)
			if err := loadFile(cfg, path); err != nil {
				logrus.Warnf("Failed to load config from %s: %v", path, err)
				continue
			}
			loadedAny = true
		}
	}

	if !loadedAny {
		logrus.Debug("No config files found, using defaults")
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	return cfg, nil
}

// loadFile loads a single config file and merges it into the existing config
func loadFile(cfg *Config, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Parse into a new config to merge
	var fileCfg Config
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Merge non-zero values
	mergeConfig(cfg, &fileCfg)
	return nil
}

// mergeConfig merges src into dst, only overwriting non-zero values
func mergeConfig(dst, src *Config) {
	// Runtime
	if src.Runtime.Podman != "" {
		dst.Runtime.Podman = src.Runtime.Podman
	}

	// Paths
	if src.Paths.Data != "" {
		dst.Paths.Data = src.Paths.Data
	}

	// Registry
	if src.Registry.Port != 0 {
		dst.Registry.Port = src.Registry.Port
	}
	if src.Registry.Image != "" {
		dst.Registry.Image = src.Registry.Image
	}

	// CI
	if src.CI.Remote != "" {
		dst.CI.Remote = src.CI.Remote
	}
	if src.CI.Port != 0 {
		dst.CI.Port = src.CI.Port
	}
	if src.CI.BootcImageBuilder != "" {
		dst.CI.BootcImageBuilder = src.CI.BootcImageBuilder
	}

	// GUI
	if src.GUI.Port != 0 {
		dst.GUI.Port = src.GUI.Port
	}

	// VM
	if src.VM.SSHUser != "" {
		dst.VM.SSHUser = src.VM.SSHUser
	}
	if src.VM.CPUs != 0 {
		dst.VM.CPUs = src.VM.CPUs
	}
	if src.VM.Memory != 0 {
		dst.VM.Memory = src.VM.Memory
	}

	// Containers
	if src.Containers.RegistryName != "" {
		dst.Containers.RegistryName = src.Containers.RegistryName
	}
	if src.Containers.CIName != "" {
		dst.Containers.CIName = src.Containers.CIName
	}
	if src.Containers.GUIName != "" {
		dst.Containers.GUIName = src.Containers.GUIName
	}
	if src.Containers.RegistryDataVolume != "" {
		dst.Containers.RegistryDataVolume = src.Containers.RegistryDataVolume
	}
	if src.Containers.TrivyCacheVolume != "" {
		dst.Containers.TrivyCacheVolume = src.Containers.TrivyCacheVolume
	}
	if src.Containers.GrypeCacheVolume != "" {
		dst.Containers.GrypeCacheVolume = src.Containers.GrypeCacheVolume
	}

	// Images
	if src.Images.Hadolint != "" {
		dst.Images.Hadolint = src.Images.Hadolint
	}
	if src.Images.Trivy != "" {
		dst.Images.Trivy = src.Images.Trivy
	}
	if src.Images.Grype != "" {
		dst.Images.Grype = src.Images.Grype
	}
	if src.Images.Syft != "" {
		dst.Images.Syft = src.Images.Syft
	}
	if src.Images.Skopeo != "" {
		dst.Images.Skopeo = src.Images.Skopeo
	}
	if src.Images.Gitleaks != "" {
		dst.Images.Gitleaks = src.Images.Gitleaks
	}
	if src.Images.Trufflehog != "" {
		dst.Images.Trufflehog = src.Images.Trufflehog
	}

	// Network
	if src.Network.VMIP != "" {
		dst.Network.VMIP = src.Network.VMIP
	}
	if src.Network.GatewayIP != "" {
		dst.Network.GatewayIP = src.Network.GatewayIP
	}
	if src.Network.SSHForwardPort != 0 {
		dst.Network.SSHForwardPort = src.Network.SSHForwardPort
	}
	if src.Network.VfkitAPIPort != 0 {
		dst.Network.VfkitAPIPort = src.Network.VfkitAPIPort
	}

	// Timeouts
	if src.Timeouts.VMBoot != 0 {
		dst.Timeouts.VMBoot = src.Timeouts.VMBoot
	}
	if src.Timeouts.SSHConnect != 0 {
		dst.Timeouts.SSHConnect = src.Timeouts.SSHConnect
	}
	if src.Timeouts.SSHRetry != 0 {
		dst.Timeouts.SSHRetry = src.Timeouts.SSHRetry
	}
	if src.Timeouts.HTTPClient != 0 {
		dst.Timeouts.HTTPClient = src.Timeouts.HTTPClient
	}
	if src.Timeouts.Socket != 0 {
		dst.Timeouts.Socket = src.Timeouts.Socket
	}

	// SSH
	if src.SSH.User != "" {
		dst.SSH.User = src.SSH.User
	}
	if src.SSH.KeyPath != "" {
		dst.SSH.KeyPath = src.SSH.KeyPath
	}
	if src.SSH.StrictHostKeyChecking != "" {
		dst.SSH.StrictHostKeyChecking = src.SSH.StrictHostKeyChecking
	}

	// Experimental (bool - always merge)
	dst.Experimental = src.Experimental
}

// applyEnvOverrides applies environment variable overrides to the config
func applyEnvOverrides(cfg *Config) {
	// Data directory
	if v := os.Getenv(EnvDataDir); v != "" {
		cfg.Paths.Data = v
	}

	// Podman path
	if v := os.Getenv(EnvPodmanPath); v != "" {
		cfg.Runtime.Podman = v
	}

	// Registry port
	if v := os.Getenv(EnvRegistryPort); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Registry.Port = port
		}
	}

	// CI port
	if v := os.Getenv(EnvCIPort); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.CI.Port = port
		}
	}

	// GUI port
	if v := os.Getenv(EnvGUIPort); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.GUI.Port = port
		}
	}

	// bootc-image-builder image
	if v := os.Getenv(EnvBootcImageBuilder); v != "" {
		cfg.CI.BootcImageBuilder = v
	}

	// Experimental mode
	if v := os.Getenv(EnvExperimental); v == "1" || v == "true" {
		cfg.Experimental = true
	}
}

// Save writes the configuration to a file
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Add header comment
	header := []byte(`# bootc-man configuration file
# See documentation for available options
#
# Configuration is loaded in the following order (later overrides earlier):
# 1. /usr/share/bootc-man/config.yaml (system default)
# 2. /etc/bootc-man/config.yaml (system admin)
# 3. ~/.config/bootc-man/config.yaml (user)
# 4. Environment variables (BOOTCMAN_*)
# 5. Command-line flags
#
`)
	data = append(header, data...)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	var errs []string

	if c.Registry.Port < 1 || c.Registry.Port > 65535 {
		errs = append(errs, fmt.Sprintf("invalid registry port: %d", c.Registry.Port))
	}

	if c.CI.Port < 1 || c.CI.Port > 65535 {
		errs = append(errs, fmt.Sprintf("invalid CI port: %d", c.CI.Port))
	}

	if c.GUI.Port < 1 || c.GUI.Port > 65535 {
		errs = append(errs, fmt.Sprintf("invalid GUI port: %d", c.GUI.Port))
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// UserConfigPath returns the path to the user's config file
func UserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "bootc-man", "config.yaml"), nil
}

// DataDir returns the data directory path, expanding ~ if needed
func (c *Config) DataDir() string {
	if strings.HasPrefix(c.Paths.Data, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, c.Paths.Data[1:])
		}
	}
	return c.Paths.Data
}
