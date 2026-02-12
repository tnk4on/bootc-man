package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/format"
	"github.com/tnk4on/bootc-man/internal/podman"
)

// formatContainerError formats container errors with clear separation
func formatContainerError(context string, err error) error {
	var podErr *podman.PodmanError
	if errors.As(err, &podErr) {
		return fmt.Errorf("%s: %w", context, podErr)
	}
	return fmt.Errorf("%s: %w", context, err)
}

// Container command and subcommands
var containerCmd = &cobra.Command{
	Use:   "container",
	Short: "Manage bootc container images",
	Long: `Manage bootc container images.

Provides commands to build, run, and manage bootc container images.
This is a wrapper around podman commands with bootc-specific defaults.`,
}

// container build
var containerBuildCmd = &cobra.Command{
	Use:   "build [options] [CONTEXT]",
	Short: "Build a bootc image from a Containerfile",
	Long: `Build a bootc image from a Containerfile.

This command wraps 'podman build' with the same option patterns.
CONTEXT defaults to the current directory if not specified.

Equivalent to: podman build [options] CONTEXT

Example:
  bootc-man container build -t localhost:5000/my-bootc:latest .
  bootc-man container build -t my-image -f Containerfile.bootc .
  bootc-man container build -t my-image --no-cache ./myapp`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runContainerBuild,
	SilenceUsage: true,
}

// container run
var containerRunCmd = &cobra.Command{
	Use:   "run [image]",
	Short: "Run a bootc image interactively",
	Long: `Run a bootc image interactively with /bin/bash.

Equivalent to: podman run -it --rm IMAGE /bin/bash

The container is automatically removed when the shell exits.
If no image is specified, a selection menu will be shown.

Example:
  bootc-man container run                              # Select from available images
  bootc-man container run quay.io/fedora/fedora-bootc:42
  bootc-man container run localhost:5000/my-bootc:latest`,
	Args:         cobra.MaximumNArgs(1),
	RunE:         runContainerRun,
	SilenceUsage: true,
}

// container push
var containerPushCmd = &cobra.Command{
	Use:   "push [image] [destination]",
	Short: "Push a bootc image to a registry",
	Long: `Push a bootc image to a registry.

Equivalent to: podman push IMAGE [DESTINATION]

If no image is specified, a selection menu will be shown.
If destination is specified, the image will be pushed to that location.

Example:
  bootc-man container push                                                   # Select from available images
  bootc-man container push localhost/my-bootc:latest                         # Push to original registry
  bootc-man container push localhost/my-bootc registry.example.com/my-bootc  # Push to different registry
  bootc-man container push my-bootc:latest --tls-verify=false`,
	Args:         cobra.MaximumNArgs(2),
	RunE:         runContainerPush,
	SilenceUsage: true,
}

// container image parent command
var containerImageCmd = &cobra.Command{
	Use:   "image",
	Short: "Manage bootc images",
	Long: `Manage bootc images in the local container storage.

Lists, inspects, and removes bootc images (images with containers.bootc=1 label).`,
}

// container image list
var containerImageListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List bootc images",
	Long: `List bootc images in the local container storage.

By default, only images with the containers.bootc=1 label are shown.
Use --all to show all images.

Equivalent to: podman images --filter=label=containers.bootc=1`,
	RunE:         runContainerImageList,
	SilenceUsage: true,
}

// container image rm
var containerImageRmCmd = &cobra.Command{
	Use:   "rm <image>...",
	Short: "Remove bootc images",
	Long: `Remove one or more bootc images.

Equivalent to: podman rmi IMAGE

Example:
  bootc-man container image rm my-bootc:latest
  bootc-man container image rm abc123def456
  bootc-man container image rm -f my-bootc  # force removal`,
	Args:         cobra.MinimumNArgs(1),
	RunE:         runContainerImageRm,
	SilenceUsage: true,
}

// container image inspect
var containerImageInspectCmd = &cobra.Command{
	Use:   "inspect <image>",
	Short: "Display detailed information about a bootc image",
	Long: `Display detailed information about a bootc image in JSON format.

Equivalent to: podman image inspect IMAGE

Example:
  bootc-man container image inspect my-bootc:latest
  bootc-man container image inspect quay.io/fedora/fedora-bootc:42`,
	Args:         cobra.ExactArgs(1),
	RunE:         runContainerImageInspect,
	SilenceUsage: true,
}

// Flags
var (
	// build flags
	buildTag       string
	buildFile      string
	buildNoCache   bool
	buildPush      bool
	buildTlsVerify bool

	// push flags
	pushTlsVerify bool

	// image list flags
	imageListAll bool

	// image rm flags
	imageRmForce bool
)

