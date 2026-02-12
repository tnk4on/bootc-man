package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tnk4on/bootc-man/internal/ci"
	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/podman"
	"github.com/tnk4on/bootc-man/internal/vm"
)

// stageSeparator is the visual separator between CI stages
var stageSeparator = config.StageSeparator

var ciCmd = &cobra.Command{
	Use:   "ci",
	Short: "Manage bootc CI pipelines",
	Long: `Manage CI pipelines for building and validating bootc images.

All CI tools run as containers via Podman, enabling cross-platform execution
on macOS, Windows, and Linux without additional tool installation.

See docs/ci-design.md for detailed requirements.`,
}

var ciCheckCmd = &cobra.Command{
	Use:   "check [pipeline-file]",
	Short: "Check CI pipeline definition file",
	Long: `Check a bootc-ci.yaml pipeline definition file for syntax and schema errors.

If no pipeline file is specified, automatically looks for bootc-ci.yaml in the current directory.

This command validates the configuration file itself, not the actual CI stages.
To run the validate stage, use: bootc-man ci run --stage validate

This command works on all platforms (macOS, Windows, Linux).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCICheck,
}

var ciRunCmd = &cobra.Command{
	Use:   "run [pipeline-file]",
	Short: "Run a CI pipeline",
	Long: `Run a bootc CI pipeline defined in bootc-ci.yaml.

If no pipeline file is specified, automatically looks for bootc-ci.yaml in the current directory.

All tools run as containers via Podman Machine (macOS; Windows not implemented) or native Podman (Linux).

Stages:
  1. validate - Containerfile lint via hadolint container
  2. build    - Container image build via podman build
  3. scan     - Vulnerability scan via trivy/syft containers
  4. convert  - Disk image conversion via bootc-image-builder container
  5. test     - Boot/upgrade/rollback test (macOS: vfkit)
  6. release  - Sign and push via cosign/skopeo containers

Use --stage to run specific stages only.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCIRun,
	// ValidArgsFunction handles completion when user types "--stage build, " (with space after comma)
	// In this case, the flag value is complete and we're completing a positional argument
	// We detect this by checking if the previous --stage value ends with a comma
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Check if --stage flag was set and ends with a comma
		if flag := cmd.Flags().Lookup("stage"); flag != nil && flag.Changed {
			val := flag.Value.String()
			if strings.HasSuffix(val, ",") {
				// User typed "--stage build, " and is now completing after the space
				// We should show remaining stages
				parts := strings.Split(strings.TrimSuffix(val, ","), ",")
				alreadySpecified := make(map[string]bool)
				for _, part := range parts {
					stage := strings.TrimSpace(part)
					if stage != "" {
						alreadySpecified[stage] = true
					}
				}

				var completions []string
				for _, stage := range stageOrder {
					if !alreadySpecified[stage] {
						completions = append(completions, stage)
					}
				}
				return completions, cobra.ShellCompDirectiveNoFileComp
			}
		}
		// Default: allow file completion for pipeline file argument
		return nil, cobra.ShellCompDirectiveDefault
	},
}

var ciStatusCmd = &cobra.Command{
	Use:        "status",
	Short:      "Show CI environment status (deprecated: use 'bootc-man status')",
	Long:       `DEPRECATED: This command has been merged into 'bootc-man status'.`,
	Deprecated: "use 'bootc-man status' instead",
	RunE:       runCIStatus,
}

var ciKeygenCmd = &cobra.Command{
	Use:   "keygen",
	Short: "Generate cosign key pair for image signing",
	Long: `Generate a cosign key pair for signing container images.

This command creates:
  - cosign.key (private key) - Keep this secret!
  - cosign.pub (public key) - Share this for verification

The keys are generated without a password for non-interactive CI use.
For production use with password protection, use cosign directly.

On macOS, this command handles Podman Machine complexity automatically (Windows not implemented).`,
	RunE: runCIKeygen,
}

// Flags for keygen
var keygenOutputDir string

// Flags
var (
	ciStage    string
	ciPipeline string // --pipeline flag for specifying pipeline file
)

// stageOrder defines the order of CI stages (references ci.StageOrder)
var stageOrder = ci.StageOrder

