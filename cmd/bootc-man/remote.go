package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tnk4on/bootc-man/internal/bootc"
	"github.com/tnk4on/bootc-man/internal/vm"
	"github.com/spf13/cobra"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Execute bootc operations on remote bootc-enabled systems via SSH",
	Long: `Execute bootc operations on remote bootc-enabled systems via SSH.

This command connects to a remote host using SSH (with ~/.ssh/config settings)
and executes bootc commands. The remote host must:
  - Be defined in ~/.ssh/config
  - Have SSH key authentication configured
  - Have bootc installed
  - Be a bootc-enabled system

Alternatively, use --vm to connect to a bootc-man managed VM.

Example:
  # Connect to remote host (via ~/.ssh/config)
  bootc-man remote status myserver
  bootc-man remote upgrade myserver --check
  bootc-man remote switch myserver quay.io/myorg/myimage:latest

  # Connect to bootc-man managed VM
  bootc-man remote status --vm myvm
  bootc-man remote upgrade --vm myvm
  bootc-man remote switch --vm myvm quay.io/myorg/myimage:latest`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Show help when no subcommand is provided
		return flag.ErrHelp
	},
}

var remoteUpgradeCmd = &cobra.Command{
	Use:   "upgrade [host]",
	Short: "Upgrade the remote system to a new image",
	Long: `Upgrade the remote system to a new image version from the current image reference.

Example:
  bootc-man remote upgrade myserver         # Check and stage upgrade
  bootc-man remote upgrade myserver --check # Only check if upgrade is available
  bootc-man remote upgrade myserver --apply # Apply upgrade immediately (reboot)
  bootc-man remote upgrade --vm myvm        # Upgrade a bootc-man managed VM`,
	Args:    validateRemoteArgs,
	PreRunE: extractRemoteHost,
	RunE:    runRemoteUpgrade,
}

var remoteSwitchCmd = &cobra.Command{
	Use:   "switch [host] <image>",
	Short: "Switch the remote system to a different image",
	Long: `Switch the remote system to a different bootable container image.

Example:
  bootc-man remote switch myserver quay.io/myorg/myimage:latest
  bootc-man remote switch myserver --apply quay.io/myorg/myimage:latest
  bootc-man remote switch myserver --transport oci /path/to/image
  bootc-man remote switch --vm myvm quay.io/myorg/myimage:latest`,
	Args:    validateRemoteSwitchArgs,
	PreRunE: extractRemoteHost,
	RunE:    runRemoteSwitch,
}

var remoteRollbackCmd = &cobra.Command{
	Use:   "rollback [host]",
	Short: "Rollback the remote system to the previous deployment",
	Long: `Rollback the remote system to the previous deployment.

Example:
  bootc-man remote rollback myserver
  bootc-man remote rollback myserver --apply  # Apply rollback immediately (triggers reboot)
  bootc-man remote rollback --vm myvm
  bootc-man remote rollback --vm myvm --apply`,
	Args:    validateRemoteArgs,
	PreRunE: extractRemoteHost,
	RunE:    runRemoteRollback,
}

var remoteStatusCmd = &cobra.Command{
	Use:   "status [host]",
	Short: "Show bootc system status on the remote host",
	Long: `Show the current bootc system status on the remote host.

The output shows:
  - Booted deployment (currently running)
  - Staged deployment (will be used on next boot)
  - Rollback deployment (previous version)

Example:
  bootc-man remote status myserver
  bootc-man remote status myserver --json
  bootc-man remote status --vm myvm`,
	Args:    validateRemoteArgs,
	PreRunE: extractRemoteHost,
	RunE:    runRemoteStatus,
}

// Flags
var (
	// Global remote flags
	remoteHost string
	remoteVM   string // VM name (mutually exclusive with host argument)

	// Upgrade flags
	remoteUpgradeCheck bool
	remoteUpgradeApply bool
	remoteUpgradeQuiet bool

	// Switch flags
	remoteSwitchTransport string
	remoteSwitchApply     bool
	remoteSwitchRetain    bool

	// Rollback flags
	remoteRollbackApply bool
)