func init() {
	// Add container command to root
	rootCmd.AddCommand(containerCmd)

	// Add subcommands to container
	containerCmd.AddCommand(containerBuildCmd)
	containerCmd.AddCommand(containerRunCmd)
	containerCmd.AddCommand(containerPushCmd)
	containerCmd.AddCommand(containerImageCmd)

	// Add subcommands to container image
	containerImageCmd.AddCommand(containerImageListCmd)
	containerImageCmd.AddCommand(containerImageRmCmd)
	containerImageCmd.AddCommand(containerImageInspectCmd)

	// Build flags
	containerBuildCmd.Flags().StringVarP(&buildTag, "tag", "t", "", "Name and optionally a tag for the image (required)")
	containerBuildCmd.Flags().StringVarP(&buildFile, "file", "f", "", "Path to Containerfile (default: ./Containerfile)")
	containerBuildCmd.Flags().BoolVar(&buildNoCache, "no-cache", false, "Do not use cache when building")
	containerBuildCmd.Flags().BoolVar(&buildPush, "push", false, "Push image to registry after build")
	containerBuildCmd.Flags().BoolVar(&buildTlsVerify, "tls-verify", true, "Verify TLS certificates when pushing")
	_ = containerBuildCmd.MarkFlagRequired("tag")

	// Push flags
	containerPushCmd.Flags().BoolVar(&pushTlsVerify, "tls-verify", true, "Verify TLS certificates when pushing")

	// Image list flags
	containerImageListCmd.Flags().BoolVarP(&imageListAll, "all", "a", false, "Show all images, not just bootc images")

	// Image rm flags
	containerImageRmCmd.Flags().BoolVarP(&imageRmForce, "force", "f", false, "Force removal of the image")

	// Set completion functions for image name completion
	containerRunCmd.ValidArgsFunction = completeBootcImages
	containerPushCmd.ValidArgsFunction = completeBootcImages
	containerImageRmCmd.ValidArgsFunction = completeBootcImagesMultiple
	containerImageInspectCmd.ValidArgsFunction = completeBootcImages
}

func getPodmanClient() (*podman.Client, error) {
	pm, err := podman.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create podman client: %w", err)
	}
	return pm, nil
}