func init() {
	// Add --pipeline flag to ci check command
	ciCheckCmd.Flags().StringVarP(&ciPipeline, "pipeline", "p", "", "Path to pipeline definition file (default: bootc-ci.yaml in current directory)")

	// Add --pipeline flag to ci run command
	ciRunCmd.Flags().StringVarP(&ciPipeline, "pipeline", "p", "", "Path to pipeline definition file (default: bootc-ci.yaml in current directory)")
	ciRunCmd.Flags().StringVar(&ciStage, "stage", "", "Run specific stage(s) only (comma-separated: validate,build,scan,convert,test,release)")
	// Note: --dry-run is a global flag inherited from rootCmd.PersistentFlags()

	// Register completion function for --stage flag with comma-separated support
	_ = ciRunCmd.RegisterFlagCompletionFunc("stage", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// When user types "build,", toComplete might be empty or contain the comma
		// We need to reconstruct the full flag value from args or from the parsed flag
		var valueToParse string
		var originalValue string // Keep original to check for trailing comma

		// First, try to get the flag value from the parsed command flags
		// This works when the flag has already been parsed by Cobra
		if flag := cmd.Flags().Lookup("stage"); flag != nil && flag.Changed {
			if val, err := cmd.Flags().GetString("stage"); err == nil && val != "" {
				valueToParse = val
				originalValue = val
			}
		}

		// If not found in parsed flags, try toComplete (may contain partial value like "build," or empty)
		// Note: toComplete may be "build," or just "build" depending on how zsh parses it
		if valueToParse == "" && toComplete != "" {
			valueToParse = toComplete
			originalValue = toComplete
		}

		// Always check args to get the full flag value (including comma if present)
		// Look for --stage or -s in args (check in reverse order for the most recent one)
		if valueToParse == "" {
			for i := len(args) - 1; i >= 0; i-- {
				arg := args[i]
				if arg == "--stage" || arg == "-s" {
					// Found --stage flag, next arg should be the value
					if i+1 < len(args) {
						// Use the value from args (may contain comma)
						valueToParse = args[i+1]
						originalValue = args[i+1]
						break
					}
				} else if strings.HasPrefix(arg, "--stage=") {
					// Found --stage=value format
					valueToParse = strings.TrimPrefix(arg, "--stage=")
					originalValue = valueToParse
					break
				} else if strings.HasPrefix(arg, "-s=") {
					// Found -s=value format
					valueToParse = strings.TrimPrefix(arg, "-s=")
					originalValue = valueToParse
					break
				}
			}
		}

		// If still empty, check if the last arg might be the flag value (without =)
		// This handles the case where user types "--stage build," and toComplete is empty
		// In zsh, when completing after a comma, toComplete might be empty and the comma
		// might be in the last arg or the previous arg
		if valueToParse == "" && len(args) > 0 {
			lastArg := args[len(args)-1]
			// Check if previous arg was --stage
			if len(args) > 1 {
				prevArg := args[len(args)-2]
				if prevArg == "--stage" || prevArg == "-s" {
					valueToParse = lastArg
					originalValue = lastArg
				}
			}
			// Also check if lastArg itself contains comma and might be the flag value
			// This handles cases where zsh passes "build," as a single arg
			if valueToParse == "" && strings.Contains(lastArg, ",") && !strings.HasPrefix(lastArg, "-") {
				valueToParse = lastArg
				originalValue = lastArg
			}
		}

		// If still empty, check if any arg contains comma (might be the flag value)
		// This is important for zsh completion where args might be parsed differently
		if valueToParse == "" {
			// Look for --stage flag first, then check the next arg
			for i := 0; i < len(args)-1; i++ {
				if args[i] == "--stage" || args[i] == "-s" {
					nextArg := args[i+1]
					if strings.Contains(nextArg, ",") || nextArg != "" {
						valueToParse = nextArg
						originalValue = nextArg
						break
					}
				}
			}
			// If still not found, check any arg with comma
			if valueToParse == "" {
				for _, arg := range args {
					if strings.Contains(arg, ",") && !strings.HasPrefix(arg, "-") {
						valueToParse = arg
						originalValue = arg
						break
					}
				}
			}
		}

		// If still empty, the flag might not be set yet, return all stages
		if valueToParse == "" {
			return stageOrder, cobra.ShellCompDirectiveNoFileComp
		}

		// Check if valueToParse ends with comma - if so, we're completing after a comma
		endsWithComma := strings.HasSuffix(originalValue, ",")

		// Parse already specified stages
		// Handle case where value ends with comma (e.g., "build,")
		// We need to preserve the comma information for proper parsing
		parts := strings.Split(valueToParse, ",")
		alreadySpecified := make(map[string]bool)
		var currentPart string

		// If ends with comma, the last part after split will be empty
		// Otherwise, we need to check the last part for completion
		if endsWithComma {
			// Value ends with comma, so all parts except the last empty one are specified
			for i := 0; i < len(parts); i++ {
				stage := strings.TrimSpace(parts[i])
				if stage != "" {
					alreadySpecified[stage] = true
				}
			}
			currentPart = "" // Completing after comma, show all remaining stages
		} else {
			// Value doesn't end with comma, last part might be partial
			if len(parts) > 1 {
				// Multiple stages specified, get the last part for completion
				for i := 0; i < len(parts)-1; i++ {
					stage := strings.TrimSpace(parts[i])
					if stage != "" {
						alreadySpecified[stage] = true
					}
				}
				currentPart = strings.TrimSpace(parts[len(parts)-1])
			} else {
				// Single stage (might be partial)
				currentPart = strings.TrimSpace(valueToParse)
			}
		}

		// Build prefix from already specified stages
		// When user types "build,", we need to return "build,validate", "build,scan", etc.
		// so that zsh can properly match the prefix
		var prefix string
		if endsWithComma {
			// Keep the original value as prefix (e.g., "build,")
			prefix = originalValue
		} else if len(parts) > 1 {
			// Multiple parts, prefix is all parts except the last one
			// e.g., "build,val" -> prefix is "build,"
			prefix = strings.Join(parts[:len(parts)-1], ",") + ","
		}
		// If single stage without comma, prefix is empty

		// Find the highest stage index among already specified stages
		// Only stages that come AFTER this index in the order should be suggested
		maxSpecifiedIndex := -1
		for i, stage := range stageOrder {
			if alreadySpecified[stage] {
				if i > maxSpecifiedIndex {
					maxSpecifiedIndex = i
				}
			}
		}

		var completions []string
		for i, stage := range stageOrder {
			// Skip already specified stages
			if alreadySpecified[stage] {
				continue
			}
			// Skip stages that come before or at the same position as the last specified stage
			// This ensures stages are selected in order
			if i <= maxSpecifiedIndex {
				continue
			}
			// Match current part (case-insensitive prefix match)
			// If currentPart is empty (e.g., after comma), show all remaining stages
			if currentPart == "" || strings.HasPrefix(strings.ToLower(stage), strings.ToLower(currentPart)) {
				// Include prefix so zsh can match properly
				completions = append(completions, prefix+stage)
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp
	})

	// Add --output flag to keygen command
	ciKeygenCmd.Flags().StringVarP(&keygenOutputDir, "output", "o", "", "Output directory for keys (default: current directory)")

	ciCmd.AddCommand(ciCheckCmd)
	ciCmd.AddCommand(ciRunCmd)
	ciCmd.AddCommand(ciStatusCmd)

	ciCmd.AddCommand(ciKeygenCmd)
}

func checkPodmanAvailable() bool {
	_, err := exec.LookPath("podman")
	return err == nil
}

// findPipelineFile finds the CI pipeline file, checking default location first
func findPipelineFile(userSpecified string) (string, error) {
	// If user specified a file, use it
	if userSpecified != "" {
		if _, err := os.Stat(userSpecified); os.IsNotExist(err) {
			return "", fmt.Errorf("pipeline file not found: %s", userSpecified)
		}
		return userSpecified, nil
	}

	// Check default location: bootc-ci.yaml in current directory only
	defaultFile := config.DefaultPipelineFileName
	if _, err := os.Stat(defaultFile); err == nil {
		return defaultFile, nil
	}

	return "", fmt.Errorf("pipeline file not found: %s (use --pipeline or specify as argument)", defaultFile)
}

func checkPodmanMachineRunning() (bool, string) {
	if runtime.GOOS == "linux" {
		return true, "native"
	}

	cmd := exec.Command("podman", "machine", "list", "--format", "{{.Name}}\t{{.Running}}")
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) >= 2 && parts[1] == "true" {
			return true, parts[0]
		}
	}
	return false, ""
}

