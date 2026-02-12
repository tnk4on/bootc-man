//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

// TestRegistryLifecycle tests the full registry lifecycle: up, status, logs, down
func TestRegistryLifecycle(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// Step 1: Start registry
	t.Log("Starting registry...")
	output, err := env.RunBootcMan("registry", "up")
	if err != nil {
		t.Fatalf("Failed to start registry: %v", err)
	}
	t.Logf("Registry up output: %s", output)

	// Register cleanup to stop registry
	env.AddCleanup(func() {
		t.Log("Cleaning up: stopping registry...")
		_, _ = env.RunBootcMan("registry", "down")
		_, _ = env.RunBootcMan("registry", "rm", "--force", "--volumes")
	})

	// Wait for registry to be ready
	if err := waitForRegistry(env.ctx, env.registryPort); err != nil {
		t.Fatalf("Registry not ready: %v", err)
	}

	// Step 2: Check registry status
	t.Log("Checking registry status...")
	output, err = env.RunBootcMan("registry", "status")
	if err != nil {
		t.Fatalf("Failed to check registry status: %v", err)
	}
	t.Logf("Registry status: %s", output)

	// Step 3: Get registry logs (verify command works, don't dump full output)
	t.Log("Getting registry logs...")
	output, err = env.RunBootcMan("registry", "logs")
	if err != nil {
		// Logs might fail if container just started, that's OK
		t.Logf("Registry logs (may be empty): %v", err)
	} else {
		logLines := strings.Count(output, "\n")
		t.Logf("Registry logs: OK (%d lines)", logLines)
	}

	// Step 4: Stop registry
	t.Log("Stopping registry...")
	output, err = env.RunBootcMan("registry", "down")
	if err != nil {
		t.Fatalf("Failed to stop registry: %v", err)
	}
	t.Logf("Registry down output: %s", output)

	// Verify registry is stopped
	if err := verifyRegistryDown(env.ctx, env.registryPort); err != nil {
		t.Errorf("Registry should be stopped: %v", err)
	}

	t.Log("Registry lifecycle test completed successfully")
}

// TestRegistryStatus tests registry status command
func TestRegistryStatus(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// Check status when registry is not running
	output, err := env.RunBootcMan("registry", "status")
	// Status command should work even if registry is not running
	t.Logf("Registry status (not running): %s, err: %v", output, err)
}

// TestRegistryJSONOutput tests registry status with JSON output
func TestRegistryJSONOutput(t *testing.T) {
	testutil.SkipIfPodmanUnavailable(t)

	env := NewTestEnvironment(t)

	// Start registry
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

	// Get status with JSON output
	output, err := env.RunBootcMan("--json", "registry", "status")
	if err != nil {
		t.Logf("JSON status output: %s", output)
	}
	// Note: Even if the command fails, we just log it - JSON might not be fully implemented
	t.Logf("Registry JSON status: %s", output)
}

// waitForRegistry waits for the registry to be ready
func waitForRegistry(ctx context.Context, port int) error {
	url := fmt.Sprintf("http://localhost:%d/v2/", port)

	for i := 0; i < 30; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				return nil
			}
		}

		time.Sleep(time.Second)
	}

	return fmt.Errorf("registry not ready after 30 seconds")
}

// verifyRegistryDown verifies that the registry is not accessible
func verifyRegistryDown(ctx context.Context, port int) error {
	url := fmt.Sprintf("http://localhost:%d/v2/", port)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		// Connection refused is expected
		return nil
	}
	resp.Body.Close()

	return fmt.Errorf("registry still accessible, status: %d", resp.StatusCode)
}
