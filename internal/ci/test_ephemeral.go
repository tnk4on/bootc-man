package ci

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ExecuteEphemeral runs the test stage using bcvk ephemeral VM.
// This skips the convert stage (no disk image needed) and boots
// directly from the container image via bcvk.
func (t *TestStage) ExecuteEphemeral(ctx context.Context) error {
	if t.pipeline.Spec.Test == nil {
		return fmt.Errorf("test stage is not configured")
	}

	cfg := t.pipeline.Spec.Test
	if cfg.Boot == nil || !cfg.Boot.Enabled {
		return fmt.Errorf("boot test is not enabled")
	}

	bcvkBin, err := exec.LookPath("bcvk")
	if err != nil {
		return fmt.Errorf("bcvk is not installed (required for ephemeral test).\n" +
			"  Install: brew install bcvk\n" +
			"  Or build from source: https://github.com/bootc-dev/bcvk")
	}

	imageTag := t.imageTag
	if imageTag == "" {
		return fmt.Errorf("image tag is required for ephemeral test")
	}

	// Build test commands from pipeline config
	var checks []string
	if cfg.Boot.Checks != nil {
		checks = cfg.Boot.Checks
	}
	if len(checks) == 0 {
		checks = []string{"sudo bootc status"}
	}

	fmt.Printf("Running ephemeral boot test via bcvk...\n")
	fmt.Printf("  Image: %s\n", imageTag)
	fmt.Printf("  Checks: %d command(s)\n", len(checks))

	for i, check := range checks {
		fmt.Printf("\n--- Check %d/%d: %s ---\n", i+1, len(checks), check)

		args := []string{
			"ephemeral", "run-ssh",
			"-K",
			"--execute", check,
		}

		args = append(args, imageTag)

		if t.verbose {
			fmt.Printf("Running: bcvk %s\n", strings.Join(args, " "))
		}

		cmd := exec.CommandContext(ctx, bcvkBin, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ephemeral test check %d failed: %s: %w", i+1, check, err)
		}

		fmt.Printf("--- Check %d/%d: PASSED ---\n", i+1, len(checks))
	}

	fmt.Printf("\nAll %d ephemeral test checks passed.\n", len(checks))
	return nil
}