func getPodmanMachineInfo() (map[string]string, error) {
	info := make(map[string]string)

	// Updated format for newer Podman versions where resources are nested
	cmd := exec.Command("podman", "machine", "inspect", "--format",
		"{{.Name}}\t{{.Resources.CPUs}}\t{{.Resources.Memory}}\t{{.Resources.DiskSize}}\t{{.Rootful}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "\t")
	if len(parts) >= 5 {
		info["name"] = parts[0]
		info["cpus"] = parts[1]
		info["memory"] = parts[2] + " MB"
		info["disk"] = parts[3] + " GB"
		info["rootful"] = parts[4]
	}
	return info, nil
}

func runCICheck(cmd *cobra.Command, args []string) error {
	// Priority: --pipeline flag > positional argument > default
	userSpecified := ciPipeline
	if userSpecified == "" && len(args) > 0 {
		userSpecified = args[0]
	}

	pipelineFile, err := findPipelineFile(userSpecified)
	if err != nil {
		fmt.Println("❌", err)
		return err
	}

	fmt.Println("🔍 Checking CI pipeline definition file...")
	fmt.Printf("   Pipeline file: %s\n", pipelineFile)
	fmt.Println()

	// Load and validate pipeline
	pipeline, err := ci.LoadPipeline(pipelineFile)
	if err != nil {
		fmt.Printf("❌ Failed to load pipeline: %v\n", err)
		return err
	}

	// Basic validation
	fmt.Println("✅ YAML syntax: valid")
	fmt.Println("✅ Schema validation: passed")
	fmt.Printf("✅ Pipeline name: %s\n", pipeline.Metadata.Name)
	if pipeline.Metadata.Description != "" {
		fmt.Printf("   Description: %s\n", pipeline.Metadata.Description)
	}

	// Check required fields
	if pipeline.Spec.Source.Containerfile == "" {
		fmt.Println("❌ spec.source.containerfile is required")
		return fmt.Errorf("invalid pipeline: missing containerfile")
	}
	fmt.Printf("✅ Containerfile: %s\n", pipeline.Spec.Source.Containerfile)

	// Check file existence
	containerfilePath, err := pipeline.ResolveContainerfilePath()
	if err != nil {
		fmt.Printf("❌ Failed to resolve containerfile path: %v\n", err)
		return err
	}
	if _, err := os.Stat(containerfilePath); os.IsNotExist(err) {
		fmt.Printf("❌ Containerfile not found: %s\n", containerfilePath)
		return fmt.Errorf("containerfile not found")
	}
	fmt.Printf("✅ Containerfile exists: %s\n", containerfilePath)

	// Check registry authentication for base images
	fmt.Println()
	fmt.Println("🔐 Registry authentication check:")
	baseImages, err := ci.ParseBaseImages(containerfilePath)
	if err != nil {
		fmt.Printf("   ⚠️  Failed to parse Containerfile: %v\n", err)
	} else if len(baseImages) > 0 {
		// Show detected base images
		for _, img := range baseImages {
			fmt.Printf("   Base image: %s\n", img)
		}

		// Check if podman is available for auth check
		podmanClient, err := podman.NewClient()
		if err != nil {
			fmt.Printf("   ⚠️  Cannot check login status (Podman not available)\n")
		} else {
			ctx := context.Background()
			notLoggedIn, err := ci.CheckRegistryAuthStatus(ctx, containerfilePath, podmanClient)
			if err != nil {
				fmt.Printf("   ⚠️  Failed to check registry auth: %v\n", err)
			} else if len(notLoggedIn) > 0 {
				fmt.Println()
				fmt.Println("   ⚠️  The following registries require authentication:")
				for _, reg := range notLoggedIn {
					fmt.Printf("      • %s\n", reg.Registry)
					fmt.Printf("        %s\n", reg.Description)
					fmt.Printf("        Run: %s\n", reg.LoginCmd)
				}
			} else {
				// Check if any base images use known auth registries
				requiresAuth := false
				for _, img := range baseImages {
					for _, regInfo := range ci.KnownAuthRegistries {
						if strings.HasPrefix(img, regInfo.Registry+"/") {
							requiresAuth = true
							break
						}
					}
					if requiresAuth {
						break
					}
				}
				if requiresAuth {
					fmt.Println("   ✅ Registry authentication: logged in")
				} else {
					fmt.Println("   ✅ No private registry authentication required")
				}
			}
		}
	} else {
		fmt.Println("   ⚠️  No base images detected")
	}

	// Check stages configuration
	fmt.Println()
	fmt.Println("📋 Configured stages:")
	stages := []struct {
		name string
		cfg  interface{}
	}{
		{"validate", pipeline.Spec.Validate},
		{"build", pipeline.Spec.Build},
		{"scan", pipeline.Spec.Scan},
		{"convert", pipeline.Spec.Convert},
		{"test", pipeline.Spec.Test},
		{"release", pipeline.Spec.Release},
	}

	for _, s := range stages {
		if s.cfg != nil {
			fmt.Printf("   ✅ %s: configured\n", s.name)
		} else {
			fmt.Printf("   ⚪ %s: not configured\n", s.name)
		}
	}

	// Check Podman environment
	fmt.Println()
	fmt.Println("🐳 Podman environment:")
	if !checkPodmanAvailable() {
		fmt.Println("   ❌ Podman is not installed")
		fmt.Println("      Install Podman Desktop: https://podman-desktop.io/")
	} else {
		fmt.Println("   ✅ Podman: installed")

		if runtime.GOOS == "linux" {
			fmt.Println("   ✅ Running on Linux (native Podman, no machine required)")
		} else {
			// macOS: check Podman Machine
			running, name := checkPodmanMachineRunning()
			if !running {
				fmt.Println("   ❌ Podman Machine is not running")
				fmt.Println("      Run: podman machine start")
			} else {
				fmt.Printf("   ✅ Podman Machine '%s' is running\n", name)

				// Compare with recommended settings
				info, err := getPodmanMachineInfo()
				if err == nil {
					rec := ci.RecommendedMachineConfig()
					checkMachineSetting("CPUs", info["cpus"], rec.CPUs)
					checkMachineSetting("Memory", strings.TrimSuffix(info["memory"], " MB"), rec.Memory)
					checkMachineSetting("Disk", strings.TrimSuffix(info["disk"], " GB"), rec.Disk)
					if info["rootful"] != "true" {
						fmt.Println("   ⚠️  Rootful: disabled (required for bootc-image-builder)")
						fmt.Println("      Run: podman machine stop && podman machine set --rootful && podman machine start")
					} else {
						fmt.Println("   ✅ Rootful: enabled")
					}
				}
			}
		}
	}

	// Check tool versions (macOS: gvproxy + vfkit, Linux: gvproxy)
	fmt.Println()
	fmt.Println("🔧 Tool versions:")
	if gvVersion := config.GetGvproxyVersion(); gvVersion != "" {
		if config.CompareVersions(gvVersion, config.MinGvproxyVersion) >= 0 {
			fmt.Printf("   ✅ gvproxy: %s (required: ≥%s)\n", gvVersion, config.MinGvproxyVersion)
		} else {
			fmt.Printf("   ❌ gvproxy: %s (required: ≥%s)\n", gvVersion, config.MinGvproxyVersion)
			fmt.Println("      Update: brew reinstall bootc-man")
		}
		fmt.Printf("      Path: %s\n", config.FindGvproxyBinary())
	} else {
		fmt.Printf("   ⚠️  gvproxy: not found (required: ≥%s)\n", config.MinGvproxyVersion)
	}

	if runtime.GOOS == "darwin" {
		if vfVersion := config.GetVfkitVersion(); vfVersion != "" {
			if config.CompareVersions(vfVersion, config.MinVfkitVersion) >= 0 {
				fmt.Printf("   ✅ vfkit: %s (required: ≥%s)\n", vfVersion, config.MinVfkitVersion)
			} else {
				fmt.Printf("   ❌ vfkit: %s (required: ≥%s)\n", vfVersion, config.MinVfkitVersion)
				fmt.Println("      Update: brew reinstall bootc-man")
			}
			fmt.Printf("      Path: %s\n", config.FindVfkitBinary())
		} else {
			fmt.Printf("   ⚠️  vfkit: not found (required: ≥%s)\n", config.MinVfkitVersion)
		}
	}

	// Check registry status if release stage uses host.containers.internal
	if pipeline.Spec.Release != nil && strings.Contains(pipeline.Spec.Release.Registry, "host.containers.internal") {
		fmt.Println()
		if err := checkLocalRegistryStatus(context.Background()); err != nil {
			// Warning only, don't fail the check
			fmt.Printf("⚠️  %s\n", err.Error())
		}
	}

	// Check cosign key file if signing is enabled
	if pipeline.Spec.Release != nil && pipeline.Spec.Release.Sign != nil && pipeline.Spec.Release.Sign.Enabled {
		fmt.Println()
		keyPath := pipeline.Spec.Release.Sign.Key
		if keyPath == "" {
			fmt.Println("❌ Cosign signing is enabled but sign.key is not specified")
			return fmt.Errorf("cosign key path not specified")
		}

		// Resolve key path relative to pipeline file
		if !filepath.IsAbs(keyPath) {
			keyPath = filepath.Join(pipeline.BaseDir(), keyPath)
		}

		if _, err := os.Stat(keyPath); os.IsNotExist(err) {
			fmt.Printf("❌ Cosign key file not found: %s\n", keyPath)
			fmt.Println("   Generate key pair with: bootc-man ci keygen")
			return fmt.Errorf("cosign key file not found")
		}
		fmt.Printf("✅ Cosign key file: %s\n", keyPath)

		// Also check for public key (informational)
		pubKeyPath := strings.TrimSuffix(keyPath, ".key") + ".pub"
		if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
			fmt.Printf("⚠️  Cosign public key not found: %s\n", pubKeyPath)
		} else {
			fmt.Printf("✅ Cosign public key: %s\n", pubKeyPath)
		}
	}

	fmt.Println()
	fmt.Println("✅ Pipeline definition is valid")
	return nil
}