func runContainerBuild(cmd *cobra.Command, args []string) error {
	pm, err := getPodmanClient()
	if err != nil {
		return err
	}

	// Default context is current directory (same as podman build)
	context := "."
	if len(args) > 0 {
		context = args[0]
	}

	// Resolve absolute path for context
	absContext, err := filepath.Abs(context)
	if err != nil {
		return fmt.Errorf("failed to resolve context path: %w", err)
	}

	// Check if context exists and is a directory
	info, err := os.Stat(absContext)
	if os.IsNotExist(err) {
		return fmt.Errorf("context directory does not exist: %s", absContext)
	}
	if !info.IsDir() {
		return fmt.Errorf("context must be a directory: %s", absContext)
	}

	// Determine Containerfile path (same as podman build -f)
	containerfile := buildFile
	if containerfile == "" {
		// Look for Containerfile in context directory
		containerfile = filepath.Join(absContext, config.DefaultContainerfileName)
		if _, err := os.Stat(containerfile); os.IsNotExist(err) {
			// Try Dockerfile as fallback
			dockerfile := filepath.Join(absContext, "Dockerfile")
			if _, err := os.Stat(dockerfile); os.IsNotExist(err) {
				return fmt.Errorf("no Containerfile or Dockerfile found in %s", absContext)
			}
			containerfile = dockerfile
		}
	} else {
		// Resolve Containerfile path
		if !filepath.IsAbs(containerfile) {
			// Relative paths are resolved from current working directory (same as podman)
			containerfile, err = filepath.Abs(containerfile)
			if err != nil {
				return fmt.Errorf("failed to resolve Containerfile path: %w", err)
			}
		}
		if _, err := os.Stat(containerfile); os.IsNotExist(err) {
			return fmt.Errorf("Containerfile not found: %s", containerfile)
		}
	}

	// Show equivalent command
	if verbose || dryRun {
		cmdArgs := []string{"podman", "build", "-t", buildTag}
		// Show -f if explicitly specified
		if buildFile != "" {
			cmdArgs = append(cmdArgs, "-f", containerfile)
		}
		if buildNoCache {
			cmdArgs = append(cmdArgs, "--no-cache")
		}
		cmdArgs = append(cmdArgs, absContext)
		fmt.Fprintf(os.Stderr, "Equivalent command: %s\n", strings.Join(cmdArgs, " "))

		if buildPush {
			pushArgs := []string{"podman", "push"}
			if !buildTlsVerify {
				pushArgs = append(pushArgs, "--tls-verify=false")
			}
			pushArgs = append(pushArgs, buildTag)
			fmt.Fprintf(os.Stderr, "Equivalent command: %s\n", strings.Join(pushArgs, " "))
		}
	}

	if dryRun {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	// Build the image
	fmt.Printf("Building image %s...\n", buildTag)
	opts := podman.BuildOptions{
		Context:    absContext,
		Tag:        buildTag,
		Dockerfile: containerfile,
		NoCache:    buildNoCache,
	}

	if err := pm.Build(cmd.Context(), opts); err != nil {
		return formatContainerError("failed to build image", err)
	}

	fmt.Printf("✓ Image built: %s\n", buildTag)

	// Push if requested
	if buildPush {
		fmt.Printf("Pushing image %s...\n", buildTag)
		if err := pm.Push(cmd.Context(), buildTag, buildTlsVerify); err != nil {
			return formatContainerError("failed to push image", err)
		}
		fmt.Printf("✓ Image pushed: %s\n", buildTag)
	}

	return nil
}

func runContainerRun(cmd *cobra.Command, args []string) error {
	pm, err := getPodmanClient()
	if err != nil {
		return err
	}

	var image string
	if len(args) > 0 {
		image = args[0]
	} else {
		// No image specified - show selection menu
		selectedImage, err := selectBootcImage(cmd, pm)
		if err != nil {
			return err
		}
		image = selectedImage
	}

	// Show equivalent command
	if verbose || dryRun {
		fmt.Fprintf(os.Stderr, "Equivalent command: podman run -it --rm %s /bin/bash\n", image)
	}

	if dryRun {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	fmt.Printf("Running %s interactively...\n", image)
	fmt.Println("(Type 'exit' or press Ctrl+D to exit the container)")
	fmt.Println()

	opts := podman.RunOptions{
		Image:  image,
		Remove: true,
		Args:   []string{"/bin/bash"},
	}

	if err := pm.RunInteractive(cmd.Context(), opts); err != nil {
		return formatContainerError("container run failed", err)
	}

	return nil
}

func runContainerPush(cmd *cobra.Command, args []string) error {
	pm, err := getPodmanClient()
	if err != nil {
		return err
	}

	var image string
	var destination string

	if len(args) > 0 {
		image = args[0]
		if len(args) > 1 {
			destination = args[1]
		}
	} else {
		// No image specified - show selection menu
		selectedImage, err := selectBootcImage(cmd, pm)
		if err != nil {
			return err
		}
		image = selectedImage
	}

	// Determine what to push (use destination if specified)
	pushTarget := image
	if destination != "" {
		pushTarget = destination
	}

	// Show equivalent command
	if verbose || dryRun {
		cmdArgs := []string{"podman", "push"}
		if !pushTlsVerify {
			cmdArgs = append(cmdArgs, "--tls-verify=false")
		}
		cmdArgs = append(cmdArgs, image)
		if destination != "" {
			cmdArgs = append(cmdArgs, destination)
		}
		fmt.Fprintf(os.Stderr, "Equivalent command: %s\n", strings.Join(cmdArgs, " "))
	}

	if dryRun {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	if destination != "" {
		fmt.Printf("Pushing %s to %s...\n", image, destination)
	} else {
		fmt.Printf("Pushing %s...\n", image)
	}

	if err := pm.PushWithDestination(cmd.Context(), image, destination, pushTlsVerify); err != nil {
		return formatContainerError("failed to push image", err)
	}

	fmt.Printf("✓ Image pushed: %s\n", pushTarget)
	return nil
}

func runContainerImageList(cmd *cobra.Command, args []string) error {
	pm, err := getPodmanClient()
	if err != nil {
		return err
	}

	bootcOnly := !imageListAll

	// Show equivalent command
	if verbose || dryRun {
		cmdArgs := []string{"podman", "images", "--format", "json"}
		if bootcOnly {
			cmdArgs = append(cmdArgs, "--filter=label=containers.bootc=1")
		}
		fmt.Fprintf(os.Stderr, "Equivalent command: %s\n", strings.Join(cmdArgs, " "))
	}

	if dryRun {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	images, err := pm.Images(cmd.Context(), bootcOnly)
	if err != nil {
		return formatContainerError("failed to list images", err)
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(images)
	}

	// Table output
	if len(images) == 0 {
		if bootcOnly {
			fmt.Println("No bootc images found.")
			fmt.Println("Hint: Use --all to show all images.")
		} else {
			fmt.Println("No images found.")
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPOSITORY\tTAG\tIMAGE ID\tCREATED\tSIZE")

	for _, img := range images {
		// Parse repository and tag from Names
		repo := "<none>"
		tag := "<none>"
		if len(img.Names) > 0 {
			name := img.Names[0]
			if idx := strings.LastIndex(name, ":"); idx != -1 {
				repo = name[:idx]
				tag = name[idx+1:]
			} else {
				repo = name
				tag = "latest"
			}
		}

		// Short ID (first 12 chars)
		shortID := img.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}

		// Human-readable created time
		created := format.TimeAgo(img.Created)

		// Human-readable size
		size := format.Size(img.Size)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", repo, tag, shortID, created, size)
	}

	return w.Flush()
}

func runContainerImageRm(cmd *cobra.Command, args []string) error {
	pm, err := getPodmanClient()
	if err != nil {
		return err
	}

	for _, image := range args {
		// Show equivalent command
		if verbose || dryRun {
			cmdArgs := []string{"podman", "rmi"}
			if imageRmForce {
				cmdArgs = append(cmdArgs, "-f")
			}
			cmdArgs = append(cmdArgs, image)
			fmt.Fprintf(os.Stderr, "Equivalent command: %s\n", strings.Join(cmdArgs, " "))
		}

		if dryRun {
			continue
		}

		fmt.Printf("Removing image %s...\n", image)
		if err := pm.ImageRemove(cmd.Context(), image, imageRmForce); err != nil {
			return formatContainerError(fmt.Sprintf("failed to remove image %s", image), err)
		}
		fmt.Printf("✓ Image removed: %s\n", image)
	}

	if dryRun {
		fmt.Println("(dry-run mode - command not executed)")
	}

	return nil
}

func runContainerImageInspect(cmd *cobra.Command, args []string) error {
	pm, err := getPodmanClient()
	if err != nil {
		return err
	}

	image := args[0]

	// Show equivalent command
	if verbose || dryRun {
		fmt.Fprintf(os.Stderr, "Equivalent command: podman image inspect %s\n", image)
	}

	if dryRun {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	info, err := pm.ImageInspect(cmd.Context(), image)
	if err != nil {
		return formatContainerError("failed to inspect image", err)
	}

	// Always output as JSON for inspect
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(info)
}

// completeBootcImages provides shell completion for bootc image names
func completeBootcImages(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Only complete the first argument
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	return getBootcImageCompletions(toComplete)
}

// completeBootcImagesMultiple provides shell completion for multiple bootc image names
func completeBootcImagesMultiple(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return getBootcImageCompletions(toComplete)
}

// getBootcImageCompletions returns completions for bootc images (excluding <none>)
func getBootcImageCompletions(toComplete string) ([]string, cobra.ShellCompDirective) {
	pm, err := podman.NewClient()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Get bootc images only (use background context for completion)
	ctx := context.Background()
	images, err := pm.Images(ctx, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var completions []string
	for _, img := range images {
		// Skip images with no names (<none>)
		if len(img.Names) == 0 {
			continue
		}

		// Add image names (repository:tag format)
		for _, name := range img.Names {
			if strings.HasPrefix(name, toComplete) {
				completions = append(completions, name)
			}
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// getSelectableBootcImages returns bootc images that can be selected (have names)
func getSelectableBootcImages(ctx context.Context, pm *podman.Client) ([]podman.ImageInfo, error) {
	images, err := pm.Images(ctx, true)
	if err != nil {
		return nil, err
	}

	// Filter out images without names
	var selectable []podman.ImageInfo
	for _, img := range images {
		if len(img.Names) > 0 {
			selectable = append(selectable, img)
		}
	}

	return selectable, nil
}

// selectBootcImage shows an interactive menu to select a bootc image
func selectBootcImage(cmd *cobra.Command, pm *podman.Client) (string, error) {
	images, err := getSelectableBootcImages(cmd.Context(), pm)
	if err != nil {
		return "", formatContainerError("failed to list images", err)
	}

	if len(images) == 0 {
		return "", fmt.Errorf("no bootc images found. Build or pull a bootc image first")
	}

	// Display available images
	fmt.Println("Available bootc images:")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  #\tREPOSITORY\tTAG\tCREATED\tSIZE")

	for i, img := range images {
		name := img.Names[0]
		repo := name
		tag := "latest"
		if idx := strings.LastIndex(name, ":"); idx != -1 {
			repo = name[:idx]
			tag = name[idx+1:]
		}
		created := format.TimeAgo(img.Created)
		size := format.Size(img.Size)
		fmt.Fprintf(w, "  %d\t%s\t%s\t%s\t%s\n", i+1, repo, tag, created, size)
	}
	w.Flush()

	fmt.Println()
	fmt.Printf("Select image [1-%d]: ", len(images))

	// Read user input
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("no selection made")
	}

	// Parse selection
	selection, err := strconv.Atoi(input)
	if err != nil || selection < 1 || selection > len(images) {
		return "", fmt.Errorf("invalid selection: %s (must be 1-%d)", input, len(images))
	}

	selectedImage := images[selection-1].Names[0]
	fmt.Printf("Selected: %s\n", selectedImage)
	fmt.Println()

	return selectedImage, nil
}
