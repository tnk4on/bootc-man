package ci

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

func TestLoadPipeline(t *testing.T) {
	tests := []struct {
		name        string
		yaml        string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *Pipeline)
	}{
		{
			name:    "valid minimal pipeline",
			yaml:    testutil.SamplePipelineYAML(),
			wantErr: false,
			validate: func(t *testing.T, p *Pipeline) {
				if p.APIVersion != "bootc-man/v1" {
					t.Errorf("APIVersion = %q, want %q", p.APIVersion, "bootc-man/v1")
				}
				if p.Kind != "Pipeline" {
					t.Errorf("Kind = %q, want %q", p.Kind, "Pipeline")
				}
				if p.Metadata.Name != "test-pipeline" {
					t.Errorf("Metadata.Name = %q, want %q", p.Metadata.Name, "test-pipeline")
				}
				if p.Spec.Source.Containerfile != "Containerfile" {
					t.Errorf("Source.Containerfile = %q, want %q", p.Spec.Source.Containerfile, "Containerfile")
				}
			},
		},
		{
			name:    "valid pipeline with build",
			yaml:    testutil.SamplePipelineYAMLWithBuild(),
			wantErr: false,
			validate: func(t *testing.T, p *Pipeline) {
				if p.Spec.Build == nil {
					t.Fatal("Build config is nil")
				}
				if p.Spec.Build.ImageTag != "localhost:5000/test:latest" {
					t.Errorf("Build.ImageTag = %q, want %q", p.Spec.Build.ImageTag, "localhost:5000/test:latest")
				}
				if len(p.Spec.Build.Platforms) != 2 {
					t.Errorf("len(Build.Platforms) = %d, want 2", len(p.Spec.Build.Platforms))
				}
				if p.Spec.Build.Args["VERSION"] != "1.0.0" {
					t.Errorf("Build.Args[VERSION] = %q, want %q", p.Spec.Build.Args["VERSION"], "1.0.0")
				}
			},
		},
		{
			name:    "valid pipeline with scan",
			yaml:    testutil.SamplePipelineYAMLWithScan(),
			wantErr: false,
			validate: func(t *testing.T, p *Pipeline) {
				if p.Spec.Scan == nil {
					t.Fatal("Scan config is nil")
				}
				if p.Spec.Scan.Vulnerability == nil {
					t.Fatal("Vulnerability config is nil")
				}
				if !p.Spec.Scan.Vulnerability.Enabled {
					t.Error("Vulnerability.Enabled = false, want true")
				}
				if p.Spec.Scan.Vulnerability.Tool != "trivy" {
					t.Errorf("Vulnerability.Tool = %q, want %q", p.Spec.Scan.Vulnerability.Tool, "trivy")
				}
				if p.Spec.Scan.SBOM == nil {
					t.Fatal("SBOM config is nil")
				}
				if p.Spec.Scan.SBOM.Format != "spdx-json" {
					t.Errorf("SBOM.Format = %q, want %q", p.Spec.Scan.SBOM.Format, "spdx-json")
				}
			},
		},
		{
			name:    "valid pipeline with test",
			yaml:    testutil.SamplePipelineYAMLWithTest(),
			wantErr: false,
			validate: func(t *testing.T, p *Pipeline) {
				if p.Spec.Test == nil {
					t.Fatal("Test config is nil")
				}
				if p.Spec.Test.Boot == nil {
					t.Fatal("Boot config is nil")
				}
				if p.Spec.Test.Boot.Timeout != 300 {
					t.Errorf("Boot.Timeout = %d, want 300", p.Spec.Test.Boot.Timeout)
				}
				if p.Spec.Test.Upgrade == nil {
					t.Fatal("Upgrade config is nil")
				}
				if p.Spec.Test.Upgrade.FromImage != testutil.TestBootcImagePrevious() {
					t.Errorf("Upgrade.FromImage = %q, want %q", p.Spec.Test.Upgrade.FromImage, testutil.TestBootcImagePrevious())
				}
			},
		},
		{
			name:    "valid pipeline with release",
			yaml:    testutil.SamplePipelineYAMLWithRelease(),
			wantErr: false,
			validate: func(t *testing.T, p *Pipeline) {
				if p.Spec.Release == nil {
					t.Fatal("Release config is nil")
				}
				if p.Spec.Release.Registry != "localhost:5000" {
					t.Errorf("Release.Registry = %q, want %q", p.Spec.Release.Registry, "localhost:5000")
				}
				if p.Spec.Release.TLS == nil || *p.Spec.Release.TLS != false {
					t.Error("Release.TLS should be false")
				}
				if len(p.Spec.Release.Tags) != 2 {
					t.Errorf("len(Release.Tags) = %d, want 2", len(p.Spec.Release.Tags))
				}
				if p.Spec.Release.Sign == nil {
					t.Fatal("Sign config is nil")
				}
				if p.Spec.Release.Sign.Key != "cosign.key" {
					t.Errorf("Sign.Key = %q, want %q", p.Spec.Release.Sign.Key, "cosign.key")
				}
			},
		},
		{
			name:        "invalid apiVersion",
			yaml:        testutil.InvalidPipelineYAML(),
			wantErr:     true,
			errContains: "unsupported apiVersion",
		},
		{
			name: "missing apiVersion",
			yaml: `kind: Pipeline
metadata:
  name: test
spec:
  source:
    containerfile: Containerfile
    context: .
`,
			wantErr:     true,
			errContains: "apiVersion is required",
		},
		{
			name: "invalid kind",
			yaml: `apiVersion: bootc-man/v1
kind: NotPipeline
metadata:
  name: test
spec:
  source:
    containerfile: Containerfile
    context: .
`,
			wantErr:     true,
			errContains: "unsupported kind",
		},
		{
			name: "missing metadata name",
			yaml: `apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  description: no name
spec:
  source:
    containerfile: Containerfile
    context: .
`,
			wantErr:     true,
			errContains: "metadata.name is required",
		},
		{
			name: "missing containerfile",
			yaml: `apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: test
spec:
  source:
    context: .
`,
			wantErr:     true,
			errContains: "spec.source.containerfile is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with Containerfile
			dir := testutil.SetupPipelineTestDir(t)

			// Write pipeline YAML
			pipelinePath := testutil.WriteFile(t, dir, "bootc-ci.yaml", tt.yaml)

			// Load pipeline
			pipeline, err := LoadPipeline(pipelinePath)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, pipeline)
			}
		})
	}
}