func runCIRun(cmd *cobra.Command, args []string) error {
	// Priority: --pipeline flag > positional argument > default
	userSpecified := ciPipeline
	if userSpecified == "" && len(args) > 0 {
		userSpecified = args[0]
	}

	pipelineFile, err := findPipelineFile(userSpecified)
	if err != nil {
		fmt.Println("❌", err)
		return err
	}

	fmt.Println("🚀 Running CI pipeline...")
	fmt.Printf("   Pipeline file: %s\n", pipelineFile)
	fmt.Printf("   Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if ciStage != "" {
		fmt.Printf("   Stage(s): %s\n", ciStage)
	}
	if dryRun {
		fmt.Println("   Mode: dry-run")
	}
	fmt.Println()

	// Parse stages if specified
	var stagesToRun []string
	if ciStage != "" {
		var err error
		stagesToRun, err = parseStages(ciStage)
		if err != nil {
			// Check if this is a completion request (via __complete command)
			// If so, don't show error - let completion handle it
			if len(os.Args) > 1 && os.Args[1] == "__complete" {
				// This is a completion request, return nil to allow completion to proceed
				return nil
			}
			fmt.Printf("❌ %v\n", err)
			return err
		}
	}

	// Skip Podman checks in dry-run mode
	// Also skip Podman checks for test stage (uses vfkit/QEMU directly)
	skipPodmanCheck := dryRun
	if !skipPodmanCheck && len(stagesToRun) > 0 {
		// Check if test stage is the only stage
		skipPodmanCheck = len(stagesToRun) == 1 && stagesToRun[0] == "test"
	}
	if !skipPodmanCheck {
		// Check Podman
		if !checkPodmanAvailable() {
			fmt.Println("❌ Podman is not installed.")
			fmt.Println("   Install Podman Desktop: https://podman-desktop.io/")
			return fmt.Errorf("podman not found")
		}

		// Check Podman Machine (macOS only; Windows not implemented)
		if runtime.GOOS != "linux" {
			running, name := checkPodmanMachineRunning()
			if !running {
				fmt.Println("❌ Podman Machine is not running.")
				fmt.Println("   Start it with: podman machine start")
				return fmt.Errorf("podman machine not running")
			}
			fmt.Printf("✅ Podman Machine '%s' is running\n", name)
		} else {
			fmt.Println("✅ Running on Linux (native Podman)")
		}
		fmt.Println()
	}

	// Load pipeline
	pipeline, err := ci.LoadPipeline(pipelineFile)
	if err != nil {
		fmt.Printf("❌ Failed to load pipeline: %v\n", err)
		return err
	}

	// Initialize Podman client
	podmanClient, err := podman.NewClient()
	if err != nil {
		fmt.Printf("❌ Failed to initialize Podman client: %v\n", err)
		return err
	}

	ctx := context.Background()

	// Execute stages
	if len(stagesToRun) == 0 {
		// Run all enabled stages
		return runAllStages(ctx, pipeline, podmanClient, dryRun, verbose)
	}

	// Run specified stages in order
	return runStages(ctx, stagesToRun, pipeline, podmanClient, dryRun, verbose)
}

// parseStages parses comma-separated stage names and validates them
// Returns stages and error if any invalid stage is found
// Note: If the string ends with a comma, it indicates incomplete input and returns an error
func parseStages(stageStr string) ([]string, error) {
	// Check if the string ends with a comma - this indicates incomplete input
	// (e.g., "build," means user typed a comma but didn't complete the next stage)
	if strings.HasSuffix(stageStr, ",") {
		return nil, fmt.Errorf("incomplete stage specification: stage list ends with a comma")
	}

	parts := strings.Split(stageStr, ",")
	var stages []string
	seen := make(map[string]bool)
	var invalidStages []string

	for _, part := range parts {
		stage := strings.TrimSpace(part)
		if stage == "" {
			// Empty part indicates incomplete input (e.g., "build,," or ",build")
			return nil, fmt.Errorf("incomplete stage specification: empty stage name found")
		}
		// Validate stage name
		valid := false
		for _, validStage := range stageOrder {
			if stage == validStage {
				valid = true
				break
			}
		}
		if !valid {
			invalidStages = append(invalidStages, stage)
			continue
		}
		// Skip duplicates
		if !seen[stage] {
			stages = append(stages, stage)
			seen[stage] = true
		}
	}

	// If there are invalid stages, return error with suggestions
	if len(invalidStages) > 0 {
		var suggestions []string
		for _, invalid := range invalidStages {
			// Find closest match
			for _, validStage := range stageOrder {
				if strings.HasPrefix(validStage, invalid) || strings.Contains(validStage, invalid) {
					suggestions = append(suggestions, fmt.Sprintf("  '%s' -> '%s'", invalid, validStage))
					break
				}
			}
		}
		errMsg := fmt.Sprintf("invalid stage(s): %s\nValid stages: %s", strings.Join(invalidStages, ", "), strings.Join(stageOrder, ", "))
		if len(suggestions) > 0 {
			errMsg += "\n\nDid you mean:\n" + strings.Join(suggestions, "\n")
		}
		return nil, errors.New(errMsg)
	}

	// Sort stages according to stageOrder
	return sortStagesByOrder(stages), nil
}

// sortStagesByOrder sorts stages according to the defined stage order
func sortStagesByOrder(stages []string) []string {
	stageIndex := make(map[string]int)
	for i, stage := range stageOrder {
		stageIndex[stage] = i
	}

	// Sort stages by their order
	sorted := make([]string, 0, len(stages))
	for _, orderedStage := range stageOrder {
		for _, stage := range stages {
			if stage == orderedStage {
				sorted = append(sorted, stage)
				break
			}
		}
	}

	return sorted
}

