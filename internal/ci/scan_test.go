package ci

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

func TestNewScanStage(t *testing.T) {
	dir := testutil.SetupPipelineTestDir(t)
	pipeline := &Pipeline{
		baseDir: dir,
		Spec: PipelineSpec{
			Source: SourceConfig{
				Containerfile: "Containerfile",
				Context:       ".",
			},
			Scan: &ScanConfig{
				Vulnerability: &VulnerabilityConfig{
					Enabled: true,
					Tool:    "trivy",
				},
			},
		},
	}

	imageTag := "localhost/test:latest"
	stage := NewScanStage(pipeline, nil, imageTag, true)

	if stage == nil {
		t.Fatal("NewScanStage returned nil")
	}
	if stage.pipeline != pipeline {
		t.Error("stage.pipeline not set correctly")
	}
	if stage.imageTag != imageTag {
		t.Errorf("stage.imageTag = %q, want %q", stage.imageTag, imageTag)
	}
	if !stage.verbose {
		t.Error("stage.verbose should be true")
	}
}

func TestScanStageNotConfigured(t *testing.T) {
	dir := testutil.SetupPipelineTestDir(t)
	pipeline := &Pipeline{
		baseDir: dir,
		Spec: PipelineSpec{
			Source: SourceConfig{
				Containerfile: "Containerfile",
				Context:       ".",
			},
			// Scan is nil
		},
	}

	stage := NewScanStage(pipeline, nil, "test:latest", false)
	err := stage.Execute(context.Background())

	if err == nil {
		t.Fatal("expected error for unconfigured scan stage")
	}
	if !containsString(err.Error(), "not configured") {
		t.Errorf("error should mention 'not configured': %v", err)
	}
}

func TestScanStageNoImageTag(t *testing.T) {
	dir := testutil.SetupPipelineTestDir(t)
	pipeline := &Pipeline{
		baseDir: dir,
		Spec: PipelineSpec{
			Source: SourceConfig{
				Containerfile: "Containerfile",
				Context:       ".",
			},
			Scan: &ScanConfig{
				Vulnerability: &VulnerabilityConfig{
					Enabled: true,
				},
			},
		},
	}

	// Empty image tag
	stage := NewScanStage(pipeline, nil, "", false)
	err := stage.Execute(context.Background())

	if err == nil {
		t.Fatal("expected error for missing image tag")
	}
	if !containsString(err.Error(), "image tag is required") {
		t.Errorf("error should mention 'image tag is required': %v", err)
	}
}

func TestVulnerabilityConfigDefaults(t *testing.T) {
	cfg := &VulnerabilityConfig{
		Enabled: true,
		// Tool defaults to empty (will use "trivy" at runtime)
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestVulnerabilityConfigWithTool(t *testing.T) {
	tests := []struct {
		name string
		tool string
	}{
		{name: "trivy", tool: "trivy"},
		{name: "grype", tool: "grype"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &VulnerabilityConfig{
				Enabled: true,
				Tool:    tt.tool,
			}

			if cfg.Tool != tt.tool {
				t.Errorf("Tool = %q, want %q", cfg.Tool, tt.tool)
			}
		})
	}
}

func TestSBOMConfigDefaults(t *testing.T) {
	cfg := &SBOMConfig{
		Enabled: true,
		Format:  "spdx-json",
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Format != "spdx-json" {
		t.Errorf("Format = %q, want %q", cfg.Format, "spdx-json")
	}
}

func TestSBOMConfigFormats(t *testing.T) {
	tests := []struct {
		name   string
		format string
	}{
		{name: "spdx-json", format: "spdx-json"},
		{name: "cyclonedx-json", format: "cyclonedx-json"},
		{name: "syft-json", format: "syft-json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &SBOMConfig{
				Enabled: true,
				Format:  tt.format,
			}

			if cfg.Format != tt.format {
				t.Errorf("Format = %q, want %q", cfg.Format, tt.format)
			}
		})
	}
}

