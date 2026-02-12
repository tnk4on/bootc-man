package testutil

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tnk4on/bootc-man/internal/config"
)

// Test image constants
// These can be overridden via environment variables for testing with different versions
const (
	// TestBootcImageRegistry is the registry for bootc test images
	TestBootcImageRegistry = "quay.io/fedora"

	// TestBootcImageName is the name of the bootc test image
	TestBootcImageName = "fedora-bootc"

	// TestBootcImageTagCurrent is the current version tag for testing
	TestBootcImageTagCurrent = "43"

	// TestBootcImageTagPrevious is the previous version tag for rollback testing
	TestBootcImageTagPrevious = "42"

	// TestBootcImageTagNext is the next version tag for upgrade testing
	TestBootcImageTagNext = "44"

	// TestRegistryImage is the registry image for testing
	TestRegistryImage = "docker.io/library/registry:2"
)

// TestBootcImage returns the full bootc image reference for a given tag
func TestBootcImage(tag string) string {
	return fmt.Sprintf("%s/%s:%s", TestBootcImageRegistry, TestBootcImageName, tag)
}

// TestBootcImageCurrent returns the current bootc image for testing
func TestBootcImageCurrent() string {
	return TestBootcImage(TestBootcImageTagCurrent)
}

// TestBootcImagePrevious returns the previous bootc image for rollback testing
func TestBootcImagePrevious() string {
	return TestBootcImage(TestBootcImageTagPrevious)
}

// TestBootcImageNext returns the next bootc image for upgrade testing
func TestBootcImageNext() string {
	return TestBootcImage(TestBootcImageTagNext)
}

// SamplePipelineYAML returns a valid pipeline YAML for testing
func SamplePipelineYAML() string {
	return `apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: test-pipeline
  description: Test pipeline for unit tests
spec:
  source:
    containerfile: Containerfile
    context: .
  validate:
    containerfileLint:
      enabled: true
  build:
    imageTag: test-image:latest
`
}

// SamplePipelineYAMLWithBuild returns a pipeline YAML with build configuration
func SamplePipelineYAMLWithBuild() string {
	return `apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: build-pipeline
  description: Build pipeline for testing
spec:
  source:
    containerfile: Containerfile
    context: .
  build:
    imageTag: localhost:5000/test:latest
    platforms:
      - linux/amd64
      - linux/arm64
    args:
      VERSION: "1.0.0"
    labels:
      maintainer: test@example.com
`
}

// SamplePipelineYAMLWithScan returns a pipeline YAML with scan configuration
func SamplePipelineYAMLWithScan() string {
	return `apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: scan-pipeline
  description: Scan pipeline for testing
spec:
  source:
    containerfile: Containerfile
    context: .
  build:
    imageTag: test-image:latest
  scan:
    vulnerability:
      enabled: true
      tool: trivy
      severity: HIGH,CRITICAL
      failOnVulnerability: true
    sbom:
      enabled: true
      tool: syft
      format: spdx-json
`
}

