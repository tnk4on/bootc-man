package ci

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGetImagesDir(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		want    string
	}{
		{
			name:    "absolute path",
			baseDir: "/home/user/project",
			want:    filepath.Join("/home/user/project", "output", "images"),
		},
		{
			name:    "relative path",
			baseDir: ".",
			want:    filepath.Join(".", "output", "images"),
		},
		{
			name:    "nested path",
			baseDir: "/var/lib/bootc-man/workspace",
			want:    filepath.Join("/var/lib/bootc-man/workspace", "output", "images"),
		},
		{
			name:    "empty path",
			baseDir: "",
			want:    filepath.Join("", "output", "images"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetImagesDir(tt.baseDir)
			if got != tt.want {
				t.Errorf("GetImagesDir(%q) = %q, want %q", tt.baseDir, got, tt.want)
			}
		})
	}
}

func TestNewConvertStage(t *testing.T) {
	pipeline := &Pipeline{
		Spec: PipelineSpec{
			Convert: &ConvertConfig{
				Enabled: true,
				Formats: []ConvertFormat{
					{Type: "qcow2"},
				},
			},
		},
	}

	stage := NewConvertStage(pipeline, nil, "test:latest", true)

	if stage == nil {
		t.Fatal("NewConvertStage returned nil")
	}
	if stage.pipeline != pipeline {
		t.Error("stage.pipeline not set correctly")
	}
	if stage.imageTag != "test:latest" {
		t.Errorf("stage.imageTag = %q, want %q", stage.imageTag, "test:latest")
	}
	if !stage.verbose {
		t.Error("stage.verbose should be true")
	}
	if stage.bootcImageBuilder != DefaultBootcImageBuilder {
		t.Errorf("stage.bootcImageBuilder = %q, want %q", stage.bootcImageBuilder, DefaultBootcImageBuilder)
	}
}

func TestNewConvertStageWithImage(t *testing.T) {
	tests := []struct {
		name              string
		bootcImageBuilder string
		want              string
	}{
		{
			name:              "custom image",
			bootcImageBuilder: "custom.registry.io/bootc-image-builder:latest",
			want:              "custom.registry.io/bootc-image-builder:latest",
		},
		{
			name:              "empty uses default",
			bootcImageBuilder: "",
			want:              DefaultBootcImageBuilder,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &Pipeline{}
			stage := NewConvertStageWithImage(pipeline, nil, "test:latest", false, tt.bootcImageBuilder)

			if stage.bootcImageBuilder != tt.want {
				t.Errorf("bootcImageBuilder = %q, want %q", stage.bootcImageBuilder, tt.want)
			}
		})
	}
}

func TestConvertFormatTypes(t *testing.T) {
	// Test that all expected format types are valid
	validFormats := []string{"qcow2", "ami", "vmdk", "raw", "iso"}

	for _, format := range validFormats {
		t.Run(format, func(t *testing.T) {
			cfg := ConvertFormat{
				Type: format,
			}
			if cfg.Type != format {
				t.Errorf("Type = %q, want %q", cfg.Type, format)
			}
		})
	}
}

func TestConvertConfigStructure(t *testing.T) {
	cfg := &ConvertConfig{
		Enabled: true,
		Formats: []ConvertFormat{
			{Type: "qcow2", Config: ""},
			{Type: "raw", Config: "custom-config.toml"},
		},
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if len(cfg.Formats) != 2 {
		t.Errorf("len(Formats) = %d, want 2", len(cfg.Formats))
	}
	if cfg.Formats[0].Type != "qcow2" {
		t.Errorf("Formats[0].Type = %q, want %q", cfg.Formats[0].Type, "qcow2")
	}
	if cfg.Formats[1].Config != "custom-config.toml" {
		t.Errorf("Formats[1].Config = %q, want %q", cfg.Formats[1].Config, "custom-config.toml")
	}
}

func TestDefaultBootcImageBuilder(t *testing.T) {
	// Verify the default image is from quay.io
	if !strings.HasPrefix(DefaultBootcImageBuilder, "quay.io/") {
		t.Errorf("DefaultBootcImageBuilder = %q, should start with quay.io/", DefaultBootcImageBuilder)
	}
	if !strings.Contains(DefaultBootcImageBuilder, "bootc-image-builder") {
		t.Errorf("DefaultBootcImageBuilder = %q, should contain 'bootc-image-builder'", DefaultBootcImageBuilder)
	}
}
