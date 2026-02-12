package registry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/podman"
)

// Use constants from config package for default values
// ContainerName returns the container name from config or default
func ContainerName(cfg *config.ContainersConfig) string {
	if cfg != nil && cfg.RegistryName != "" {
		return cfg.RegistryName
	}
	return config.ContainerNameRegistry
}

// VolumeName returns the volume name from config or default
func VolumeName(cfg *config.ContainersConfig) string {
	if cfg != nil && cfg.RegistryDataVolume != "" {
		return cfg.RegistryDataVolume
	}
	return config.VolumeNameRegistryData
}

// Service manages the OCI registry container
type Service struct {
	config        *config.RegistryConfig
	podman        *podman.Client
	verbose       bool
	dryRun        bool
	containerName string
	volumeName    string
}

// ServiceOptions contains options for creating a registry service
type ServiceOptions struct {
	Config           *config.RegistryConfig
	ContainersConfig *config.ContainersConfig
	Podman           *podman.Client
	Verbose          bool
	DryRun           bool
}

// NewService creates a new registry service
func NewService(opts ServiceOptions) *Service {
	return &Service{
		config:        opts.Config,
		podman:        opts.Podman,
		verbose:       opts.Verbose,
		dryRun:        opts.DryRun,
		containerName: ContainerName(opts.ContainersConfig),
		volumeName:    VolumeName(opts.ContainersConfig),
	}
}

// GetContainerName returns the registry container name
func (s *Service) GetContainerName() string {
	return s.containerName
}

// GetVolumeName returns the registry volume name
func (s *Service) GetVolumeName() string {
	return s.volumeName
}

// showCommand displays the equivalent podman command
func (s *Service) showCommand(description, cmd string) {
	if s.verbose || s.dryRun {
		fmt.Printf("📋 Equivalent command (%s):\n   %s\n\n", description, cmd)
	}
}

// IsDryRun returns whether the service is in dry-run mode
func (s *Service) IsDryRun() bool {
	return s.dryRun
}

// Status represents the registry service status
type Status struct {
	State   string
	Port    int
	Image   string
	Created string
}

// UpResult represents the result of starting the registry service
type UpResult struct {
	AlreadyRunning bool
}

// Up starts the registry service
func (s *Service) Up(ctx context.Context) (*UpResult, error) {
	result := &UpResult{}

	if s.dryRun {
		// Show equivalent command
		runCmd := fmt.Sprintf("podman run -d --name %s -p %d:%d -v %s:%s %s",
			s.containerName, s.config.Port, config.DefaultRegistryContainerPort,
			s.volumeName, config.DefaultRegistryDataPath, s.config.Image)
		s.showCommand("run registry", runCmd)
		return result, nil
	}

	// Check if container exists
	exists, err := s.podman.Exists(ctx, s.containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check container: %w", err)
	}

	if exists {
		// Container exists, check if running
		info, err := s.podman.Inspect(ctx, s.containerName)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect container: %w", err)
		}

		if info.State.Running {
			result.AlreadyRunning = true
			return result, nil // Already running
		}

		// Start existing container
		s.showCommand("start existing", fmt.Sprintf("podman start %s", s.containerName))
		if err := s.podman.Start(ctx, s.containerName); err != nil {
			return nil, formatPortError(err, s.config.Port)
		}
		return result, nil
	}

	// Create and start new container
	// Note: podman run will automatically pull the image if it doesn't exist
	runCmd := fmt.Sprintf("podman run -d --name %s -p %d:%d -v %s:%s %s",
		s.containerName, s.config.Port, config.DefaultRegistryContainerPort,
		s.volumeName, config.DefaultRegistryDataPath, s.config.Image)
	s.showCommand("run registry", runCmd)

	_, err = s.podman.Run(ctx, podman.RunOptions{
		Name:   s.containerName,
		Image:  s.config.Image,
		Detach: true,
		Ports: []podman.PortMapping{
			{Host: s.config.Port, Container: config.DefaultRegistryContainerPort},
		},
		Volumes: []podman.VolumeMapping{
			{Host: s.volumeName, Container: config.DefaultRegistryDataPath},
		},
	})

	if err != nil {
		return nil, formatPortError(err, s.config.Port)
	}

	return result, nil
}

// DownResult represents the result of stopping the registry service
type DownResult struct {
	AlreadyStopped bool
	NotCreated     bool
}

