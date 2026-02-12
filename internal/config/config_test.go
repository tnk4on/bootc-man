package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	// Check default values
	if cfg.Runtime.Podman != "auto" {
		t.Errorf("expected Podman='auto', got %q", cfg.Runtime.Podman)
	}

	if cfg.Registry.Port != 5000 {
		t.Errorf("expected Registry.Port=5000, got %d", cfg.Registry.Port)
	}

	if cfg.Registry.Image != "docker.io/library/registry:2" {
		t.Errorf("expected Registry.Image='docker.io/library/registry:2', got %q", cfg.Registry.Image)
	}

	if cfg.CI.Port != 8080 {
		t.Errorf("expected CI.Port=8080, got %d", cfg.CI.Port)
	}

	if cfg.CI.BootcImageBuilder != DefaultBootcImageBuilder {
		t.Errorf("expected CI.BootcImageBuilder=%q, got %q", DefaultBootcImageBuilder, cfg.CI.BootcImageBuilder)
	}

	if cfg.GUI.Port != 3000 {
		t.Errorf("expected GUI.Port=3000, got %d", cfg.GUI.Port)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name:    "invalid registry port (0)",
			modify:  func(c *Config) { c.Registry.Port = 0 },
			wantErr: true,
		},
		{
			name:    "invalid registry port (too high)",
			modify:  func(c *Config) { c.Registry.Port = 70000 },
			wantErr: true,
		},
		{
			name:    "invalid CI port",
			modify:  func(c *Config) { c.CI.Port = -1 },
			wantErr: true,
		},
		{
			name:    "invalid GUI port",
			modify:  func(c *Config) { c.GUI.Port = 100000 },
			wantErr: true,
		},
		{
			name:    "valid custom ports",
			modify:  func(c *Config) { c.Registry.Port = 5001; c.CI.Port = 9000; c.GUI.Port = 8080 },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
runtime:
  podman: /usr/local/bin/podman
paths:
  data: /custom/data/path
registry:
  port: 5001
  image: custom/registry:v2
ci:
  port: 9000
  remote: ssh://buildhost
  bootc_image_builder: custom/bootc-image-builder:v1
gui:
  port: 4000
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Runtime.Podman != "/usr/local/bin/podman" {
		t.Errorf("expected Podman='/usr/local/bin/podman', got %q", cfg.Runtime.Podman)
	}

	if cfg.Paths.Data != "/custom/data/path" {
		t.Errorf("expected Paths.Data='/custom/data/path', got %q", cfg.Paths.Data)
	}

	if cfg.Registry.Port != 5001 {
		t.Errorf("expected Registry.Port=5001, got %d", cfg.Registry.Port)
	}

	if cfg.Registry.Image != "custom/registry:v2" {
		t.Errorf("expected Registry.Image='custom/registry:v2', got %q", cfg.Registry.Image)
	}

	if cfg.CI.Port != 9000 {
		t.Errorf("expected CI.Port=9000, got %d", cfg.CI.Port)
	}

	if cfg.CI.Remote != "ssh://buildhost" {
		t.Errorf("expected CI.Remote='ssh://buildhost', got %q", cfg.CI.Remote)
	}

	if cfg.CI.BootcImageBuilder != "custom/bootc-image-builder:v1" {
		t.Errorf("expected CI.BootcImageBuilder='custom/bootc-image-builder:v1', got %q", cfg.CI.BootcImageBuilder)
	}

	if cfg.GUI.Port != 4000 {
		t.Errorf("expected GUI.Port=4000, got %d", cfg.GUI.Port)
	}
}

