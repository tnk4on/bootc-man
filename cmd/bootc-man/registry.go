package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/tnk4on/bootc-man/internal/podman"
	"github.com/tnk4on/bootc-man/internal/registry"
	"github.com/spf13/cobra"
)

// formatRegistryError formats registry errors with clear separation between bootc-man and podman errors
func formatRegistryError(context string, err error) error {
	var regErr *registry.RegistryError
	if errors.As(err, &regErr) {
		// Return the structured error as-is, it will be formatted in main.go
		return fmt.Errorf("%s: %w", context, regErr)
	}
	return fmt.Errorf("%s: %w", context, err)
}

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage the local OCI registry",
	Long:  `Manage the local OCI registry for storing bootc images.`,
}

var registryUpCmd = &cobra.Command{
	Use:          "up",
	Short:        "Start the registry service",
	RunE:         runRegistryUp,
	SilenceUsage: true,
}

var registryDownCmd = &cobra.Command{
	Use:          "down",
	Short:        "Stop the registry service",
	RunE:         runRegistryDown,
	SilenceUsage: true,
}

var registryStatusCmd = &cobra.Command{
	Use:          "status",
	Short:        "Show registry service status",
	RunE:         runRegistryStatus,
	SilenceUsage: true,
}

var registryLogsCmd = &cobra.Command{
	Use:          "logs",
	Short:        "Show registry service logs",
	RunE:         runRegistryLogs,
	SilenceUsage: true,
}

var registryRmCmd = &cobra.Command{
	Use:          "rm",
	Short:        "Remove the registry container",
	Long:         `Remove the registry container. Use --force to remove even if the container is running. Use --volumes to also remove the associated data volume.`,
	RunE:         runRegistryRm,
	SilenceUsage: true,
}

var (
	registryLogsFollow bool
	registryRmForce    bool
	registryRmVolumes  bool
)

func init() {
	registryCmd.AddCommand(registryUpCmd)
	registryCmd.AddCommand(registryDownCmd)
	registryCmd.AddCommand(registryStatusCmd)
	registryCmd.AddCommand(registryLogsCmd)
	registryCmd.AddCommand(registryRmCmd)

	registryLogsCmd.Flags().BoolVarP(&registryLogsFollow, "follow", "f", false, "Follow log output")
	registryRmCmd.Flags().BoolVarP(&registryRmForce, "force", "f", false, "Force removal even if container is running")
	registryRmCmd.Flags().BoolVar(&registryRmVolumes, "volumes", false, "Remove the associated volume as well")
}

func getRegistryService() (*registry.Service, error) {
	pm, err := podman.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create podman client: %w", err)
	}

	cfg := getConfig()
	return registry.NewService(registry.ServiceOptions{
		Config:           &cfg.Registry,
		ContainersConfig: &cfg.Containers,
		Podman:           pm,
		Verbose:          verbose,
		DryRun:           dryRun,
	}), nil
}

func runRegistryUp(cmd *cobra.Command, args []string) error {
	svc, err := getRegistryService()
	if err != nil {
		return err
	}

	result, err := svc.Up(cmd.Context())
	if err != nil {
		return formatRegistryError("failed to start registry", err)
	}

	// In dry-run mode, service already printed the command
	if svc.IsDryRun() {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	cfg := getConfig()
	if result.AlreadyRunning {
		fmt.Printf("✓ Registry is already running on port %d\n", cfg.Registry.Port)
	} else {
		fmt.Printf("✓ Registry started on port %d\n", cfg.Registry.Port)
	}
	fmt.Printf("  Push images to: localhost:%d/<image>:<tag>\n", cfg.Registry.Port)
	return nil
}

func runRegistryDown(cmd *cobra.Command, args []string) error {
	svc, err := getRegistryService()
	if err != nil {
		return err
	}

	result, err := svc.Down(cmd.Context())
	if err != nil {
		return formatRegistryError("failed to stop registry", err)
	}

	// In dry-run mode, service already printed the command
	if svc.IsDryRun() {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	if result.NotCreated {
		fmt.Println("✓ Registry container does not exist")
	} else if result.AlreadyStopped {
		fmt.Println("✓ Registry is already stopped")
	} else {
		fmt.Println("✓ Registry stopped")
	}
	return nil
}

func runRegistryStatus(cmd *cobra.Command, args []string) error {
	// Dry-run mode: show commands that would be executed
	if dryRun {
		cfg := getConfig()
		fmt.Printf("📋 Equivalent command (check status):\n   podman inspect %s --format json\n\n", cfg.Containers.RegistryName)
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	svc, err := getRegistryService()
	if err != nil {
		return err
	}

	status, err := svc.Status(cmd.Context())
	if err != nil {
		return formatRegistryError("failed to get status", err)
	}

	// JSON output
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	// Text output
	fmt.Printf("Registry Status: %s\n", status.State)
	if status.Port > 0 {
		fmt.Printf("Port: %d\n", status.Port)
	}
	if status.Image != "" {
		fmt.Printf("Image: %s\n", status.Image)
	}
	if status.Created != "" {
		fmt.Printf("Created: %s\n", status.Created)
	}

	return nil
}

func runRegistryLogs(cmd *cobra.Command, args []string) error {
	svc, err := getRegistryService()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	if registryLogsFollow {
		// Setup signal handling for graceful shutdown
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()
	}

	reader, err := svc.Logs(ctx, registryLogsFollow)
	if err != nil {
		return formatRegistryError("failed to get logs", err)
	}

	// In dry-run mode, reader is nil
	if reader == nil {
		return nil
	}
	defer reader.Close()

	_, err = io.Copy(os.Stdout, reader)
	if err != nil && err != context.Canceled {
		return err
	}

	return nil
}

func runRegistryRm(cmd *cobra.Command, args []string) error {
	svc, err := getRegistryService()
	if err != nil {
		return err
	}

	if err := svc.Remove(cmd.Context(), registryRmForce, registryRmVolumes); err != nil {
		return formatRegistryError("failed to remove registry", err)
	}

	// In dry-run mode, service already printed the command
	if svc.IsDryRun() {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	if registryRmVolumes {
		fmt.Println("✓ Registry container and volume removed")
	} else {
		fmt.Println("✓ Registry container removed")
	}
	return nil
}
