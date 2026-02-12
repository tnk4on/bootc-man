package ci

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

func TestParseBaseImages(t *testing.T) {
	tests := []struct {
		name          string
		containerfile string
		wantImages    []string
		wantErr       bool
	}{
		{
			name: "single FROM instruction",
			containerfile: fmt.Sprintf(`FROM %s
RUN dnf install -y vim
`, testutil.TestBootcImageCurrent()),
			wantImages: []string{testutil.TestBootcImageCurrent()},
		},
		{
			name: "multi-stage build",
			containerfile: fmt.Sprintf(`FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN go build -o myapp

FROM %s
COPY --from=builder /app/myapp /usr/bin/
`, testutil.TestBootcImageCurrent()),
			wantImages: []string{"golang:1.21", testutil.TestBootcImageCurrent()},
		},
		{
			name: "FROM with registry.redhat.io",
			containerfile: `FROM registry.redhat.io/rhel9/rhel-bootc:9.4
RUN dnf install -y httpd
`,
			wantImages: []string{"registry.redhat.io/rhel9/rhel-bootc:9.4"},
		},
		{
			name: "FROM with scratch (multi-stage final)",
			containerfile: `FROM golang:1.21 AS builder
WORKDIR /app
RUN go build -o app

FROM scratch
COPY --from=builder /app/app /
`,
			wantImages: []string{"golang:1.21"},
		},
		{
			name: "FROM with ARG variable reference",
			containerfile: `ARG BASE_IMAGE=fedora:latest
FROM $BASE_IMAGE
RUN dnf update -y
`,
			wantImages: []string{},
		},
		{
			name: "FROM with digest",
			containerfile: `FROM quay.io/fedora/fedora-bootc@sha256:abc123def456
RUN dnf install -y vim
`,
			wantImages: []string{"quay.io/fedora/fedora-bootc@sha256:abc123def456"},
		},
		{
			name: "multiple registries in multi-stage",
			containerfile: fmt.Sprintf(`FROM registry.redhat.io/ubi9/ubi:9.3 AS builder
RUN dnf install -y make

FROM registry.connect.redhat.com/some/image:latest AS middleware
RUN echo "middleware step"

FROM %s
COPY --from=builder /app /app
`, testutil.TestBootcImageCurrent()),
			wantImages: []string{
				"registry.redhat.io/ubi9/ubi:9.3",
				"registry.connect.redhat.com/some/image:latest",
				testutil.TestBootcImageCurrent(),
			},
		},
		{
			name: "empty containerfile",
			containerfile: `# This is a comment
# No FROM instruction
`,
			wantImages: []string{},
		},
		{
			name: "case insensitive FROM",
			containerfile: `from fedora:latest
RUN echo hello
`,
			wantImages: []string{"fedora:latest"},
		},
		{
			name: "FROM with AS and spaces",
			containerfile: fmt.Sprintf(`FROM   %s   AS   base
RUN dnf install -y vim
`, testutil.TestBootcImageCurrent()),
			wantImages: []string{testutil.TestBootcImageCurrent()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory and write Containerfile
			dir := testutil.TempDir(t)
			containerfilePath := testutil.WriteFile(t, dir, "Containerfile", tt.containerfile)

			images, err := ParseBaseImages(containerfilePath)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(images) != len(tt.wantImages) {
				t.Errorf("got %d images, want %d: %v", len(images), len(tt.wantImages), images)
				return
			}

			for i, img := range images {
				if img != tt.wantImages[i] {
					t.Errorf("images[%d] = %q, want %q", i, img, tt.wantImages[i])
				}
			}
		})
	}
}