func init() {
	// Add --vm flag to all remote subcommands
	for _, cmd := range []*cobra.Command{remoteUpgradeCmd, remoteSwitchCmd, remoteRollbackCmd, remoteStatusCmd} {
		cmd.Flags().StringVar(&remoteVM, "vm", "", "Connect to a bootc-man managed VM instead of SSH host")
	}

	// Upgrade flags
	remoteUpgradeCmd.Flags().BoolVar(&remoteUpgradeCheck, "check", false, "Only check if upgrade is available")
	remoteUpgradeCmd.Flags().BoolVar(&remoteUpgradeApply, "apply", false, "Apply upgrade immediately (triggers reboot)")
	remoteUpgradeCmd.Flags().BoolVarP(&remoteUpgradeQuiet, "quiet", "q", false, "Suppress output")

	// Switch flags
	remoteSwitchCmd.Flags().StringVar(&remoteSwitchTransport, "transport", "registry", "Image transport (registry, oci, oci-archive)")
	remoteSwitchCmd.Flags().BoolVar(&remoteSwitchApply, "apply", false, "Apply switch immediately (triggers reboot)")
	remoteSwitchCmd.Flags().BoolVar(&remoteSwitchRetain, "retain", false, "Retain existing deployments")

	// Rollback flags
	remoteRollbackCmd.Flags().BoolVar(&remoteRollbackApply, "apply", false, "Apply rollback immediately (triggers reboot)")

	// Set completion functions for host/vm name completion
	remoteUpgradeCmd.ValidArgsFunction = completeRemoteTarget
	remoteRollbackCmd.ValidArgsFunction = completeRemoteTarget
	remoteStatusCmd.ValidArgsFunction = completeRemoteTarget
	// For switch command, we need custom completion that handles both host and image
	remoteSwitchCmd.ValidArgsFunction = completeRemoteTargetForSwitch

	// Register --vm flag completion
	for _, cmd := range []*cobra.Command{remoteUpgradeCmd, remoteSwitchCmd, remoteRollbackCmd, remoteStatusCmd} {
		if err := cmd.RegisterFlagCompletionFunc("vm", completeVMNames); err != nil {
			// Ignore error - completion is optional
			_ = err
		}
	}

	// Add subcommands
	remoteCmd.AddCommand(remoteUpgradeCmd)
	remoteCmd.AddCommand(remoteSwitchCmd)
	remoteCmd.AddCommand(remoteRollbackCmd)
	remoteCmd.AddCommand(remoteStatusCmd)
}

// validateRemoteArgs validates arguments for remote commands (upgrade, rollback, status)
// Either --vm must be specified, or exactly 1 host argument is required
func validateRemoteArgs(cmd *cobra.Command, args []string) error {
	vmFlag, _ := cmd.Flags().GetString("vm")

	if vmFlag != "" {
		// --vm is specified, no host argument should be provided
		if len(args) > 0 {
			return fmt.Errorf("cannot specify both --vm and host argument")
		}
		return nil
	}

	// No --vm flag, require exactly 1 host argument
	if len(args) != 1 {
		return fmt.Errorf("requires 1 host argument (or use --vm flag)")
	}
	return nil
}

// validateRemoteSwitchArgs validates arguments for remote switch command
// Either --vm + image, or host + image
func validateRemoteSwitchArgs(cmd *cobra.Command, args []string) error {
	vmFlag, _ := cmd.Flags().GetString("vm")

	if vmFlag != "" {
		// --vm is specified, only image argument should be provided
		if len(args) != 1 {
			return fmt.Errorf("requires 1 image argument when using --vm")
		}
		return nil
	}

	// No --vm flag, require host + image
	if len(args) != 2 {
		return fmt.Errorf("requires 2 arguments: <host> <image> (or use --vm <vm> <image>)")
	}
	return nil
}

