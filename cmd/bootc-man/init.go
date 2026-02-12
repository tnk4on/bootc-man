package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tnk4on/bootc-man/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize bootc-man configuration",
	Long: `Initialize bootc-man configuration and data directories.

This command will:
  - Create the configuration directory (~/.config/bootc-man/)
  - Create the data directory (~/.local/share/bootc-man/)
  - Generate a default configuration file
  - Check for required dependencies:
      All: podman
      macOS: vfkit, gvproxy (for CI test stage)
      Linux: qemu-kvm, gvproxy (for CI test stage)
  - Optionally create a sample pipeline (Fedora, CentOS Stream, or RHEL)
  - Optionally start the local registry`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println("Initializing bootc-man...")

	// Get home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create config directory
	configDir := filepath.Join(home, ".config", "bootc-man")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	logrus.Debugf("Created config directory: %s", configDir)

	// Create data directory
	dataDir := filepath.Join(home, ".local", "share", "bootc-man")
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}
	logrus.Debugf("Created data directory: %s", dataDir)

	// Create CI pipelines directory
	pipelinesDir := filepath.Join(dataDir, "ci", "pipelines")
	if err := os.MkdirAll(pipelinesDir, 0755); err != nil {
		return fmt.Errorf("failed to create pipelines directory: %w", err)
	}
	logrus.Debugf("Created pipelines directory: %s", pipelinesDir)

	// Create default config
	configPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		cfg := config.DefaultConfig()
		cfg.Paths.Data = dataDir
		if err := cfg.Save(configPath); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		fmt.Printf("Created configuration file: %s\n", configPath)
	} else {
		fmt.Printf("Configuration file already exists: %s\n", configPath)
	}

	// Check dependencies (platform-specific: macOS needs vfkit/gvproxy for CI test stage)
	fmt.Println("\nChecking dependencies...")
	platformName := runtime.GOOS
	if platformName == "darwin" {
		platformName = "macOS"
	}
	fmt.Printf("  Platform: %s\n", platformName)

	var missingDeps []string
	if !checkDependencyWithInstall("podman") {
		missingDeps = append(missingDeps, "podman")
	}
	if runtime.GOOS == "darwin" {
		// macOS: vfkit and gvproxy for CI test stage
		if !checkDependencyWithInstall(config.BinaryVfkit) {
			missingDeps = append(missingDeps, config.BinaryVfkit)
		}
		if !checkDependencyWithInstall(config.BinaryGvproxy) {
			missingDeps = append(missingDeps, config.BinaryGvproxy)
		}
	} else if runtime.GOOS == "linux" {
		// Linux: qemu-kvm and gvproxy for CI test stage
		// RHEL/Fedora: /usr/libexec/qemu-kvm, Debian/Ubuntu: /usr/bin/qemu-system-x86_64
		if !checkDependencyWithInstall("qemu-kvm") {
			missingDeps = append(missingDeps, "qemu-kvm")
		}
		if !checkDependencyWithInstall(config.BinaryGvproxy) {
			missingDeps = append(missingDeps, config.BinaryGvproxy)
		}
	}

	// Show missing dependencies summary
	if len(missingDeps) > 0 {
		fmt.Println()
		fmt.Println("⚠️  Missing dependencies detected. Install them to use all features.")
	}

	// Print warning
	fmt.Println()
	printWarning()

	// Create sample pipeline in current directory (Fedora / CentOS Stream / RHEL / None)
	sampleDir, err := runSamplePrompt()
	if err != nil {
		return err
	}

	// Optionally start the registry
	if err := runRegistryPrompt(configPath); err != nil {
		return err
	}

	fmt.Println("\n✓ bootc-man initialized successfully!")
	fmt.Printf("  Config: %s\n", configPath)
	fmt.Printf("  Data:   %s\n", dataDir)

	printNextSteps(sampleDir)

	return nil
}

// checkDependencyWithInstall checks if a dependency exists and shows install instructions if missing.
// Returns true if found, false if missing.
func checkDependencyWithInstall(name string) bool {
	path, err := findBinary(name)
	if err == nil {
		fmt.Printf("  ✓ %s: %s\n", name, path)
		return true
	}

	// Show not found with install instructions
	fmt.Printf("  ✗ %s: not found\n", name)
	showInstallInstructions(name)
	return false
}

// showInstallInstructions displays platform-specific install commands for the given tool.
func showInstallInstructions(name string) {
	switch runtime.GOOS {
	case "darwin":
		switch name {
		case "podman":
			fmt.Println("    → brew install podman")
		case config.BinaryVfkit:
			fmt.Println("    → brew install vfkit")
		case config.BinaryGvproxy:
			fmt.Println("    → brew install podman  (includes gvproxy)")
		}
	case "linux":
		switch name {
		case "podman":
			fmt.Println("    → dnf install podman  (or apt install podman)")
		case "qemu-kvm":
			fmt.Println("    → dnf install qemu-kvm  (or apt install qemu-kvm)")
		case config.BinaryGvproxy:
			fmt.Println("    → dnf install gvisor-tap-vsock")
		}
	default:
		// No specific instructions for other platforms
	}
}

func findBinary(name string) (string, error) {
	// Check common paths (including /usr/libexec for RHEL/Fedora)
	paths := []string{
		"/usr/bin/" + name,
		"/usr/local/bin/" + name,
		"/usr/sbin/" + name,
		"/usr/libexec/" + name,
		"/usr/libexec/podman/" + name, // gvproxy on RHEL/Fedora
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Try PATH
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("binary not found: %s", name)
}

func printWarning() {
	fmt.Println("⚠️  WARNING: bootc-man is designed for testing/development only.")
}

// printNextSteps displays next-step instructions after init.
// If sampleDir is non-empty, a sample pipeline was created in that directory.
func printNextSteps(sampleDir string) {
	fmt.Println("\nNext steps:")
	if sampleDir != "" {
		fmt.Printf("  cd %s\n", sampleDir)
		fmt.Println()
		fmt.Println("  # Validate pipeline definition")
		fmt.Println("  bootc-man ci check")
		fmt.Println()
		fmt.Println("  # Build image and run CI pipeline")
		fmt.Println("  bootc-man ci run")
		fmt.Println()
		fmt.Println("  # Boot the image as a VM and verify via SSH")
		fmt.Println("  bootc-man vm start")
		fmt.Println("  bootc-man vm ssh")
	} else {
		fmt.Println("  1. Create a Containerfile for your bootc image")
		fmt.Println("  2. Create a bootc-ci.yaml pipeline definition")
		fmt.Println()
		fmt.Println("  # Validate pipeline definition")
		fmt.Println("  bootc-man ci check")
		fmt.Println()
		fmt.Println("  # Build image and run CI pipeline")
		fmt.Println("  bootc-man ci run")
		fmt.Println()
		fmt.Println("  # Boot the image as a VM and verify via SSH")
		fmt.Println("  bootc-man vm start")
		fmt.Println("  bootc-man vm ssh")
	}
}