// runStages runs multiple stages in the correct order
func runStages(ctx context.Context, stageNames []string, pipeline *ci.Pipeline, podmanClient *podman.Client, dryRun, verbose bool) error {
	fmt.Printf("📋 Running stages: %s\n", strings.Join(stageNames, ", "))
	fmt.Println()

	for _, stageName := range stageNames {
		if err := runStage(ctx, stageName, pipeline, podmanClient, dryRun, verbose); err != nil {
			return fmt.Errorf("stage %s failed: %w", stageName, err)
		}
	}

	fmt.Println()
	fmt.Println("✅ All specified stages completed successfully")
	return nil
}

// runAllStages runs all enabled stages in order
func runAllStages(ctx context.Context, pipeline *ci.Pipeline, podmanClient *podman.Client, dryRun, verbose bool) error {
	fmt.Println("📋 Running all enabled stages...")
	fmt.Println()

	stages := []struct {
		name string
		run  func() error
	}{
		{"validate", func() error {
			if pipeline.Spec.Validate == nil {
				return nil // Skip if not configured
			}
			return runValidateStage(ctx, pipeline, podmanClient, dryRun, verbose)
		}},
		{"build", func() error {
			if pipeline.Spec.Build == nil {
				return fmt.Errorf("build stage is not configured in pipeline")
			}
			return runBuildStage(ctx, pipeline, podmanClient, dryRun, verbose)
		}},
		{"scan", func() error {
			if pipeline.Spec.Scan == nil {
				return nil
			}
			// Get image tag from build stage
			imageTag := generateImageTag(pipeline)
			return runScanStage(ctx, pipeline, podmanClient, imageTag, dryRun, verbose)
		}},
		{"convert", func() error {
			if pipeline.Spec.Convert == nil {
				return nil
			}
			// Get image tag from build stage
			imageTag := generateImageTag(pipeline)
			return runConvertStage(ctx, pipeline, podmanClient, imageTag, dryRun, verbose)
		}},
		{"test", func() error {
			if pipeline.Spec.Test == nil {
				return nil
			}
			// Get image tag from build stage
			imageTag := generateImageTag(pipeline)
			return runTestStage(ctx, pipeline, imageTag, dryRun, verbose)
		}},
		{"release", func() error {
			if pipeline.Spec.Release == nil {
				return nil
			}
			imageTag := generateImageTag(pipeline)
			return runReleaseStage(ctx, pipeline, podmanClient, imageTag, dryRun, verbose)
		}},
	}

	for _, stage := range stages {
		if err := stage.run(); err != nil {
			return fmt.Errorf("stage %s failed: %w", stage.name, err)
		}
	}

	fmt.Println()
	fmt.Println("✅ All stages completed successfully")
	return nil
}

// runStage runs a specific stage
func runStage(ctx context.Context, stageName string, pipeline *ci.Pipeline, podmanClient *podman.Client, dryRun, verbose bool) error {
	switch stageName {
	case "validate":
		return runValidateStage(ctx, pipeline, podmanClient, dryRun, verbose)
	case "build":
		return runBuildStage(ctx, pipeline, podmanClient, dryRun, verbose)
	case "scan":
		imageTag := generateImageTag(pipeline)
		return runScanStage(ctx, pipeline, podmanClient, imageTag, dryRun, verbose)
	case "convert":
		imageTag := generateImageTag(pipeline)
		return runConvertStage(ctx, pipeline, podmanClient, imageTag, dryRun, verbose)
	case "test":
		imageTag := generateImageTag(pipeline)
		return runTestStage(ctx, pipeline, imageTag, dryRun, verbose)
	case "release":
		imageTag := generateImageTag(pipeline)
		return runReleaseStage(ctx, pipeline, podmanClient, imageTag, dryRun, verbose)
	default:
		return fmt.Errorf("unknown stage: %s", stageName)
	}
}

// runValidateStage executes the validate stage
func runValidateStage(ctx context.Context, pipeline *ci.Pipeline, podmanClient *podman.Client, dryRun, verbose bool) error {
	if pipeline.Spec.Validate == nil {
		return fmt.Errorf("validate stage is not configured")
	}

	fmt.Println(stageSeparator)
	fmt.Println("📋 Stage 1: Validate")
	fmt.Println(stageSeparator)
	fmt.Println()

	if dryRun {
		fmt.Println("🔍 [DRY-RUN] Would execute validate stage:")
		if pipeline.Spec.Validate.ContainerfileLint != nil && pipeline.Spec.Validate.ContainerfileLint.Enabled {
			containerfilePath, _ := pipeline.ResolveContainerfilePath()
			fmt.Printf("   - Containerfile lint (hadolint):\n")
			fmt.Printf("     podman run --rm -i %s < %s\n", config.DefaultHadolintImage, containerfilePath)
			if pipeline.Spec.Validate.ContainerfileLint.RequireBootcLint {
				fmt.Printf("   - Check for 'bootc container lint' in Containerfile\n")
			}
		}
		if pipeline.Spec.Validate.ConfigToml != nil && pipeline.Spec.Validate.ConfigToml.Enabled {
			configPath := pipeline.Spec.Validate.ConfigToml.Path
			if !filepath.IsAbs(configPath) {
				contextPath, _ := pipeline.ResolveContextPath()
				configPath = filepath.Join(contextPath, configPath)
			}
			fmt.Printf("   - Validate config.toml: %s\n", configPath)
		}
		if pipeline.Spec.Validate.SecretDetection != nil && pipeline.Spec.Validate.SecretDetection.Enabled {
			tool := pipeline.Spec.Validate.SecretDetection.Tool
			if tool == "" {
				tool = "gitleaks"
			}
			contextPath, _ := pipeline.ResolveContextPath()
			var image string
			switch tool {
			case "gitleaks":
				image = config.DefaultGitleaksImage
			case "trufflehog":
				image = config.DefaultTrufflehogImage
			}
			fmt.Printf("   - Secret detection (%s):\n", tool)
			fmt.Printf("     podman run --rm -v %s:/workspace %s\n", contextPath, image)
		}
		return nil
	}

	validateStage := ci.NewValidateStage(pipeline, podmanClient, verbose)
	if err := validateStage.Execute(ctx); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Validate stage completed successfully")
	return nil
}

