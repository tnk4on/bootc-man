//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

// TestContainerBuild tests building a bootc container image
func TestContainerBuild(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// Create test Containerfile
	containerfile := fmt.Sprintf(`FROM %s

LABEL containers.bootc=1

# Create test user
RUN useradd -m -G wheel testuser && \
    echo "testuser ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/testuser

# Install test package
RUN dnf install -y htop && dnf clean all
`, testutil.TestBootcImageCurrent())

	containerfilePath := filepath.Join(env.workDir, "Containerfile")
	if err := writeFile(containerfilePath, containerfile); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	// Build the image
	imageTag := fmt.Sprintf("e2e-test-image:%d", nowUnixNano())

	t.Logf("Building container image: %s", imageTag)
	output, err := env.RunBootcMan("container", "build", "-t", imageTag, env.workDir)
	if err != nil {
		t.Fatalf("Failed to build container: %v\nOutput: %s", err, output)
	}
	t.Logf("Build output: OK (%d lines)", strings.Count(output, "\n"))

	// Register cleanup to remove the image
	env.AddCleanup(func() {
		t.Log("Cleaning up: removing built image...")
		_, _ = env.RunCommand("podman", "rmi", "-f", imageTag)
	})

	// Verify the image exists
	output, err = env.RunCommand("podman", "images", "--format", "{{.Repository}}:{{.Tag}}", imageTag)
	if err != nil {
		t.Fatalf("Failed to list images: %v", err)
	}

	if !strings.Contains(output, imageTag) {
		t.Errorf("Built image not found in image list. Output: %s", output)
	}

	t.Log("Container build test completed successfully")
}

// TestContainerBuildWithTag tests building with explicit tag
func TestContainerBuildWithTag(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// Create minimal Containerfile
	containerfile := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN echo "test" > /etc/e2e-test
`, testutil.TestBootcImageCurrent())

	containerfilePath := filepath.Join(env.workDir, "Containerfile")
	if err := writeFile(containerfilePath, containerfile); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	imageTag := fmt.Sprintf("localhost:5000/e2e-test:%d", nowUnixNano())

	t.Logf("Building with tag: %s", imageTag)
	output, err := env.RunBootcMan("container", "build", "-t", imageTag, env.workDir)
	if err != nil {
		t.Fatalf("Failed to build: %v\nOutput: %s", err, output)
	}

	env.AddCleanup(func() {
		_, _ = env.RunCommand("podman", "rmi", "-f", imageTag)
	})

	// Verify image has bootc label
	output, err = env.RunCommand("podman", "inspect", "--format", "{{index .Config.Labels \"containers.bootc\"}}", imageTag)
	if err != nil {
		t.Fatalf("Failed to inspect image: %v", err)
	}

	if strings.TrimSpace(output) != "1" {
		t.Errorf("Image missing bootc label. Label value: %s", output)
	}

	t.Log("Container build with tag test completed successfully")
}

// TestContainerImageList tests listing bootc images
func TestContainerImageList(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// List bootc images
	output, err := env.RunBootcMan("container", "image", "list")
	if err != nil {
		// This is OK if no bootc images exist
		t.Logf("container image list: no images or error: %v", err)
	} else {
		imageCount := len(strings.Split(strings.TrimSpace(output), "\n"))
		t.Logf("Bootc images: OK (%d entries)", imageCount)
	}
}

// TestContainerBuildWithCustomContainerfile tests building with custom Containerfile path
func TestContainerBuildWithCustomContainerfile(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// Create Containerfile with custom name
	containerfile := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
`, testutil.TestBootcImageCurrent())

	customPath := filepath.Join(env.workDir, "Containerfile.custom")
	if err := writeFile(customPath, containerfile); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	imageTag := fmt.Sprintf("e2e-custom-test:%d", nowUnixNano())

	t.Logf("Building with custom Containerfile: %s", customPath)
	output, err := env.RunBootcMan("container", "build", "-t", imageTag, "-f", customPath, env.workDir)
	if err != nil {
		t.Fatalf("Failed to build: %v\nOutput: %s", err, output)
	}

	env.AddCleanup(func() {
		_, _ = env.RunCommand("podman", "rmi", "-f", imageTag)
	})

	t.Log("Container build with custom Containerfile test completed successfully")
}

// TestContainerPushToLocalRegistry tests pushing to local registry
func TestContainerPushToLocalRegistry(t *testing.T) {
	testutil.SkipIfShort(t)
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// Start registry
	t.Log("Starting local registry...")
	_, err := env.RunBootcMan("registry", "up")
	if err != nil {
		t.Fatalf("Failed to start registry: %v", err)
	}

	env.AddCleanup(func() {
		_, _ = env.RunBootcMan("registry", "down")
		_, _ = env.RunBootcMan("registry", "rm", "--force", "--volumes")
	})

	// Wait for registry
	if err := waitForRegistry(env.ctx, env.registryPort); err != nil {
		t.Fatalf("Registry not ready: %v", err)
	}

	// Create and build image
	containerfile := fmt.Sprintf(`FROM %s
LABEL containers.bootc=1
RUN echo "push-test" > /etc/push-test
`, testutil.TestBootcImageCurrent())

	containerfilePath := filepath.Join(env.workDir, "Containerfile")
	if err := writeFile(containerfilePath, containerfile); err != nil {
		t.Fatalf("Failed to create Containerfile: %v", err)
	}

	imageTag := fmt.Sprintf("localhost:5000/e2e-push-test:%d", nowUnixNano())

	t.Logf("Building image: %s", imageTag)
	output, err := env.RunBootcMan("container", "build", "-t", imageTag, env.workDir)
	if err != nil {
		t.Fatalf("Failed to build: %v\nOutput: %s", err, output)
	}

	env.AddCleanup(func() {
		_, _ = env.RunCommand("podman", "rmi", "-f", imageTag)
	})

	// Push to registry (local registry uses HTTP, so disable TLS verification)
	t.Logf("Pushing image to registry: %s", imageTag)
	output, err = env.RunBootcMan("container", "push", "--tls-verify=false", imageTag)
	if err != nil {
		t.Fatalf("Failed to push: %v\nOutput: %s", err, output)
	}
	t.Logf("Push output: OK (%d lines)", strings.Count(output, "\n"))

	t.Log("Container push to local registry test completed successfully")
}

// writeFile writes content to a file
func writeFile(path, content string) error {
	return testutil.WriteFileToPath(path, content)
}

// nowUnixNano returns current time in nanoseconds (for unique names)
func nowUnixNano() int64 {
	return testutil.NowUnixNano()
}
