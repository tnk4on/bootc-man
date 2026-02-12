// Package ci provides CI pipeline definition and execution
package ci

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tnk4on/bootc-man/internal/config"
	"gopkg.in/yaml.v3"
)

// Pipeline represents a bootc-man CI pipeline definition
type Pipeline struct {
	APIVersion string           `yaml:"apiVersion"`
	Kind       string           `yaml:"kind"`
	Metadata   PipelineMetadata `yaml:"metadata"`
	Spec       PipelineSpec     `yaml:"spec"`
	baseDir    string           // Directory of the pipeline file (for resolving relative paths)
}

// PipelineMetadata contains pipeline metadata
type PipelineMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// PipelineSpec contains the pipeline specification
type PipelineSpec struct {
	Source    SourceConfig     `yaml:"source"`
	BaseImage *BaseImageConfig `yaml:"baseImage,omitempty"`
	Validate  *ValidateConfig  `yaml:"validate,omitempty"`
	Build     *BuildConfig     `yaml:"build,omitempty"`
	Scan      *ScanConfig      `yaml:"scan,omitempty"`
	Convert   *ConvertConfig   `yaml:"convert,omitempty"`
	Test      *TestConfig      `yaml:"test,omitempty"`
	Release   *ReleaseConfig   `yaml:"release,omitempty"`
}

// SourceConfig defines source files
type SourceConfig struct {
	Containerfile string `yaml:"containerfile"`
	Context       string `yaml:"context"`
}

// BaseImageConfig defines base image settings
type BaseImageConfig struct {
	Ref    string `yaml:"ref,omitempty"`
	Digest string `yaml:"digest,omitempty"`
}

// ValidateConfig defines validate stage settings
type ValidateConfig struct {
	ContainerfileLint *ContainerfileLintConfig `yaml:"containerfileLint,omitempty"`
	ConfigToml        *ConfigTomlConfig        `yaml:"configToml,omitempty"`
	SecretDetection   *SecretDetectionConfig   `yaml:"secretDetection,omitempty"`
}

// ContainerfileLintConfig defines Containerfile lint settings
type ContainerfileLintConfig struct {
	Enabled          bool `yaml:"enabled"`
	RequireBootcLint bool `yaml:"requireBootcLint,omitempty"`
	WarnIfMissing    bool `yaml:"warnIfMissing,omitempty"`
	FailIfMissing    bool `yaml:"failIfMissing,omitempty"`
}

// ConfigTomlConfig defines Config TOML validation settings
type ConfigTomlConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path,omitempty"`
}

// SecretDetectionConfig defines secret detection settings
type SecretDetectionConfig struct {
	Enabled bool   `yaml:"enabled"`
	Tool    string `yaml:"tool,omitempty"` // gitleaks or trufflehog
}

// BuildConfig defines build stage settings
type BuildConfig struct {
	ImageTag  string            `yaml:"imageTag,omitempty"` // Custom image tag (overrides auto-generated tag)
	Platforms []string          `yaml:"platforms,omitempty"`
	Args      map[string]string `yaml:"args,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty"`
}

// ScanConfig defines scan stage settings
type ScanConfig struct {
	Vulnerability *VulnerabilityConfig `yaml:"vulnerability,omitempty"`
	SBOM          *SBOMConfig          `yaml:"sbom,omitempty"`
	Lint          *LintConfig          `yaml:"lint,omitempty"`
}

// VulnerabilityConfig defines vulnerability scan settings
type VulnerabilityConfig struct {
	Enabled             bool   `yaml:"enabled"`
	Tool                string `yaml:"tool,omitempty"` // trivy (default) or grype
	Severity            string `yaml:"severity,omitempty"`
	FailOnVulnerability bool   `yaml:"failOnVulnerability,omitempty"`
	SkipDbUpdate        bool   `yaml:"skipDbUpdate,omitempty"` // skip DB update for offline mode
}

