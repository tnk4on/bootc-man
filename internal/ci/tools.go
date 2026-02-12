// Package ci provides CI pipeline definition and execution
package ci

import "github.com/tnk4on/bootc-man/internal/config"

// ContainerizedTool represents a CI tool that runs as a container
type ContainerizedTool struct {
	Name       string
	Image      string
	Privileged bool
	EntryPoint string // Override entrypoint if needed
}

// CITools defines all containerized CI tools
var CITools = map[string]ContainerizedTool{
	"hadolint": {
		Name:  "hadolint",
		Image: config.DefaultHadolintImage,
	},
	"trivy": {
		Name:  "trivy",
		Image: config.DefaultTrivyImage,
	},
	"syft": {
		Name:  "syft",
		Image: config.DefaultSyftImage,
	},
	"cosign": {
		Name:  "cosign",
		Image: "gcr.io/projectsigstore/cosign:latest",
	},
	"skopeo": {
		Name:  "skopeo",
		Image: config.DefaultSkopeoImage,
	},
	"bootc-image-builder": {
		Name:       "bootc-image-builder",
		Image:      config.DefaultBootcImageBuilder,
		Privileged: true,
	},
}

// StageOrder defines the canonical order of CI stages
var StageOrder = []string{"validate", "build", "scan", "convert", "test", "release"}

// GetTool returns a CI tool by name, or nil if not found
func GetTool(name string) *ContainerizedTool {
	if tool, ok := CITools[name]; ok {
		return &tool
	}
	return nil
}