// SamplePipelineYAMLWithTest returns a pipeline YAML with test configuration
func SamplePipelineYAMLWithTest() string {
	return fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: test-pipeline
  description: Full test pipeline
spec:
  source:
    containerfile: Containerfile
    context: .
  build:
    imageTag: test-image:latest
  test:
    boot:
      enabled: true
      timeout: 300
      checks:
        - ssh-ready
        - systemd-healthy
    upgrade:
      enabled: true
      fromImage: %s
    rollback:
      enabled: true
`, TestBootcImagePrevious())
}

// SamplePipelineYAMLWithRelease returns a pipeline YAML with release configuration
func SamplePipelineYAMLWithRelease() string {
	return `apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: release-pipeline
  description: Release pipeline for testing
spec:
  source:
    containerfile: Containerfile
    context: .
  build:
    imageTag: test-image:latest
  release:
    registry: localhost:5000
    repository: myorg/myimage
    tls: false
    tags:
      - latest
      - v1.0.0
    sign:
      enabled: true
      key: cosign.key
`
}

// SampleContainerfile returns a simple Containerfile for testing
func SampleContainerfile() string {
	return fmt.Sprintf(`FROM %s

# Install custom packages
RUN dnf install -y vim htop && dnf clean all

# Add custom configuration
COPY config.toml /etc/bootc-man/config.toml
`, TestBootcImageCurrent())
}

// SampleBootcContainerfile returns a bootc-compatible Containerfile
func SampleBootcContainerfile() string {
	return fmt.Sprintf(`FROM %s

LABEL containers.bootc=1

# Custom user
RUN useradd -m -G wheel testuser && \
    echo "testuser ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/testuser

# SSH key setup
RUN mkdir -p /home/testuser/.ssh && \
    chmod 700 /home/testuser/.ssh

# Custom packages
RUN dnf install -y \
    vim-enhanced \
    htop \
    tmux \
    && dnf clean all
`, TestBootcImageCurrent())
}

// SampleBootcStatusJSON returns a sample bootc status JSON output
func SampleBootcStatusJSON() string {
	current := TestBootcImageCurrent()
	previous := TestBootcImagePrevious()
	return fmt.Sprintf(`{
  "apiVersion": "org.containers.bootc/v1",
  "kind": "BootcHost",
  "metadata": {
    "name": "host"
  },
  "spec": {
    "image": {
      "image": "%s",
      "transport": "registry"
    }
  },
  "status": {
    "staged": null,
    "booted": {
      "image": {
        "image": {
          "image": "%s",
          "transport": "registry"
        },
        "version": "%s.20240101.0",
        "timestamp": "2024-01-01T00:00:00Z",
        "imageDigest": "sha256:abc123def456"
      },
      "cachedUpdate": null,
      "incompatible": false,
      "pinned": false
    },
    "rollback": {
      "image": {
        "image": {
          "image": "%s",
          "transport": "registry"
        },
        "version": "%s.20231201.0",
        "timestamp": "2023-12-01T00:00:00Z",
        "imageDigest": "sha256:old123old456"
      },
      "cachedUpdate": null,
      "incompatible": false,
      "pinned": false
    },
    "type": "bootcHost"
  }
}`, current, current, TestBootcImageTagCurrent, previous, TestBootcImageTagPrevious)
}

// SampleBootcStatusWithStagedJSON returns bootc status with a staged update
func SampleBootcStatusWithStagedJSON() string {
	next := TestBootcImageNext()
	current := TestBootcImageCurrent()
	return fmt.Sprintf(`{
  "apiVersion": "org.containers.bootc/v1",
  "kind": "BootcHost",
  "metadata": {
    "name": "host"
  },
  "spec": {
    "image": {
      "image": "%s",
      "transport": "registry"
    }
  },
  "status": {
    "staged": {
      "image": {
        "image": {
          "image": "%s",
          "transport": "registry"
        },
        "version": "%s.20240201.0",
        "timestamp": "2024-02-01T00:00:00Z",
        "imageDigest": "sha256:new123new456"
      },
      "cachedUpdate": null,
      "incompatible": false,
      "pinned": false
    },
    "booted": {
      "image": {
        "image": {
          "image": "%s",
          "transport": "registry"
        },
        "version": "%s.20240101.0",
        "timestamp": "2024-01-01T00:00:00Z",
        "imageDigest": "sha256:abc123def456"
      },
      "cachedUpdate": null,
      "incompatible": false,
      "pinned": false
    },
    "rollback": null,
    "type": "bootcHost"
  }
}`, next, next, TestBootcImageTagNext, current, TestBootcImageTagCurrent)
}

// SampleIgnitionConfig returns a sample Ignition configuration
func SampleIgnitionConfig() string {
	return `{
  "ignition": {
    "version": "3.4.0"
  },
  "passwd": {
    "users": [
      {
        "name": "core",
        "sshAuthorizedKeys": [
          "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI..."
        ]
      }
    ]
  }
}`
}

// InvalidPipelineYAML returns an invalid pipeline YAML for error testing
func InvalidPipelineYAML() string {
	return `apiVersion: invalid/v1
kind: NotPipeline
metadata:
  name: invalid
spec:
  source:
    containerfile: ""
`
}

// SetupPipelineTestDir creates a temporary directory with pipeline files for testing.
// Returns the temp directory path.
func SetupPipelineTestDir(t *testing.T) string {
	t.Helper()
	dir := TempDir(t)

	// Create Containerfile
	WriteFile(t, dir, config.DefaultContainerfileName, SampleBootcContainerfile())

	// Create config.toml
	WriteFile(t, dir, "config.toml", "[bootc]\nversion = \"1.0\"")

	return dir
}

// SetupPipelineTestDirWithYAML creates a test directory with pipeline YAML and Containerfile.
// Returns the temp directory path.
func SetupPipelineTestDirWithYAML(t *testing.T, pipelineYAML string) string {
	t.Helper()
	dir := SetupPipelineTestDir(t)

	// Create pipeline YAML
	WriteFile(t, dir, config.DefaultPipelineFileName, pipelineYAML)

	return dir
}

// ContainsString checks if a string contains a substring.
// This is a simple helper for test assertions.
func ContainsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
