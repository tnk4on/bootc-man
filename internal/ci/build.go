package ci

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/tnk4on/bootc-man/internal/podman"
)

// RegistryAuthInfo contains information about a registry that requires authentication
type RegistryAuthInfo struct {
	Registry    string
	LoginCmd    string
	Description string
}

// KnownAuthRegistries lists registries that require authentication
// Exported for use by CLI commands (ci check)
var KnownAuthRegistries = []RegistryAuthInfo{
	{
		Registry:    "registry.redhat.io",
		LoginCmd:    "podman login registry.redhat.io",
		Description: "Red Hat Container Registry (requires Red Hat subscription)",
	},
	{
		Registry:    "registry.connect.redhat.com",
		LoginCmd:    "podman login registry.connect.redhat.com",
		Description: "Red Hat Partner Connect Registry",
	},
}

// BuildStage executes the build stage
type BuildStage struct {
	pipeline *Pipeline
	podman   *podman.Client
	verbose  bool
}

// NewBuildStage creates a new build stage executor
func NewBuildStage(pipeline *Pipeline, podmanClient *podman.Client, verbose bool) *BuildStage {
	return &BuildStage{
		pipeline: pipeline,
		podman:   podmanClient,
		verbose:  verbose,
	}
}

// Execute runs the build stage
func (b *BuildStage) Execute(ctx context.Context) error {
	if b.pipeline.Spec.Build == nil {
		return fmt.Errorf("build stage is not configured")
	}

	cfg := b.pipeline.Spec.Build
	// cfg should not be nil at this point, but handle it just in case
	if cfg == nil {
		// Create empty config if nil (shouldn't happen, but be defensive)
		cfg = &BuildConfig{}
	}

	// Resolve paths
	containerfilePath, err := b.pipeline.ResolveContainerfilePath()
	if err != nil {
		return fmt.Errorf("failed to resolve containerfile path: %w", err)
	}

	// Check for registries that require authentication
	if err := b.checkRegistryAuth(ctx, containerfilePath); err != nil {
		return err
	}

	contextPath, err := b.pipeline.ResolveContextPath()
	if err != nil {
		return fmt.Errorf("failed to resolve context path: %w", err)
	}

	// Generate image tag from pipeline name or use custom tag if specified
	imageTag := b.generateImageTag()
	if cfg.ImageTag != "" {
		// Use custom image tag if specified
		imageTag = cfg.ImageTag
	}

	// Build for each platform (or single build if no platforms specified)
	platforms := cfg.Platforms
	if len(platforms) == 0 {
		// Default to native platform based on host architecture
		platforms = []string{b.getDefaultPlatform()}
	}

	for _, platform := range platforms {
		if err := b.buildForPlatform(ctx, containerfilePath, contextPath, imageTag, platform, cfg); err != nil {
			return fmt.Errorf("build failed for platform %s: %w", platform, err)
		}
	}

	return nil
}

// buildForPlatform builds the image for a specific platform
func (b *BuildStage) buildForPlatform(ctx context.Context, containerfilePath, contextPath, imageTag, platform string, cfg *BuildConfig) error {
	// Generate platform-specific tag
	tag := imageTag
	if len(b.pipeline.Spec.Build.Platforms) > 1 {
		// Add platform suffix for multi-arch builds
		platformSuffix := strings.ReplaceAll(platform, "/", "-")
		tag = fmt.Sprintf("%s-%s", imageTag, platformSuffix)
	}

	// Calculate relative path from context to containerfile
	relPath, err := filepath.Rel(contextPath, containerfilePath)
	var containerfileRelPath, containerfileAbsPath string
	if err != nil {
		// If relative path calculation fails, use absolute path
		containerfileAbsPath = containerfilePath
	} else {
		containerfileRelPath = relPath
	}

	// Build arguments using the pure function
	buildArgs := BuildPodmanBuildArgs(BuildArgsOptions{
		Tag:                  tag,
		Platform:             platform,
		ContainerfileRelPath: containerfileRelPath,
		ContainerfileAbsPath: containerfileAbsPath,
		ContextPath:          contextPath,
		BuildArgs:            cfg.Args,
		Labels:               cfg.Labels,
	})

	if b.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(buildArgs, " "))
	}

	// Execute build using podman client
	// Note: We need to use exec.Command directly since podman client's Build method
	// doesn't support all the options we need (platform, build-args, labels)
	return b.runBuildCommand(ctx, buildArgs)
}