func TestLoadPartialConfig(t *testing.T) {
	// Test that partial config files merge with defaults
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial.yaml")

	// Only specify registry port
	configContent := `
registry:
  port: 6000
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Registry port should be from file
	if cfg.Registry.Port != 6000 {
		t.Errorf("expected Registry.Port=6000, got %d", cfg.Registry.Port)
	}

	// Other values should be defaults
	if cfg.Runtime.Podman != "auto" {
		t.Errorf("expected Podman='auto', got %q", cfg.Runtime.Podman)
	}

	if cfg.CI.Port != 8080 {
		t.Errorf("expected CI.Port=8080 (default), got %d", cfg.CI.Port)
	}
}

func TestEnvOverrides(t *testing.T) {
	// Save original env vars
	origDataDir := os.Getenv(EnvDataDir)
	origRegistryPort := os.Getenv(EnvRegistryPort)
	origCIPort := os.Getenv(EnvCIPort)
	origGUIPort := os.Getenv(EnvGUIPort)
	origPodman := os.Getenv(EnvPodmanPath)
	origBootcImageBuilder := os.Getenv(EnvBootcImageBuilder)

	// Restore env vars after test
	defer func() {
		os.Setenv(EnvDataDir, origDataDir)
		os.Setenv(EnvRegistryPort, origRegistryPort)
		os.Setenv(EnvCIPort, origCIPort)
		os.Setenv(EnvGUIPort, origGUIPort)
		os.Setenv(EnvPodmanPath, origPodman)
		os.Setenv(EnvBootcImageBuilder, origBootcImageBuilder)
	}()

	// Set env vars
	os.Setenv(EnvDataDir, "/env/data/dir")
	os.Setenv(EnvRegistryPort, "5555")
	os.Setenv(EnvCIPort, "9999")
	os.Setenv(EnvGUIPort, "4444")
	os.Setenv(EnvPodmanPath, "/custom/podman")
	os.Setenv(EnvBootcImageBuilder, "env/bootc-image-builder:latest")

	// Create minimal config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("registry:\n  port: 5001\n"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Env vars should override file and defaults
	if cfg.Paths.Data != "/env/data/dir" {
		t.Errorf("expected Paths.Data='/env/data/dir', got %q", cfg.Paths.Data)
	}

	if cfg.Registry.Port != 5555 {
		t.Errorf("expected Registry.Port=5555 (env override), got %d", cfg.Registry.Port)
	}

	if cfg.CI.Port != 9999 {
		t.Errorf("expected CI.Port=9999 (env override), got %d", cfg.CI.Port)
	}

	if cfg.GUI.Port != 4444 {
		t.Errorf("expected GUI.Port=4444 (env override), got %d", cfg.GUI.Port)
	}

	if cfg.Runtime.Podman != "/custom/podman" {
		t.Errorf("expected Podman='/custom/podman', got %q", cfg.Runtime.Podman)
	}

	if cfg.CI.BootcImageBuilder != "env/bootc-image-builder:latest" {
		t.Errorf("expected CI.BootcImageBuilder='env/bootc-image-builder:latest' (env override), got %q", cfg.CI.BootcImageBuilder)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "saved-config.yaml")

	// Create a custom config
	cfg := &Config{
		Runtime: RuntimeConfig{
			Podman: "/opt/podman/bin/podman",
		},
		Paths: PathsConfig{
			Data: "/var/lib/bootc-man",
		},
		Registry: RegistryConfig{
			Port:  5002,
			Image: "myregistry:latest",
		},
		CI: CIConfig{
			Port:              7000,
			Remote:            "ssh://remote-builder",
			BootcImageBuilder: "registry.redhat.io/rhel9/bootc-image-builder",
		},
		GUI: GUIConfig{
			Port: 8000,
		},
	}

	// Save it
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Load it back
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Compare values
	if loaded.Runtime.Podman != cfg.Runtime.Podman {
		t.Errorf("Podman mismatch: got %q, want %q", loaded.Runtime.Podman, cfg.Runtime.Podman)
	}

	if loaded.Paths.Data != cfg.Paths.Data {
		t.Errorf("Paths.Data mismatch: got %q, want %q", loaded.Paths.Data, cfg.Paths.Data)
	}

	if loaded.Registry.Port != cfg.Registry.Port {
		t.Errorf("Registry.Port mismatch: got %d, want %d", loaded.Registry.Port, cfg.Registry.Port)
	}

	if loaded.Registry.Image != cfg.Registry.Image {
		t.Errorf("Registry.Image mismatch: got %q, want %q", loaded.Registry.Image, cfg.Registry.Image)
	}

	if loaded.CI.Port != cfg.CI.Port {
		t.Errorf("CI.Port mismatch: got %d, want %d", loaded.CI.Port, cfg.CI.Port)
	}

	if loaded.CI.Remote != cfg.CI.Remote {
		t.Errorf("CI.Remote mismatch: got %q, want %q", loaded.CI.Remote, cfg.CI.Remote)
	}

	if loaded.CI.BootcImageBuilder != cfg.CI.BootcImageBuilder {
		t.Errorf("CI.BootcImageBuilder mismatch: got %q, want %q", loaded.CI.BootcImageBuilder, cfg.CI.BootcImageBuilder)
	}

	if loaded.GUI.Port != cfg.GUI.Port {
		t.Errorf("GUI.Port mismatch: got %d, want %d", loaded.GUI.Port, cfg.GUI.Port)
	}
}

func TestDataDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot get home dir: %v", err)
	}

	tests := []struct {
		name     string
		dataPath string
		want     string
	}{
		{
			name:     "absolute path",
			dataPath: "/var/lib/bootc-man",
			want:     "/var/lib/bootc-man",
		},
		{
			name:     "tilde path",
			dataPath: "~/.local/share/bootc-man",
			want:     filepath.Join(home, ".local/share/bootc-man"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Paths: PathsConfig{
					Data: tt.dataPath,
				},
			}
			got := cfg.DataDir()
			if got != tt.want {
				t.Errorf("DataDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUserConfigPath(t *testing.T) {
	path, err := UserConfigPath()
	if err != nil {
		t.Fatalf("UserConfigPath() failed: %v", err)
	}

	if path == "" {
		t.Error("UserConfigPath() returned empty string")
	}

	// Should contain .config/bootc-man
	if !filepath.IsAbs(path) {
		t.Error("UserConfigPath() should return absolute path")
	}

	// Should end with config.yaml
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("UserConfigPath() should end with config.yaml, got %q", filepath.Base(path))
	}
}

func TestMergeConfig(t *testing.T) {
	dst := DefaultConfig()
	src := &Config{
		Runtime: RuntimeConfig{
			Podman: "/new/podman",
		},
		Registry: RegistryConfig{
			Port: 6000,
			// Image is empty, should not override
		},
		CI: CIConfig{
			BootcImageBuilder: "custom/bootc-image-builder:test",
		},
	}

	mergeConfig(dst, src)

	if dst.Runtime.Podman != "/new/podman" {
		t.Errorf("expected Podman='/new/podman', got %q", dst.Runtime.Podman)
	}

	if dst.Registry.Port != 6000 {
		t.Errorf("expected Registry.Port=6000, got %d", dst.Registry.Port)
	}

	// Image should still be default since src.Registry.Image was empty
	if dst.Registry.Image != "docker.io/library/registry:2" {
		t.Errorf("expected Registry.Image to be default, got %q", dst.Registry.Image)
	}

	// BootcImageBuilder should be overridden
	if dst.CI.BootcImageBuilder != "custom/bootc-image-builder:test" {
		t.Errorf("expected CI.BootcImageBuilder='custom/bootc-image-builder:test', got %q", dst.CI.BootcImageBuilder)
	}
}

// TestMergeConfigAllSections tests merging all config sections
func TestMergeConfigAllSections(t *testing.T) {
	dst := DefaultConfig()
	src := &Config{
		Runtime: RuntimeConfig{
			Podman: "/merged/podman",
		},
		Paths: PathsConfig{
			Data: "/merged/data",
		},
		Registry: RegistryConfig{
			Port:  7000,
			Image: "merged/registry:v3",
		},
		CI: CIConfig{
			Remote:            "ssh://merged-host",
			Port:              7001,
			BootcImageBuilder: "merged/bib:v1",
		},
		GUI: GUIConfig{
			Port: 7002,
		},
		VM: VMConfig{
			SSHUser: "mergeduser",
			CPUs:    8,
			Memory:  16384,
		},
		Containers: ContainersConfig{
			RegistryName:       "merged-registry",
			CIName:             "merged-ci",
			GUIName:            "merged-gui",
			RegistryDataVolume: "merged-registry-data",
			TrivyCacheVolume:   "merged-trivy-cache",
			GrypeCacheVolume:   "merged-grype-cache",
		},
		Images: ImagesConfig{
			Hadolint:   "merged/hadolint:v1",
			Trivy:      "merged/trivy:v1",
			Grype:      "merged/grype:v1",
			Syft:       "merged/syft:v1",
			Skopeo:     "merged/skopeo:v1",
			Gitleaks:   "merged/gitleaks:v1",
			Trufflehog: "merged/trufflehog:v1",
		},
		Network: NetworkConfig{
			VMIP:           "10.0.0.1",
			GatewayIP:      "10.0.0.254",
			SSHForwardPort: 3333,
			VfkitAPIPort:   54321,
		},
		Timeouts: TimeoutsConfig{
			VMBoot:     120,
			SSHConnect: 30,
			SSHRetry:   45,
			HTTPClient: 15,
			Socket:     25,
		},
		SSH: SSHConfig{
			User:                  "sshmerged",
			KeyPath:               ".ssh/merged_key",
			StrictHostKeyChecking: "no",
		},
		Experimental: true,
	}

	mergeConfig(dst, src)

	// Verify all sections were merged
	if dst.Runtime.Podman != "/merged/podman" {
		t.Errorf("Runtime.Podman = %q, want %q", dst.Runtime.Podman, "/merged/podman")
	}
	if dst.Paths.Data != "/merged/data" {
		t.Errorf("Paths.Data = %q, want %q", dst.Paths.Data, "/merged/data")
	}
	if dst.Registry.Port != 7000 {
		t.Errorf("Registry.Port = %d, want %d", dst.Registry.Port, 7000)
	}
	if dst.Registry.Image != "merged/registry:v3" {
		t.Errorf("Registry.Image = %q, want %q", dst.Registry.Image, "merged/registry:v3")
	}
	if dst.CI.Remote != "ssh://merged-host" {
		t.Errorf("CI.Remote = %q, want %q", dst.CI.Remote, "ssh://merged-host")
	}
	if dst.CI.Port != 7001 {
		t.Errorf("CI.Port = %d, want %d", dst.CI.Port, 7001)
	}
	if dst.GUI.Port != 7002 {
		t.Errorf("GUI.Port = %d, want %d", dst.GUI.Port, 7002)
	}
	if dst.VM.SSHUser != "mergeduser" {
		t.Errorf("VM.SSHUser = %q, want %q", dst.VM.SSHUser, "mergeduser")
	}
	if dst.VM.CPUs != 8 {
		t.Errorf("VM.CPUs = %d, want %d", dst.VM.CPUs, 8)
	}
	if dst.VM.Memory != 16384 {
		t.Errorf("VM.Memory = %d, want %d", dst.VM.Memory, 16384)
	}
	if dst.Containers.RegistryName != "merged-registry" {
		t.Errorf("Containers.RegistryName = %q, want %q", dst.Containers.RegistryName, "merged-registry")
	}
	if dst.Containers.TrivyCacheVolume != "merged-trivy-cache" {
		t.Errorf("Containers.TrivyCacheVolume = %q, want %q", dst.Containers.TrivyCacheVolume, "merged-trivy-cache")
	}
	if dst.Images.Hadolint != "merged/hadolint:v1" {
		t.Errorf("Images.Hadolint = %q, want %q", dst.Images.Hadolint, "merged/hadolint:v1")
	}
	if dst.Images.Trufflehog != "merged/trufflehog:v1" {
		t.Errorf("Images.Trufflehog = %q, want %q", dst.Images.Trufflehog, "merged/trufflehog:v1")
	}
	if dst.Network.VMIP != "10.0.0.1" {
		t.Errorf("Network.VMIP = %q, want %q", dst.Network.VMIP, "10.0.0.1")
	}
	if dst.Network.VfkitAPIPort != 54321 {
		t.Errorf("Network.VfkitAPIPort = %d, want %d", dst.Network.VfkitAPIPort, 54321)
	}
	if dst.Timeouts.VMBoot != 120 {
		t.Errorf("Timeouts.VMBoot = %d, want %d", dst.Timeouts.VMBoot, 120)
	}
	if dst.Timeouts.Socket != 25 {
		t.Errorf("Timeouts.Socket = %d, want %d", dst.Timeouts.Socket, 25)
	}
	if dst.SSH.User != "sshmerged" {
		t.Errorf("SSH.User = %q, want %q", dst.SSH.User, "sshmerged")
	}
	if dst.SSH.StrictHostKeyChecking != "no" {
		t.Errorf("SSH.StrictHostKeyChecking = %q, want %q", dst.SSH.StrictHostKeyChecking, "no")
	}
	if !dst.Experimental {
		t.Error("Experimental = false, want true")
	}
}

// TestLoadWithEnvConfig tests loading config via BOOTCMAN_CONFIG environment variable
func TestLoadWithEnvConfig(t *testing.T) {
	// Save original env var
	origEnvConfig := os.Getenv(EnvConfig)
	defer func() {
		if origEnvConfig == "" {
			os.Unsetenv(EnvConfig)
		} else {
			os.Setenv(EnvConfig, origEnvConfig)
		}
	}()

	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "env-config.yaml")
	configContent := `
registry:
  port: 7777
ci:
  port: 8888
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Set environment variable
	os.Setenv(EnvConfig, configPath)

	// Load without explicit path (should use env var)
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Registry.Port != 7777 {
		t.Errorf("Registry.Port = %d, want %d", cfg.Registry.Port, 7777)
	}
	if cfg.CI.Port != 8888 {
		t.Errorf("CI.Port = %d, want %d", cfg.CI.Port, 8888)
	}
}

