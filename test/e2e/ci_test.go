//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

// skipIfConvertUnavailable skips if the convert stage cannot run.
// On Linux, convert requires rootful Podman for image transfer to rootful storage.
// On macOS, convert runs inside Podman Machine (no rootful check needed).
func skipIfConvertUnavailable(t *testing.T) {
	t.Helper()
	testutil.SkipIfPodmanUnavailable(t)
	if runtime.GOOS == "linux" {
		testutil.SkipIfPodmanNotRootful(t)
	}
}

// CITestEnvironment extends TestEnvironment with CI-specific functionality
type CITestEnvironment struct {
	*TestEnvironment
	pipelineFile  string
	containerfile string
	imageTag      string
}

// NewCITestEnvironment creates a new CI test environment
func NewCITestEnvironment(t *testing.T) *CITestEnvironment {
	t.Helper()

	base := NewTestEnvironment(t)

	return &CITestEnvironment{
		TestEnvironment: base,
		imageTag:        fmt.Sprintf("localhost:5000/e2e-ci-test:%d", nowUnixNano()),
	}
}

// SetupMinimalPipeline creates a minimal CI pipeline for testing
func (e *CITestEnvironment) SetupMinimalPipeline(t *testing.T) {
	t.Helper()

	// Create Containerfile
	containerfileContent := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN echo "e2e-ci-test" > /etc/e2e-ci-test
`, testutil.TestBootcImageCurrent())

	e.containerfile = filepath.Join(e.workDir, "Containerfile")
	if err := os.WriteFile(e.containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	// Create minimal bootc-ci.yaml
	pipelineContent := fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: e2e-ci-test
  description: "E2E CI Test Pipeline"

spec:
  source:
    containerfile: ./Containerfile
    context: .

  validate:
    containerfileLint:
      enabled: true
      requireBootcLint: false
      failIfMissing: false

  build:
    imageTag: %s
`, e.imageTag)

	e.pipelineFile = filepath.Join(e.workDir, "bootc-ci.yaml")
	if err := os.WriteFile(e.pipelineFile, []byte(pipelineContent), 0644); err != nil {
		t.Fatalf("Failed to create bootc-ci.yaml: %v", err)
	}
}

