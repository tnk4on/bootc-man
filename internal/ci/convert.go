package ci

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/podman"
)

// ConvertStage handles the convert stage execution
type ConvertStage struct {
	pipeline          *Pipeline
	podman            *podman.Client
	imageTag          string
	verbose           bool
	bootcImageBuilder string
}

// DefaultBootcImageBuilder is the default bootc-image-builder image
// Uses CentOS bootc image builder which is publicly available without authentication
// Prefer using Config.Images.BootcImageBuilder from bootc-ci.yaml for custom images
const DefaultBootcImageBuilder = "quay.io/centos-bootc/bootc-image-builder"

// NewConvertStage creates a new convert stage executor
func NewConvertStage(pipeline *Pipeline, podmanClient *podman.Client, imageTag string, verbose bool) *ConvertStage {
	return NewConvertStageWithImage(pipeline, podmanClient, imageTag, verbose, DefaultBootcImageBuilder)
}

// NewConvertStageWithImage creates a new convert stage executor with a custom bootc-image-builder image
func NewConvertStageWithImage(pipeline *Pipeline, podmanClient *podman.Client, imageTag string, verbose bool, bootcImageBuilder string) *ConvertStage {
	if bootcImageBuilder == "" {
		bootcImageBuilder = DefaultBootcImageBuilder
	}
	return &ConvertStage{
		pipeline:          pipeline,
		podman:            podmanClient,
		imageTag:          imageTag,
		verbose:           verbose,
		bootcImageBuilder: bootcImageBuilder,
	}
}

// Execute runs the convert stage
func (c *ConvertStage) Execute(ctx context.Context) error {
	if c.pipeline.Spec.Convert == nil {
		return fmt.Errorf("convert stage is not configured")
	}

	cfg := c.pipeline.Spec.Convert
	if !cfg.Enabled {
		return fmt.Errorf("convert stage is disabled")
	}

	// Note: convert stage requires bootc-image-builder which needs privileged containers
	// On macOS, this runs inside Podman Machine (Linux VM) (Windows not implemented)
	// The podman run command will execute inside the VM, so it should work
	if runtime.GOOS != "linux" {
		fmt.Printf("⚠️  Warning: convert stage on %s will run inside Podman Machine\n", runtime.GOOS)
		fmt.Println("   This requires privileged containers and may need additional configuration")
	}

	if c.imageTag == "" {
		return fmt.Errorf("image tag is required for conversion (build stage must run first)")
	}

	if len(cfg.Formats) == 0 {
		return fmt.Errorf("no conversion formats specified")
	}

	// Get images directory: <project-root>/output/images
	imagesDir := filepath.Join(c.pipeline.baseDir, "output", "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("failed to create images directory: %w", err)
	}

	fmt.Printf("📁 Output directory: %s\n", imagesDir)

	// Ensure image exists in Podman Machine (macOS only; Windows not implemented)
	// On macOS, images built on host are not available in Podman Machine
	// We need to pull or ensure the image exists in the machine
	if runtime.GOOS != "linux" {
		if err := c.ensureImageInMachine(ctx); err != nil {
			return fmt.Errorf("failed to ensure image exists in Podman Machine: %w", err)
		}
	}

	// On Linux with rootless Podman, we need to transfer the image to rootful storage
	// because bootc-image-builder requires rootful podman
	if runtime.GOOS == "linux" && c.shouldUseSudo() {
		if err := c.ensureImageInRootful(ctx); err != nil {
			return fmt.Errorf("failed to transfer image to rootful storage: %w", err)
		}
	}

	// Convert to each specified format
	for _, format := range cfg.Formats {
		if err := c.convertToFormat(ctx, format, imagesDir); err != nil {
			return fmt.Errorf("failed to convert to %s: %w", format.Type, err)
		}
	}

	return nil
}