// TestLoadInvalidYAML tests loading an invalid YAML file
func TestLoadInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `
registry:
  port: [invalid yaml
  not closed
`
	if err := os.WriteFile(configPath, []byte(invalidContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load() should fail for invalid YAML")
	}
	if !strings.Contains(err.Error(), "failed to parse config file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestLoadNonexistentFile tests loading a nonexistent file
func TestLoadNonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() should fail for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to read config file") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestConfigPaths tests the configPaths function
func TestConfigPaths(t *testing.T) {
	paths := configPaths()

	if len(paths) < 2 {
		t.Errorf("configPaths() should return at least 2 paths, got %d", len(paths))
	}

	// First path should be system default
	if paths[0] != "/usr/share/bootc-man/config.yaml" {
		t.Errorf("paths[0] = %q, want %q", paths[0], "/usr/share/bootc-man/config.yaml")
	}

	// Second path should be system admin
	if paths[1] != "/etc/bootc-man/config.yaml" {
		t.Errorf("paths[1] = %q, want %q", paths[1], "/etc/bootc-man/config.yaml")
	}

	// If home dir is available, should have user config path
	if home, err := os.UserHomeDir(); err == nil {
		expectedUserPath := filepath.Join(home, ".config", "bootc-man", "config.yaml")
		found := false
		for _, p := range paths {
			if p == expectedUserPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("configPaths() should include user config path %q", expectedUserPath)
		}
	}
}

// TestEnvOverridesInvalidPort tests environment variable overrides with invalid port values
func TestEnvOverridesInvalidPort(t *testing.T) {
	// Save original env vars
	origRegistryPort := os.Getenv(EnvRegistryPort)
	origCIPort := os.Getenv(EnvCIPort)
	origGUIPort := os.Getenv(EnvGUIPort)

	defer func() {
		os.Setenv(EnvRegistryPort, origRegistryPort)
		os.Setenv(EnvCIPort, origCIPort)
		os.Setenv(EnvGUIPort, origGUIPort)
	}()

	// Set invalid port values
	os.Setenv(EnvRegistryPort, "invalid")
	os.Setenv(EnvCIPort, "not-a-number")
	os.Setenv(EnvGUIPort, "abc123")

	cfg := DefaultConfig()
	applyEnvOverrides(cfg)

	// Ports should remain at default values (invalid env vars are ignored)
	if cfg.Registry.Port != DefaultRegistryPort {
		t.Errorf("Registry.Port = %d, want %d (invalid env should be ignored)", cfg.Registry.Port, DefaultRegistryPort)
	}
	if cfg.CI.Port != DefaultCIPort {
		t.Errorf("CI.Port = %d, want %d (invalid env should be ignored)", cfg.CI.Port, DefaultCIPort)
	}
	if cfg.GUI.Port != DefaultGUIPort {
		t.Errorf("GUI.Port = %d, want %d (invalid env should be ignored)", cfg.GUI.Port, DefaultGUIPort)
	}
}

// TestLoadWithoutConfigFiles tests loading when no config files exist
func TestLoadWithoutConfigFiles(t *testing.T) {
	// Clear env vars that might point to config files
	origEnvConfig := os.Getenv(EnvConfig)
	defer func() {
		if origEnvConfig == "" {
			os.Unsetenv(EnvConfig)
		} else {
			os.Setenv(EnvConfig, origEnvConfig)
		}
	}()
	os.Unsetenv(EnvConfig)

	// Load without explicit path (will use defaults if no files exist)
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should return default config
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
	if cfg.Runtime.Podman != "auto" {
		t.Errorf("Runtime.Podman = %q, want %q", cfg.Runtime.Podman, "auto")
	}
}

// TestDefaultConfigContainerNames tests container names in default config
func TestDefaultConfigContainerNames(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Containers.RegistryName != ContainerNameRegistry {
		t.Errorf("Containers.RegistryName = %q, want %q", cfg.Containers.RegistryName, ContainerNameRegistry)
	}
	if cfg.Containers.CIName != ContainerNameCI {
		t.Errorf("Containers.CIName = %q, want %q", cfg.Containers.CIName, ContainerNameCI)
	}
	if cfg.Containers.GUIName != ContainerNameGUI {
		t.Errorf("Containers.GUIName = %q, want %q", cfg.Containers.GUIName, ContainerNameGUI)
	}
	if cfg.Containers.RegistryDataVolume != VolumeNameRegistryData {
		t.Errorf("Containers.RegistryDataVolume = %q, want %q", cfg.Containers.RegistryDataVolume, VolumeNameRegistryData)
	}
}

// TestDefaultConfigImages tests image defaults
func TestDefaultConfigImages(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Images.Hadolint != DefaultHadolintImage {
		t.Errorf("Images.Hadolint = %q, want %q", cfg.Images.Hadolint, DefaultHadolintImage)
	}
	if cfg.Images.Trivy != DefaultTrivyImage {
		t.Errorf("Images.Trivy = %q, want %q", cfg.Images.Trivy, DefaultTrivyImage)
	}
	if cfg.Images.Grype != DefaultGrypeImage {
		t.Errorf("Images.Grype = %q, want %q", cfg.Images.Grype, DefaultGrypeImage)
	}
	if cfg.Images.Syft != DefaultSyftImage {
		t.Errorf("Images.Syft = %q, want %q", cfg.Images.Syft, DefaultSyftImage)
	}
	if cfg.Images.Skopeo != DefaultSkopeoImage {
		t.Errorf("Images.Skopeo = %q, want %q", cfg.Images.Skopeo, DefaultSkopeoImage)
	}
	if cfg.Images.Gitleaks != DefaultGitleaksImage {
		t.Errorf("Images.Gitleaks = %q, want %q", cfg.Images.Gitleaks, DefaultGitleaksImage)
	}
	if cfg.Images.Trufflehog != DefaultTrufflehogImage {
		t.Errorf("Images.Trufflehog = %q, want %q", cfg.Images.Trufflehog, DefaultTrufflehogImage)
	}
}

// TestDefaultConfigNetwork tests network defaults
func TestDefaultConfigNetwork(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Network.VMIP != DefaultVMIP {
		t.Errorf("Network.VMIP = %q, want %q", cfg.Network.VMIP, DefaultVMIP)
	}
	if cfg.Network.GatewayIP != DefaultGatewayIP {
		t.Errorf("Network.GatewayIP = %q, want %q", cfg.Network.GatewayIP, DefaultGatewayIP)
	}
	if cfg.Network.SSHForwardPort != DefaultSSHForwardPort {
		t.Errorf("Network.SSHForwardPort = %d, want %d", cfg.Network.SSHForwardPort, DefaultSSHForwardPort)
	}
	if cfg.Network.VfkitAPIPort != DefaultVfkitAPIPort {
		t.Errorf("Network.VfkitAPIPort = %d, want %d", cfg.Network.VfkitAPIPort, DefaultVfkitAPIPort)
	}
}

// TestDefaultConfigTimeouts tests timeout defaults
func TestDefaultConfigTimeouts(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeouts.VMBoot != int(DefaultVMBootTimeout.Seconds()) {
		t.Errorf("Timeouts.VMBoot = %d, want %d", cfg.Timeouts.VMBoot, int(DefaultVMBootTimeout.Seconds()))
	}
	if cfg.Timeouts.SSHConnect != int(DefaultSSHConnectTimeout.Seconds()) {
		t.Errorf("Timeouts.SSHConnect = %d, want %d", cfg.Timeouts.SSHConnect, int(DefaultSSHConnectTimeout.Seconds()))
	}
	if cfg.Timeouts.SSHRetry != int(DefaultSSHRetryTimeout.Seconds()) {
		t.Errorf("Timeouts.SSHRetry = %d, want %d", cfg.Timeouts.SSHRetry, int(DefaultSSHRetryTimeout.Seconds()))
	}
	if cfg.Timeouts.HTTPClient != int(DefaultHTTPClientTimeout.Seconds()) {
		t.Errorf("Timeouts.HTTPClient = %d, want %d", cfg.Timeouts.HTTPClient, int(DefaultHTTPClientTimeout.Seconds()))
	}
	if cfg.Timeouts.Socket != int(DefaultSocketTimeout.Seconds()) {
		t.Errorf("Timeouts.Socket = %d, want %d", cfg.Timeouts.Socket, int(DefaultSocketTimeout.Seconds()))
	}
}

// TestDefaultConfigSSH tests SSH defaults
func TestDefaultConfigSSH(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.SSH.User != DefaultSSHUser {
		t.Errorf("SSH.User = %q, want %q", cfg.SSH.User, DefaultSSHUser)
	}
	if cfg.SSH.KeyPath != DefaultSSHKeyPath {
		t.Errorf("SSH.KeyPath = %q, want %q", cfg.SSH.KeyPath, DefaultSSHKeyPath)
	}
	if cfg.SSH.StrictHostKeyChecking != "accept-new" {
		t.Errorf("SSH.StrictHostKeyChecking = %q, want %q", cfg.SSH.StrictHostKeyChecking, "accept-new")
	}
}

// TestDefaultConfigVM tests VM defaults
func TestDefaultConfigVM(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.VM.SSHUser != DefaultSSHUser {
		t.Errorf("VM.SSHUser = %q, want %q", cfg.VM.SSHUser, DefaultSSHUser)
	}
	if cfg.VM.CPUs != DefaultVMCPUs {
		t.Errorf("VM.CPUs = %d, want %d", cfg.VM.CPUs, DefaultVMCPUs)
	}
	if cfg.VM.Memory != DefaultVMMemoryMB {
		t.Errorf("VM.Memory = %d, want %d", cfg.VM.Memory, DefaultVMMemoryMB)
	}
}

// TestValidateMultipleErrors tests that Validate returns all errors
func TestValidateMultipleErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Registry.Port = 0
	cfg.CI.Port = -1
	cfg.GUI.Port = 100000

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() should return error for multiple invalid ports")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "registry port") {
		t.Error("error should mention registry port")
	}
	if !strings.Contains(errStr, "CI port") {
		t.Error("error should mention CI port")
	}
	if !strings.Contains(errStr, "GUI port") {
		t.Error("error should mention GUI port")
	}
}

