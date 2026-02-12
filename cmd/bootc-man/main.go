package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/tnk4on/bootc-man/internal/podman"
	"github.com/tnk4on/bootc-man/internal/registry"
)

var version = "dev"

func main() {
	// Create a context that cancels on interrupt signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals (SIGINT, SIGTERM)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	// Execute with context
	if err := ExecuteWithContext(ctx); err != nil {
		printError(err)
		os.Exit(1)
	}
}

// printError formats and prints errors with clear separation between bootc-man and podman errors
func printError(err error) {
	var regErr *registry.RegistryError
	if errors.As(err, &regErr) {
		// Print bootc-man error message
		fmt.Fprintf(os.Stderr, "Error: %s\n", regErr.Error())
		
		// Print podman error details if available
		if regErr.PodmanError != nil {
			fmt.Fprintf(os.Stderr, "\nPodman error:\n")
			if regErr.PodmanError.Stderr != "" {
				fmt.Fprintf(os.Stderr, "  %s\n", regErr.PodmanError.Stderr)
			} else {
				fmt.Fprintf(os.Stderr, "  %v\n", regErr.PodmanError.Err)
			}
		}
		return
	}

	// Check if it's a podman error directly
	var podmanErr *podman.PodmanError
	if errors.As(err, &podmanErr) {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		if podmanErr.Stderr != "" {
			fmt.Fprintf(os.Stderr, "\nPodman stderr:\n  %s\n", podmanErr.Stderr)
		}
		return
	}

	// Generic error
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
}