// runBuildCommand executes podman build command
// With rootful mode on macOS, podman commands go through the rootful (Windows not implemented)
// connection automatically, so we can use the same code path as Linux.
func (b *BuildStage) runBuildCommand(ctx context.Context, args []string) error {
	if b.verbose {
		fmt.Printf("Running: podman %s\n", strings.Join(args, " "))
	}

	// With rootful mode, podman build goes through the rootful socket
	// and the image is stored in root storage (accessible by convert stage)
	cmd := b.podman.Command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// getDefaultPlatform returns the default platform based on host architecture
func (b *BuildStage) getDefaultPlatform() string {
	arch := runtime.GOARCH
	switch arch {
	case "arm64":
		return "linux/arm64"
	case "amd64", "x86_64":
		return "linux/amd64"
	default:
		// Default to amd64 for unknown architectures
		return "linux/amd64"
	}
}

// generateImageTag generates an image tag from pipeline metadata
func (b *BuildStage) generateImageTag() string {
	// Use localhost registry for staging
	// Format: localhost/bootc-man-<pipeline-name>:latest
	name := strings.ToLower(b.pipeline.Metadata.Name)
	// Replace spaces and special characters with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	return fmt.Sprintf("localhost/bootc-man-%s:latest", name)
}

// BuildArgsOptions contains options for building podman build arguments
type BuildArgsOptions struct {
	Tag             string
	Platform        string
	ContainerfileRelPath string // Relative path from context to Containerfile
	ContainerfileAbsPath string // Absolute path (fallback if relative fails)
	ContextPath     string
	BuildArgs       map[string]string
	Labels          map[string]string
}

// BuildPodmanBuildArgs constructs the argument list for podman build command
// This is a pure function that can be easily unit tested
func BuildPodmanBuildArgs(opts BuildArgsOptions) []string {
	args := []string{"build"}
	
	// Add tag
	if opts.Tag != "" {
		args = append(args, "-t", opts.Tag)
	}
	
	// Add platform
	if opts.Platform != "" {
		args = append(args, "--platform", opts.Platform)
	}
	
	// Add Dockerfile path (prefer relative, fallback to absolute)
	if opts.ContainerfileRelPath != "" {
		args = append(args, "-f", opts.ContainerfileRelPath)
	} else if opts.ContainerfileAbsPath != "" {
		args = append(args, "-f", opts.ContainerfileAbsPath)
	}
	
	// Add build arguments (sorted for deterministic output)
	for key, value := range opts.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}
	
	// Add labels (sorted for deterministic output)
	for key, value := range opts.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}
	
	// Add context path
	if opts.ContextPath != "" {
		args = append(args, opts.ContextPath)
	}
	
	return args
}

// checkRegistryAuth checks if any base images in the Containerfile require authentication
// and warns the user if they are not logged in
func (b *BuildStage) checkRegistryAuth(ctx context.Context, containerfilePath string) error {
	// Parse base images from Containerfile
	baseImages, err := ParseBaseImages(containerfilePath)
	if err != nil {
		// Don't fail on parse errors, just skip the check
		if b.verbose {
			fmt.Printf("Warning: failed to parse Containerfile for registry check: %v\n", err)
		}
		return nil
	}

	// Check each base image against known auth registries
	var notLoggedIn []RegistryAuthInfo
	for _, image := range baseImages {
		for _, regInfo := range KnownAuthRegistries {
			if strings.HasPrefix(image, regInfo.Registry+"/") {
				// Check if user is logged in
				loggedIn, err := b.podman.IsLoggedIn(ctx, regInfo.Registry)
				if err != nil {
					if b.verbose {
						fmt.Printf("Warning: failed to check login status for %s: %v\n", regInfo.Registry, err)
					}
					continue
				}
				if !loggedIn {
					// Avoid duplicates
					found := false
					for _, ni := range notLoggedIn {
						if ni.Registry == regInfo.Registry {
							found = true
							break
						}
					}
					if !found {
						notLoggedIn = append(notLoggedIn, regInfo)
					}
				}
			}
		}
	}

	// Display warnings for registries that require authentication
	if len(notLoggedIn) > 0 {
		fmt.Println()
		fmt.Println("⚠️  Registry Authentication Required")
		fmt.Println("────────────────────────────────────────────────────────────────────────────────")
		fmt.Println("The following registries require authentication:")
		fmt.Println()
		for _, reg := range notLoggedIn {
			fmt.Printf("  • %s\n", reg.Registry)
			fmt.Printf("    %s\n", reg.Description)
			fmt.Printf("    Run: %s\n", reg.LoginCmd)
			fmt.Println()
		}
		fmt.Println("Please login before running the build.")
		fmt.Println("────────────────────────────────────────────────────────────────────────────────")
		fmt.Println()
		return fmt.Errorf("registry authentication required: please run '%s' first", notLoggedIn[0].LoginCmd)
	}

	return nil
}

// CheckRegistryAuthStatus checks if any base images in the Containerfile require authentication
// and returns the list of registries that are not logged in.
// This is a standalone function for use by CLI commands (ci check).
func CheckRegistryAuthStatus(ctx context.Context, containerfilePath string, podmanClient *podman.Client) ([]RegistryAuthInfo, error) {
	// Parse base images from Containerfile
	baseImages, err := ParseBaseImages(containerfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Containerfile: %w", err)
	}

	// Check each base image against known auth registries
	var notLoggedIn []RegistryAuthInfo
	for _, image := range baseImages {
		for _, regInfo := range KnownAuthRegistries {
			if strings.HasPrefix(image, regInfo.Registry+"/") {
				// Check if user is logged in
				loggedIn, err := podmanClient.IsLoggedIn(ctx, regInfo.Registry)
				if err != nil {
					continue
				}
				if !loggedIn {
					// Avoid duplicates
					found := false
					for _, ni := range notLoggedIn {
						if ni.Registry == regInfo.Registry {
							found = true
							break
						}
					}
					if !found {
						notLoggedIn = append(notLoggedIn, regInfo)
					}
				}
			}
		}
	}

	return notLoggedIn, nil
}

// ParseBaseImages extracts base image references from a Containerfile
// It parses FROM instructions including multi-stage builds
// Exported for use by CLI commands (ci check)
func ParseBaseImages(containerfilePath string) ([]string, error) {
	file, err := os.Open(containerfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Containerfile: %w", err)
	}
	defer file.Close()

	var images []string
	// Regex to match FROM instructions
	// Handles: FROM image, FROM image AS name, FROM image:tag, FROM image@digest
	fromRegex := regexp.MustCompile(`(?i)^\s*FROM\s+([^\s]+)`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := fromRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			image := matches[1]
			// Skip ARG variable references like $BASE_IMAGE or ${BASE_IMAGE}
			if strings.HasPrefix(image, "$") {
				continue
			}
			// Skip scratch (special case for multi-stage builds)
			if image == "scratch" {
				continue
			}
			images = append(images, image)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read Containerfile: %w", err)
	}

	return images, nil
}