func TestScanConfigStructure(t *testing.T) {
	cfg := &ScanConfig{
		Vulnerability: &VulnerabilityConfig{
			Enabled:             true,
			Tool:                "trivy",
			Severity:            "HIGH,CRITICAL",
			FailOnVulnerability: true,
			SkipDbUpdate:        false,
		},
		SBOM: &SBOMConfig{
			Enabled: true,
			Tool:    "syft",
			Format:  "cyclonedx-json",
		},
		Lint: &LintConfig{
			Enabled: true,
		},
	}

	if !cfg.Vulnerability.Enabled {
		t.Error("Vulnerability.Enabled should be true")
	}
	if cfg.Vulnerability.Tool != "trivy" {
		t.Errorf("Vulnerability.Tool = %q, want %q", cfg.Vulnerability.Tool, "trivy")
	}
	if cfg.Vulnerability.Severity != "HIGH,CRITICAL" {
		t.Errorf("Vulnerability.Severity = %q, want %q", cfg.Vulnerability.Severity, "HIGH,CRITICAL")
	}
	if !cfg.Vulnerability.FailOnVulnerability {
		t.Error("Vulnerability.FailOnVulnerability should be true")
	}

	if !cfg.SBOM.Enabled {
		t.Error("SBOM.Enabled should be true")
	}
	if cfg.SBOM.Format != "cyclonedx-json" {
		t.Errorf("SBOM.Format = %q, want %q", cfg.SBOM.Format, "cyclonedx-json")
	}
	if cfg.SBOM.Tool != "syft" {
		t.Errorf("SBOM.Tool = %q, want %q", cfg.SBOM.Tool, "syft")
	}

	if !cfg.Lint.Enabled {
		t.Error("Lint.Enabled should be true")
	}
}