// runBuildStage executes the build stage
func runBuildStage(ctx context.Context, pipeline *ci.Pipeline, podmanClient *podman.Client, dryRun, verbose bool) error {
	if pipeline.Spec.Build == nil {
		return fmt.Errorf("build stage is not configured")
	}

	fmt.Println(stageSeparator)
	fmt.Println("📋 Stage 2: Build")
	fmt.Println(stageSeparator)
	fmt.Println()

	if dryRun {
		fmt.Println("🔍 [DRY-RUN] Would execute build stage:")
		containerfilePath, _ := pipeline.ResolveContainerfilePath()
		contextPath, _ := pipeline.ResolveContextPath()

		// Generate image tag (same logic as internal/ci/build.go)
		imageTag := generateImageTag(pipeline)

		// Determine platforms
		platforms := pipeline.Spec.Build.Platforms
		if len(platforms) == 0 {
			// Default to native platform based on host architecture
			if runtime.GOARCH == "arm64" {
				platforms = []string{"linux/arm64"}
			} else {
				platforms = []string{"linux/amd64"}
			}
		}

		for _, platform := range platforms {
			// Generate platform-specific tag for multi-arch
			tag := imageTag
			if len(platforms) > 1 {
				platformSuffix := strings.ReplaceAll(platform, "/", "-")
				tag = fmt.Sprintf("%s-%s", imageTag, platformSuffix)
			}

			// Build the command arguments
			args := []string{"build", "-t", tag, "--platform", platform}

			// Calculate relative path from context to containerfile
			relPath, err := filepath.Rel(contextPath, containerfilePath)
			if err != nil {
				args = append(args, "-f", containerfilePath)
			} else {
				args = append(args, "-f", relPath)
			}

			// Add build arguments
			for key, value := range pipeline.Spec.Build.Args {
				args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
			}

			// Add labels
			for key, value := range pipeline.Spec.Build.Labels {
				args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
			}

			// Add context path
			args = append(args, contextPath)

			fmt.Printf("   podman %s\n", strings.Join(args, " "))
		}
		return nil
	}

	buildStage := ci.NewBuildStage(pipeline, podmanClient, verbose)
	if err := buildStage.Execute(ctx); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Build stage completed successfully")
	return nil
}

// generateImageTag generates an image tag from pipeline metadata
// This matches the logic in internal/ci/build.go
// If build.imageTag is specified in the pipeline, it will be used instead
func generateImageTag(pipeline *ci.Pipeline) string {
	// Check if custom image tag is specified in build config
	if pipeline.Spec.Build != nil && pipeline.Spec.Build.ImageTag != "" {
		return pipeline.Spec.Build.ImageTag
	}
	// Otherwise, generate from pipeline name
	name := strings.ToLower(pipeline.Metadata.Name)
	name = strings.ReplaceAll(name, " ", "-")
	return fmt.Sprintf("localhost/bootc-man-%s:latest", name)
}

// runScanStage executes the scan stage
func runScanStage(ctx context.Context, pipeline *ci.Pipeline, podmanClient *podman.Client, imageTag string, dryRun, verbose bool) error {
	if pipeline.Spec.Scan == nil {
		return fmt.Errorf("scan stage is not configured")
	}

	fmt.Println(stageSeparator)
	fmt.Println("📋 Stage 3: Scan")
	fmt.Println(stageSeparator)
	fmt.Println()

	if dryRun {
		fmt.Println("🔍 [DRY-RUN] Would execute scan stage:")
		archivePath := "/tmp/" + config.ScanArchiveTempPattern

		// Show image export command
		fmt.Printf("   - Export image to archive:\n")
		fmt.Printf("     podman save -o %s %s\n", archivePath, imageTag)

		if pipeline.Spec.Scan.Vulnerability != nil && pipeline.Spec.Scan.Vulnerability.Enabled {
			tool := pipeline.Spec.Scan.Vulnerability.Tool
			if tool == "" {
				tool = "trivy"
			}

			switch tool {
			case "trivy":
				trivyImage := config.DefaultTrivyImage
				args := []string{"run", "--rm", "-v", fmt.Sprintf("%s:/image.tar:ro", archivePath), trivyImage, "image", "--input", "/image.tar"}
				if pipeline.Spec.Scan.Vulnerability.Severity != "" {
					args = append(args, "--severity", pipeline.Spec.Scan.Vulnerability.Severity)
				}
				args = append(args, "--format", "table")
				if !pipeline.Spec.Scan.Vulnerability.FailOnVulnerability {
					args = append(args, "--exit-code", "0")
				}
				fmt.Printf("   - Vulnerability scan (Trivy):\n")
				fmt.Printf("     podman %s\n", strings.Join(args, " "))
			case "grype":
				grypeImage := config.DefaultGrypeImage
				args := []string{"run", "--rm", "-v", fmt.Sprintf("%s:/image.tar:ro", archivePath), grypeImage, "docker-archive:/image.tar"}
				if pipeline.Spec.Scan.Vulnerability.Severity != "" {
					args = append(args, "--fail-on", strings.ToLower(strings.Split(pipeline.Spec.Scan.Vulnerability.Severity, ",")[0]))
				}
				args = append(args, "--output", "table")
				fmt.Printf("   - Vulnerability scan (Grype):\n")
				fmt.Printf("     podman %s\n", strings.Join(args, " "))
			}
		}

		if pipeline.Spec.Scan.SBOM != nil && pipeline.Spec.Scan.SBOM.Enabled {
			tool := pipeline.Spec.Scan.SBOM.Tool
			if tool == "" {
				tool = "syft"
			}
			format := pipeline.Spec.Scan.SBOM.Format
			if format == "" {
				format = "spdx-json"
			}

			switch tool {
			case "syft":
				syftImage := config.DefaultSyftImage
				args := []string{"run", "--rm", "-v", fmt.Sprintf("%s:/image.tar:ro", archivePath), syftImage, "scan", "--output", format, "docker-archive:/image.tar"}
				fmt.Printf("   - SBOM generation (Syft):\n")
				fmt.Printf("     podman %s\n", strings.Join(args, " "))
			case "trivy":
				trivyImage := config.DefaultTrivyImage
				args := []string{"run", "--rm", "-v", fmt.Sprintf("%s:/image.tar:ro", archivePath), trivyImage, "image", "--input", "/image.tar", "--format", format}
				fmt.Printf("   - SBOM generation (Trivy):\n")
				fmt.Printf("     podman %s\n", strings.Join(args, " "))
			}
		}
		return nil
	}

	scanStage := ci.NewScanStage(pipeline, podmanClient, imageTag, verbose)
	if err := scanStage.Execute(ctx); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Scan stage completed successfully")
	return nil
}

