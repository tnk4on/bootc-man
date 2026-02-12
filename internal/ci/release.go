package ci

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/tnk4on/bootc-man/internal/podman"
)

// ReleaseStage executes the release stage
type ReleaseStage struct {
	pipeline *Pipeline
	podman   *podman.Client
	imageTag string // Image tag from build stage
	verbose  bool
}

// NewReleaseStage creates a new release stage executor
func NewReleaseStage(pipeline *Pipeline, podmanClient *podman.Client, imageTag string, verbose bool) *ReleaseStage {
	return &ReleaseStage{
		pipeline: pipeline,
		podman:   podmanClient,
		imageTag: imageTag,
		verbose:  verbose,
	}
}

// Execute runs the release stage
func (r *ReleaseStage) Execute(ctx context.Context) error {
	cfg := r.pipeline.Spec.Release
	if cfg == nil {
		return fmt.Errorf("release stage is not configured")
	}

	// Validate configuration
	if cfg.Registry == "" {
		return fmt.Errorf("release.registry is required")
	}
	if cfg.Repository == "" {
		return fmt.Errorf("release.repository is required")
	}
	if len(cfg.Tags) == 0 {
		return fmt.Errorf("release.tags is required (at least one tag)")
	}

	// On Linux, replace host.containers.internal with localhost
	// host.containers.internal is only resolvable from within containers
	registry := cfg.Registry
	if runtime.GOOS == "linux" {
		registry = r.resolveRegistryHost(registry)
	}

	// Check if image exists before release
	if r.imageTag == "" {
		return fmt.Errorf("image tag is required for release stage (build stage must run first)")
	}
	if err := r.checkImageExists(ctx); err != nil {
		return err
	}

	// Determine TLS verification setting
	tlsVerify := true
	if cfg.TLS != nil {
		tlsVerify = *cfg.TLS
	}

	fmt.Printf("📦 Releasing image to %s/%s\n", cfg.Registry, cfg.Repository)
	if !tlsVerify {
		fmt.Println("   ⚠️  TLS verification disabled")
	}
	fmt.Println()

	// Step 1: Push image with primary tag and get digest
	primaryTag := cfg.Tags[0]
	// Use resolved registry for operations, configured registry for display
	primaryRef := fmt.Sprintf("%s/%s:%s", registry, cfg.Repository, primaryTag)
	primaryRefDisplay := fmt.Sprintf("%s/%s:%s", cfg.Registry, cfg.Repository, primaryTag)

	digest, err := r.pushImageWithDigest(ctx, primaryRef, tlsVerify)
	if err != nil {
		return fmt.Errorf("failed to push image: %w", err)
	}

	digestRef := fmt.Sprintf("%s/%s@%s", registry, cfg.Repository, digest)
	digestRefDisplay := fmt.Sprintf("%s/%s@%s", cfg.Registry, cfg.Repository, digest)
	fmt.Printf("✅ Image pushed: %s\n", primaryRefDisplay)
	fmt.Printf("   Digest: %s\n", digest)

	// Step 2: Sign image (optional, digest-based)
	if cfg.Sign != nil && cfg.Sign.Enabled {
		if err := r.signImage(ctx, digestRef, cfg.Sign, tlsVerify); err != nil {
			return fmt.Errorf("failed to sign image: %w", err)
		}
		fmt.Printf("✅ Image signed: %s\n", digestRefDisplay)
	}

	// Step 3: Push additional tags
	for _, tag := range cfg.Tags[1:] {
		destRef := fmt.Sprintf("%s/%s:%s", registry, cfg.Repository, tag)
		destRefDisplay := fmt.Sprintf("%s/%s:%s", cfg.Registry, cfg.Repository, tag)
		if err := r.pushImage(ctx, destRef, tlsVerify); err != nil {
			return fmt.Errorf("failed to push tag %s: %w", tag, err)
		}
		fmt.Printf("✅ Tag added: %s\n", destRefDisplay)
	}

	fmt.Println()
	fmt.Printf("🎉 Release complete: %s/%s\n", cfg.Registry, cfg.Repository)
	fmt.Printf("   Tags: %s\n", strings.Join(cfg.Tags, ", "))
	if cfg.Sign != nil && cfg.Sign.Enabled {
		fmt.Printf("   Signed: yes (signature at %s/%s:sha256-%s.sig)\n",
			cfg.Registry, cfg.Repository, strings.TrimPrefix(digest, "sha256:"))

		// Show transparency log status
		if cfg.Sign.TransparencyLog != nil && cfg.Sign.TransparencyLog.Enabled {
			if cfg.Sign.TransparencyLog.RekorURL != "" {
				fmt.Printf("   Transparency log: %s (private)\n", cfg.Sign.TransparencyLog.RekorURL)
			} else {
				fmt.Printf("   Transparency log: rekor.sigstore.dev (public)\n")
			}
		} else {
			fmt.Printf("   Transparency log: disabled (offline mode)\n")
		}
	}

	return nil
}