// convertToFormat converts the image to a specific format
func (c *ConvertStage) convertToFormat(ctx context.Context, format ConvertFormat, imagesDir string) error {
	// Use bootc-image-builder container image from config
	image := c.bootcImageBuilder

	// Generate output filename from metadata.name
	// e.g., bootc-ci-test.raw, bootc-ci-test.qcow2
	pipelineName := c.pipeline.Metadata.Name
	// Sanitize name for filesystem
	pipelineName = strings.ReplaceAll(pipelineName, "/", "-")
	pipelineName = strings.ReplaceAll(pipelineName, " ", "-")
	pipelineName = strings.ToLower(pipelineName)

	// Final output path
	outputFileName := fmt.Sprintf("%s.%s", pipelineName, format.Type)
	finalOutputPath := filepath.Join(imagesDir, outputFileName)

	// bootc-image-builder outputs to a subdirectory with fixed filename (e.g., qcow2/disk.qcow2, image/disk.raw)
	// We need to use a temporary output directory and then move the file
	tempOutputDir := filepath.Join(imagesDir, ".tmp-"+pipelineName+"-"+format.Type)
	if err := os.MkdirAll(tempOutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp output directory: %w", err)
	}
	// Clean up temp directory on completion
	defer os.RemoveAll(tempOutputDir)

	// Prepare bootc-image-builder command arguments
	args := []string{"run", "--rm"}

	// bootc-image-builder requires privileged container
	args = append(args, "--privileged")

	// Security option for SELinux (required for bootc-image-builder)
	args = append(args, "--security-opt", "label=type:unconfined_t")

	// Pull newer image if available
	args = append(args, "--pull=newer")

	// Mount the container storage (not just /var/lib/containers)
	args = append(args, "-v", "/var/lib/containers/storage:/var/lib/containers/storage")

	// Mount output directory for artifacts (use temp directory)
	args = append(args, "-v", fmt.Sprintf("%s:/output", tempOutputDir))

	// Config file handling
	// bootc-image-builder requires filesystem settings via --rootfs flag.
	// Additionally, a config.toml can be provided for customizations (user, SSH keys, etc.)
	// --rootfs and --config are complementary: --rootfs sets the default filesystem type,
	// while --config provides additional customizations like user accounts and SSH keys.
	//
	// When InsecureRegistries is configured, we inject a registries.conf file into the
	// VM image using [[customizations.files]] in config.toml. This allows the VM to
	// access insecure (HTTP) registries like host.containers.internal:5000.
	hasConfigFile := false

	// Build the effective config.toml content
	var configContent string

	if format.Config != "" {
		// Read the user-specified config file
		configPath := format.Config
		if !filepath.IsAbs(configPath) {
			configPath = filepath.Join(c.pipeline.baseDir, configPath)
		}
		data, err := os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("config file not found: %s", configPath)
		}
		configContent = string(data)
	}

	// Append insecure registry config via [[customizations.files]]
	if c.pipeline.Spec.Convert != nil && len(c.pipeline.Spec.Convert.InsecureRegistries) > 0 {
		registryConf := c.generateRegistryConf(c.pipeline.Spec.Convert.InsecureRegistries)
		configContent += fmt.Sprintf("\n[[customizations.files]]\npath = \"/etc/containers/registries.conf.d/local-registry.conf\"\ndata = \"\"\"\n%s\"\"\"\n", registryConf)
		if c.verbose {
			fmt.Printf("   📋 Injecting insecure registry config for: %v\n", c.pipeline.Spec.Convert.InsecureRegistries)
		}
	}

	// Mount config.toml if we have content
	if configContent != "" {
		// Write effective config to a temp file
		effectiveConfigPath := filepath.Join(imagesDir, ".tmp-config-"+pipelineName+".toml")
		if err := os.WriteFile(effectiveConfigPath, []byte(configContent), 0644); err != nil {
			return fmt.Errorf("failed to write effective config.toml: %w", err)
		}
		defer os.Remove(effectiveConfigPath)

		args = append(args, "-v", fmt.Sprintf("%s:/config.toml:ro", effectiveConfigPath))
		hasConfigFile = true
	}

	// bootc-image-builder image
	args = append(args, image)

	// bootc-image-builder command arguments
	// Format: bootc-image-builder --type <format> --rootfs <type> [--config <config>] <image>
	// Note: flags come before the image name (positional argument)

	// Output format (--type flag)
	args = append(args, "--type", format.Type)

	// Filesystem type (always required - sets the default filesystem for partitions)
	args = append(args, "--rootfs", "ext4")

	// Config file for additional customizations (SSH keys, users, etc.)
	if hasConfigFile {
		args = append(args, "--config", "/config.toml")
	}

	// Output directory
	args = append(args, "--output", "/output")

	// Image to convert (positional argument - must be last)
	args = append(args, c.imageTag)

	// Execute podman command
	// On macOS with rootful mode, podman commands go through the rootful
	// connection automatically. On Linux, we may need sudo for rootless setups.
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" {
		// On Linux, check if we need sudo
		needSudo := c.shouldUseSudo()
		if needSudo {
			sudoArgs := []string{"podman"}
			sudoArgs = append(sudoArgs, args...)
			cmd = exec.CommandContext(ctx, "sudo", sudoArgs...)
			if c.verbose {
				fmt.Printf("Running: sudo podman %s\n", strings.Join(args, " "))
			}
		} else {
			cmd = c.podman.Command(ctx, args...)
			if c.verbose {
				fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
			}
		}
	} else {
		// On macOS, use podman directly (rootful mode handles root access)
		cmd = c.podman.Command(ctx, args...)
		if c.verbose {
			fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
		}
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bootc-image-builder failed: %w", err)
	}

	// bootc-image-builder outputs files in subdirectories with fixed names:
	// - raw: image/disk.raw
	// - qcow2: qcow2/disk.qcow2
	// - vmdk: vmdk/disk.vmdk
	// - iso: bootiso/install.iso
	// - ami: image/disk.raw (same as raw)
	var sourceFile string
	switch format.Type {
	case "raw", "ami":
		sourceFile = filepath.Join(tempOutputDir, "image", "disk.raw")
	case "qcow2":
		sourceFile = filepath.Join(tempOutputDir, "qcow2", "disk.qcow2")
	case "vmdk":
		sourceFile = filepath.Join(tempOutputDir, "vmdk", "disk.vmdk")
	case "iso":
		sourceFile = filepath.Join(tempOutputDir, "bootiso", "install.iso")
	default:
		// Try common patterns
		sourceFile = filepath.Join(tempOutputDir, format.Type, "disk."+format.Type)
	}

	// Check if source file exists
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		// Try to find the output file
		var foundFile string
		err := filepath.Walk(tempOutputDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && (strings.HasSuffix(path, "."+format.Type) || strings.HasSuffix(path, ".iso")) {
				foundFile = path
				return filepath.SkipAll
			}
			return nil
		})
		if err != nil && err != filepath.SkipAll {
			return fmt.Errorf("failed to find output file: %w", err)
		}
		if foundFile == "" {
			return fmt.Errorf("output file not found in %s", tempOutputDir)
		}
		sourceFile = foundFile
	}

	// Move the file to final destination with proper name
	if err := os.Rename(sourceFile, finalOutputPath); err != nil {
		// If rename fails (e.g., cross-device), try copy
		if err := copyFile(sourceFile, finalOutputPath); err != nil {
			return fmt.Errorf("failed to move output file: %w", err)
		}
	}

	fmt.Printf("✅ Converted to %s: %s\n", format.Type, finalOutputPath)

	return nil
}

