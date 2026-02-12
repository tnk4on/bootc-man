package ci

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// KeygenOptions defines options for key generation
type KeygenOptions struct {
	OutputDir string
	Verbose   bool
}

// GenerateCosignKeyPair generates a cosign key pair
func GenerateCosignKeyPair(ctx context.Context, opts KeygenOptions) error {
	outputDir := opts.OutputDir
	if outputDir == "" {
		var err error
		outputDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Convert to absolute path
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("failed to resolve output directory: %w", err)
	}

	// Check if keys already exist
	keyPath := filepath.Join(absOutputDir, "cosign.key")
	pubPath := filepath.Join(absOutputDir, "cosign.pub")
	if _, err := os.Stat(keyPath); err == nil {
		return fmt.Errorf("cosign.key already exists in %s (delete it first if you want to regenerate)", absOutputDir)
	}
	if _, err := os.Stat(pubPath); err == nil {
		return fmt.Errorf("cosign.pub already exists in %s (delete it first if you want to regenerate)", absOutputDir)
	}

	fmt.Println("🔑 Generating cosign key pair...")
	fmt.Printf("   Output directory: %s\n", absOutputDir)
	fmt.Println()

	// On macOS with Podman Machine, we need to work around virtiofs permission issues (Windows not implemented)
	// On Linux, we can mount the output directory directly
	if runtime.GOOS != "linux" {
		return generateKeyViaMachine(ctx, absOutputDir, opts.Verbose)
	}

	return generateKeyDirect(ctx, absOutputDir, opts.Verbose)
}

