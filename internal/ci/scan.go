package ci

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/podman"
)

// ScanStage executes the scan stage
type ScanStage struct {
	pipeline *Pipeline
	podman   *podman.Client
	verbose  bool
	imageTag string // Image tag from build stage
}

// NewScanStage creates a new scan stage executor
func NewScanStage(pipeline *Pipeline, podmanClient *podman.Client, imageTag string, verbose bool) *ScanStage {
	return &ScanStage{
		pipeline: pipeline,
		podman:   podmanClient,
		imageTag: imageTag,
		verbose:  verbose,
	}
}

// Execute runs the scan stage
func (s *ScanStage) Execute(ctx context.Context) error {
	if s.pipeline.Spec.Scan == nil {
		return fmt.Errorf("scan stage is not configured")
	}

	// Check if image exists before running scan
	if s.imageTag == "" {
		return fmt.Errorf("image tag is required for scan stage (build stage must run first)")
	}
	if err := s.checkImageExists(ctx); err != nil {
		return err
	}

	cfg := s.pipeline.Spec.Scan

	// Vulnerability scan
	if cfg.Vulnerability != nil && cfg.Vulnerability.Enabled {
		if err := s.runVulnerabilityScan(ctx, cfg.Vulnerability); err != nil {
			return fmt.Errorf("vulnerability scan failed: %w", err)
		}
	}

	// SBOM generation
	if cfg.SBOM != nil && cfg.SBOM.Enabled {
		if err := s.runSBOMGeneration(ctx, cfg.SBOM); err != nil {
			return fmt.Errorf("SBOM generation failed: %w", err)
		}
	}

	// Lint (if enabled)
	// TODO: Implement lint scan when needed - currently a no-op
	_ = cfg.Lint // Suppress unused warning until implemented

	return nil
}

// runVulnerabilityScan runs vulnerability scan using configured tool
func (s *ScanStage) runVulnerabilityScan(ctx context.Context, cfg *VulnerabilityConfig) error {
	if s.imageTag == "" {
		return fmt.Errorf("image tag is required for vulnerability scan (build stage must run first)")
	}

	// Determine which tool to use (default: trivy)
	tool := cfg.Tool
	if tool == "" {
		tool = "trivy"
	}

	switch tool {
	case "trivy":
		return s.runTrivyScan(ctx, cfg)
	case "grype":
		return s.runGrypeScan(ctx, cfg)
	default:
		return fmt.Errorf("unsupported vulnerability scan tool: %s (supported: trivy, grype)", tool)
	}
}

// runTrivyScan runs Trivy vulnerability scan
func (s *ScanStage) runTrivyScan(ctx context.Context, cfg *VulnerabilityConfig) error {
	// Export image to docker-archive format for Trivy to scan
	// This works reliably across all platforms (Linux, macOS, Windows)
	// Podman Machine on macOS uses SSH connections, so direct socket access is not possible (Windows not implemented)
	archivePath, err := s.exportImageToArchive(ctx)
	if err != nil {
		return fmt.Errorf("failed to export image: %w", err)
	}
	defer os.Remove(archivePath) // Clean up temporary archive

	image := config.DefaultTrivyImage

	// Prepare trivy command arguments
	args := []string{"run", "--rm"}

	// Mount Podman named volume for DB persistence
	// This allows DB to be reused across runs and enables offline mode
	// Using named volume instead of host path to avoid polluting host filesystem
	args = append(args, "-v", config.VolumeNameTrivyCache+":/root/.cache/trivy")

	// Mount the archive file
	// Use :z for SELinux relabeling (required on Fedora/RHEL)
	args = append(args, "-v", fmt.Sprintf("%s:/image.tar:ro,z", archivePath))

	// Trivy image
	args = append(args, image)

	// Trivy command: image scan
	args = append(args, "image")

	// Use --input option for docker-archive format
	args = append(args, "--input", "/image.tar")

	// Skip DB update for offline mode
	if cfg.SkipDbUpdate {
		args = append(args, "--skip-db-update")
		args = append(args, "--skip-java-db-update")
		args = append(args, "--offline-scan")
	}

	// Add severity filter if specified
	if cfg.Severity != "" {
		args = append(args, "--severity", cfg.Severity)
	}

	// Output format: table (default)
	args = append(args, "--format", "table")

	// Exit code: 0 even if vulnerabilities found (unless failOnVulnerability is true)
	if !cfg.FailOnVulnerability {
		args = append(args, "--exit-code", "0")
	}

	if s.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	cmd := s.podman.Command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return s.handleVulnerabilityScanError(err, cfg, "trivy")
	}

	return nil
}