// resolveRegistryHost replaces special container hostnames with localhost
// host.containers.internal is only resolvable from within containers
func (r *ReleaseStage) resolveRegistryHost(registry string) string {
	// Replace host.containers.internal with localhost
	if strings.HasPrefix(registry, "host.containers.internal") {
		resolved := strings.Replace(registry, "host.containers.internal", "localhost", 1)
		if r.verbose {
			fmt.Printf("   Resolving registry: %s -> %s\n", registry, resolved)
		}
		return resolved
	}
	return registry
}

// checkImageExists checks if the image exists in the local Podman storage
func (r *ReleaseStage) checkImageExists(ctx context.Context) error {
	args := []string{"image", "exists", r.imageTag}

	if r.verbose {
		fmt.Printf("Checking image exists: podman %s\n", strings.Join(args, " "))
	}

	cmd := r.podman.Command(ctx, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("image '%s' not found. Run build stage first: bootc-man ci run --stage build", r.imageTag)
	}

	return nil
}

// pushImageWithDigest pushes the image and returns the digest
// With rootful mode, podman push works directly (no SSH needed)
func (r *ReleaseStage) pushImageWithDigest(ctx context.Context, destRef string, tlsVerify bool) (string, error) {
	// Create temporary file for digest
	digestFile, err := os.CreateTemp("", "bootc-man-digest-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create digest file: %w", err)
	}
	digestFile.Close()
	digestFilePath := digestFile.Name()
	defer os.Remove(digestFilePath)

	args := []string{"push", "--digestfile", digestFilePath}
	if !tlsVerify {
		args = append(args, "--tls-verify=false")
	}
	args = append(args, r.imageTag, destRef)

	if r.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	cmd := r.podman.Command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("push failed: %w", err)
	}

	// Read digest
	digestBytes, err := os.ReadFile(digestFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read digest file: %w", err)
	}

	return strings.TrimSpace(string(digestBytes)), nil
}

// pushImage pushes the image to the destination reference
func (r *ReleaseStage) pushImage(ctx context.Context, destRef string, tlsVerify bool) error {
	args := []string{"push"}
	if !tlsVerify {
		args = append(args, "--tls-verify=false")
	}
	args = append(args, r.imageTag, destRef)

	if r.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	cmd := r.podman.Command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// signImage signs the image using cosign container
func (r *ReleaseStage) signImage(ctx context.Context, imageRef string, cfg *SignConfig, tlsVerify bool) error {
	if cfg.Key == "" {
		return fmt.Errorf("sign.key is required when signing is enabled")
	}

	// Resolve key path
	keyPath := cfg.Key
	if !filepath.IsAbs(keyPath) {
		keyPath = filepath.Join(r.pipeline.BaseDir(), cfg.Key)
	}
	absKeyPath, err := filepath.Abs(keyPath)
	if err != nil {
		return fmt.Errorf("failed to resolve key path: %w", err)
	}

	// Check if key file exists
	if _, err := os.Stat(absKeyPath); os.IsNotExist(err) {
		return fmt.Errorf("cosign key file not found: %s", absKeyPath)
	}

	// Determine transparency log settings
	tlogEnabled := false
	rekorURL := ""
	if cfg.TransparencyLog != nil {
		tlogEnabled = cfg.TransparencyLog.Enabled
		rekorURL = cfg.TransparencyLog.RekorURL
	}

	// On macOS, need to copy key to machine's temp dir due to virtiofs permissions (Windows not implemented)
	if runtime.GOOS != "linux" {
		return r.signImageViaMachine(ctx, imageRef, absKeyPath, tlsVerify, tlogEnabled, rekorURL)
	}

	return r.signImageDirect(ctx, imageRef, absKeyPath, tlsVerify, tlogEnabled, rekorURL)
}

// signImageDirect signs the image directly on Linux
func (r *ReleaseStage) signImageDirect(ctx context.Context, imageRef, keyPath string, tlsVerify, tlogEnabled bool, rekorURL string) error {
	cosignImage := "gcr.io/projectsigstore/cosign:latest"

	// Prepare cosign command arguments
	// Use --user root to allow reading mounted files
	// Use --security-opt label=disable for SELinux compatibility
	args := []string{"run", "--rm", "--network=host", "--user", "root", "--security-opt", "label=disable"}

	// Mount auth config (only if it exists)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		dockerAuthPath := filepath.Join(homeDir, ".docker", "config.json")
		podmanAuthPath := filepath.Join(homeDir, ".config", "containers", "auth.json")

		if _, err := os.Stat(dockerAuthPath); err == nil {
			args = append(args, "-v", fmt.Sprintf("%s:/root/.docker/config.json:ro", dockerAuthPath))
		} else if _, err := os.Stat(podmanAuthPath); err == nil {
			args = append(args, "-v", fmt.Sprintf("%s:/root/.docker/config.json:ro", podmanAuthPath))
		}
	}

	// Mount the cosign key
	args = append(args, "-v", fmt.Sprintf("%s:/cosign.key:ro", keyPath))

	// Add environment variables for non-interactive signing
	args = append(args, "-e", "COSIGN_PASSWORD=")

	// If transparency log is disabled, set COSIGN_OFFLINE to skip network operations
	if !tlogEnabled {
		args = append(args, "-e", "COSIGN_OFFLINE=1")
	}

	// cosign image
	args = append(args, cosignImage)

	// cosign command: sign with key
	cosignArgs := []string{"sign", "--key", "/cosign.key", "--yes"}

	// Transparency log settings
	if tlogEnabled {
		// Enable transparency log upload
		if rekorURL != "" {
			// Use custom Rekor URL (private instance)
			cosignArgs = append(cosignArgs, "--rekor-url="+rekorURL)
		}
		// Default: use public Sigstore Rekor
	} else {
		// Disable transparency log upload (offline/PoC mode)
		cosignArgs = append(cosignArgs, "--use-signing-config=false", "--tlog-upload=false")
	}

	if !tlsVerify {
		// --allow-http-registry: allows HTTP (non-TLS) connections
		// --allow-insecure-registry: allows self-signed/expired TLS certificates
		cosignArgs = append(cosignArgs, "--allow-http-registry", "--allow-insecure-registry")
	}
	cosignArgs = append(cosignArgs, imageRef)
	args = append(args, cosignArgs...)

	if r.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	cmd := r.podman.Command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		registry := strings.Split(imageRef, "/")[0]
		return fmt.Errorf("cosign sign failed: %w\n\nHint: Make sure you have logged in to the registry:\n  podman login %s", err, registry)
	}

	return nil
}