// runConvertStage executes the convert stage
func runConvertStage(ctx context.Context, pipeline *ci.Pipeline, podmanClient *podman.Client, imageTag string, dryRun, verbose bool) error {
	if pipeline.Spec.Convert == nil {
		return fmt.Errorf("convert stage is not configured")
	}

	fmt.Println(stageSeparator)
	fmt.Println("📋 Stage 4: Convert")
	fmt.Println(stageSeparator)
	fmt.Println()

	if dryRun {
		fmt.Println("🔍 [DRY-RUN] Would execute convert stage:")
		// Show the actual command that would be executed (same as other stages)
		// On macOS, use podman machine ssh (Windows not implemented)
		useMachineSSH := runtime.GOOS != "linux"

		// Get images directory: <project-root>/output/images
		imagesDir := ci.GetImagesDir(pipeline.BaseDir())
		fmt.Printf("   Output directory: %s\n", imagesDir)

		// Generate output filename from metadata.name
		pipelineName := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(pipeline.Metadata.Name, "/", "-"), " ", "-"))

		// Get bootc-image-builder image from config
		cfg, err := config.Load("")
		if err != nil {
			cfg = config.DefaultConfig()
		}
		bootcImageBuilderImage := cfg.CI.BootcImageBuilder
		if bootcImageBuilderImage == "" {
			bootcImageBuilderImage = config.DefaultBootcImageBuilder
		}

		for _, format := range pipeline.Spec.Convert.Formats {
			outputFile := filepath.Join(imagesDir, fmt.Sprintf("%s.%s", pipelineName, format.Type))
			fmt.Printf("   Output file: %s\n", outputFile)

			// Build the command arguments (same as convertToFormat)
			image := bootcImageBuilderImage
			args := []string{"run", "--rm", "--privileged", "--security-opt", "label=type:unconfined_t", "--pull=newer"}
			args = append(args, "-v", "/var/lib/containers/storage:/var/lib/containers/storage")
			args = append(args, "-v", fmt.Sprintf("%s:/output", imagesDir))
			if format.Config != "" {
				// Resolve config path relative to pipeline file
				configPath := format.Config
				if !filepath.IsAbs(configPath) {
					// Get pipeline file directory
					pipelineFile, _ := findPipelineFile("")
					if pipelineFile != "" {
						configPath = filepath.Join(filepath.Dir(pipelineFile), format.Config)
					}
				}
				args = append(args, "-v", fmt.Sprintf("%s:/config.toml:ro", configPath))
			}
			args = append(args, image)
			args = append(args, "--type", format.Type)
			if format.Config != "" {
				args = append(args, "--config", "/config.toml")
			} else {
				args = append(args, "--rootfs", "ext4")
			}
			args = append(args, "--output", "/output")
			args = append(args, imageTag)

			if useMachineSSH {
				// Get machine name for display
				running, name := checkPodmanMachineRunning()
				if running && name != "" {
					// Remove any wildcard characters (*) from machine name
					machineName := strings.TrimSuffix(name, "*")
					fmt.Printf("   podman machine ssh %s \"sudo podman %s\"\n", machineName, strings.Join(args, " "))
				} else {
					fmt.Printf("   podman machine ssh <machine> \"sudo podman %s\"\n", strings.Join(args, " "))
				}
			} else {
				fmt.Printf("   podman %s\n", strings.Join(args, " "))
			}
		}
		return nil
	}

	// Get bootc-image-builder image from config
	cfg, err := config.Load("")
	if err != nil {
		cfg = config.DefaultConfig()
	}
	bootcImageBuilderImage := cfg.CI.BootcImageBuilder
	if bootcImageBuilderImage == "" {
		bootcImageBuilderImage = config.DefaultBootcImageBuilder
	}

	convertStage := ci.NewConvertStageWithImage(pipeline, podmanClient, imageTag, verbose, bootcImageBuilderImage)
	if err := convertStage.Execute(ctx); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Convert stage completed successfully")
	return nil
}

// runTestStage executes the test stage
func runTestStage(ctx context.Context, pipeline *ci.Pipeline, imageTag string, dryRun, verbose bool) error {
	if pipeline.Spec.Test == nil {
		return fmt.Errorf("test stage is not configured")
	}

	fmt.Println(stageSeparator)
	fmt.Println("📋 Stage 5: Test")
	fmt.Println(stageSeparator)
	fmt.Println()

	// Check if hypervisor is available (platform-specific)
	if !dryRun {
		// Create a temporary driver to check availability
		vmType := vm.GetDefaultVMType()
		tempOpts := vm.VMOptions{Name: "check"}
		driver, err := vm.NewDriver(tempOpts, false)
		if err != nil {
			fmt.Printf("❌ Failed to create VM driver: %v\n", err)
			return fmt.Errorf("VM driver not available: %w", err)
		}
		if err := driver.Available(); err != nil {
			fmt.Printf("❌ %s hypervisor is not available for test stage\n", vmType.String())
			fmt.Println()
			fmt.Println(err.Error())
			return fmt.Errorf("%s not available", vmType.String())
		}

		if verbose {
			fmt.Printf("Using hypervisor: %s\n", vmType.String())
		}
	}

	if dryRun {
		fmt.Println("🔍 [DRY-RUN] Would execute test stage:")

		// Generate VM name from pipeline name
		pipelineName := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(pipeline.Metadata.Name, "/", "-"), " ", "-"))
		vmName := pipelineName

		// Paths
		imagesDir := ci.GetImagesDir(pipeline.BaseDir())
		diskImagePath := filepath.Join(imagesDir, fmt.Sprintf("%s.raw", pipelineName))
		vmDir := config.RuntimeDir()
		socketPath := filepath.Join(vmDir, fmt.Sprintf("bootc-man-%s-gvproxy.sock", vmName))
		serviceSocketPath := filepath.Join(vmDir, fmt.Sprintf("bootc-man-%s-gvproxy-service.sock", vmName))
		logFile := filepath.Join(vmDir, fmt.Sprintf("bootc-man-%s.log", vmName))
		efiStorePath := filepath.Join(vmDir, fmt.Sprintf("bootc-man-%s-efi-store", vmName))
		sshPort := "<dynamic>"
		restfulPort := "<dynamic>"

		fmt.Printf("   Disk image: %s\n", diskImagePath)
		fmt.Println()

		// gvproxy command
		fmt.Println("   1. Start gvproxy for VM networking:")
		gvproxyArgs := []string{
			"-listen-vfkit", fmt.Sprintf("unixgram://%s", socketPath),
			"-services", fmt.Sprintf("unix://%s", serviceSocketPath),
			"-ssh-port", sshPort, // Dynamically allocated SSH port
		}
		fmt.Printf("      gvproxy %s &\n", strings.Join(gvproxyArgs, " "))
		fmt.Println()

		// vfkit command
		fmt.Println("   2. Start VM with vfkit:")
		vfkitArgs := []string{
			"--cpus", "2",
			"--memory", "4096",
			"--bootloader", fmt.Sprintf("efi,variable-store=%s,create", efiStorePath),
			"--device", fmt.Sprintf("virtio-blk,path=%s", diskImagePath),
			"--device", "virtio-rng",
			"--device", fmt.Sprintf("virtio-net,unixSocketPath=%s", socketPath),
			"--device", fmt.Sprintf("virtio-serial,logFilePath=%s", logFile),
			"--restful-uri", fmt.Sprintf("http://localhost:%s", restfulPort),
		}
		if pipeline.Spec.Test.Boot != nil && pipeline.Spec.Test.Boot.GUI {
			vfkitArgs = append(vfkitArgs, "--gui")
		}
		fmt.Printf("      vfkit %s\n", strings.Join(vfkitArgs, " "))
		fmt.Println()

		// Boot checks
		if pipeline.Spec.Test.Boot != nil && len(pipeline.Spec.Test.Boot.Checks) > 0 {
			fmt.Println("   3. Run boot checks (via SSH):")
			for _, check := range pipeline.Spec.Test.Boot.Checks {
				fmt.Printf("      ssh -i ~/.ssh/id_ed25519 -p %s -o StrictHostKeyChecking=no user@localhost \"%s\"\n", sshPort, check)
			}
		} else {
			fmt.Println("   3. Run default boot check:")
			fmt.Printf("      ssh -i ~/.ssh/id_ed25519 -p %s -o StrictHostKeyChecking=no user@localhost \"bootc status\"\n", sshPort)
		}
		fmt.Println()

		// Cleanup
		fmt.Println("   4. Cleanup:")
		fmt.Println("      - Stop VM (send SIGTERM to vfkit process)")
		fmt.Println("      - Stop gvproxy (send SIGTERM to gvproxy process)")
		fmt.Println("      - Remove test disk image copy")

		return nil
	}

	testStage := ci.NewTestStage(pipeline, imageTag, verbose)
	if err := testStage.Execute(ctx); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Test stage completed successfully")
	return nil
}

