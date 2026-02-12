package podman

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/tnk4on/bootc-man/internal/config"
)

// PodmanError represents an error from podman command execution
type PodmanError struct {
	Command string
	Stderr  string
	Err     error
}

func (e *PodmanError) Error() string {
	return fmt.Sprintf("podman %s failed: %v", e.Command, e.Err)
}

func (e *PodmanError) Unwrap() error {
	return e.Err
}

// Client wraps podman CLI commands
type Client struct {
	binary string
}

// NewClient creates a new podman client
func NewClient() (*Client, error) {
	binary, err := findPodman()
	if err != nil {
		return nil, err
	}
	return &Client{binary: binary}, nil
}

func findPodman() (string, error) {
	// Check common paths
	paths := []string{
		"/usr/bin/podman",
		"/usr/local/bin/podman",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Try PATH
	path, err := exec.LookPath("podman")
	if err != nil {
		return "", fmt.Errorf("podman not found: %w", err)
	}
	return path, nil
}

// run executes a podman command and returns stdout
func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, &PodmanError{
			Command: strings.Join(args, " "),
			Stderr:  strings.TrimSpace(stderr.String()),
			Err:     err,
		}
	}
	return stdout.Bytes(), nil
}

// PodmanInfo contains podman system info
type PodmanInfo struct {
	Version  string
	Rootless bool
}

// Info returns podman system information
func (c *Client) Info(ctx context.Context) (*PodmanInfo, error) {
	output, err := c.run(ctx, "info", "--format", "json")
	if err != nil {
		return nil, err
	}

	var info struct {
		Version struct {
			Version string `json:"Version"`
		} `json:"version"`
		Host struct {
			Security struct {
				Rootless bool `json:"rootless"`
			} `json:"security"`
		} `json:"host"`
	}

	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse podman info: %w", err)
	}

	return &PodmanInfo{
		Version:  info.Version.Version,
		Rootless: info.Host.Security.Rootless,
	}, nil
}

// RunOptions contains options for running a container
type RunOptions struct {
	Name       string
	Image      string
	Ports      []PortMapping
	Volumes    []VolumeMapping
	Detach     bool
	Remove     bool
	Privileged bool
	Env        map[string]string
	Args       []string
}

// PortMapping represents a port mapping
type PortMapping struct {
	Host      int
	Container int
}

// VolumeMapping represents a volume mapping
type VolumeMapping struct {
	Host      string
	Container string
	Options   string // e.g., "ro", "Z"
}

// FormatPortMapping formats a port mapping for podman command line
// This is a pure function that can be easily unit tested
func FormatPortMapping(p PortMapping) string {
	return fmt.Sprintf("%d:%d", p.Host, p.Container)
}

// FormatVolumeMapping formats a volume mapping for podman command line
// This is a pure function that can be easily unit tested
func FormatVolumeMapping(v VolumeMapping) string {
	mapping := fmt.Sprintf("%s:%s", v.Host, v.Container)
	if v.Options != "" {
		mapping += ":" + v.Options
	}
	return mapping
}

// BuildRunArgs constructs the argument list for podman run command
// This is a pure function that can be easily unit tested
func BuildRunArgs(opts RunOptions, interactive bool) []string {
	var args []string
	if interactive {
		args = []string{"run", "-it"}
	} else {
		args = []string{"run"}
	}

	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}
	if opts.Detach && !interactive {
		args = append(args, "-d")
	}
	if opts.Remove {
		args = append(args, "--rm")
	}
	if opts.Privileged {
		args = append(args, "--privileged")
	}

	for _, p := range opts.Ports {
		args = append(args, "-p", FormatPortMapping(p))
	}

	for _, v := range opts.Volumes {
		args = append(args, "-v", FormatVolumeMapping(v))
	}

	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, opts.Image)
	args = append(args, opts.Args...)

	return args
}

// Run runs a container
func (c *Client) Run(ctx context.Context, opts RunOptions) (string, error) {
	args := BuildRunArgs(opts, false)

	output, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// Start starts a stopped container
func (c *Client) Start(ctx context.Context, name string) error {
	_, err := c.run(ctx, "start", name)
	return err
}

// Stop stops a running container
func (c *Client) Stop(ctx context.Context, name string) error {
	_, err := c.run(ctx, "stop", name)
	return err
}

// Remove removes a container
func (c *Client) Remove(ctx context.Context, name string, force bool) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	_, err := c.run(ctx, args...)
	return err
}