// SetupValidatePipeline creates a pipeline focused on validate stage testing
func (e *CITestEnvironment) SetupValidatePipeline(t *testing.T, requireBootcLint bool) {
	t.Helper()

	// Create Containerfile with optional bootc lint
	var containerfileContent string
	if requireBootcLint {
		containerfileContent = fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN bootc container lint
RUN echo "validated" > /etc/validated
`, testutil.TestBootcImageCurrent())
	} else {
		containerfileContent = fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN echo "no-lint" > /etc/no-lint
`, testutil.TestBootcImageCurrent())
	}

	e.containerfile = filepath.Join(e.workDir, "Containerfile")
	if err := os.WriteFile(e.containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	// Create validate-focused bootc-ci.yaml
	pipelineContent := fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: e2e-validate-test

spec:
  source:
    containerfile: ./Containerfile
    context: .

  validate:
    containerfileLint:
      enabled: true
      requireBootcLint: %t
      warnIfMissing: true
      failIfMissing: false
`, requireBootcLint)

	e.pipelineFile = filepath.Join(e.workDir, "bootc-ci.yaml")
	if err := os.WriteFile(e.pipelineFile, []byte(pipelineContent), 0644); err != nil {
		t.Fatalf("Failed to create bootc-ci.yaml: %v", err)
	}
}

// SetupScanPipeline creates a pipeline focused on scan stage testing
func (e *CITestEnvironment) SetupScanPipeline(t *testing.T, enableVulnerability, enableSBOM bool) {
	t.Helper()

	// Create Containerfile
	containerfileContent := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN echo "scan-test" > /etc/scan-test
`, testutil.TestBootcImageCurrent())

	e.containerfile = filepath.Join(e.workDir, "Containerfile")
	if err := os.WriteFile(e.containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	// Create scan-focused bootc-ci.yaml
	pipelineContent := fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: e2e-scan-test

spec:
  source:
    containerfile: ./Containerfile
    context: .

  build:
    imageTag: %s

  scan:
    vulnerability:
      enabled: %t
      tool: trivy
      severity: HIGH,CRITICAL
      failOnVulnerability: false

    sbom:
      enabled: %t
      tool: syft
      format: spdx-json
`, e.imageTag, enableVulnerability, enableSBOM)

	e.pipelineFile = filepath.Join(e.workDir, "bootc-ci.yaml")
	if err := os.WriteFile(e.pipelineFile, []byte(pipelineContent), 0644); err != nil {
		t.Fatalf("Failed to create bootc-ci.yaml: %v", err)
	}
}

// SetupBuildScanPipeline creates a pipeline for build and scan stages
func (e *CITestEnvironment) SetupBuildScanPipeline(t *testing.T) {
	t.Helper()
	e.SetupScanPipeline(t, true, true)
}

// RunCICommand runs bootc-man ci with the given arguments
func (e *CITestEnvironment) RunCICommand(args ...string) (string, error) {
	ciArgs := append([]string{"ci"}, args...)
	return e.RunBootcMan(ciArgs...)
}

// RunCIRun runs bootc-man ci run with the given arguments
func (e *CITestEnvironment) RunCIRun(args ...string) (string, error) {
	runArgs := append([]string{"run"}, args...)
	return e.RunCICommand(runArgs...)
}

// RunCIStage runs a specific CI stage
func (e *CITestEnvironment) RunCIStage(stage string, extraArgs ...string) (string, error) {
	args := append([]string{"--stage", stage}, extraArgs...)
	return e.RunCIRun(args...)
}

// CleanupImage removes the test image if it exists
func (e *CITestEnvironment) CleanupImage() {
	if e.imageTag != "" {
		_, _ = e.RunCommand("podman", "rmi", "-f", e.imageTag)
	}
}

// GetPipelineFile returns the path to the pipeline file
func (e *CITestEnvironment) GetPipelineFile() string {
	return e.pipelineFile
}

// GetContainerfile returns the path to the Containerfile
func (e *CITestEnvironment) GetContainerfile() string {
	return e.containerfile
}

// GetImageTag returns the test image tag
func (e *CITestEnvironment) GetImageTag() string {
	return e.imageTag
}

// === Basic CI E2E Tests ===

// TestCIEnvironmentSetup verifies that the CI test environment can be created
func TestCIEnvironmentSetup(t *testing.T) {
	env := NewCITestEnvironment(t)

	if env.workDir == "" {
		t.Fatal("workDir should not be empty")
	}

	if env.imageTag == "" {
		t.Fatal("imageTag should not be empty")
	}

	t.Logf("CI test environment created:")
	t.Logf("  workDir: %s", env.workDir)
	t.Logf("  imageTag: %s", env.imageTag)
}

// TestCIPipelineFileCreation verifies that pipeline files can be created
func TestCIPipelineFileCreation(t *testing.T) {
	env := NewCITestEnvironment(t)
	env.SetupMinimalPipeline(t)

	// Verify files exist
	if _, err := os.Stat(env.GetPipelineFile()); os.IsNotExist(err) {
		t.Fatal("bootc-ci.yaml was not created")
	}

	if _, err := os.Stat(env.GetContainerfile()); os.IsNotExist(err) {
		t.Fatal("Containerfile was not created")
	}

	t.Logf("Pipeline file: %s", env.GetPipelineFile())
	t.Logf("Containerfile: %s", env.GetContainerfile())
}

// TestCIValidatePipelineCreation verifies validate-focused pipeline creation
func TestCIValidatePipelineCreation(t *testing.T) {
	env := NewCITestEnvironment(t)
	env.SetupValidatePipeline(t, true)

	// Verify Containerfile contains bootc lint
	content, err := os.ReadFile(env.GetContainerfile())
	if err != nil {
		t.Fatalf("Failed to read Containerfile: %v", err)
	}

	if !testutil.ContainsString(string(content), "bootc container lint") {
		t.Error("Containerfile should contain 'bootc container lint'")
	}

	t.Log("Validate pipeline with bootc lint created successfully")
}

// TestCIScanPipelineCreation verifies scan-focused pipeline creation
func TestCIScanPipelineCreation(t *testing.T) {
	env := NewCITestEnvironment(t)
	env.SetupScanPipeline(t, true, true)

	// Verify pipeline file contains scan configuration
	content, err := os.ReadFile(env.GetPipelineFile())
	if err != nil {
		t.Fatalf("Failed to read pipeline file: %v", err)
	}

	if !testutil.ContainsString(string(content), "vulnerability:") {
		t.Error("Pipeline should contain vulnerability config")
	}

	if !testutil.ContainsString(string(content), "sbom:") {
		t.Error("Pipeline should contain SBOM config")
	}

	t.Log("Scan pipeline created successfully")
}

// TestCIDryRun tests that ci run --dry-run works
func TestCIDryRun(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewCITestEnvironment(t)
	env.SetupMinimalPipeline(t)

	// Run with --dry-run
	output, err := env.RunCIRun("--dry-run")
	if err != nil {
		t.Logf("Dry run output: %s", output)
		// Dry run might fail if binary not found, that's OK for this test
		if findBootcManBinary() == "" {
			t.Skip("bootc-man binary not found")
		}
		t.Fatalf("Dry run failed: %v", err)
	}

	t.Logf("Dry run output: OK (%d lines)", strings.Count(output, "\n"))
	t.Log("CI dry run test completed")
}

// === Phase 2: Stage-specific E2E Tests ===

// TestCIValidateHadolint tests hadolint execution in validate stage
func TestCIValidateHadolint(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfHadolintUnavailable(t)
	testutil.SkipIfShort(t)

	env := NewCITestEnvironment(t)
	env.SetupValidatePipeline(t, false) // without bootc lint requirement

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// Run validate stage
	output, err := env.RunCIStage("validate")
	if err != nil {
		t.Logf("Validate output: %s", output)
		// Non-zero exit is OK for linting, just log
		t.Logf("Validate stage returned error (may be expected): %v", err)
	}

	t.Logf("Validate stage output: OK (%d lines)", strings.Count(output, "\n"))
	t.Log("Hadolint validate test completed")
}

// TestCIBuildWithPipeline tests build stage using bootc-ci.yaml
func TestCIBuildWithPipeline(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfShort(t)

	env := NewCITestEnvironment(t)
	env.SetupMinimalPipeline(t)
	defer env.CleanupImage()

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// Run build stage
	output, err := env.RunCIStage("build")
	if err != nil {
		t.Logf("Build output: %s", output)
		t.Fatalf("Build stage failed: %v", err)
	}

	// Verify image was created
	inspectOutput, err := env.RunCommand("podman", "image", "exists", env.GetImageTag())
	if err != nil {
		t.Logf("Image exists check: %s", inspectOutput)
		t.Fatalf("Image was not created: %v", err)
	}

	t.Logf("Build stage output: OK (%d lines)", strings.Count(output, "\n"))
	t.Log("Build with pipeline test completed")
}

// TestCIScanTrivy tests vulnerability scanning with trivy
func TestCIScanTrivy(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfTrivyUnavailable(t)
	testutil.SkipIfShort(t)

	env := NewCITestEnvironment(t)
	env.SetupScanPipeline(t, true, false) // vulnerability only
	defer env.CleanupImage()

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// First build the image
	_, err := env.RunCIStage("build")
	if err != nil {
		t.Fatalf("Build stage failed (prerequisite): %v", err)
	}

	// Run scan stage
	output, err := env.RunCIStage("scan")
	if err != nil {
		t.Logf("Scan output: %s", output)
		// Vulnerabilities found is OK, just log
		t.Logf("Scan stage returned error (may be expected for vulnerabilities): %v", err)
	}

	t.Logf("Scan stage output: OK (%d lines)", strings.Count(output, "\n"))
	t.Log("Trivy scan test completed")
}

// TestCIScanSBOM tests SBOM generation with syft
func TestCIScanSBOM(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfSyftUnavailable(t)
	testutil.SkipIfShort(t)

	env := NewCITestEnvironment(t)
	env.SetupScanPipeline(t, false, true) // SBOM only
	defer env.CleanupImage()

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// First build the image
	_, err := env.RunCIStage("build")
	if err != nil {
		t.Fatalf("Build stage failed (prerequisite): %v", err)
	}

	// Run scan stage
	output, err := env.RunCIStage("scan")
	if err != nil {
		t.Logf("SBOM output: %s", output)
		t.Fatalf("SBOM generation failed: %v", err)
	}

	t.Logf("SBOM stage output: OK (%d lines)", strings.Count(output, "\n"))
	t.Log("SBOM generation test completed")
}

// === Phase 3: Integration Tests ===

// TestCIPipelineValidateBuildScan tests 3 stages in sequence
func TestCIPipelineValidateBuildScan(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfShort(t)

	env := NewCITestEnvironment(t)
	env.SetupBuildScanPipeline(t)
	defer env.CleanupImage()

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// Run stages as subtests for clear per-stage pass/fail reporting
	t.Run("validate", func(t *testing.T) {
		output, err := env.RunCIStage("validate")
		if err != nil {
			t.Logf("Validate output: %s", output)
			// Continue even if validate has warnings
		}
	})

	t.Run("build", func(t *testing.T) {
		output, err := env.RunCIStage("build")
		if err != nil {
			t.Logf("Build output: %s", output)
			t.Fatalf("Build stage failed: %v", err)
		}
	})

	t.Run("scan", func(t *testing.T) {
		output, err := env.RunCIStage("scan")
		if err != nil {
			t.Logf("Scan output: %s", output)
			// Vulnerabilities found is OK
		}
	})
}

// TestCIPipelineDryRunAllStages tests dry-run for all stages
func TestCIPipelineDryRunAllStages(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewCITestEnvironment(t)
	env.SetupBuildScanPipeline(t)

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	stages := []string{"build", "scan"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			output, err := env.RunCIRun("--dry-run", "--stage", stage)
			if err != nil {
				t.Logf("Dry run output for %s: %s", stage, output)
				t.Fatalf("Dry run failed for stage %s: %v", stage, err)
			}
			t.Logf("Stage %s dry-run output: OK (%d lines)", stage, strings.Count(output, "\n"))
		})
	}
}

// === Phase 4: Advanced Tests (Linux/CI Environment) ===

// TestCIConvertRaw tests disk image conversion
// On macOS, runs inside Podman Machine. On Linux, runs directly.
func TestCIConvertRaw(t *testing.T) {
	skipIfConvertUnavailable(t)
	testutil.SkipIfShort(t)

	env := NewCITestEnvironment(t)
	defer env.CleanupImage()

	// Create pipeline with convert stage
	containerfileContent := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN echo "convert-test" > /etc/convert-test
`, testutil.TestBootcImageCurrent())

	containerfile := filepath.Join(env.workDir, "Containerfile")
	if err := os.WriteFile(containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	pipelineContent := fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: e2e-convert-test

spec:
  source:
    containerfile: ./Containerfile
    context: .

  build:
    imageTag: %s

  convert:
    enabled: true
    formats:
      - type: raw
`, env.GetImageTag())

	pipelineFile := filepath.Join(env.workDir, "bootc-ci.yaml")
	if err := os.WriteFile(pipelineFile, []byte(pipelineContent), 0644); err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// Build first
	_, err := env.RunCIStage("build")
	if err != nil {
		t.Fatalf("Build stage failed: %v", err)
	}

	// Run convert stage
	output, err := env.RunCIStage("convert")
	if err != nil {
		t.Logf("Convert output: %s", output)
		t.Fatalf("Convert stage failed: %v", err)
	}

	// Check output file exists
	outputDir := filepath.Join(env.workDir, "output")
	files, err := os.ReadDir(outputDir)
	if err != nil || len(files) == 0 {
		t.Fatalf("No output files created in %s", outputDir)
	}

	t.Logf("Convert stage output: OK (%d lines)", strings.Count(output, "\n"))
	t.Log("Disk image conversion test completed")
}

// TestCIPipelineFull tests validate, build, scan, convert stages in sequence
// On macOS, convert runs inside Podman Machine. On Linux, runs directly.
func TestCIPipelineFull(t *testing.T) {
	skipIfConvertUnavailable(t)
	testutil.SkipIfShort(t)

	env := NewCITestEnvironment(t)
	defer env.CleanupImage()

	// Create full pipeline
	containerfileContent := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN bootc container lint || true
RUN echo "full-test" > /etc/full-test
`, testutil.TestBootcImageCurrent())

	containerfile := filepath.Join(env.workDir, "Containerfile")
	if err := os.WriteFile(containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	pipelineContent := fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: e2e-full-test

spec:
  source:
    containerfile: ./Containerfile
    context: .

  validate:
    containerfileLint:
      enabled: true
      requireBootcLint: false

  build:
    imageTag: %s

  scan:
    vulnerability:
      enabled: true
      failOnVulnerability: false
    sbom:
      enabled: true

  convert:
    enabled: true
    formats:
      - type: raw
`, env.GetImageTag())

	pipelineFile := filepath.Join(env.workDir, "bootc-ci.yaml")
	if err := os.WriteFile(pipelineFile, []byte(pipelineContent), 0644); err != nil {
		t.Fatalf("Failed to create pipeline: %v", err)
	}

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// Run stages as subtests for clear per-stage pass/fail reporting
	stages := []string{"validate", "build", "scan", "convert"}
	for _, stage := range stages {
		stage := stage // capture loop variable
		t.Run(stage, func(t *testing.T) {
			output, err := env.RunCIStage(stage)
			if err != nil {
				t.Logf("Stage %s output: %s", stage, output)
				// validate and scan may have non-zero exit for warnings
				if stage == "build" || stage == "convert" {
					t.Fatalf("Stage %s failed: %v", stage, err)
				}
			}
		})
	}
}

// === Phase 5: Test and Release Stage Tests ===

// SetupConvertPipeline creates a pipeline for convert stage testing with SSH key injection
func (e *CITestEnvironment) SetupConvertPipeline(t *testing.T) {
	t.Helper()

	// Create Containerfile with user for SSH access
	containerfileContent := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN useradd -m -G wheel user && \
    echo "user ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/user
`, testutil.TestBootcImageCurrent())

	e.containerfile = filepath.Join(e.workDir, "Containerfile")
	if err := os.WriteFile(e.containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	// Create config.toml with SSH key injection
	sshPubKey := getHostSSHPublicKey(t)
	_, err := createConfigToml(e.workDir, sshPubKey)
	if err != nil {
		t.Fatalf("Failed to create config.toml: %v", err)
	}

	// Create pipeline with build, convert, and test stages
	pipelineContent := fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: e2e-test-stage

spec:
  source:
    containerfile: ./Containerfile
    context: .

  build:
    imageTag: %s

  convert:
    enabled: true
    formats:
      - type: raw
        config: config.toml

  test:
    boot:
      enabled: true
      timeout: 300
      checks:
        - cat /etc/os-release
`, e.imageTag)

	e.pipelineFile = filepath.Join(e.workDir, "bootc-ci.yaml")
	if err := os.WriteFile(e.pipelineFile, []byte(pipelineContent), 0644); err != nil {
		t.Fatalf("Failed to create bootc-ci.yaml: %v", err)
	}
}

// SetupReleasePipeline creates a pipeline for release stage testing with local registry
func (e *CITestEnvironment) SetupReleasePipeline(t *testing.T) {
	t.Helper()

	containerfileContent := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN echo "release-test" > /etc/release-test
`, testutil.TestBootcImageCurrent())

	e.containerfile = filepath.Join(e.workDir, "Containerfile")
	if err := os.WriteFile(e.containerfile, []byte(containerfileContent), 0644); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	// Release to local registry (no TLS)
	pipelineContent := fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: e2e-release-test