// generateRegistryConf generates a containers registries.conf content
// for the given insecure registries. This is injected into the VM image at
// /etc/containers/registries.conf.d/local-registry.conf via config.toml [[customizations.files]].
func (c *ConvertStage) generateRegistryConf(registries []string) string {
	var sb strings.Builder
	sb.WriteString("# Generated by bootc-man: insecure registry configuration\n")
	for _, reg := range registries {
		sb.WriteString(fmt.Sprintf("\n[[registry]]\nlocation = \"%s\"\ninsecure = true\n", reg))
	}
	return sb.String()
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// GetImagesDir returns the images directory path relative to project root
func GetImagesDir(baseDir string) string {
	return filepath.Join(baseDir, "output", "images")
}

// shouldUseSudo determines if sudo should be used for podman commands on Linux
// Returns true if Podman is rootless and sudo is available
func (c *ConvertStage) shouldUseSudo() bool {
	// Only check on Linux
	if runtime.GOOS != "linux" {
		return false
	}

	// Check if podman is running in rootless mode
	cmd := exec.Command("podman", "info", "--format", "{{.Host.Security.Rootless}}")
	output, err := cmd.Output()
	if err != nil {
		// If we can't determine, assume rootless and try sudo
		return c.isSudoAvailable()
	}

	isRootless := strings.TrimSpace(string(output)) == "true"
	if !isRootless {
		// Already running as root, no sudo needed
		return false
	}

	// Rootless mode - check if sudo is available
	return c.isSudoAvailable()
}

// isSudoAvailable checks if sudo command is available and can be used
func (c *ConvertStage) isSudoAvailable() bool {
	// Check if sudo command exists
	_, err := exec.LookPath("sudo")
	if err != nil {
		return false
	}

	// Check if sudo can be used (may require password, but command exists)
	// We don't use -n (non-interactive) because we want to allow password prompt
	return true
}

// ensureImageInRootful transfers an image from rootless to rootful Podman storage
// This is needed because bootc-image-builder requires rootful podman
// Uses 'podman image scp' for efficient transfer between user storage and root storage
func (c *ConvertStage) ensureImageInRootful(ctx context.Context) error {
	fmt.Printf("🔄 Checking image in rootful Podman storage...\n")

	// Get image ID from rootless storage
	rootlessIDCmd := c.podman.Command(ctx, "image", "inspect", "--format", "{{.Id}}", c.imageTag)
	rootlessIDOutput, err := rootlessIDCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get rootless image ID: %w", err)
	}
	rootlessID := strings.TrimSpace(string(rootlessIDOutput))

	// Check if image exists in rootful storage and get its ID
	rootfulIDCmd := exec.CommandContext(ctx, "sudo", "podman", "image", "inspect", "--format", "{{.Id}}", c.imageTag)
	rootfulIDOutput, _ := rootfulIDCmd.Output()
	rootfulID := strings.TrimSpace(string(rootfulIDOutput))

	// If image exists in rootful with same ID, skip transfer
	if rootfulID != "" && rootfulID == rootlessID {
		if c.verbose {
			fmt.Printf("   Image already up-to-date in rootful storage: %s\n", c.imageTag)
			fmt.Printf("   Image ID: %s\n", rootlessID[:12])
		}
		return nil
	}

	// If image exists but with different ID, remove old one first
	if rootfulID != "" && rootfulID != rootlessID {
		fmt.Printf("   🔄 Image exists with different ID, updating...\n")
		if c.verbose {
			fmt.Printf("   Rootless ID: %s\n", rootlessID[:12])
			fmt.Printf("   Rootful ID:  %s\n", rootfulID[:12])
		}
		// Remove old image from rootful storage
		rmCmd := exec.CommandContext(ctx, "sudo", "podman", "rmi", "-f", c.imageTag)
		_ = rmCmd.Run() // Ignore error, image might be in use
	}

	// Get current user for podman image scp source
	currentUser := os.Getenv("USER")
	if currentUser == "" {
		// Fallback: try to get username from whoami
		whoamiCmd := exec.Command("whoami")
		output, err := whoamiCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to determine current user: %w", err)
		}
		currentUser = strings.TrimSpace(string(output))
	}

	// Use podman image scp to transfer from rootless to rootful storage
	// Format: podman image scp USER@localhost::IMAGE root@localhost::
	source := fmt.Sprintf("%s@localhost::%s", currentUser, c.imageTag)
	dest := "root@localhost::"

	fmt.Printf("   🔄 Transferring image to rootful Podman storage...\n")
	if c.verbose {
		fmt.Printf("   Running: podman image scp %s %s\n", source, dest)
	}

	scpCmd := c.podman.Command(ctx, "image", "scp", source, dest)
	scpCmd.Stdout = os.Stdout
	scpCmd.Stderr = os.Stderr
	if err := scpCmd.Run(); err != nil {
		return fmt.Errorf("failed to transfer image with podman image scp: %w", err)
	}

	fmt.Printf("   ✅ Image transferred to rootful storage: %s (ID: %s)\n", c.imageTag, rootlessID[:12])
	return nil
}