// signImageViaMachine signs the image on macOS via Podman Machine (Windows not implemented)
// Copies the key file to machine's temp dir to avoid virtiofs permission issues
func (r *ReleaseStage) signImageViaMachine(ctx context.Context, imageRef, keyPath string, tlsVerify, tlogEnabled bool, rekorURL string) error {
	machineName := getPodmanMachineName()
	if machineName == "" {
		return fmt.Errorf("podman machine is not running")
	}

	cosignImage := "gcr.io/projectsigstore/cosign:latest"
	tmpDir := "/var/tmp/bootc-man-sign"

	// Step 1: Create temp directory and copy key file
	mkdirCmd := fmt.Sprintf("mkdir -p %s && chmod 700 %s", tmpDir, tmpDir)
	mkdirArgs := []string{"machine", "ssh", machineName, mkdirCmd}
	if err := exec.CommandContext(ctx, "podman", mkdirArgs...).Run(); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Read key content and write to machine
	keyContent, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	// Write key to machine via ssh cat
	// Make key readable by container user (cosign runs as non-root)
	machineKeyPath := filepath.Join(tmpDir, "cosign.key")
	catCmd := fmt.Sprintf("cat > %s && chmod 644 %s", machineKeyPath, machineKeyPath)
	catArgs := []string{"machine", "ssh", machineName, catCmd}
	catExec := exec.CommandContext(ctx, "podman", catArgs...)
	catExec.Stdin = strings.NewReader(string(keyContent))
	if err := catExec.Run(); err != nil {
		return fmt.Errorf("failed to copy key to machine: %w", err)
	}

	// Step 2: Run cosign container
	args := []string{"run", "--rm", "--network=host", "--security-opt", "label=disable"}

	// Mount the key from machine's temp dir
	args = append(args, "-v", fmt.Sprintf("%s:/cosign.key:ro,z", machineKeyPath))

	// Add environment variables for non-interactive signing
	args = append(args, "-e", "COSIGN_PASSWORD=")

	// If transparency log is disabled, set COSIGN_OFFLINE to skip network operations
	if !tlogEnabled {
		args = append(args, "-e", "COSIGN_OFFLINE=1")
	}

	// cosign image
	args = append(args, cosignImage)

	// cosign command: sign with key
	cosignArgs := []string{"sign", "--key", "/cosign.key", "--yes"}

	// Transparency log settings
	if tlogEnabled {
		// Enable transparency log upload
		if rekorURL != "" {
			// Use custom Rekor URL (private instance)
			cosignArgs = append(cosignArgs, "--rekor-url="+rekorURL)
		}
		// Default: use public Sigstore Rekor
	} else {
		// Disable transparency log upload (offline/PoC mode)
		cosignArgs = append(cosignArgs, "--use-signing-config=false", "--tlog-upload=false")
	}

	if !tlsVerify {
		// --allow-http-registry: allows HTTP (non-TLS) connections
		// --allow-insecure-registry: allows self-signed/expired TLS certificates
		cosignArgs = append(cosignArgs, "--allow-http-registry", "--allow-insecure-registry")
	}
	cosignArgs = append(cosignArgs, imageRef)
	args = append(args, cosignArgs...)

	if r.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	cmd := r.podman.Command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()

	// Step 3: Clean up
	cleanArgs := []string{"machine", "ssh", machineName, fmt.Sprintf("rm -rf %s", tmpDir)}
	_ = exec.CommandContext(ctx, "podman", cleanArgs...).Run() // Ignore error

	if err != nil {
		registry := strings.Split(imageRef, "/")[0]
		return fmt.Errorf("cosign sign failed: %w\n\nHint: Make sure you have logged in to the registry:\n  podman login %s", err, registry)
	}

	return nil
}