// SBOMConfig defines SBOM generation settings
type SBOMConfig struct {
	Enabled bool   `yaml:"enabled"`
	Tool    string `yaml:"tool,omitempty"`   // syft (default) or trivy
	Format  string `yaml:"format,omitempty"` // spdx-json, cyclonedx-json
}

// LintConfig defines lint settings
type LintConfig struct {
	Enabled bool `yaml:"enabled"`
}

// ConvertConfig defines convert stage settings
type ConvertConfig struct {
	Enabled            bool            `yaml:"enabled"`
	Formats            []ConvertFormat `yaml:"formats,omitempty"`
	InsecureRegistries []string        `yaml:"insecureRegistries,omitempty"` // Registries to configure as insecure (HTTP) in the VM image
}

// ConvertFormat defines a conversion format
type ConvertFormat struct {
	Type   string `yaml:"type"` // qcow2, ami, vmdk, raw, iso
	Config string `yaml:"config,omitempty"`
}

// TestConfig defines test stage settings
type TestConfig struct {
	Boot     *BootTestConfig     `yaml:"boot,omitempty"`
	Upgrade  *UpgradeTestConfig  `yaml:"upgrade,omitempty"`
	Rollback *RollbackTestConfig `yaml:"rollback,omitempty"`
}

// BootTestConfig defines boot test settings
type BootTestConfig struct {
	Enabled bool     `yaml:"enabled"`
	Timeout int      `yaml:"timeout,omitempty"`
	Checks  []string `yaml:"checks,omitempty"`
	GUI     bool     `yaml:"gui,omitempty"` // Display VM console in GUI window (macOS only)
}

// UpgradeTestConfig defines upgrade test settings
type UpgradeTestConfig struct {
	Enabled   bool     `yaml:"enabled"`
	FromImage string   `yaml:"fromImage,omitempty"`
	Checks    []string `yaml:"checks,omitempty"`
}

// RollbackTestConfig defines rollback test settings
type RollbackTestConfig struct {
	Enabled bool     `yaml:"enabled"`
	Checks  []string `yaml:"checks,omitempty"`
}

// ReleaseConfig defines release stage settings
type ReleaseConfig struct {
	Registry   string      `yaml:"registry"`
	Repository string      `yaml:"repository"`
	TLS        *bool       `yaml:"tls,omitempty"` // Enable TLS verification (default: true)
	Sign       *SignConfig `yaml:"sign,omitempty"`
	Tags       []string    `yaml:"tags,omitempty"`
}

// SignConfig defines image signing settings
type SignConfig struct {
	Enabled         bool                   `yaml:"enabled"`
	Key             string                 `yaml:"key,omitempty"`
	TransparencyLog *TransparencyLogConfig `yaml:"transparencyLog,omitempty"`
}

// TransparencyLogConfig defines transparency log settings for cosign
type TransparencyLogConfig struct {
	Enabled  bool   `yaml:"enabled"`            // Enable transparency log upload (default: false for offline/PoC)
	RekorURL string `yaml:"rekorUrl,omitempty"` // Custom Rekor URL for private instance
}

// LoadPipeline loads a pipeline definition from a YAML file
func LoadPipeline(path string) (*Pipeline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read pipeline file %s: %w", path, err)
	}

	var pipeline Pipeline
	if err := yaml.Unmarshal(data, &pipeline); err != nil {
		return nil, fmt.Errorf("failed to parse pipeline file %s: %w", path, err)
	}

	// Set base directory for resolving relative paths
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve pipeline file path: %w", err)
	}
	pipeline.baseDir = filepath.Dir(absPath)

	// Validate basic structure
	if err := pipeline.Validate(); err != nil {
		return nil, fmt.Errorf("invalid pipeline definition: %w", err)
	}

	return &pipeline, nil
}