// Note: getPodmanMachineName was removed as it is currently unused.
// It can be restored if needed for future functionality.

// ensureImageInMachine ensures the image exists in Podman Machine (as root)
// On macOS, images built by rootless user are not available to root
// We need to either:
// 1. Build with sudo (root) in the machine
// 2. Push to registry and pull as root
func (c *ConvertStage) ensureImageInMachine(ctx context.Context) error {
	// With rootful mode on macOS, podman commands use root storage directly
	// Check if image exists (uses rootful connection automatically)
	args := []string{"image", "exists", c.imageTag}
	cmd := c.podman.Command(ctx, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil

	err := cmd.Run()
	if err == nil {
		// Image exists in root storage
		if c.verbose {
			fmt.Printf("Image %s already exists in root storage\n", c.imageTag)
		}
		return nil
	}

	// Image doesn't exist in root storage
	// For localhost images, we may need to rebuild (build stage should have created it)
	if strings.HasPrefix(c.imageTag, "localhost/") {
		// Try to push to local registry first (if available)
		registryTag := fmt.Sprintf("localhost:%d/%s", config.DefaultRegistryPort, strings.TrimPrefix(c.imageTag, "localhost/"))
		if c.tryPushAndPullFromRegistry(ctx, c.imageTag, registryTag) {
			c.imageTag = registryTag
			return nil
		}

		// If registry push/pull failed, build the image
		if err := c.buildImage(ctx); err != nil {
			return fmt.Errorf("failed to ensure image %s: %w. "+
				"Options: 1) Run build stage first: bootc-man ci run --stage build, "+
				"2) Push to registry and pull, 3) Start local registry with 'bootc-man registry up'", c.imageTag, err)
		}
		return nil
	}

	// Try to pull the image from registry
	if c.verbose {
		fmt.Printf("Pulling image %s...\n", c.imageTag)
	}
	pullArgs := []string{"pull", c.imageTag}
	pullCmd := c.podman.Command(ctx, pullArgs...)
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr

	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("failed to pull image %s: %w", c.imageTag, err)
	}

	return nil
}