// Exists checks if a container exists
func (c *Client) Exists(ctx context.Context, name string) (bool, error) {
	_, err := c.run(ctx, "container", "exists", name)
	if err != nil {
		// Exit code 1 means container doesn't exist
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ContainerState represents container state
type ContainerState struct {
	Running    bool   `json:"Running"`
	Paused     bool   `json:"Paused"`
	Restarting bool   `json:"Restarting"`
	OOMKilled  bool   `json:"OOMKilled"`
	Dead       bool   `json:"Dead"`
	Pid        int    `json:"Pid"`
	ExitCode   int    `json:"ExitCode"`
	Error      string `json:"Error"`
	StartedAt  string `json:"StartedAt"`
	FinishedAt string `json:"FinishedAt"`
}

// ContainerInfo contains detailed container information
type ContainerInfo struct {
	ID      string         `json:"Id"`
	Name    string         `json:"Name"`
	Image   string         `json:"Image"`
	Created string         `json:"Created"`
	State   ContainerState `json:"State"`
}

// Inspect returns detailed information about a container
func (c *Client) Inspect(ctx context.Context, name string) (*ContainerInfo, error) {
	output, err := c.run(ctx, "inspect", "--format", "json", name)
	if err != nil {
		return nil, err
	}

	var infos []ContainerInfo
	if err := json.Unmarshal(output, &infos); err != nil {
		return nil, fmt.Errorf("failed to parse inspect output: %w", err)
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("container not found: %s", name)
	}

	return &infos[0], nil
}

// Logs returns container logs
func (c *Client) Logs(ctx context.Context, name string, follow bool) (io.ReadCloser, error) {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, name)

	cmd := exec.CommandContext(ctx, c.binary, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// Return a reader that waits for the command to finish
	return &logReader{
		ReadCloser: stdout,
		cmd:        cmd,
	}, nil
}

type logReader struct {
	io.ReadCloser
	cmd *exec.Cmd
}

func (r *logReader) Close() error {
	r.ReadCloser.Close()
	return r.cmd.Wait()
}

// Pull pulls a container image
func (c *Client) Pull(ctx context.Context, image string) error {
	_, err := c.run(ctx, "pull", image)
	return err
}

// BuildOptions contains options for building an image
type BuildOptions struct {
	Context    string
	Tag        string
	Dockerfile string
	NoCache    bool
}

// Build builds a container image
func (c *Client) Build(ctx context.Context, opts BuildOptions) error {
	args := []string{"build"}

	if opts.Tag != "" {
		args = append(args, "-t", opts.Tag)
	}
	if opts.Dockerfile != "" {
		args = append(args, "-f", opts.Dockerfile)
	}
	if opts.NoCache {
		args = append(args, "--no-cache")
	}

	args = append(args, opts.Context)

	_, err := c.run(ctx, args...)
	return err
}

// Push pushes an image to a registry
func (c *Client) Push(ctx context.Context, image string, tlsVerify bool) error {
	args := []string{"push"}
	if !tlsVerify {
		args = append(args, "--tls-verify=false")
	}
	args = append(args, image)

	_, err := c.run(ctx, args...)
	return err
}

// PushWithDestination pushes an image to a registry, optionally to a different destination
func (c *Client) PushWithDestination(ctx context.Context, image string, destination string, tlsVerify bool) error {
	args := []string{"push"}
	if !tlsVerify {
		args = append(args, "--tls-verify=false")
	}
	args = append(args, image)
	if destination != "" {
		args = append(args, destination)
	}

	_, err := c.run(ctx, args...)
	return err
}

// VolumeExists checks if a volume exists
func (c *Client) VolumeExists(ctx context.Context, name string) (bool, error) {
	_, err := c.run(ctx, "volume", "exists", name)
	if err != nil {
		// Exit code 1 means volume doesn't exist
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// VolumeRemove removes a volume
func (c *Client) VolumeRemove(ctx context.Context, name string, force bool) error {
	args := []string{"volume", "rm"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, name)
	_, err := c.run(ctx, args...)
	return err
}

// Command creates an exec.Cmd for running podman with the given arguments
// This allows callers to control stdout/stderr directly
func (c *Client) Command(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, c.binary, args...)
}

// BootcLabel is the label used to identify bootc images
// Deprecated: Use config.LabelBootc instead
const BootcLabel = config.LabelBootc

// ImageInfo contains information about a container image
type ImageInfo struct {
	ID         string            `json:"Id"`
	Names      []string          `json:"Names"`
	Created    int64             `json:"Created"`
	CreatedAt  string            `json:"CreatedAt"`
	Size       int64             `json:"Size"`
	Labels     map[string]string `json:"Labels"`
	Repository string            `json:"repository"`
	Tag        string            `json:"tag"`
}

// IsBootc returns true if the image has the bootc label
func (i *ImageInfo) IsBootc() bool {
	if i.Labels == nil {
		return false
	}
	return i.Labels[BootcLabel] == "1"
}

// Images lists container images, optionally filtering for bootc images only
func (c *Client) Images(ctx context.Context, bootcOnly bool) ([]ImageInfo, error) {
	args := []string{"images", "--format", "json"}
	if bootcOnly {
		args = append(args, "--filter", "label="+BootcLabel+"=1")
	}

	output, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var images []ImageInfo
	if err := json.Unmarshal(output, &images); err != nil {
		return nil, fmt.Errorf("failed to parse images output: %w", err)
	}

	return images, nil
}

// ImageRemove removes a container image
func (c *Client) ImageRemove(ctx context.Context, image string, force bool) error {
	args := []string{"rmi"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, image)

	_, err := c.run(ctx, args...)
	return err
}

// ImageInspectInfo contains detailed information about a container image
type ImageInspectInfo struct {
	ID           string            `json:"Id"`
	Digest       string            `json:"Digest"`
	RepoTags     []string          `json:"RepoTags"`
	RepoDigests  []string          `json:"RepoDigests"`
	Created      string            `json:"Created"`
	Size         int64             `json:"Size"`
	VirtualSize  int64             `json:"VirtualSize"`
	Labels       map[string]string `json:"Labels"`
	Architecture string            `json:"Architecture"`
	Os           string            `json:"Os"`
	Config       struct {
		Cmd        []string          `json:"Cmd"`
		Env        []string          `json:"Env"`
		Labels     map[string]string `json:"Labels"`
		WorkingDir string            `json:"WorkingDir"`
	} `json:"Config"`
}

// IsBootc returns true if the image has the bootc label
func (i *ImageInspectInfo) IsBootc() bool {
	// Check Config.Labels first (more reliable)
	if i.Config.Labels != nil {
		if i.Config.Labels[BootcLabel] == "1" {
			return true
		}
	}
	// Fallback to top-level Labels
	if i.Labels != nil {
		return i.Labels[BootcLabel] == "1"
	}
	return false
}

// ImageInspect returns detailed information about a container image
func (c *Client) ImageInspect(ctx context.Context, image string) (*ImageInspectInfo, error) {
	output, err := c.run(ctx, "image", "inspect", "--format", "json", image)
	if err != nil {
		return nil, err
	}

	var infos []ImageInspectInfo
	if err := json.Unmarshal(output, &infos); err != nil {
		return nil, fmt.Errorf("failed to parse image inspect output: %w", err)
	}

	if len(infos) == 0 {
		return nil, fmt.Errorf("image not found: %s", image)
	}

	return &infos[0], nil
}

// IsLoggedIn checks if the user is logged in to a specific registry
// Returns true if logged in, false otherwise
func (c *Client) IsLoggedIn(ctx context.Context, registry string) (bool, error) {
	// Use podman login --get-login to check if credentials exist
	output, err := c.run(ctx, "login", "--get-login", registry)
	if err != nil {
		// Exit code 1 means not logged in
		if podmanErr, ok := err.(*PodmanError); ok {
			// Not logged in - this is expected, not an error
			if strings.Contains(podmanErr.Stderr, "not logged in") ||
				strings.Contains(podmanErr.Stderr, "Error: not logged into") {
				return false, nil
			}
		}
		// Other errors (e.g., registry doesn't exist) - treat as not logged in
		return false, nil
	}

	// If we got output (username), user is logged in
	return len(strings.TrimSpace(string(output))) > 0, nil
}

// RunInteractive runs a container interactively with stdin/stdout/stderr attached
func (c *Client) RunInteractive(ctx context.Context, opts RunOptions) error {
	args := BuildRunArgs(opts, true)

	cmd := exec.CommandContext(ctx, c.binary, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