// TestSaveCreatesDirectory tests that Save creates the directory if it doesn't exist
func TestSaveCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "deeply", "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(nestedPath); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify directory was created
	dir := filepath.Dir(nestedPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Save() should create parent directories")
	}

	// Verify file was created
	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Error("Save() should create config file")
	}
}

// TestSaveIncludesHeader tests that saved config includes header comment
func TestSaveIncludesHeader(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	cfg := DefaultConfig()
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "# bootc-man configuration file") {
		t.Error("saved config should include header comment")
	}
}

// TestLoadFromFullConfig tests loading a config with all sections
func TestLoadFromFullConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "full-config.yaml")

	configContent := `
runtime:
  podman: /full/podman
paths:
  data: /full/data
registry:
  port: 5500
  image: full/registry:latest
ci:
  remote: ssh://full-remote
  port: 8500
  bootc_image_builder: full/bib:latest
gui:
  port: 3500
vm:
  ssh_user: fulluser
  cpus: 4
  memory: 8192
containers:
  registry_name: full-registry
  ci_name: full-ci
  gui_name: full-gui
  registry_data_volume: full-registry-data
  trivy_cache_volume: full-trivy-cache
  grype_cache_volume: full-grype-cache
images:
  hadolint: full/hadolint:v1
  trivy: full/trivy:v1
  grype: full/grype:v1
  syft: full/syft:v1
  skopeo: full/skopeo:v1
  gitleaks: full/gitleaks:v1
  trufflehog: full/trufflehog:v1
network:
  vm_ip: 10.1.1.1
  gateway_ip: 10.1.1.254
  ssh_forward_port: 2223
  vfkit_api_port: 12346
timeouts:
  vm_boot: 60
  ssh_connect: 20
  ssh_retry: 30
  http_client: 10
  socket: 15
ssh:
  user: sshfull
  key_path: .ssh/full_key
  strict_host_key_checking: "no"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify all sections were loaded
	if cfg.Runtime.Podman != "/full/podman" {
		t.Errorf("Runtime.Podman = %q, want %q", cfg.Runtime.Podman, "/full/podman")
	}
	if cfg.VM.CPUs != 4 {
		t.Errorf("VM.CPUs = %d, want %d", cfg.VM.CPUs, 4)
	}
	if cfg.Network.VMIP != "10.1.1.1" {
		t.Errorf("Network.VMIP = %q, want %q", cfg.Network.VMIP, "10.1.1.1")
	}
	if cfg.Timeouts.VMBoot != 60 {
		t.Errorf("Timeouts.VMBoot = %d, want %d", cfg.Timeouts.VMBoot, 60)
	}
	if cfg.SSH.User != "sshfull" {
		t.Errorf("SSH.User = %q, want %q", cfg.SSH.User, "sshfull")
	}
	if cfg.Images.Hadolint != "full/hadolint:v1" {
		t.Errorf("Images.Hadolint = %q, want %q", cfg.Images.Hadolint, "full/hadolint:v1")
	}
}
