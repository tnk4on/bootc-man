package ci

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

func TestNewValidateStage(t *testing.T) {
	dir := testutil.SetupPipelineTestDir(t)
	pipeline := &Pipeline{
		baseDir: dir,
		Spec: PipelineSpec{
			Source: SourceConfig{
				Containerfile: "Containerfile",
				Context:       ".",
			},
			Validate: &ValidateConfig{
				ContainerfileLint: &ContainerfileLintConfig{
					Enabled: true,
				},
			},
		},
	}

	stage := NewValidateStage(pipeline, nil, true)
	if stage == nil {
		t.Fatal("NewValidateStage returned nil")
	}
	if stage.pipeline != pipeline {
		t.Error("stage.pipeline not set correctly")
	}
	if !stage.verbose {
		t.Error("stage.verbose should be true")
	}
}

func TestValidateStageNotConfigured(t *testing.T) {
	dir := testutil.SetupPipelineTestDir(t)
	pipeline := &Pipeline{
		baseDir: dir,
		Spec: PipelineSpec{
			Source: SourceConfig{
				Containerfile: "Containerfile",
				Context:       ".",
			},
			// Validate is nil
		},
	}

	stage := NewValidateStage(pipeline, nil, false)
	err := stage.Execute(context.Background())

	if err == nil {
		t.Fatal("expected error for unconfigured validate stage")
	}
	if !containsString(err.Error(), "not configured") {
		t.Errorf("error should mention 'not configured': %v", err)
	}
}

func TestContainerfileLintConfigDefaults(t *testing.T) {
	cfg := &ContainerfileLintConfig{
		Enabled: true,
		// Other fields use defaults
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	// RequireBootcLint defaults to false (zero value)
	if cfg.RequireBootcLint {
		t.Error("RequireBootcLint should default to false")
	}
}

func TestConfigTomlConfigDefaults(t *testing.T) {
	cfg := &ConfigTomlConfig{
		Enabled: true,
		Path:    "config.toml",
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Path != "config.toml" {
		t.Errorf("Path = %q, want %q", cfg.Path, "config.toml")
	}
}

func TestSecretDetectionConfigDefaults(t *testing.T) {
	cfg := &SecretDetectionConfig{
		Enabled: true,
		Tool:    "gitleaks",
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Tool != "gitleaks" {
		t.Errorf("Tool = %q, want %q", cfg.Tool, "gitleaks")
	}
}

func TestValidateConfigStructure(t *testing.T) {
	cfg := &ValidateConfig{
		ContainerfileLint: &ContainerfileLintConfig{
			Enabled:          true,
			RequireBootcLint: true,
			FailIfMissing:    false,
		},
		ConfigToml: &ConfigTomlConfig{
			Enabled: true,
			Path:    "/etc/config.toml",
		},
		SecretDetection: &SecretDetectionConfig{
			Enabled: true,
			Tool:    "trufflehog",
		},
	}

	if !cfg.ContainerfileLint.Enabled {
		t.Error("ContainerfileLint.Enabled should be true")
	}
	if !cfg.ContainerfileLint.RequireBootcLint {
		t.Error("ContainerfileLint.RequireBootcLint should be true")
	}
	if cfg.ContainerfileLint.FailIfMissing {
		t.Error("ContainerfileLint.FailIfMissing should be false")
	}

	if !cfg.ConfigToml.Enabled {
		t.Error("ConfigToml.Enabled should be true")
	}
	if cfg.ConfigToml.Path != "/etc/config.toml" {
		t.Errorf("ConfigToml.Path = %q, want %q", cfg.ConfigToml.Path, "/etc/config.toml")
	}

	if !cfg.SecretDetection.Enabled {
		t.Error("SecretDetection.Enabled should be true")
	}
	if cfg.SecretDetection.Tool != "trufflehog" {
		t.Errorf("SecretDetection.Tool = %q, want %q", cfg.SecretDetection.Tool, "trufflehog")
	}
}

// TestParseHadolintOutput tests the pure function for parsing hadolint output
func TestParseHadolintOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		hasError bool
	}{
		{
			name:     "empty output",
			output:   "",
			hasError: false,
		},
		{
			name:     "warnings only",
			output:   "-:5 DL3008 warning: Pin versions in apt get install\n-:10 DL3015 info: Avoid additional packages",
			hasError: false,
		},
		{
			name:     "single error",
			output:   "-:1 DL3000 error: Use absolute WORKDIR",
			hasError: true,
		},
		{
			name:     "mixed warnings and errors",
			output:   "-:5 DL3008 warning: Pin versions in apt get install\n-:8 DL3000 error: Use absolute WORKDIR\n-:10 DL3015 info: Avoid additional packages",
			hasError: true,
		},
		{
			name:     "word error in message should not match",
			output:   "-:5 DL3008 warning: This might cause an error in production",
			hasError: false,
		},
		{
			name:     "style level only",
			output:   "-:3 DL3006 style: Always tag the version of an image explicitly",
			hasError: false,
		},
		{
			name:     "info level only",
			output:   "-:7 DL4006 info: Set the SHELL option -o pipefail",
			hasError: false,
		},
		{
			name:     "multiple errors",
			output:   "-:1 DL3000 error: Use absolute WORKDIR\n-:5 DL3001 error: For some images, pip or pip3 does not exist",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseHadolintOutput(tt.output)
			if got != tt.hasError {
				t.Errorf("ParseHadolintOutput() = %v, want %v", got, tt.hasError)
			}
		})
	}
}