spec:
  source:
    containerfile: ./Containerfile
    context: .

  build:
    imageTag: %s

  release:
    registry: localhost:5000
    repository: e2e-release-test
    tls: false
    tags:
      - latest
      - v0.0.1-test
`, e.imageTag)

	e.pipelineFile = filepath.Join(e.workDir, "bootc-ci.yaml")
	if err := os.WriteFile(e.pipelineFile, []byte(pipelineContent), 0644); err != nil {
		t.Fatalf("Failed to create bootc-ci.yaml: %v", err)
	}
}

// TestCITestStageDryRun tests dry-run of the test stage configuration
func TestCITestStageDryRun(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewCITestEnvironment(t)
	env.SetupConvertPipeline(t)

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	output, err := env.RunCIRun("--dry-run", "--stage", "test")
	if err != nil {
		t.Logf("Dry run output: %s", output)
		t.Fatalf("Test stage dry-run failed: %v", err)
	}
	t.Logf("Test stage dry-run: OK (%d lines)", strings.Count(output, "\n"))
}

// TestCIPipelineWithTestStage tests build → convert → test stages in sequence
// This verifies the full pipeline through VM boot and SSH health checks.
func TestCIPipelineWithTestStage(t *testing.T) {
	skipIfConvertUnavailable(t)
	testutil.SkipIfShort(t)
	RequireVMInfrastructure(t)

	env := NewCITestEnvironment(t)
	env.SetupConvertPipeline(t)
	defer env.CleanupImage()

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// Ensure local registry is available for image transfer
	registryOutput, err := env.RunBootcMan("registry", "up")
	if err != nil {
		t.Fatalf("Failed to start registry: %v\nOutput: %s", err, registryOutput)
	}
	env.AddCleanup(func() {
		_, _ = env.RunBootcMan("registry", "down")
	})

	// Run build → convert → test as subtests
	stages := []string{"build", "convert", "test"}
	for _, stage := range stages {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			output, err := env.RunCIStage(stage)
			if err != nil {
				t.Logf("Stage %s output: %s", stage, output)
				t.Fatalf("Stage %s failed: %v", stage, err)
			}
			t.Logf("Stage %s: OK (%d lines)", stage, strings.Count(output, "\n"))
		})
	}
}

// TestCIReleaseStageDryRun tests dry-run of the release stage configuration
func TestCIReleaseStageDryRun(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewCITestEnvironment(t)
	env.SetupReleasePipeline(t)

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	output, err := env.RunCIRun("--dry-run", "--stage", "release")
	if err != nil {
		t.Logf("Dry run output: %s", output)
		t.Fatalf("Release stage dry-run failed: %v", err)
	}
	t.Logf("Release stage dry-run: OK (%d lines)", strings.Count(output, "\n"))
}

// TestCIReleaseToLocalRegistry tests build → release pipeline with local registry
func TestCIReleaseToLocalRegistry(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)
	testutil.SkipIfShort(t)

	env := NewCITestEnvironment(t)
	env.SetupReleasePipeline(t)
	defer env.CleanupImage()

	if findBootcManBinary() == "" {
		t.Skip("bootc-man binary not found")
	}

	// Start local registry
	registryOutput, err := env.RunBootcMan("registry", "up")
	if err != nil {
		t.Fatalf("Failed to start registry: %v\nOutput: %s", err, registryOutput)
	}
	env.AddCleanup(func() {
		_, _ = env.RunBootcMan("registry", "down")
	})

	// Run build → release as subtests
	t.Run("build", func(t *testing.T) {
		output, err := env.RunCIStage("build")
		if err != nil {
			t.Logf("Build output: %s", output)
			t.Fatalf("Build stage failed: %v", err)
		}
		t.Logf("Build: OK (%d lines)", strings.Count(output, "\n"))
	})

	t.Run("release", func(t *testing.T) {
		output, err := env.RunCIStage("release")
		if err != nil {
			t.Logf("Release output: %s", output)
			t.Fatalf("Release stage failed: %v", err)
		}
		t.Logf("Release: OK (%d lines)", strings.Count(output, "\n"))
	})
}