// runGrypeScan runs Grype vulnerability scan
func (s *ScanStage) runGrypeScan(ctx context.Context, cfg *VulnerabilityConfig) error {
	// Export image to docker-archive format for Grype to scan
	archivePath, err := s.exportImageToArchive(ctx)
	if err != nil {
		return fmt.Errorf("failed to export image: %w", err)
	}
	defer os.Remove(archivePath) // Clean up temporary archive

	image := config.DefaultGrypeImage

	// Prepare grype command arguments
	args := []string{"run", "--rm"}

	// Skip DB update for offline mode
	// Grype container image includes a built-in DB, so offline mode works from first run
	if cfg.SkipDbUpdate {
		args = append(args, "-e", "GRYPE_DB_AUTO_UPDATE=false")
		args = append(args, "-e", "GRYPE_DB_VALIDATE_AGE=false")
	}

	// Mount Podman named volume for DB persistence (allows DB updates to persist)
	// Using named volume instead of host path to avoid polluting host filesystem
	args = append(args, "-v", config.VolumeNameGrypeCache+":/root/.cache/grype")

	// Mount the archive file
	// Use :z for SELinux relabeling (required on Fedora/RHEL)
	args = append(args, "-v", fmt.Sprintf("%s:/image.tar:ro,z", archivePath))

	// Grype image
	args = append(args, image)

	// Image to scan - use docker-archive: prefix
	args = append(args, "docker-archive:/image.tar")

	// Add severity filter if specified (Grype uses --fail-on for severity threshold)
	if cfg.Severity != "" {
		// Grype uses --only-fixed and --fail-on for severity
		// --fail-on sets the minimum severity to fail on
		args = append(args, "--fail-on", strings.ToLower(strings.Split(cfg.Severity, ",")[0]))
	}

	// Output format: table (default)
	args = append(args, "--output", "table")

	if s.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	cmd := s.podman.Command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return s.handleVulnerabilityScanError(err, cfg, "grype")
	}

	return nil
}

// handleVulnerabilityScanError handles errors from vulnerability scan tools
func (s *ScanStage) handleVulnerabilityScanError(err error, cfg *VulnerabilityConfig, toolName string) error {
	// Check if it's an execution error (not just vulnerabilities found)
	if exitError, ok := err.(*exec.ExitError); ok {
		exitCode := exitError.ExitCode()
		// Exit code 1 typically means vulnerabilities found
		// Exit code 2+ typically means execution errors
		if exitCode == 1 && !cfg.FailOnVulnerability {
			// Vulnerabilities found, but we don't fail
			return nil
		}
		// Exit code 2+ or failOnVulnerability is true: return error
		if cfg.FailOnVulnerability && exitCode == 1 {
			return fmt.Errorf("vulnerability scan found issues: %w", err)
		}
		// Execution error (exit code 2+)
		return fmt.Errorf("%s scan failed with exit code %d: %w", toolName, exitCode, err)
	}
	// Non-exit error (e.g., command not found, socket error, etc.)
	return fmt.Errorf("%s scan failed: %w", toolName, err)
}

// runSBOMGeneration runs SBOM generation using configured tool
func (s *ScanStage) runSBOMGeneration(ctx context.Context, cfg *SBOMConfig) error {
	if s.imageTag == "" {
		return fmt.Errorf("image tag is required for SBOM generation (build stage must run first)")
	}

	// Determine which tool to use (default: syft)
	tool := cfg.Tool
	if tool == "" {
		tool = "syft"
	}

	switch tool {
	case "syft":
		return s.runSyftSBOM(ctx, cfg)
	case "trivy":
		return s.runTrivySBOM(ctx, cfg)
	default:
		return fmt.Errorf("unsupported SBOM tool: %s (supported: syft, trivy)", tool)
	}
}

