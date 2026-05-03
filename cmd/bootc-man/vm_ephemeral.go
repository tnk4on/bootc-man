package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var vmEphemeralCmd = &cobra.Command{
	Use:   "ephemeral [image]",
	Short: "Run a bootc container as an ephemeral VM via bcvk",
	Long: `Run a bootc container image as an ephemeral VM using bcvk.

This launches a temporary VM directly from a container image without
creating a disk image (skipping the convert stage). Useful for quick
testing and development.

Requires bcvk to be installed: brew install bcvk

Examples:
  bootc-man vm ephemeral quay.io/fedora/fedora-bootc:42
  bootc-man vm ephemeral --ssh quay.io/fedora/fedora-bootc:42
  bootc-man vm ephemeral --execute "bootc status" quay.io/fedora/fedora-bootc:42
  bootc-man vm ephemeral --gui --ssh quay.io/fedora/fedora-bootc:42`,
	Args: cobra.ExactArgs(1),
	RunE: runVMEphemeral,
}

// Alias: "start" = "ephemeral run"
var vmEphemeralStartCmd = &cobra.Command{
	Use:   "start [image]",
	Short: "Start an ephemeral VM (alias for 'ephemeral')",
	Args:  cobra.ExactArgs(1),
	RunE:  runVMEphemeral,
}

var vmEphemeralListCmd = &cobra.Command{
	Use:   "list",
	Short: "List running ephemeral VMs",
	RunE:  runVMEphemeralList,
}

var vmEphemeralStopCmd = &cobra.Command{
	Use:   "stop <name>",
	Short: "Stop an ephemeral VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runVMEphemeralStop,
}

var vmEphemeralSshCmd = &cobra.Command{
	Use:   "ssh <name>",
	Short: "SSH into a running ephemeral VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runVMEphemeralSsh,
}

var (
	ephemeralSSH        bool
	ephemeralExecute    string
	ephemeralCPUs       int
	ephemeralMemory     int
	ephemeralName       string
	ephemeralGUI        bool
	ephemeralDetach     bool
	ephemeralKernelArgs []string
)

func init() {
	vmCmd.AddCommand(vmEphemeralCmd)
	vmEphemeralCmd.AddCommand(vmEphemeralStartCmd)
	vmEphemeralCmd.AddCommand(vmEphemeralListCmd)
	vmEphemeralCmd.AddCommand(vmEphemeralStopCmd)
	vmEphemeralCmd.AddCommand(vmEphemeralSshCmd)

	vmEphemeralCmd.Flags().BoolVar(&ephemeralSSH, "ssh", false, "SSH into the VM after boot")
	vmEphemeralCmd.Flags().StringVar(&ephemeralExecute, "execute", "", "Command to execute inside the VM via SSH")
	vmEphemeralCmd.Flags().IntVar(&ephemeralCPUs, "cpus", 0, "Number of virtual CPUs (default: bcvk default)")
	vmEphemeralCmd.Flags().IntVar(&ephemeralMemory, "memory", 0, "Memory in MiB (default: bcvk default)")
	vmEphemeralCmd.Flags().StringVar(&ephemeralName, "name", "", "VM name for identification")
	vmEphemeralCmd.Flags().BoolVar(&ephemeralGUI, "gui", false, "Display VM console in GUI window")
	vmEphemeralCmd.Flags().BoolVarP(&ephemeralDetach, "detach", "d", false, "Run VM in background")
	vmEphemeralCmd.Flags().StringArrayVar(&ephemeralKernelArgs, "karg", nil, "Additional kernel command-line arguments")
}

func findBcvk() (string, error) {
	bin, err := exec.LookPath("bcvk")
	if err != nil {
		return "", fmt.Errorf("bcvk is not installed.\n" +
			"  Install: brew install bcvk\n" +
			"  Or build from source: https://github.com/bootc-dev/bcvk")
	}
	return bin, nil
}

func runBcvk(args ...string) error {
	bin, err := findBcvk()
	if err != nil {
		return err
	}
	if verbose {
		fmt.Printf("Running: bcvk %s\n", strings.Join(args, " "))
	}
	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runVMEphemeral(cmd *cobra.Command, args []string) error {
	image := args[0]

	var bcvkArgs []string

	if ephemeralSSH || ephemeralExecute != "" {
		bcvkArgs = append(bcvkArgs, "ephemeral", "run-ssh")
	} else {
		bcvkArgs = append(bcvkArgs, "ephemeral", "run")
	}

	if ephemeralCPUs > 0 {
		bcvkArgs = append(bcvkArgs, "--cpus", fmt.Sprintf("%d", ephemeralCPUs))
	}
	if ephemeralMemory > 0 {
		bcvkArgs = append(bcvkArgs, "--memory", fmt.Sprintf("%d", ephemeralMemory))
	}
	if ephemeralName != "" {
		bcvkArgs = append(bcvkArgs, "--name", ephemeralName)
	}
	if ephemeralExecute != "" {
		bcvkArgs = append(bcvkArgs, "--execute", ephemeralExecute)
	}
	if ephemeralGUI {
		bcvkArgs = append(bcvkArgs, "--gui")
	}
	if ephemeralDetach {
		bcvkArgs = append(bcvkArgs, "--detach")
	}
	for _, arg := range ephemeralKernelArgs {
		bcvkArgs = append(bcvkArgs, "--karg", arg)
	}

	if ephemeralSSH || ephemeralExecute != "" {
		bcvkArgs = append(bcvkArgs, "-K")
	}
	bcvkArgs = append(bcvkArgs, image)

	return runBcvk(bcvkArgs...)
}

func runVMEphemeralList(cmd *cobra.Command, args []string) error {
	return runBcvk("ephemeral", "ps")
}

func runVMEphemeralSsh(cmd *cobra.Command, args []string) error {
	return runBcvk("ephemeral", "ssh", args[0])
}

type ephemeralMeta struct {
	Name       string `json:"name"`
	PID        int    `json:"pid"`
	GvproxyPID int    `json:"gvproxy_pid"`
}

func runVMEphemeralStop(cmd *cobra.Command, args []string) error {
	name := args[0]
	metaPath := filepath.Join("/private/tmp/bcvk/vms", name+".json")

	data, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("VM '%s' not found: %w", name, err)
	}

	var meta ephemeralMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return fmt.Errorf("invalid metadata: %w", err)
	}

	if meta.PID > 0 {
		_ = syscall.Kill(meta.PID, syscall.SIGTERM)
	}
	if meta.GvproxyPID > 0 {
		_ = syscall.Kill(meta.GvproxyPID, syscall.SIGKILL)
	}

	_ = os.Remove(metaPath)
	fmt.Printf("Stopped %s\n", name)
	return nil
}