// extractRemoteHost extracts the host name from command line arguments or --vm flag
// For "remote status edge-root", args[0] should be "edge-root"
// For "remote switch edge-root image", args[0] should be "edge-root"
// For "remote status --vm myvm", remoteVM should be set
func extractRemoteHost(cmd *cobra.Command, args []string) error {
	// Get --vm flag value (it's command-specific, not persistent)
	vmFlag, _ := cmd.Flags().GetString("vm")
	if vmFlag != "" {
		remoteVM = vmFlag
		return nil
	}

	if remoteHost != "" {
		// Already set, skip
		return nil
	}

	// For most commands, the host is the first argument
	// Exception: switch command has <host> <image>
	if len(args) > 0 {
		// First arg should be the host name
		// Verify it's not a flag
		if args[0] != "" && args[0][0] != '-' {
			remoteHost = args[0]
			return nil
		}
	}

	return fmt.Errorf("host name is required (or use --vm flag)")
}

// getSSHConfigPath returns the path to the SSH config file
func getSSHConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "config")
}

// parseSSHConfigHosts parses ~/.ssh/config and returns a list of host names
func parseSSHConfigHosts() []string {
	configPath := getSSHConfigPath()
	if configPath == "" {
		return nil
	}

	file, err := os.Open(configPath)
	if err != nil {
		// File doesn't exist or can't be read - silently return empty list
		return nil
	}
	defer file.Close()

	var hosts []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for "Host" keyword (case-insensitive)
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// SSH config keywords are case-insensitive
		if strings.EqualFold(fields[0], "Host") {
			// Add all host names after "Host" keyword
			for i := 1; i < len(fields); i++ {
				host := fields[i]
				// Skip wildcards and patterns
				if host != "*" && !strings.Contains(host, "*") && !strings.Contains(host, "?") && !strings.Contains(host, "!") {
					hosts = append(hosts, host)
				}
			}
		}
	}

	return hosts
}

// completeSSHHosts provides shell completion for SSH host names from ~/.ssh/config
func completeSSHHosts(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If host name is already provided, allow flag completion
	if len(args) >= 1 {
		return nil, cobra.ShellCompDirectiveDefault
	}

	hosts := parseSSHConfigHosts()
	if len(hosts) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Filter hosts that match the prefix
	var matches []string
	for _, host := range hosts {
		if strings.HasPrefix(host, toComplete) {
			matches = append(matches, host)
		}
	}

	return matches, cobra.ShellCompDirectiveNoFileComp
}

// completeSSHHostsForSwitch provides shell completion for switch command
// First argument is host, second is image
func completeSSHHostsForSwitch(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// If we already have a host (args[0]), we're completing the image or flags
	if len(args) >= 1 {
		// If we already have both host and image, allow flag completion
		if len(args) >= 2 {
			return nil, cobra.ShellCompDirectiveDefault
		}
		// If we only have host, complete image (let shell do file completion)
		return nil, cobra.ShellCompDirectiveDefault
	}

	// Otherwise, complete host names
	return completeSSHHosts(cmd, args, toComplete)
}