// TestContainsBootcLint tests the pure function for checking bootc lint patterns
func TestContainsBootcLint(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{
			name:    "contains bootc container lint",
			content: "RUN bootc container lint",
			want:    true,
		},
		{
			name:    "contains bootc-container-lint hyphenated",
			content: "RUN bootc-container-lint",
			want:    true,
		},
		{
			name:    "case insensitive - uppercase",
			content: "RUN BOOTC CONTAINER LINT",
			want:    true,
		},
		{
			name:    "case insensitive - mixed case",
			content: "RUN Bootc Container Lint",
			want:    true,
		},
		{
			name:    "with sudo prefix",
			content: "RUN sudo bootc container lint",
			want:    true,
		},
		{
			name:    "in comment",
			content: "# RUN bootc container lint",
			want:    true,
		},
		{
			name:    "not present",
			content: "RUN dnf install -y httpd",
			want:    false,
		},
		{
			name:    "partial match - bootc only",
			content: "RUN bootc status",
			want:    false,
		},
		{
			name:    "partial match - lint only",
			content: "RUN lint check",
			want:    false,
		},
		{
			name:    "empty content",
			content: "",
			want:    false,
		},
		{
			name:    "multi-line with bootc lint",
			content: "FROM fedora\nRUN dnf update\nRUN bootc container lint\nCMD [\"bash\"]",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsBootcLint(tt.content)
			if got != tt.want {
				t.Errorf("ContainsBootcLint() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCheckBootcLintConfig tests the Containerfile parsing for bootc lint check
func TestCheckBootcLintConfig(t *testing.T) {
	tests := []struct {
		name                 string
		containerfileContent string
		requireBootcLint     bool
		expectError          bool
	}{
		{
			name: "contains bootc container lint",
			containerfileContent: fmt.Sprintf(`FROM %s
RUN dnf install -y httpd
RUN bootc container lint
`, testutil.TestBootcImageCurrent()),
			requireBootcLint: true,
			expectError:      false,
		},
		{
			name: "contains bootc container lint with sudo",
			containerfileContent: fmt.Sprintf(`FROM %s
RUN sudo bootc container lint
`, testutil.TestBootcImageCurrent()),
			requireBootcLint: true,
			expectError:      false,
		},
		{
			name: "contains bootc-container-lint hyphenated",
			containerfileContent: fmt.Sprintf(`FROM %s
RUN bootc-container-lint
`, testutil.TestBootcImageCurrent()),
			requireBootcLint: true,
			expectError:      false,
		},
		{
			name: "missing bootc lint with require=true",
			containerfileContent: fmt.Sprintf(`FROM %s
RUN dnf install -y httpd
`, testutil.TestBootcImageCurrent()),
			requireBootcLint: true,
			expectError:      true,
		},
		{
			name: "missing bootc lint with require=false",
			containerfileContent: fmt.Sprintf(`FROM %s
RUN dnf install -y httpd
`, testutil.TestBootcImageCurrent()),
			requireBootcLint: false,
			expectError:      false,
		},
		{
			name: "case insensitive matching",
			containerfileContent: fmt.Sprintf(`FROM %s
RUN BOOTC CONTAINER LINT
`, testutil.TestBootcImageCurrent()),
			requireBootcLint: true,
			expectError:      false,
		},
		{
			name: "lint in comment should match",
			containerfileContent: fmt.Sprintf(`FROM %s
# RUN bootc container lint
RUN echo "test"
`, testutil.TestBootcImageCurrent()),
			requireBootcLint: true,
			expectError:      false, // Comment contains the pattern
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with Containerfile
			dir := t.TempDir()
			containerfilePath := dir + "/Containerfile"
			if err := os.WriteFile(containerfilePath, []byte(tt.containerfileContent), 0644); err != nil {
				t.Fatalf("failed to write Containerfile: %v", err)
			}

			pipeline := &Pipeline{
				baseDir: dir,
				Spec: PipelineSpec{
					Source: SourceConfig{
						Containerfile: "Containerfile",
						Context:       ".",
					},
				},
			}

			stage := NewValidateStage(pipeline, nil, false)
			cfg := &ContainerfileLintConfig{
				Enabled:          true,
				RequireBootcLint: tt.requireBootcLint,
			}

			err := stage.checkBootcLintConfig(cfg)

			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