func TestLoadPipelineFileNotFound(t *testing.T) {
	_, err := LoadPipeline("/nonexistent/path/pipeline.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestPipelineValidate(t *testing.T) {
	tests := []struct {
		name        string
		pipeline    Pipeline
		wantErr     bool
		errContains string
	}{
		{
			name: "valid pipeline",
			pipeline: Pipeline{
				APIVersion: "bootc-man/v1",
				Kind:       "Pipeline",
				Metadata:   PipelineMetadata{Name: "test"},
				Spec: PipelineSpec{
					Source: SourceConfig{Containerfile: "Containerfile", Context: "."},
				},
				baseDir: "/tmp",
			},
			wantErr: false,
		},
		{
			name: "missing apiVersion",
			pipeline: Pipeline{
				Kind:     "Pipeline",
				Metadata: PipelineMetadata{Name: "test"},
				Spec: PipelineSpec{
					Source: SourceConfig{Containerfile: "Containerfile"},
				},
			},
			wantErr:     true,
			errContains: "apiVersion is required",
		},
		{
			name: "wrong apiVersion",
			pipeline: Pipeline{
				APIVersion: "wrong/v1",
				Kind:       "Pipeline",
				Metadata:   PipelineMetadata{Name: "test"},
				Spec: PipelineSpec{
					Source: SourceConfig{Containerfile: "Containerfile"},
				},
			},
			wantErr:     true,
			errContains: "unsupported apiVersion",
		},
		{
			name: "missing kind",
			pipeline: Pipeline{
				APIVersion: "bootc-man/v1",
				Metadata:   PipelineMetadata{Name: "test"},
				Spec: PipelineSpec{
					Source: SourceConfig{Containerfile: "Containerfile"},
				},
			},
			wantErr:     true,
			errContains: "kind is required",
		},
		{
			name: "wrong kind",
			pipeline: Pipeline{
				APIVersion: "bootc-man/v1",
				Kind:       "Wrong",
				Metadata:   PipelineMetadata{Name: "test"},
				Spec: PipelineSpec{
					Source: SourceConfig{Containerfile: "Containerfile"},
				},
			},
			wantErr:     true,
			errContains: "unsupported kind",
		},
		{
			name: "missing name",
			pipeline: Pipeline{
				APIVersion: "bootc-man/v1",
				Kind:       "Pipeline",
				Metadata:   PipelineMetadata{},
				Spec: PipelineSpec{
					Source: SourceConfig{Containerfile: "Containerfile"},
				},
			},
			wantErr:     true,
			errContains: "metadata.name is required",
		},
		{
			name: "missing containerfile",
			pipeline: Pipeline{
				APIVersion: "bootc-man/v1",
				Kind:       "Pipeline",
				Metadata:   PipelineMetadata{Name: "test"},
				Spec: PipelineSpec{
					Source: SourceConfig{Context: "."},
				},
			},
			wantErr:     true,
			errContains: "spec.source.containerfile is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip path validation for these unit tests
			if !tt.wantErr {
				// Create temp dir with Containerfile for valid test
				dir := testutil.SetupPipelineTestDir(t)
				tt.pipeline.baseDir = dir
			}

			err := tt.pipeline.Validate()

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestResolveContainerfilePath(t *testing.T) {
	tests := []struct {
		name           string
		containerfile  string
		context        string
		wantContains   string
		setupExtraFile string
	}{
		{
			name:          "relative containerfile in base dir",
			containerfile: "Containerfile",
			context:       ".",
			wantContains:  "Containerfile",
		},
		{
			name:           "containerfile in subdirectory",
			containerfile:  "sub/Containerfile",
			context:        ".",
			wantContains:   "sub/Containerfile",
			setupExtraFile: "sub/Containerfile",
		},
		{
			name:          "context is subdirectory",
			containerfile: "Containerfile",
			context:       "build",
			wantContains:  "Containerfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp dir
			dir := testutil.TempDir(t)

			// Create main Containerfile
			testutil.WriteFile(t, dir, "Containerfile", "FROM fedora")

			// Create context directory if needed
			if tt.context != "." {
				contextDir := filepath.Join(dir, tt.context)
				testutil.CreateDir(t, contextDir)
				testutil.WriteFile(t, contextDir, "Containerfile", "FROM fedora")
			}

			// Create extra file if needed
			if tt.setupExtraFile != "" {
				testutil.WriteFile(t, dir, tt.setupExtraFile, "FROM fedora")
			}

			p := &Pipeline{
				baseDir: dir,
				Spec: PipelineSpec{
					Source: SourceConfig{
						Containerfile: tt.containerfile,
						Context:       tt.context,
					},
				},
			}

			path, err := p.ResolveContainerfilePath()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !containsString(path, tt.wantContains) {
				t.Errorf("path %q does not contain %q", path, tt.wantContains)
			}

			// Verify file exists
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("resolved path does not exist: %s", path)
			}
		})
	}
}

func TestResolveContextPath(t *testing.T) {
	tests := []struct {
		name         string
		context      string
		wantContains string
	}{
		{
			name:         "current directory",
			context:      ".",
			wantContains: "",
		},
		{
			name:         "empty context defaults to current",
			context:      "",
			wantContains: "",
		},
		{
			name:         "subdirectory context",
			context:      "build",
			wantContains: "build",
		},
		{
			name:         "nested context",
			context:      "src/docker",
			wantContains: "src/docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := testutil.TempDir(t)

			p := &Pipeline{
				baseDir: dir,
				Spec: PipelineSpec{
					Source: SourceConfig{
						Containerfile: "Containerfile",
						Context:       tt.context,
					},
				},
			}

			path, err := p.ResolveContextPath()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantContains != "" && !containsString(path, tt.wantContains) {
				t.Errorf("path %q does not contain %q", path, tt.wantContains)
			}

			// Path should be absolute
			if !filepath.IsAbs(path) {
				t.Errorf("path %q is not absolute", path)
			}
		})
	}
}

func TestPipelineBaseDir(t *testing.T) {
	p := &Pipeline{baseDir: "/test/dir"}
	if p.BaseDir() != "/test/dir" {
		t.Errorf("BaseDir() = %q, want %q", p.BaseDir(), "/test/dir")
	}
}

// containsString is a helper to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