// runReleaseStage executes the release stage
func runReleaseStage(ctx context.Context, pipeline *ci.Pipeline, podmanClient *podman.Client, imageTag string, dryRun, verbose bool) error {
	if pipeline.Spec.Release == nil {
		return fmt.Errorf("release stage is not configured")
	}

	fmt.Println(stageSeparator)
	fmt.Println("📋 Stage 6: Release")
	fmt.Println(stageSeparator)
	fmt.Println()

	cfg := pipeline.Spec.Release

	if dryRun {
		fmt.Println("🔍 [DRY-RUN] Would execute release stage:")
		fmt.Printf("   Source image: %s\n", imageTag)
		fmt.Printf("   Destination: %s/%s\n", cfg.Registry, cfg.Repository)
		fmt.Printf("   Tags: %v\n", cfg.Tags)
		fmt.Println()

		// Determine TLS verification setting
		tlsVerify := true
		if cfg.TLS != nil {
			tlsVerify = *cfg.TLS
		}

		step := 1
		if len(cfg.Tags) > 0 {
			primaryRef := fmt.Sprintf("%s/%s:%s", cfg.Registry, cfg.Repository, cfg.Tags[0])
			args := []string{"push", "--digestfile", "/tmp/" + config.DigestFileTempPattern}
			if !tlsVerify {
				args = append(args, "--tls-verify=false")
			}
			args = append(args, imageTag, primaryRef)
			fmt.Printf("   %d. Push with digest:\n", step)
			fmt.Printf("      podman %s\n", strings.Join(args, " "))
			step++
		}

		if cfg.Sign != nil && cfg.Sign.Enabled {
			cosignImage := "gcr.io/projectsigstore/cosign:latest"
			digestRef := fmt.Sprintf("%s/%s@sha256:<digest>", cfg.Registry, cfg.Repository)

			// Build cosign command
			args := []string{"run", "--rm", "--network=host"}
			args = append(args, "-v", fmt.Sprintf("%s:/cosign.key:ro", cfg.Sign.Key))
			args = append(args, "-e", "COSIGN_PASSWORD=")

			// Transparency log settings
			tlogEnabled := false
			if cfg.Sign.TransparencyLog != nil {
				tlogEnabled = cfg.Sign.TransparencyLog.Enabled
			}
			if !tlogEnabled {
				args = append(args, "-e", "COSIGN_OFFLINE=1")
			}

			args = append(args, cosignImage, "sign", "--key", "/cosign.key", "--yes")

			if tlogEnabled {
				if cfg.Sign.TransparencyLog.RekorURL != "" {
					args = append(args, "--rekor-url="+cfg.Sign.TransparencyLog.RekorURL)
				}
			} else {
				args = append(args, "--use-signing-config=false", "--tlog-upload=false")
			}

			if !tlsVerify {
				args = append(args, "--allow-insecure-registry")
			}
			args = append(args, digestRef)

			fmt.Printf("   %d. Sign image:\n", step)
			fmt.Printf("      podman %s\n", strings.Join(args, " "))
			step++
		}

		for _, tag := range cfg.Tags[1:] {
			destRef := fmt.Sprintf("%s/%s:%s", cfg.Registry, cfg.Repository, tag)
			args := []string{"push"}
			if !tlsVerify {
				args = append(args, "--tls-verify=false")
			}
			args = append(args, imageTag, destRef)
			fmt.Printf("   %d. Push additional tag:\n", step)
			fmt.Printf("      podman %s\n", strings.Join(args, " "))
			step++
		}
		return nil
	}

	releaseStage := ci.NewReleaseStage(pipeline, podmanClient, imageTag, verbose)
	if err := releaseStage.Execute(ctx); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✅ Release stage completed successfully")
	return nil
}

func runCIStatus(cmd *cobra.Command, args []string) error {
	// Delegate to the main status command
	// Note: Cobra will automatically print the deprecation warning
	return runStatus(cmd, args)
}

// checkLocalRegistryStatus checks if the local registry (bootc-man-registry) is running
// Returns an error with a warning message if the registry is not running
func checkLocalRegistryStatus(ctx context.Context) error {
	// Initialize Podman client
	podmanClient, err := podman.NewClient()
	if err != nil {
		return fmt.Errorf("Registry check: Cannot initialize Podman client: %v", err)
	}

	containerName := config.ContainerNameRegistry

	// Check if registry container exists
	exists, err := podmanClient.Exists(ctx, containerName)
	if err != nil {
		return fmt.Errorf("Registry check: Cannot check container status: %v", err)
	}

	if !exists {
		return fmt.Errorf("Registry check: Local registry (%s) is not created.\n"+
			"   Release stage requires the registry to push images to host.containers.internal.\n"+
			"   Run: bootc-man registry up", containerName)
	}

	// Check if container is running
	info, err := podmanClient.Inspect(ctx, containerName)
	if err != nil {
		return fmt.Errorf("Registry check: Cannot inspect container: %v", err)
	}

	if !info.State.Running {
		return fmt.Errorf("Registry check: Local registry (%s) is stopped.\n"+
			"   Release stage requires the registry to push images to host.containers.internal.\n"+
			"   Run: bootc-man registry up", containerName)
	}

	fmt.Printf("✅ Registry check: Local registry (%s) is running\n", containerName)
	return nil
}

// checkMachineSetting compares a Podman Machine setting against the recommended value
func checkMachineSetting(label string, actual string, recommended int) {
	val, err := strconv.Atoi(actual)
	if err != nil {
		fmt.Printf("   ⚠️  %s: %s (recommended: %d)\n", label, actual, recommended)
		return
	}
	if val < recommended {
		fmt.Printf("   ⚠️  %s: %d (recommended: %d)\n", label, val, recommended)
	} else {
		fmt.Printf("   ✅ %s: %d\n", label, val)
	}
}

func runCIKeygen(cmd *cobra.Command, args []string) error {
	// Check Podman
	if !checkPodmanAvailable() {
		fmt.Println("❌ Podman is not installed.")
		fmt.Println("   Install Podman Desktop: https://podman-desktop.io/")
		return fmt.Errorf("podman not found")
	}

	// Check Podman Machine (macOS only; Windows not implemented)
	if runtime.GOOS != "linux" {
		running, _ := checkPodmanMachineRunning()
		if !running {
			fmt.Println("❌ Podman Machine is not running.")
			fmt.Println("   Start it with: podman machine start")
			return fmt.Errorf("podman machine not running")
		}
	}

	ctx := context.Background()
	opts := ci.KeygenOptions{
		OutputDir: keygenOutputDir,
		Verbose:   verbose,
	}

	return ci.GenerateCosignKeyPair(ctx, opts)
}