// Validate validates the pipeline definition
func (p *Pipeline) Validate() error {
	if p.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if p.APIVersion != config.PipelineAPIVersion {
		return fmt.Errorf("unsupported apiVersion: %s (expected %s)", p.APIVersion, config.PipelineAPIVersion)
	}

	if p.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if p.Kind != config.PipelineKind {
		return fmt.Errorf("unsupported kind: %s (expected %s)", p.Kind, config.PipelineKind)
	}

	if p.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}

	if p.Spec.Source.Containerfile == "" {
		return fmt.Errorf("spec.source.containerfile is required")
	}

	// Validate file paths exist
	if err := p.validatePaths(); err != nil {
		return err
	}

	return nil
}

// validatePaths checks that referenced files exist
func (p *Pipeline) validatePaths() error {
	// Containerfile path validation
	containerfilePath, err := p.ResolveContainerfilePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(containerfilePath); os.IsNotExist(err) {
		return fmt.Errorf("containerfile not found: %s", containerfilePath)
	}

	// Config TOML path validation (if enabled)
	if p.Spec.Validate != nil && p.Spec.Validate.ConfigToml != nil && p.Spec.Validate.ConfigToml.Enabled {
		if p.Spec.Validate.ConfigToml.Path != "" {
			if _, err := os.Stat(p.Spec.Validate.ConfigToml.Path); os.IsNotExist(err) {
				return fmt.Errorf("config.toml not found: %s", p.Spec.Validate.ConfigToml.Path)
			}
		}
	}

	return nil
}

// ResolveContainerfilePath returns the absolute path to the Containerfile
// This follows the same pattern as Buildah/Podman:
// 1. If Containerfile path is absolute, use it as-is
// 2. If Containerfile path is relative, try to resolve it relative to pipeline base directory first
// 3. If not found, try resolving relative to context directory (Buildah pattern)
func (p *Pipeline) ResolveContainerfilePath() (string, error) {
	containerfilePath := p.Spec.Source.Containerfile
	if filepath.IsAbs(containerfilePath) {
		return containerfilePath, nil
	}

	// First, try resolving relative to pipeline base directory
	baseResolved := filepath.Join(p.baseDir, containerfilePath)
	baseResolved, _ = filepath.Abs(baseResolved)
	if _, err := os.Stat(baseResolved); err == nil {
		return baseResolved, nil
	}

	// If not found, resolve context path and try relative to context (Buildah pattern)
	context := p.Spec.Source.Context
	if context == "" {
		context = "."
	}

	var contextPath string
	if filepath.IsAbs(context) {
		contextPath = context
	} else {
		contextPath = filepath.Join(p.baseDir, context)
	}
	contextPath, _ = filepath.Abs(contextPath)

	// Try resolving containerfile relative to context
	// Clean the path to handle "./" prefixes
	cleanPath := strings.TrimPrefix(containerfilePath, "./")

	// If containerfile path already includes context, remove the context prefix
	cleanContext := strings.TrimPrefix(context, "./")
	if strings.HasPrefix(cleanPath, cleanContext+string(filepath.Separator)) {
		// Remove context prefix
		cleanPath = strings.TrimPrefix(cleanPath, cleanContext+string(filepath.Separator))
	}

	resolvedPath := filepath.Join(contextPath, cleanPath)
	absPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve containerfile path: %w", err)
	}

	return absPath, nil
}

// ResolveContextPath returns the absolute path to the build context
func (p *Pipeline) ResolveContextPath() (string, error) {
	context := p.Spec.Source.Context
	if context == "" {
		context = "."
	}

	// Resolve context relative to pipeline file's base directory
	var contextPath string
	if filepath.IsAbs(context) {
		contextPath = context
	} else {
		contextPath = filepath.Join(p.baseDir, context)
	}

	absPath, err := filepath.Abs(contextPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve context path: %w", err)
	}

	return absPath, nil
}

// BaseDir returns the base directory of the pipeline file
func (p *Pipeline) BaseDir() string {
	return p.baseDir
}