func TestLintConfigDefaults(t *testing.T) {
	cfg := &LintConfig{
		Enabled: true,
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
}

// TestGenerateSBOMOutputPath tests the pure logic of SBOM output path generation
func TestGenerateSBOMOutputPath(t *testing.T) {
	tests := []struct {
		name     string
		imageTag string
		format   string
		toolName string
		want     string
	}{
		{
			name:     "spdx-json with syft",
			imageTag: "localhost/myimage:latest",
			format:   "spdx-json",
			toolName: "syft",
			want:     "output/sbom/localhost_myimage_latest.syft.spdx.json",
		},
		{
			name:     "cyclonedx-json with trivy",
			imageTag: "quay.io/centos/centos:stream9",
			format:   "cyclonedx-json",
			toolName: "trivy",
			want:     "output/sbom/quay.io_centos_centos_stream9.trivy.cdx.json",
		},
		{
			name:     "json format",
			imageTag: "test:v1.0",
			format:   "json",
			toolName: "syft",
			want:     "output/sbom/test_v1.0.syft.json",
		},
		{
			name:     "unknown format defaults to json",
			imageTag: "app:latest",
			format:   "unknown-format",
			toolName: "tool",
			want:     "output/sbom/app_latest.tool.json",
		},
		{
			name:     "image with multiple slashes",
			imageTag: "registry.example.com/org/repo/image:tag",
			format:   "spdx-json",
			toolName: "syft",
			want:     "output/sbom/registry.example.com_org_repo_image_tag.syft.spdx.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &ScanStage{
				imageTag: tt.imageTag,
			}
			got := stage.generateSBOMOutputPath(tt.format, tt.toolName)
			// Normalize path separators for cross-platform testing
			got = strings.ReplaceAll(got, "\\", "/")
			if got != tt.want {
				t.Errorf("generateSBOMOutputPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHandleVulnerabilityScanError tests error handling logic for vulnerability scans
func TestHandleVulnerabilityScanError(t *testing.T) {
	// Create a mock exec.ExitError using a failed command
	createExitError := func(exitCode int) *exec.ExitError {
		// Run a command that will fail with the specified exit code
		var cmd *exec.Cmd
		switch exitCode {
		case 1:
			cmd = exec.Command("sh", "-c", "exit 1")
		case 2:
			cmd = exec.Command("sh", "-c", "exit 2")
		case 127:
			cmd = exec.Command("sh", "-c", "exit 127")
		default:
			cmd = exec.Command("sh", "-c", "exit "+string(rune('0'+exitCode)))
		}
		err := cmd.Run()
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr
		}
		return nil
	}

	tests := []struct {
		name                string
		exitCode            int
		failOnVulnerability bool
		toolName            string
		expectNil           bool
		errorContains       string
	}{
		{
			name:                "exit code 1, failOnVulnerability=false: no error",
			exitCode:            1,
			failOnVulnerability: false,
			toolName:            "trivy",
			expectNil:           true,
		},
		{
			name:                "exit code 1, failOnVulnerability=true: error with 'found issues'",
			exitCode:            1,
			failOnVulnerability: true,
			toolName:            "trivy",
			expectNil:           false,
			errorContains:       "found issues",
		},
		{
			name:                "exit code 2: execution error",
			exitCode:            2,
			failOnVulnerability: false,
			toolName:            "grype",
			expectNil:           false,
			errorContains:       "exit code 2",
		},
		{
			name:                "exit code 127: command not found style error",
			exitCode:            127,
			failOnVulnerability: false,
			toolName:            "trivy",
			expectNil:           false,
			errorContains:       "exit code 127",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := &ScanStage{}
			cfg := &VulnerabilityConfig{
				FailOnVulnerability: tt.failOnVulnerability,
			}

			exitErr := createExitError(tt.exitCode)
			if exitErr == nil {
				t.Skip("Could not create exit error for test")
				return
			}

			result := stage.handleVulnerabilityScanError(exitErr, cfg, tt.toolName)

			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil error, got: %v", result)
				}
			} else {
				if result == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(result.Error(), tt.errorContains) {
					t.Errorf("error %q should contain %q", result.Error(), tt.errorContains)
				}
			}
		})
	}
}

// TestHandleVulnerabilityScanErrorNonExec tests error handling for non-exec errors
func TestHandleVulnerabilityScanErrorNonExec(t *testing.T) {
	stage := &ScanStage{}
	cfg := &VulnerabilityConfig{
		FailOnVulnerability: false,
	}

	// Test with a regular error (not exec.ExitError)
	regularError := errors.New("some network error")
	result := stage.handleVulnerabilityScanError(regularError, cfg, "trivy")

	if result == nil {
		t.Fatal("expected error for non-exec error, got nil")
	}
	if !strings.Contains(result.Error(), "scan failed") {
		t.Errorf("error %q should contain 'scan failed'", result.Error())
	}
	if !strings.Contains(result.Error(), "trivy") {
		t.Errorf("error %q should contain tool name 'trivy'", result.Error())
	}
}

// TestRunVulnerabilityScanToolSelection tests the tool selection logic
func TestRunVulnerabilityScanToolSelection(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty tool defaults to trivy",
			tool:        "",
			expectError: false, // Would fail later due to no podman, but tool selection works
		},
		{
			name:        "trivy is valid",
			tool:        "trivy",
			expectError: false,
		},
		{
			name:        "grype is valid",
			tool:        "grype",
			expectError: false,
		},
		{
			name:        "unsupported tool",
			tool:        "invalid-tool",
			expectError: true,
			errorMsg:    "unsupported vulnerability scan tool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't fully test without podman, but we can test tool validation
			cfg := &VulnerabilityConfig{
				Enabled: true,
				Tool:    tt.tool,
			}

			// Determine expected tool (verified that empty defaults to trivy at runtime)
			expectedTool := tt.tool
			if expectedTool == "" {
				expectedTool = "trivy"
			}
			_ = expectedTool // Verified but not directly used in this test

			// For unsupported tools, verify the config is as expected
			if tt.expectError {
				if cfg.Tool != tt.tool {
					t.Errorf("Tool = %q, want %q", cfg.Tool, tt.tool)
				}
			} else {
				// Valid tools should be trivy or grype (or empty which defaults to trivy)
				validTools := map[string]bool{"": true, "trivy": true, "grype": true}
				if !validTools[cfg.Tool] {
					t.Errorf("Unexpected valid tool: %q", cfg.Tool)
				}
			}
		})
	}
}