// generateKeyDirect generates keys on Linux (native podman)
// Uses a temporary directory strategy to work around cosign container permission issues
func generateKeyDirect(ctx context.Context, outputDir string, verbose bool) error {
	cosignImage := "gcr.io/projectsigstore/cosign:latest"

	// Create a temporary directory for key generation
	tmpDir, err := os.MkdirTemp("", "bootc-man-keygen-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Make temp directory world-writable so cosign container can write to it
	if err := os.Chmod(tmpDir, 0777); err != nil {
		return fmt.Errorf("failed to set temp directory permissions: %w", err)
	}

	// Run cosign container with temp directory mounted
	// Use --user root to create files readable by host user
	// Use --security-opt label=disable to bypass SELinux
	args := []string{
		"run", "--rm",
		"--user", "root",
		"--security-opt", "label=disable",
		"-v", fmt.Sprintf("%s:/output", tmpDir),
		"-w", "/output",
		"-e", "COSIGN_PASSWORD=",
		cosignImage,
		"generate-key-pair",
	}

	if verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate keys: %w", err)
	}

	// Copy keys from temp directory to output directory
	keyData, err := os.ReadFile(filepath.Join(tmpDir, "cosign.key"))
	if err != nil {
		return fmt.Errorf("failed to read generated cosign.key: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "cosign.key"), keyData, 0600); err != nil {
		return fmt.Errorf("failed to write cosign.key: %w", err)
	}

	pubData, err := os.ReadFile(filepath.Join(tmpDir, "cosign.pub"))
	if err != nil {
		return fmt.Errorf("failed to read generated cosign.pub: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "cosign.pub"), pubData, 0644); err != nil {
		return fmt.Errorf("failed to write cosign.pub: %w", err)
	}

	return printKeygenSuccess(outputDir)
}

// generateKeyViaMachine generates keys via Podman Machine (macOS only; Windows not implemented)
// Uses rootful mode - SSH connection is as root, no sudo needed
func generateKeyViaMachine(ctx context.Context, outputDir string, verbose bool) error {
	machineName := getPodmanMachineName()
	if machineName == "" {
		return fmt.Errorf("podman machine is not running. Start it with: podman machine start")
	}

	cosignImage := "gcr.io/projectsigstore/cosign:latest"
	tmpDir := "/var/tmp/bootc-man-keygen"

	// Strategy:
	// 1. Create temp directory in machine (SSH as root, no sudo needed with rootful mode)
	// 2. Run cosign container via podman run (goes through rootful connection)
	// 3. Copy keys from machine to host
	// 4. Clean up
	//
	// Note: With rootful mode:
	// - podman machine ssh connects as root
	// - podman run goes through rootful socket

	// Step 1: Create temp directory in machine
	mkdirCmd := fmt.Sprintf("mkdir -p %s && chmod 777 %s", tmpDir, tmpDir)
	mkdirArgs := []string{"machine", "ssh", machineName, mkdirCmd}

	if verbose {
		fmt.Printf("Running: podman machine ssh %s \"%s\"\n", machineName, mkdirCmd)
	}

	if err := exec.CommandContext(ctx, "podman", mkdirArgs...).Run(); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Step 2: Generate keys using podman run (goes through rootful connection)
	args := []string{
		"run", "--rm",
		"--security-opt", "label=disable",
		"-v", fmt.Sprintf("%s:/output:z", tmpDir),
		"-w", "/output",
		"-e", "COSIGN_PASSWORD=",
		cosignImage,
		"generate-key-pair",
	}

	if verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Clean up on error
		cleanArgs := []string{"machine", "ssh", machineName, fmt.Sprintf("rm -rf %s", tmpDir)}
		_ = exec.CommandContext(ctx, "podman", cleanArgs...).Run()
		return fmt.Errorf("failed to generate keys: %w", err)
	}

	// Step 3: Copy keys from machine to host
	fmt.Println("📋 Copying keys to host...")

	// Copy cosign.key (SSH as root with rootful mode, no sudo needed)
	keyData, err := readFileFromMachine(ctx, machineName, filepath.Join(tmpDir, "cosign.key"))
	if err != nil {
		return fmt.Errorf("failed to read cosign.key from machine: %w", err)
	}
	keyPath := filepath.Join(outputDir, "cosign.key")
	if err := os.WriteFile(keyPath, keyData, 0600); err != nil {
		return fmt.Errorf("failed to write cosign.key: %w", err)
	}

	// Copy cosign.pub
	pubData, err := readFileFromMachine(ctx, machineName, filepath.Join(tmpDir, "cosign.pub"))
	if err != nil {
		return fmt.Errorf("failed to read cosign.pub from machine: %w", err)
	}
	pubPath := filepath.Join(outputDir, "cosign.pub")
	if err := os.WriteFile(pubPath, pubData, 0644); err != nil {
		return fmt.Errorf("failed to write cosign.pub: %w", err)
	}

	// Step 4: Clean up machine's temp directory (no sudo needed with rootful mode)
	cleanArgs := []string{"machine", "ssh", machineName, fmt.Sprintf("rm -rf %s", tmpDir)}
	_ = exec.CommandContext(ctx, "podman", cleanArgs...).Run() // Ignore error

	return printKeygenSuccess(outputDir)
}

// readFileFromMachine reads a file from the Podman Machine
// With rootful mode, SSH is as root - no sudo needed
func readFileFromMachine(ctx context.Context, machineName, filePath string) ([]byte, error) {
	sshArgs := []string{"machine", "ssh", machineName, fmt.Sprintf("cat %s", filePath)}

	cmd := exec.CommandContext(ctx, "podman", sshArgs...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

// printKeygenSuccess prints success message with next steps
func printKeygenSuccess(outputDir string) error {
	keyPath := filepath.Join(outputDir, "cosign.key")
	pubPath := filepath.Join(outputDir, "cosign.pub")

	fmt.Println()
	fmt.Println("✅ Key pair generated successfully!")
	fmt.Println()
	fmt.Println("📁 Generated files:")
	fmt.Printf("   Private key: %s\n", keyPath)
	fmt.Printf("   Public key:  %s\n", pubPath)
	fmt.Println()
	fmt.Println("⚠️  IMPORTANT:")
	fmt.Println("   - Keep cosign.key secret and secure")
	fmt.Println("   - Add cosign.key to .gitignore")
	fmt.Println("   - Share cosign.pub for signature verification")
	fmt.Println()
	fmt.Println("📝 Next steps:")
	fmt.Println("   1. Add to bootc-ci.yaml:")
	fmt.Println("      release:")
	fmt.Println("        sign:")
	fmt.Println("          enabled: true")
	fmt.Println("          key: ./cosign.key")
	fmt.Println()
	fmt.Println("   2. Run release stage:")
	fmt.Println("      bootc-man ci run --stage release")

	return nil
}