// completeRemoteTarget provides shell completion for remote targets (SSH hosts)
// VM names are completed via --vm flag completion
func completeRemoteTarget(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Check if --vm flag is set
	vmFlag, _ := cmd.Flags().GetString("vm")
	if vmFlag != "" {
		// --vm is set, no positional args needed
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Complete SSH hosts
	return completeSSHHosts(cmd, args, toComplete)
}

// completeRemoteTargetForSwitch provides shell completion for switch command
func completeRemoteTargetForSwitch(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Check if --vm flag is set
	vmFlag, _ := cmd.Flags().GetString("vm")
	if vmFlag != "" {
		// --vm is set, only need image argument
		if len(args) >= 1 {
			return nil, cobra.ShellCompDirectiveDefault
		}
		// Complete image (let shell do file completion for now)
		return nil, cobra.ShellCompDirectiveDefault
	}

	// Complete SSH hosts and image
	return completeSSHHostsForSwitch(cmd, args, toComplete)
}

// RemoteDriver is an interface for both SSH and VM drivers
type RemoteDriver interface {
	Host() string
	IsDryRun() bool
	CheckConnection(ctx context.Context) error
	CheckBootc(ctx context.Context) error
	Upgrade(ctx context.Context, opts bootc.UpgradeOptions) error
	Switch(ctx context.Context, image string, opts bootc.SwitchOptions) error
	Rollback(ctx context.Context, opts bootc.RollbackOptions) error
	Status(ctx context.Context) (*bootc.Status, error)
}

// getDriver creates an SSH or VM driver based on flags and verifies connectivity
func getDriver(ctx context.Context) (RemoteDriver, error) {
	if remoteVM != "" {
		return getVMDriver(ctx)
	}
	return getSSHDriver(ctx)
}

// getSSHDriver creates an SSH driver and verifies connectivity
func getSSHDriver(ctx context.Context) (*bootc.SSHDriver, error) {
	driver := bootc.NewSSHDriver(bootc.SSHDriverOptions{
		Host:    remoteHost,
		Verbose: verbose,
		DryRun:  dryRun,
	})

	// Skip connectivity checks in dry-run mode
	if dryRun {
		return driver, nil
	}

	// Check SSH connectivity
	if err := driver.CheckConnection(ctx); err != nil {
		return nil, err
	}

	// Check if bootc is available
	if err := driver.CheckBootc(ctx); err != nil {
		return nil, err
	}

	return driver, nil
}

// getVMDriver creates a VM driver and verifies connectivity
func getVMDriver(ctx context.Context) (*bootc.VMDriver, error) {
	// Load VM info
	vmInfo, err := vm.LoadVMInfo(remoteVM)
	if err != nil {
		return nil, fmt.Errorf("failed to load VM info: %w", err)
	}

	// Check if VM is actually running (by checking process, not just saved state)
	if !vm.IsVMRunning(vmInfo) {
		return nil, fmt.Errorf("VM '%s' is not running\n  Start it with: bootc-man vm start %s",
			remoteVM, remoteVM)
	}

	driver := bootc.NewVMDriver(bootc.VMDriverOptions{
		VMName:     remoteVM,
		SSHHost:    vmInfo.SSHHost,
		SSHPort:    vmInfo.SSHPort,
		SSHUser:    vmInfo.SSHUser,
		SSHKeyPath: vmInfo.SSHKeyPath,
		Verbose:    verbose,
		DryRun:     dryRun,
	})

	// Skip connectivity checks in dry-run mode
	if dryRun {
		return driver, nil
	}

	// Check SSH connectivity
	if err := driver.CheckConnection(ctx); err != nil {
		return nil, err
	}

	// Check if bootc is available
	if err := driver.CheckBootc(ctx); err != nil {
		return nil, err
	}

	return driver, nil
}

func runRemoteUpgrade(cmd *cobra.Command, args []string) error {
	driver, err := getDriver(cmd.Context())
	if err != nil {
		return err
	}

	opts := bootc.UpgradeOptions{
		Check: remoteUpgradeCheck,
		Apply: remoteUpgradeApply,
		Quiet: remoteUpgradeQuiet,
	}

	action := "Upgrading"
	if remoteUpgradeCheck {
		action = "Checking for upgrade on"
	}
	fmt.Printf("⬆️  %s %s...\n", action, driver.Host())

	if remoteUpgradeApply {
		fmt.Println("⚠️  --apply specified: system will reboot after staging!")
	}
	fmt.Println()

	if err := driver.Upgrade(cmd.Context(), opts); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}

	if driver.IsDryRun() {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	fmt.Println()
	fmt.Printf("✓ Upgrade operation completed on %s\n", driver.Host())
	return nil
}

func runRemoteSwitch(cmd *cobra.Command, args []string) error {
	driver, err := getDriver(cmd.Context())
	if err != nil {
		return err
	}

	// Determine image argument position based on --vm flag
	var image string
	if remoteVM != "" {
		// With --vm, args[0] is image
		image = args[0]
	} else {
		// Without --vm, args[0] is host (already extracted), args[1] is image
		image = args[1]
	}

	opts := bootc.SwitchOptions{
		Transport: remoteSwitchTransport,
		Apply:     remoteSwitchApply,
		Retain:    remoteSwitchRetain,
	}

	fmt.Printf("🔄 Switching %s to image: %s\n", driver.Host(), image)
	fmt.Printf("   Transport: %s\n", remoteSwitchTransport)
	if remoteSwitchApply {
		fmt.Println("⚠️  --apply specified: system will reboot after staging!")
	}
	fmt.Println()

	if err := driver.Switch(cmd.Context(), image, opts); err != nil {
		return fmt.Errorf("switch failed: %w", err)
	}

	if driver.IsDryRun() {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	fmt.Println()
	fmt.Printf("✓ Switch completed on %s\n", driver.Host())
	fmt.Println("  Reboot the system to apply the new image")
	return nil
}

func runRemoteRollback(cmd *cobra.Command, args []string) error {
	driver, err := getDriver(cmd.Context())
	if err != nil {
		return err
	}

	opts := bootc.RollbackOptions{
		Apply: remoteRollbackApply,
	}

	fmt.Printf("⏪ Rolling back %s to previous deployment...\n", driver.Host())
	if remoteRollbackApply {
		fmt.Println("⚠️  --apply specified: system will reboot after rollback!")
	}
	fmt.Println()

	if err := driver.Rollback(cmd.Context(), opts); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	if driver.IsDryRun() {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	fmt.Println()
	fmt.Printf("✓ Rollback completed on %s\n", driver.Host())
	if !remoteRollbackApply {
		fmt.Println("  Reboot the system to apply the rollback")
	}
	return nil
}

func runRemoteStatus(cmd *cobra.Command, args []string) error {
	driver, err := getDriver(cmd.Context())
	if err != nil {
		return err
	}

	status, err := driver.Status(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	// In dry-run mode, just show the command was displayed
	if driver.IsDryRun() {
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	fmt.Printf("📊 Bootc System Status - %s\n", driver.Host())
	fmt.Println()

	// Booted deployment
	if status.Status.Booted != nil && status.Status.Booted.Image != nil {
		fmt.Println("Booted (current):")
		printBootEntry(status.Status.Booted)
	}

	// Staged deployment
	if status.Status.Staged != nil && status.Status.Staged.Image != nil {
		fmt.Println()
		fmt.Println("Staged (next boot):")
		printBootEntry(status.Status.Staged)
	}

	// Rollback deployment
	if status.Status.Rollback != nil && status.Status.Rollback.Image != nil {
		fmt.Println()
		fmt.Println("Rollback (previous):")
		printBootEntry(status.Status.Rollback)
	}

	return nil
}

func printBootEntry(entry *bootc.BootEntry) {
	if entry == nil || entry.Image == nil {
		return
	}

	img := entry.Image
	fmt.Printf("  Image: %s\n", img.Image.Image)
	if img.Image.Transport != "" {
		fmt.Printf("  Transport: %s\n", img.Image.Transport)
	}
	if img.Version != "" {
		fmt.Printf("  Version: %s\n", img.Version)
	}
	if img.ImageDigest != "" {
		digest := img.ImageDigest
		if len(digest) > 20 {
			digest = digest[:20] + "..."
		}
		fmt.Printf("  Digest: %s\n", digest)
	}
	if img.Timestamp != "" {
		fmt.Printf("  Timestamp: %s\n", img.Timestamp)
	}
	if entry.Pinned {
		fmt.Println("  Pinned: yes")
	}
}