// tryPushAndPullFromRegistry tries to push image to local registry and pull
// Returns true if successful, false otherwise
// With rootful mode, podman commands use root storage directly
func (c *ConvertStage) tryPushAndPullFromRegistry(ctx context.Context, sourceTag, registryTag string) bool {
	if c.verbose {
		fmt.Printf("Attempting to push %s to local registry %s...\n", sourceTag, registryTag)
	}

	// Push to registry
	pushArgs := []string{"push", sourceTag, registryTag}
	pushCmd := c.podman.Command(ctx, pushArgs...)
	pushCmd.Stdout = os.Stdout
	pushCmd.Stderr = os.Stderr

	if err := pushCmd.Run(); err != nil {
		if c.verbose {
			fmt.Printf("⚠️  Failed to push to registry (registry may not be running): %v\n", err)
		}
		return false
	}

	// Pull from registry
	if c.verbose {
		fmt.Printf("Pulling %s from local registry...\n", registryTag)
	}
	pullArgs := []string{"pull", registryTag}
	pullCmd := c.podman.Command(ctx, pullArgs...)
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr

	if err := pullCmd.Run(); err != nil {
		if c.verbose {
			fmt.Printf("⚠️  Failed to pull from registry: %v\n", err)
		}
		return false
	}

	// Tag as original name
	tagArgs := []string{"tag", registryTag, sourceTag}
	tagCmd := c.podman.Command(ctx, tagArgs...)
	tagCmd.Stdout = os.Stdout
	tagCmd.Stderr = os.Stderr

	if err := tagCmd.Run(); err != nil {
		if c.verbose {
			fmt.Printf("⚠️  Failed to tag image: %v\n", err)
		}
		return false
	}

	if c.verbose {
		fmt.Printf("✅ Successfully pushed and pulled image via local registry\n")
	}
	return true
}

// buildImage builds the image using podman build
// With rootful mode, podman commands use root storage directly
func (c *ConvertStage) buildImage(ctx context.Context) error {
	// Get Containerfile and context paths
	containerfilePath, err := c.pipeline.ResolveContainerfilePath()
	if err != nil {
		return fmt.Errorf("failed to resolve containerfile path: %w", err)
	}
	contextPath, err := c.pipeline.ResolveContextPath()
	if err != nil {
		return fmt.Errorf("failed to resolve context path: %w", err)
	}

	// Resolve absolute paths
	containerfileAbs, err := filepath.Abs(containerfilePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for containerfile: %w", err)
	}
	contextAbs, err := filepath.Abs(contextPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for context: %w", err)
	}

	// Calculate relative path from context to containerfile (for -f flag)
	relPath, err := filepath.Rel(contextAbs, containerfileAbs)
	if err != nil {
		// If relative path calculation fails, use absolute path
		relPath = containerfileAbs
	}

	if c.verbose {
		fmt.Printf("Building image %s...\n", c.imageTag)
		fmt.Printf("  Containerfile: %s\n", containerfileAbs)
		fmt.Printf("  Context: %s\n", contextAbs)
	}

	// Build arguments
	buildArgs := []string{"build", "-t", c.imageTag}
	if relPath != containerfileAbs {
		buildArgs = append(buildArgs, "-f", relPath)
	} else {
		buildArgs = append(buildArgs, "-f", containerfileAbs)
	}
	buildArgs = append(buildArgs, contextAbs)

	cmd := c.podman.Command(ctx, buildArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if c.verbose {
		fmt.Printf("✅ Image %s built successfully\n", c.imageTag)
	}

	return nil
}