// Down stops the registry service
func (s *Service) Down(ctx context.Context) (*DownResult, error) {
	result := &DownResult{}

	if s.dryRun {
		s.showCommand("stop registry", fmt.Sprintf("podman stop %s", s.containerName))
		return result, nil
	}

	exists, err := s.podman.Exists(ctx, s.containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check container: %w", err)
	}

	if !exists {
		result.NotCreated = true
		return result, nil // Nothing to stop
	}

	// Check if container is running
	info, err := s.podman.Inspect(ctx, s.containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	if !info.State.Running {
		result.AlreadyStopped = true
		return result, nil // Already stopped
	}

	// Stop the container
	s.showCommand("stop registry", fmt.Sprintf("podman stop %s", s.containerName))
	if err := s.podman.Stop(ctx, s.containerName); err != nil {
		return nil, err
	}

	return result, nil
}

// Status returns the registry service status
func (s *Service) Status(ctx context.Context) (*Status, error) {
	s.showCommand("check status", fmt.Sprintf("podman ps -a -f name=%s --format json", s.containerName))

	status := &Status{
		Port: s.config.Port,
	}

	if s.dryRun {
		status.State = "(dry-run)"
		return status, nil
	}

	exists, err := s.podman.Exists(ctx, s.containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check container: %w", err)
	}

	if !exists {
		status.State = "not created"
		return status, nil
	}

	info, err := s.podman.Inspect(ctx, s.containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	status.Image = info.Image
	status.Created = info.Created

	if info.State.Running {
		status.State = "running"
	} else {
		status.State = "stopped"
	}

	return status, nil
}

// Logs returns the registry service logs
func (s *Service) Logs(ctx context.Context, follow bool) (io.ReadCloser, error) {
	followFlag := ""
	if follow {
		followFlag = " -f"
	}
	s.showCommand("get logs", fmt.Sprintf("podman logs%s %s", followFlag, s.containerName))

	if s.dryRun {
		return nil, nil
	}

	exists, err := s.podman.Exists(ctx, s.containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to check container: %w", err)
	}

	if !exists {
		return nil, fmt.Errorf("registry container does not exist")
	}

	return s.podman.Logs(ctx, s.containerName, follow)
}

// GetRegistryURL returns the registry URL
func (s *Service) GetRegistryURL() string {
	return fmt.Sprintf("localhost:%d", s.config.Port)
}

// GetDataDir returns the path to the registry data directory
func (s *Service) GetDataDir(dataRoot string) string {
	return filepath.Join(dataRoot, "registry")
}

// formatPortError formats port-related errors with helpful messages
func formatPortError(err error, port int) error {
	if err == nil {
		return nil
	}

	// Check if this is a podman error
	var podmanErr *podman.PodmanError
	if errors.As(err, &podmanErr) {
		// Check if stderr contains port-related error
		if strings.Contains(podmanErr.Stderr, "address already in use") ||
			strings.Contains(podmanErr.Stderr, "bind: address already in use") {
			return &RegistryError{
				Message: fmt.Sprintf("port %d is already in use by another container or process. Please stop the conflicting container or use a different port", port),
				PodmanError: podmanErr,
			}
		}
		// Return structured error for other podman errors
		return &RegistryError{
			Message:     "failed to execute podman command",
			PodmanError: podmanErr,
		}
	}

	// For non-podman errors, check error message
	errStr := err.Error()
	if strings.Contains(errStr, "address already in use") || strings.Contains(errStr, "bind: address already in use") {
		return fmt.Errorf("port %d is already in use by another container or process. "+
			"Please stop the conflicting container or use a different port: %w", port, err)
	}

	return err
}

// RegistryError represents a registry operation error with podman error details
type RegistryError struct {
	Message     string
	PodmanError *podman.PodmanError
}

func (e *RegistryError) Error() string {
	return e.Message
}

func (e *RegistryError) Unwrap() error {
	if e.PodmanError != nil {
		return e.PodmanError
	}
	return nil
}

// Remove removes the registry container
func (s *Service) Remove(ctx context.Context, force bool, removeVolume bool) error {
	rmCmd := "podman rm"
	if force {
		rmCmd += " -f"
	}
	rmCmd += " " + s.containerName
	s.showCommand("remove registry", rmCmd)

	if s.dryRun {
		if removeVolume {
			volRmCmd := fmt.Sprintf("podman volume rm %s", s.volumeName)
			s.showCommand("remove volume", volRmCmd)
		}
		return nil
	}

	exists, err := s.podman.Exists(ctx, s.containerName)
	if err != nil {
		return fmt.Errorf("failed to check container: %w", err)
	}

	if exists {
		if err := s.podman.Remove(ctx, s.containerName, force); err != nil {
			return err
		}
	}

	// Remove volume if requested
	if removeVolume {
		volExists, err := s.podman.VolumeExists(ctx, s.volumeName)
		if err != nil {
			return fmt.Errorf("failed to check volume: %w", err)
		}

		if volExists {
			volRmCmd := fmt.Sprintf("podman volume rm %s", s.volumeName)
			s.showCommand("remove volume", volRmCmd)
			if err := s.podman.VolumeRemove(ctx, s.volumeName, false); err != nil {
				return fmt.Errorf("failed to remove volume: %w", err)
			}
		}
	}

	return nil
}