// runSyftSBOM runs Syft to generate SBOM
func (s *ScanStage) runSyftSBOM(ctx context.Context, cfg *SBOMConfig) error {
	// Syft doesn't support --image-src podman, so we need to export the image
	// Export image to docker-archive format for Syft to scan
	archivePath, err := s.exportImageToArchive(ctx)
	if err != nil {
		return fmt.Errorf("failed to export image: %w", err)
	}
	defer os.Remove(archivePath) // Clean up temporary archive

	image := config.DefaultSyftImage

	// Determine output format
	format := cfg.Format
	if format == "" {
		format = "spdx-json" // Default
	}

	// Prepare syft command arguments
	args := []string{"run", "--rm"}
	
	// Mount the archive file
	// Use :z for SELinux relabeling (required on Fedora/RHEL)
	args = append(args, "-v", fmt.Sprintf("%s:/image.tar:ro,z", archivePath))
	
	// Syft image
	args = append(args, image)
	
	// Syft command: scan (packages is deprecated)
	args = append(args, "scan")
	
	// Output format
	args = append(args, "--output", format)
	
	// Image to scan - use docker-archive: prefix
	args = append(args, "docker-archive:/image.tar")

	if s.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	// Generate output file path
	outputFile := s.generateSBOMOutputPath(format, "syft")
	
	// Create output directory if needed
	outputDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Redirect output to file
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	cmd := s.podman.Command(ctx, args...)
	cmd.Stdout = file
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("syft SBOM generation failed: %w", err)
	}

	fmt.Printf("✅ SBOM generated: %s\n", outputFile)
	return nil
}

// runTrivySBOM runs Trivy to generate SBOM
func (s *ScanStage) runTrivySBOM(ctx context.Context, cfg *SBOMConfig) error {
	// Export image to docker-archive format for Trivy to scan
	archivePath, err := s.exportImageToArchive(ctx)
	if err != nil {
		return fmt.Errorf("failed to export image: %w", err)
	}
	defer os.Remove(archivePath) // Clean up temporary archive

	image := config.DefaultTrivyImage

	// Determine output format
	// Trivy supports: spdx, spdx-json, cyclonedx, cyclonedx-json
	format := cfg.Format
	if format == "" {
		format = "spdx-json" // Default
	}

	// Prepare trivy command arguments
	args := []string{"run", "--rm"}
	
	// Mount the archive file
	// Use :z for SELinux relabeling (required on Fedora/RHEL)
	args = append(args, "-v", fmt.Sprintf("%s:/image.tar:ro,z", archivePath))
	
	// Trivy image
	args = append(args, image)
	
	// Trivy command: image with SBOM output
	args = append(args, "image")
	
	// Use --input option for docker-archive format
	args = append(args, "--input", "/image.tar")
	
	// Output format for SBOM
	args = append(args, "--format", format)

	if s.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	// Generate output file path
	outputFile := s.generateSBOMOutputPath(format, "trivy")
	
	// Create output directory if needed
	outputDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Redirect output to file
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	cmd := s.podman.Command(ctx, args...)
	cmd.Stdout = file
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("trivy SBOM generation failed: %w", err)
	}

	fmt.Printf("✅ SBOM generated: %s\n", outputFile)
	return nil
}

// generateSBOMOutputPath generates the output path for SBOM file
func (s *ScanStage) generateSBOMOutputPath(format string, toolName string) string {
	// Generate filename based on image tag, tool, and format
	imageName := strings.ReplaceAll(s.imageTag, "/", "_")
	imageName = strings.ReplaceAll(imageName, ":", "_")
	
	var ext string
	switch format {
	case "spdx-json":
		ext = "spdx.json"
	case "cyclonedx-json":
		ext = "cdx.json"
	case "json":
		ext = "json"
	default:
		ext = "json"
	}
	
	// Output to output/sbom/ directory with tool name prefix
	return filepath.Join("output", "sbom", fmt.Sprintf("%s.%s.%s", imageName, toolName, ext))
}

// checkImageExists checks if the image exists in the local Podman storage
func (s *ScanStage) checkImageExists(ctx context.Context) error {
	args := []string{"image", "exists", s.imageTag}

	if s.verbose {
		fmt.Printf("Checking image exists: podman %s\n", strings.Join(args, " "))
	}

	cmd := s.podman.Command(ctx, args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("image '%s' not found. Run build stage first: bootc-man ci run --stage build", s.imageTag)
	}

	return nil
}

// exportImageToArchive exports the Podman image to docker-archive format
// Returns the path to the temporary archive file
// This is used for Syft which doesn't support --image-src podman
func (s *ScanStage) exportImageToArchive(ctx context.Context) (string, error) {
	// Create temporary file for the archive
	tmpFile, err := os.CreateTemp("", "bootc-man-scan-*.tar")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpFile.Close()
	archivePath := tmpFile.Name()

	// Use podman save to export the image
	args := []string{"save", "-o", archivePath, s.imageTag}

	if s.verbose {
		fmt.Printf("Exporting image: podman %s\n", strings.Join(args, " "))
	}

	cmd := s.podman.Command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.Remove(archivePath) // Clean up on error
		return "", fmt.Errorf("failed to export image: %w", err)
	}

	return archivePath, nil
}


