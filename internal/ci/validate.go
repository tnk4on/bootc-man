package ci

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/podman"
)

// ValidateStage executes the validate stage
type ValidateStage struct {
	pipeline *Pipeline
	podman   *podman.Client
	verbose  bool
}

// NewValidateStage creates a new validate stage executor
func NewValidateStage(pipeline *Pipeline, podmanClient *podman.Client, verbose bool) *ValidateStage {
	return &ValidateStage{
		pipeline: pipeline,
		podman:   podmanClient,
		verbose:  verbose,
	}
}

// Execute runs the validate stage
func (v *ValidateStage) Execute(ctx context.Context) error {
	if v.pipeline.Spec.Validate == nil {
		return fmt.Errorf("validate stage is not configured")
	}

	cfg := v.pipeline.Spec.Validate

	// Containerfile lint (hadolint)
	if cfg.ContainerfileLint != nil && cfg.ContainerfileLint.Enabled {
		if err := v.runContainerfileLint(ctx); err != nil {
			return fmt.Errorf("containerfile lint failed: %w", err)
		}

		// Check for bootc container lint configuration
		// Only check if requireBootcLint is true (default behavior)
		if cfg.ContainerfileLint.RequireBootcLint {
			if err := v.checkBootcLintConfig(cfg.ContainerfileLint); err != nil {
				if cfg.ContainerfileLint.FailIfMissing {
					return err
				}
				// warnIfMissing is true by default, so we just print warning
				fmt.Fprintf(os.Stderr, "⚠️  %v\n", err)
			}
		}
	}

	// Config TOML validation
	if cfg.ConfigToml != nil && cfg.ConfigToml.Enabled {
		if err := v.validateConfigToml(ctx); err != nil {
			return fmt.Errorf("config.toml validation failed: %w", err)
		}
	}

	// Secret detection
	if cfg.SecretDetection != nil && cfg.SecretDetection.Enabled {
		if err := v.runSecretDetection(ctx); err != nil {
			return fmt.Errorf("secret detection failed: %w", err)
		}
	}

	return nil
}

// runContainerfileLint runs hadolint on the Containerfile
func (v *ValidateStage) runContainerfileLint(ctx context.Context) error {
	containerfilePath, err := v.pipeline.ResolveContainerfilePath()
	if err != nil {
		return err
	}

	// Read Containerfile
	file, err := os.Open(containerfilePath)
	if err != nil {
		return fmt.Errorf("failed to open containerfile: %w", err)
	}
	defer file.Close()

	if v.verbose {
		fmt.Printf("Running: podman run --rm -i %s < %s\n", config.DefaultHadolintImage, containerfilePath)
	}

	// Run hadolint container
	// Note: We need to pass stdin to the container, which requires a different approach
	// For now, we'll use exec.Command directly since podman client doesn't support stdin yet
	// TODO: Enhance podman client to support stdin
	return v.runHadolintWithStdin(ctx, containerfilePath)
}

// runHadolintWithStdin runs hadolint with stdin input
func (v *ValidateStage) runHadolintWithStdin(ctx context.Context, containerfilePath string) error {
	// This is a temporary implementation
	// In a full implementation, we'd enhance the podman client to support stdin
	// For now, we'll use exec.Command directly
	file, err := os.Open(containerfilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Run hadolint and capture output to analyze warnings vs errors
	// hadolint returns exit code 1 for both warnings and errors,
	// so we need to parse the output to distinguish them
	cmd := exec.CommandContext(ctx, "podman", "run", "--rm", "-i", config.DefaultHadolintImage)
	cmd.Stdin = file

	// Capture stdout and stderr to analyze output
	var stdout, stderr bytes.Buffer
	// Also write to os.Stdout/Stderr for user visibility
	cmd.Stdout = io.MultiWriter(os.Stdout, &stdout)
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

	err = cmd.Run()

	// Parse output to check for errors (not just warnings)
	output := stdout.String() + stderr.String()
	hasError := ParseHadolintOutput(output)

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode := exitError.ExitCode()
			// hadolint returns 1 for any issues (warnings or errors)
			// Only fail if we found actual errors in the output
			if exitCode == 1 && hasError {
				return fmt.Errorf("hadolint found errors in Containerfile")
			}
			// If exit code is 1 but only warnings/info, don't fail
			if exitCode == 1 && !hasError {
				// Warnings only, continue
				return nil
			}
			// Other exit codes are unexpected
			if exitCode != 1 {
				return fmt.Errorf("hadolint failed with exit code %d: %w", exitCode, err)
			}
		} else {
			return fmt.Errorf("hadolint execution failed: %w", err)
		}
	}

	return nil
}