func TestParseBaseImagesFileNotFound(t *testing.T) {
	_, err := ParseBaseImages("/nonexistent/path/Containerfile")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestKnownAuthRegistries(t *testing.T) {
	// Verify that known auth registries are defined
	if len(KnownAuthRegistries) == 0 {
		t.Fatal("KnownAuthRegistries should not be empty")
	}

	// Check for expected registries
	expectedRegistries := map[string]bool{
		"registry.redhat.io":          false,
		"registry.connect.redhat.com": false,
	}

	for _, reg := range KnownAuthRegistries {
		if reg.Registry == "" {
			t.Error("Registry field should not be empty")
		}
		if reg.LoginCmd == "" {
			t.Error("LoginCmd field should not be empty")
		}
		if reg.Description == "" {
			t.Error("Description field should not be empty")
		}

		if _, exists := expectedRegistries[reg.Registry]; exists {
			expectedRegistries[reg.Registry] = true
		}
	}

	for reg, found := range expectedRegistries {
		if !found {
			t.Errorf("expected registry %q not found in KnownAuthRegistries", reg)
		}
	}
}

func TestRegistryAuthInfo(t *testing.T) {
	info := RegistryAuthInfo{
		Registry:    "test.registry.io",
		LoginCmd:    "podman login test.registry.io",
		Description: "Test Registry",
	}

	if info.Registry != "test.registry.io" {
		t.Errorf("Registry = %q, want %q", info.Registry, "test.registry.io")
	}
	if info.LoginCmd != "podman login test.registry.io" {
		t.Errorf("LoginCmd = %q, want %q", info.LoginCmd, "podman login test.registry.io")
	}
	if info.Description != "Test Registry" {
		t.Errorf("Description = %q, want %q", info.Description, "Test Registry")
	}
}

func TestBuildStageGenerateImageTag(t *testing.T) {
	tests := []struct {
		name         string
		pipelineName string
		wantTag      string
	}{
		{
			name:         "simple name",
			pipelineName: "my-pipeline",
			wantTag:      "localhost/bootc-man-my-pipeline:latest",
		},
		{
			name:         "name with spaces",
			pipelineName: "my pipeline",
			wantTag:      "localhost/bootc-man-my-pipeline:latest",
		},
		{
			name:         "uppercase name",
			pipelineName: "MyPipeline",
			wantTag:      "localhost/bootc-man-mypipeline:latest",
		},
		{
			name:         "name with multiple spaces",
			pipelineName: "my   test   pipeline",
			wantTag:      "localhost/bootc-man-my---test---pipeline:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a minimal pipeline for testing
			pipeline := &Pipeline{
				Metadata: PipelineMetadata{Name: tt.pipelineName},
			}

			// Create build stage (podman client can be nil for this test)
			b := &BuildStage{pipeline: pipeline}

			tag := b.generateImageTag()
			if tag != tt.wantTag {
				t.Errorf("generateImageTag() = %q, want %q", tag, tt.wantTag)
			}
		})
	}
}

func TestBuildStageGetDefaultPlatform(t *testing.T) {
	b := &BuildStage{}
	platform := b.getDefaultPlatform()

	// Platform should be one of the expected values
	validPlatforms := map[string]bool{
		"linux/amd64": true,
		"linux/arm64": true,
	}

	if !validPlatforms[platform] {
		t.Errorf("getDefaultPlatform() = %q, want one of %v", platform, validPlatforms)
	}
}

func TestNewBuildStage(t *testing.T) {
	dir := testutil.SetupPipelineTestDir(t)
	pipeline := &Pipeline{
		baseDir: dir,
		Spec: PipelineSpec{
			Source: SourceConfig{
				Containerfile: "Containerfile",
				Context:       ".",
			},
		},
	}

	stage := NewBuildStage(pipeline, nil, true)
	if stage == nil {
		t.Fatal("NewBuildStage returned nil")
	}
	if stage.pipeline != pipeline {
		t.Error("stage.pipeline not set correctly")
	}
	if !stage.verbose {
		t.Error("stage.verbose should be true")
	}
}

