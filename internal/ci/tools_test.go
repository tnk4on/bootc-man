package ci

import (
	"testing"

	"github.com/tnk4on/bootc-man/internal/config"
)

func TestGetTool(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		wantNil  bool
		wantName string
	}{
		{
			name:     "hadolint exists",
			toolName: "hadolint",
			wantNil:  false,
			wantName: "hadolint",
		},
		{
			name:     "trivy exists",
			toolName: "trivy",
			wantNil:  false,
			wantName: "trivy",
		},
		{
			name:     "syft exists",
			toolName: "syft",
			wantNil:  false,
			wantName: "syft",
		},
		{
			name:     "cosign exists",
			toolName: "cosign",
			wantNil:  false,
			wantName: "cosign",
		},
		{
			name:     "skopeo exists",
			toolName: "skopeo",
			wantNil:  false,
			wantName: "skopeo",
		},
		{
			name:     "bootc-image-builder exists",
			toolName: "bootc-image-builder",
			wantNil:  false,
			wantName: "bootc-image-builder",
		},
		{
			name:     "unknown tool returns nil",
			toolName: "unknown-tool",
			wantNil:  true,
		},
		{
			name:     "empty string returns nil",
			toolName: "",
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetTool(tt.toolName)
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetTool(%q) = %v, want nil", tt.toolName, got)
				}
			} else {
				if got == nil {
					t.Fatalf("GetTool(%q) = nil, want non-nil", tt.toolName)
				}
				if got.Name != tt.wantName {
					t.Errorf("GetTool(%q).Name = %q, want %q", tt.toolName, got.Name, tt.wantName)
				}
			}
		})
	}
}

func TestCIToolsMap(t *testing.T) {
	// Verify all expected tools exist
	expectedTools := []string{"hadolint", "trivy", "syft", "cosign", "skopeo", "bootc-image-builder"}

	for _, name := range expectedTools {
		t.Run(name, func(t *testing.T) {
			tool, ok := CITools[name]
			if !ok {
				t.Errorf("CITools[%q] not found", name)
				return
			}
			if tool.Name != name {
				t.Errorf("CITools[%q].Name = %q, want %q", name, tool.Name, name)
			}
			if tool.Image == "" {
				t.Errorf("CITools[%q].Image is empty", name)
			}
		})
	}

	// Verify tool count
	if len(CITools) != len(expectedTools) {
		t.Errorf("len(CITools) = %d, want %d", len(CITools), len(expectedTools))
	}
}

func TestCIToolsDefaultImages(t *testing.T) {
	// Verify specific tools use config defaults
	tests := []struct {
		toolName      string
		expectedImage string
	}{
		{"hadolint", config.DefaultHadolintImage},
		{"trivy", config.DefaultTrivyImage},
		{"syft", config.DefaultSyftImage},
		{"skopeo", config.DefaultSkopeoImage},
		{"bootc-image-builder", config.DefaultBootcImageBuilder},
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			tool := CITools[tt.toolName]
			if tool.Image != tt.expectedImage {
				t.Errorf("CITools[%q].Image = %q, want %q", tt.toolName, tool.Image, tt.expectedImage)
			}
		})
	}
}

func TestCIToolsPrivileged(t *testing.T) {
	// Only bootc-image-builder should be privileged
	for name, tool := range CITools {
		t.Run(name, func(t *testing.T) {
			if name == "bootc-image-builder" {
				if !tool.Privileged {
					t.Errorf("CITools[%q].Privileged = false, want true", name)
				}
			} else {
				if tool.Privileged {
					t.Errorf("CITools[%q].Privileged = true, want false", name)
				}
			}
		})
	}
}

func TestStageOrder(t *testing.T) {
	expectedOrder := []string{"validate", "build", "scan", "convert", "test", "release"}

	if len(StageOrder) != len(expectedOrder) {
		t.Errorf("len(StageOrder) = %d, want %d", len(StageOrder), len(expectedOrder))
	}

	for i, stage := range expectedOrder {
		if i >= len(StageOrder) {
			break
		}
		if StageOrder[i] != stage {
			t.Errorf("StageOrder[%d] = %q, want %q", i, StageOrder[i], stage)
		}
	}
}