// checkBootcLintConfig checks if Containerfile contains bootc container lint
func (v *ValidateStage) checkBootcLintConfig(cfg *ContainerfileLintConfig) error {
	containerfilePath, err := v.pipeline.ResolveContainerfilePath()
	if err != nil {
		return err
	}

	file, err := os.Open(containerfilePath)
	if err != nil {
		return fmt.Errorf("failed to open containerfile: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	found := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Check for RUN bootc container lint (case-insensitive, allow variations)
		if strings.Contains(strings.ToLower(line), "bootc container lint") ||
			strings.Contains(strings.ToLower(line), "bootc-container-lint") {
			found = true
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read containerfile: %w", err)
	}

	if !found {
		// requireBootcLint defaults to true (when enabled)
		// If RequireBootcLint is explicitly false, we skip the check
		// Otherwise (true or unset when enabled), we check
		if cfg.RequireBootcLint {
			return fmt.Errorf("Containerfile does not contain 'bootc container lint'\n" +
				"Consider adding 'RUN bootc container lint' to validate image structure")
		}
		// If RequireBootcLint is false, we don't check (but this is unusual)
		// Default behavior when enabled is to require it
	}

	return nil
}

// validateConfigToml validates the config.toml file
func (v *ValidateStage) validateConfigToml(ctx context.Context) error {
	if v.pipeline.Spec.Validate.ConfigToml.Path == "" {
		return fmt.Errorf("config.toml path is not specified")
	}

	// Check if file exists (already validated in Pipeline.Validate)
	// TODO: Add TOML syntax validation
	path := v.pipeline.Spec.Validate.ConfigToml.Path
	if !filepath.IsAbs(path) {
		contextPath, err := v.pipeline.ResolveContextPath()
		if err != nil {
			return err
		}
		path = filepath.Join(contextPath, path)
	}

	if v.verbose {
		fmt.Printf("Validating config.toml: %s\n", path)
	}

	// Basic file existence check (TOML parsing can be added later)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config.toml not found: %s", path)
	}

	return nil
}

// ParseHadolintOutput analyzes hadolint output and returns true if errors were found
// This is a pure function that can be easily unit tested
// hadolint output format: "-:line rule level: message"
// Levels: error, warning, info, style
// Returns true only for "error" level, not "warning", "info", or "style"
func ParseHadolintOutput(output string) bool {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Check if line contains " error:" (error level, not just the word "error" in the message)
		// Format: "-:line rule error: message"
		if strings.Contains(line, " error:") {
			return true
		}
	}
	return false
}

// ContainsBootcLint checks if the given content contains bootc container lint pattern
// This is a pure function for testing purposes
func ContainsBootcLint(content string) bool {
	lowerContent := strings.ToLower(content)
	return strings.Contains(lowerContent, "bootc container lint") ||
		strings.Contains(lowerContent, "bootc-container-lint")
}

// runSecretDetection runs secret detection tool
func (v *ValidateStage) runSecretDetection(ctx context.Context) error {
	cfg := v.pipeline.Spec.Validate.SecretDetection
	tool := cfg.Tool
	if tool == "" {
		tool = "gitleaks" // Default
	}

	contextPath, err := v.pipeline.ResolveContextPath()
	if err != nil {
		return err
	}

	var image string
	switch tool {
	case "gitleaks":
		image = config.DefaultGitleaksImage
	case "trufflehog":
		image = config.DefaultTrufflehogImage
	default:
		return fmt.Errorf("unsupported secret detection tool: %s (supported: gitleaks, trufflehog)", tool)
	}

	if v.verbose {
		fmt.Printf("Running: podman run --rm -v %s:/workspace %s\n", contextPath, image)
	}

	// TODO: Implement secret detection execution
	// For now, return not implemented
	return fmt.Errorf("secret detection is an experimental feature (not yet implemented for tool: %s)", tool)
}