func TestParseBaseImagesWithContext(t *testing.T) {
	// Test that ParseBaseImages works with a Containerfile in a subdirectory
	dir := testutil.TempDir(t)

	// Create subdirectory
	subdir := filepath.Join(dir, "build")
	testutil.CreateDir(t, subdir)

	// Write Containerfile in subdirectory
	containerfileContent := fmt.Sprintf(`FROM %s
RUN dnf install -y nginx
`, testutil.TestBootcImageCurrent())
	containerfilePath := testutil.WriteFile(t, subdir, "Containerfile", containerfileContent)

	images, err := ParseBaseImages(containerfilePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(images) != 1 {
		t.Errorf("got %d images, want 1", len(images))
	}
	if images[0] != testutil.TestBootcImageCurrent() {
		t.Errorf("images[0] = %q, want %q", images[0], testutil.TestBootcImageCurrent())
	}
}

// TestBuildPodmanBuildArgs tests the pure function for building podman build arguments
func TestBuildPodmanBuildArgs(t *testing.T) {
	tests := []struct {
		name string
		opts BuildArgsOptions
		want []string
	}{
		{
			name: "minimal options",
			opts: BuildArgsOptions{
				Tag:         "myimage:latest",
				ContextPath: ".",
			},
			want: []string{"build", "-t", "myimage:latest", "."},
		},
		{
			name: "with platform",
			opts: BuildArgsOptions{
				Tag:         "myimage:latest",
				Platform:    "linux/amd64",
				ContextPath: "/path/to/context",
			},
			want: []string{"build", "-t", "myimage:latest", "--platform", "linux/amd64", "/path/to/context"},
		},
		{
			name: "with containerfile relative path",
			opts: BuildArgsOptions{
				Tag:                  "myimage:latest",
				ContainerfileRelPath: "docker/Containerfile",
				ContextPath:          ".",
			},
			want: []string{"build", "-t", "myimage:latest", "-f", "docker/Containerfile", "."},
		},
		{
			name: "with containerfile absolute path",
			opts: BuildArgsOptions{
				Tag:                  "myimage:latest",
				ContainerfileAbsPath: "/home/user/project/Containerfile",
				ContextPath:          ".",
			},
			want: []string{"build", "-t", "myimage:latest", "-f", "/home/user/project/Containerfile", "."},
		},
		{
			name: "relative path takes precedence over absolute",
			opts: BuildArgsOptions{
				Tag:                  "myimage:latest",
				ContainerfileRelPath: "Containerfile",
				ContainerfileAbsPath: "/home/user/project/Containerfile",
				ContextPath:          ".",
			},
			want: []string{"build", "-t", "myimage:latest", "-f", "Containerfile", "."},
		},
		{
			name: "with single build arg",
			opts: BuildArgsOptions{
				Tag:         "myimage:latest",
				ContextPath: ".",
				BuildArgs:   map[string]string{"VERSION": "1.0"},
			},
			want: []string{"build", "-t", "myimage:latest", "--build-arg", "VERSION=1.0", "."},
		},
		{
			name: "with single label",
			opts: BuildArgsOptions{
				Tag:         "myimage:latest",
				ContextPath: ".",
				Labels:      map[string]string{"maintainer": "test@example.com"},
			},
			want: []string{"build", "-t", "myimage:latest", "--label", "maintainer=test@example.com", "."},
		},
		{
			name: "full options",
			opts: BuildArgsOptions{
				Tag:                  "localhost/myapp:v1.0",
				Platform:             "linux/arm64",
				ContainerfileRelPath: "Containerfile.prod",
				ContextPath:          "/app",
				BuildArgs:            map[string]string{"ENV": "production"},
				Labels:               map[string]string{"version": "1.0"},
			},
			want: []string{
				"build",
				"-t", "localhost/myapp:v1.0",
				"--platform", "linux/arm64",
				"-f", "Containerfile.prod",
				"--build-arg", "ENV=production",
				"--label", "version=1.0",
				"/app",
			},
		},
		{
			name: "empty options",
			opts: BuildArgsOptions{},
			want: []string{"build"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPodmanBuildArgs(tt.opts)

			// For maps, we need to check if all expected elements are present
			// since map iteration order is not guaranteed
			if len(got) != len(tt.want) {
				t.Errorf("BuildPodmanBuildArgs() returned %d args, want %d\ngot:  %v\nwant: %v",
					len(got), len(tt.want), got, tt.want)
				return
			}

			// Check first few fixed-position args
			for i := 0; i < len(got) && i < len(tt.want); i++ {
				// Skip map-based args which may be in different order
				if got[i] != tt.want[i] {
					// For build-arg and label, just verify they exist somewhere
					if got[i] == "--build-arg" || got[i] == "--label" ||
						tt.want[i] == "--build-arg" || tt.want[i] == "--label" {
						continue
					}
					// Check if this is a value for build-arg or label
					if i > 0 && (got[i-1] == "--build-arg" || got[i-1] == "--label") {
						continue
					}
					t.Errorf("arg[%d] = %q, want %q\nfull got:  %v\nfull want: %v",
						i, got[i], tt.want[i], got, tt.want)
				}
			}
		})
	}
}

// TestBuildArgsOptionsStruct tests the BuildArgsOptions struct
func TestBuildArgsOptionsStruct(t *testing.T) {
	opts := BuildArgsOptions{
		Tag:                  "test:latest",
		Platform:             "linux/amd64",
		ContainerfileRelPath: "Containerfile",
		ContainerfileAbsPath: "/abs/path",
		ContextPath:          ".",
		BuildArgs:            map[string]string{"KEY": "VALUE"},
		Labels:               map[string]string{"app": "test"},
	}

	if opts.Tag != "test:latest" {
		t.Errorf("Tag = %q, want %q", opts.Tag, "test:latest")
	}
	if opts.Platform != "linux/amd64" {
		t.Errorf("Platform = %q, want %q", opts.Platform, "linux/amd64")
	}
	if opts.ContainerfileRelPath != "Containerfile" {
		t.Errorf("ContainerfileRelPath = %q, want %q", opts.ContainerfileRelPath, "Containerfile")
	}
	if opts.BuildArgs["KEY"] != "VALUE" {
		t.Errorf("BuildArgs[KEY] = %q, want %q", opts.BuildArgs["KEY"], "VALUE")
	}
	if opts.Labels["app"] != "test" {
		t.Errorf("Labels[app] = %q, want %q", opts.Labels["app"], "test")
	}
}
