package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tnk4on/bootc-man/internal/ci"
	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/vm"
)

var vmCmd = &cobra.Command{
	Use:   "vm",
	Short: "Manage VMs for bootc testing",
	Long:  `Manage VMs for bootc testing. Requires build and convert stages to be completed first.`,
}

var vmStartCmd = &cobra.Command{
	Use:   "start [name]",
	Short: "Start a VM from build and convert artifacts",
	Long: `Start a VM using artifacts from build and convert stages.

This command requires:
  - Build stage to be completed (container image exists)
  - Convert stage to be completed (disk image exists)

The VM will be started with vfkit (macOS) and can be accessed via SSH.
By default, the VM name is derived from the pipeline name in bootc-ci.yaml.
You can specify a VM name as an argument or use --name flag.
Use --gui to show the VM console in a GUI window (macOS only).`,
	Args:              cobra.RangeArgs(0, 1),
	RunE:              runVMStart,
	ValidArgsFunction: completeStartableVMNames,
}

var vmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all VMs",
	Long:  `List all VMs that have been created.`,
	Args:  cobra.NoArgs,
	RunE:  runVMList,
}

var vmStatusCmd = &cobra.Command{
	Use:   "status [name]",
	Short: "Show VM status",
	Long: `Show detailed status information for a VM.
If name is omitted and bootc-ci.yaml exists in current directory,
uses the pipeline name as default VM name.`,
	RunE:              runVMStatus,
	ValidArgsFunction: completeRunningVMNames,
}

var vmStopCmd = &cobra.Command{
	Use:   "stop [name]",
	Short: "Stop a VM",
	Long: `Stop a running VM.
If name is omitted and bootc-ci.yaml exists in current directory,
uses the pipeline name as default VM name.`,
	RunE:              runVMStop,
	ValidArgsFunction: completeRunningVMNames,
}

var vmSSHCmd = &cobra.Command{
	Use:   "ssh [name]",
	Short: "Connect to VM via SSH",
	Long: `Connect to a VM via SSH.
If name is omitted and bootc-ci.yaml exists in current directory,
uses the pipeline name as default VM name.`,
	RunE:              runVMSSH,
	ValidArgsFunction: completeRunningVMNames,
}

var vmRemoveCmd = &cobra.Command{
	Use:   "rm [name]",
	Short: "Remove a VM",
	Long: `Remove a VM and its associated files. Use --force to remove without confirmation.
If name is omitted and bootc-ci.yaml exists in current directory,
uses the pipeline name as default VM name.`,
	RunE:              runVMRemove,
	ValidArgsFunction: completeVMNames,
}

var (
	vmStartName         string
	vmStartPipelineFile string
	vmStartCPUs         int
	vmStartMemory       int
	vmStartGUI          bool
	vmRemoveForce       bool
	vmSSHUser           string
	// Shared pipeline file flag for VM subcommands
	vmPipelineFile string
)

// getDefaultVMName generates the default VM name from pipeline file
// Returns the VM name based on pipeline metadata.name, or error if pipeline not found
func getDefaultVMName(pipelineFile string) (string, error) {
	pipelineFilePath, err := findPipelineFile(pipelineFile)
	if err != nil {
		return "", fmt.Errorf("no pipeline file found: %w", err)
	}

	pipeline, err := ci.LoadPipeline(pipelineFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to load pipeline: %w", err)
	}

	// Generate VM name from pipeline name (matching ci run --stage test naming)
	pipelineName := pipeline.Metadata.Name
	pipelineName = strings.ReplaceAll(pipelineName, "/", "-")
	pipelineName = strings.ReplaceAll(pipelineName, " ", "-")
	pipelineName = strings.ToLower(pipelineName)

	return vm.SanitizeVMName(pipelineName), nil
}

// getSSHUser returns the default SSH user from config
func getSSHUser() string {
	cfg, err := config.Load("")
	if err != nil {
		return "user" // fallback default
	}
	if cfg.VM.SSHUser != "" {
		return cfg.VM.SSHUser
	}
	return "user" // fallback default
}

// StartableCandidate represents a VM that can be started
type StartableCandidate struct {
	Name        string // VM name or pipeline name
	Type        string // "stopped_vm" or "new_pipeline"
	Description string // Human-readable description
	Command     string // Suggested command argument
}

// Note: findStartableCandidates was removed as it is currently unused.
// It can be restored if needed for future completion functionality.

func init() {
	vmCmd.AddCommand(vmStartCmd)
	vmCmd.AddCommand(vmListCmd)
	vmCmd.AddCommand(vmStatusCmd)
	vmCmd.AddCommand(vmStopCmd)
	vmCmd.AddCommand(vmSSHCmd)
	vmCmd.AddCommand(vmRemoveCmd)

	vmStartCmd.Flags().StringVar(&vmStartName, "name", "", "VM name (default: derived from pipeline name, can also be specified as argument)")
	vmStartCmd.Flags().StringVarP(&vmStartPipelineFile, "pipeline", "p", "", "Pipeline file path (default: bootc-ci.yaml)")
	vmStartCmd.Flags().IntVar(&vmStartCPUs, "cpus", 2, "Number of CPUs")
	vmStartCmd.Flags().IntVar(&vmStartMemory, "memory", 4096, "Memory size in MB")
	vmStartCmd.Flags().BoolVar(&vmStartGUI, "gui", false, "Display VM console in GUI window (macOS only)")

	// Register completion for --name flag
	_ = vmStartCmd.RegisterFlagCompletionFunc("name", completeStartableVMNames)

	vmRemoveCmd.Flags().BoolVarP(&vmRemoveForce, "force", "f", false, "Force removal even if VM is running")

	// Add --pipeline flag to VM subcommands that need pipeline file
	pipelineHelp := "Pipeline file path (default: bootc-ci.yaml in current directory)"
	vmStopCmd.Flags().StringVarP(&vmPipelineFile, "pipeline", "p", "", pipelineHelp)
	vmStatusCmd.Flags().StringVarP(&vmPipelineFile, "pipeline", "p", "", pipelineHelp)
	vmSSHCmd.Flags().StringVarP(&vmPipelineFile, "pipeline", "p", "", pipelineHelp)
	vmSSHCmd.Flags().StringVarP(&vmSSHUser, "user", "u", "", "SSH user name (default: from config or 'user')")
	vmRemoveCmd.Flags().StringVarP(&vmPipelineFile, "pipeline", "p", "", pipelineHelp)
	vmListCmd.Flags().StringVarP(&vmPipelineFile, "pipeline", "p", "", pipelineHelp)
}

func runVMStart(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Dry-run mode: show commands that would be executed
	if dryRun {
		vmType := vm.GetDefaultVMType()
		fmt.Println("📋 Equivalent command (start VM):")

		switch vmType {
		case vm.VfkitVM:
			fmt.Println("   vfkit --cpus <n> --memory <mb> --bootloader efi,variable-store=<efi-store> \\")
			fmt.Println("         --device virtio-blk,path=<disk.raw> --device virtio-net,nat,natLocalhost")
		case vm.QemuVM:
			fmt.Println("   qemu-system-x86_64 -enable-kvm -m <mb> -smp <n> \\")
			fmt.Println("         -drive file=<disk.raw>,format=raw,if=virtio \\")
			fmt.Println("         -netdev user,id=net0,hostfwd=tcp::<port>-:22")
		}
		fmt.Println()
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	// Check if hypervisor is available (platform-specific)
	vmType := vm.GetDefaultVMType()
	tempOpts := vm.VMOptions{Name: "check"}
	tempDriver, err := vm.NewDriver(tempOpts, false)
	if err != nil {
		fmt.Printf("❌ %s is not available on this platform\n", vmType.String())
		return err
	}
	if err := tempDriver.Available(); err != nil {
		fmt.Printf("❌ %s\n", err)
		return err
	}

	// Generate VM name: argument takes precedence, then --name flag, then pipeline name
	var vmName string
	if len(args) > 0 && args[0] != "" {
		vmName = args[0]
	} else if vmStartName != "" {
		vmName = vmStartName
	} else {
		// Use default VM name from pipeline
		var err error
		vmName, err = getDefaultVMName(vmStartPipelineFile)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			fmt.Println("   Specify VM name as argument, use --name flag, or use --pipeline-file to specify pipeline file")
			return err
		}
	}
	vmName = vm.SanitizeVMName(vmName)

	// First, check if we're restarting an existing stopped VM
	// In this case, we don't need podman (skip prerequisites check)
	existingVM, err := vm.LoadVMInfo(vmName)
	if err == nil && existingVM != nil {
		// Check if VM is actually running by checking process
		isRunning := false
		processID := existingVM.ProcessID
		if processID == 0 {
			// Fallback for old VM info format
			processID = existingVM.VfkitPID
		}
		if processID > 0 {
			process, err := os.FindProcess(processID)
			if err == nil {
				if err := process.Signal(os.Signal(syscall.Signal(0))); err == nil {
					isRunning = true
				}
			}
		}
		if isRunning {
			fmt.Printf("⚠️  VM '%s' is already running\n", vmName)
			fmt.Printf("   Use 'bootc-man vm stop %s' to stop it first\n", vmName)
			return fmt.Errorf("VM already running")
		}

		// Check if disk image still exists - if so, we can restart without podman
		if _, err := os.Stat(existingVM.DiskImage); err == nil {
			// Display absolute path for clarity when running from different directories
			absDiskPath, _ := filepath.Abs(existingVM.DiskImage)
			if absDiskPath == "" {
				absDiskPath = existingVM.DiskImage
			}
			fmt.Printf("🔄 Restarting existing VM '%s'...\n", vmName)
			fmt.Printf("   VM disk: %s\n", absDiskPath)
			fmt.Println()

			// Use existing VM info to restart
			return restartExistingVM(ctx, existingVM)
		}
		// Disk image doesn't exist, fall through to create new VM
		fmt.Printf("⚠️  VM '%s' exists but disk image not found, will create new VM\n", vmName)
	} else {
		// VM info doesn't exist, but check if disk image exists in artifacts
		// This handles the case where VM was removed but disk image still exists
		// Look in current directory for pipeline artifacts
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		diskImagePath := findDiskImageInArtifacts(wd)
		if diskImagePath != "" {
			// Display absolute paths for clarity when running from different directories
			absSourcePath, _ := filepath.Abs(diskImagePath)
			if absSourcePath == "" {
				absSourcePath = diskImagePath
			}
			vmsDir, _ := vm.GetVMsDir()
			absVMDiskPath := filepath.Join(vmsDir, vmName+".raw")
			fmt.Printf("🚀 Starting new VM '%s'...\n", vmName)
			fmt.Printf("   Source image: %s\n", absSourcePath)
			fmt.Printf("   VM disk:      %s\n", absVMDiskPath)
			fmt.Println()

			// Create a new VM info and start without podman
			return startVMWithDiskImage(ctx, vmName, diskImagePath)
		}
	}

	// Find pipeline file (needed for new VM creation)
	pipelineFile := vmStartPipelineFile
	if pipelineFile == "" {
		var err error
		pipelineFile, err = findPipelineFile("")
		if err != nil {
			fmt.Println("❌", err)
			return err
		}
	}

	// Load pipeline
	pipeline, err := ci.LoadPipeline(pipelineFile)
	if err != nil {
		fmt.Printf("❌ Failed to load pipeline: %v\n", err)
		return err
	}

	// Generate image tag
	customTag := ""
	if pipeline.Spec.Build != nil {
		customTag = pipeline.Spec.Build.ImageTag
	}
	imageTag := vm.GenerateImageTag(pipeline.Metadata.Name, customTag)

	// Check prerequisites (requires podman for new VM)
	fmt.Println("🔍 Checking prerequisites...")
	prereq, err := vm.CheckPrerequisites(ctx, pipeline.BaseDir(), imageTag)
	if err != nil {
		fmt.Printf("❌ Failed to check prerequisites: %v\n", err)
		return err
	}

	if !prereq.BuildCompleted || !prereq.ConvertCompleted {
		fmt.Println("❌ Prerequisites not met:")
		for _, errMsg := range prereq.Errors {
			fmt.Printf("   %s\n", errMsg)
		}
		return fmt.Errorf("prerequisites not met")
	}

	fmt.Println("✅ Prerequisites met")
	fmt.Println()

	// Get SSH key path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	sshKeyPath := filepath.Join(homeDir, ".ssh", "id_ed25519")
	if _, err := os.Stat(sshKeyPath); err != nil {
		// Try RSA key
		sshKeyPath = filepath.Join(homeDir, ".ssh", "id_rsa")
		if _, err := os.Stat(sshKeyPath); err != nil {
			return fmt.Errorf("no SSH private key found. Please ensure ~/.ssh/id_ed25519 or ~/.ssh/id_rsa exists")
		}
	}

	// Prepare disk image path
	diskImagePath := prereq.DiskImagePath

	// Copy disk image to VM directory
	vmDiskPath, err := copyDiskImageToVMs(diskImagePath, vmName)
	if err != nil {
		return fmt.Errorf("failed to copy disk image: %w", err)
	}

	// Create driver options
	// SSHPort is set to 0 to allow dynamic allocation by the driver
	driverOpts := vm.VMOptions{
		Name:       vmName,
		DiskImage:  vmDiskPath,
		CPUs:       vmStartCPUs,
		Memory:     vmStartMemory,
		SSHKeyPath: sshKeyPath,
		SSHUser:    getSSHUser(),
		SSHPort:    0, // Dynamic allocation
		GUI:        vmStartGUI,
	}

	// Create platform-specific driver
	driver, err := vm.NewDriver(driverOpts, verbose)
	if err != nil {
		return fmt.Errorf("failed to create VM driver: %w", err)
	}

	// Start VM
	fmt.Printf("🚀 Starting VM with %s...\n", vmType.String())
	if err := driver.Start(ctx, driverOpts); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	// Wait for VM to be ready
	fmt.Println("⏳ Waiting for VM to boot...")
	if err := driver.WaitForReady(ctx); err != nil {
		_ = driver.Cleanup()
		return fmt.Errorf("VM failed to boot: %w", err)
	}
	fmt.Println("✅ VM is running")

	// Wait for SSH to be available
	fmt.Println("⏳ Waiting for SSH to be available...")
	if err := driver.WaitForSSH(ctx); err != nil {
		fmt.Printf("⚠️  Warning: SSH not available: %v\n", err)
		fmt.Println("   VM is running but SSH may not be ready yet")
	} else {
		fmt.Println("✅ SSH connection established")
	}

	// Get SSH config from driver
	sshConfig := driver.GetSSHConfig()

	// Save VM info using driver
	vmInfo := driver.ToVMInfo(vmName, pipeline.Metadata.Name, pipelineFile, imageTag)

	if err := vm.SaveVMInfo(vmInfo); err != nil {
		fmt.Printf("⚠️  Warning: Failed to save VM info: %v\n", err)
	}

	// Display SSH connection information
	fmt.Println()
	fmt.Println("SSH connection information:")
	fmt.Printf("  Host: %s\n", sshConfig.Host)
	fmt.Printf("  Port: %d\n", sshConfig.Port)
	fmt.Printf("  User: %s\n", sshConfig.User)
	fmt.Printf("  Key: %s\n", sshConfig.KeyPath)
	fmt.Println()
	fmt.Printf("To connect:\n")
	fmt.Printf("  bootc-man vm ssh %s\n", vmName)
	fmt.Println()
	fmt.Printf("Alternative (direct SSH):\n")
	fmt.Printf("  ssh -i %s -p %d %s@%s\n", sshConfig.KeyPath, sshConfig.Port, sshConfig.User, sshConfig.Host)
	fmt.Println()
	fmt.Printf("To stop:\n")
	fmt.Printf("  bootc-man vm stop %s\n", vmName)

	// Note: We don't defer cleanup here because we want the VM to keep running
	// The user will need to explicitly stop it with `vm stop`

	return nil
}

// restartExistingVM restarts an existing stopped VM using its saved info
// This does not require podman - uses platform-specific hypervisor
func restartExistingVM(ctx context.Context, existingVM *vm.VMInfo) error {
	vmName := existingVM.Name
	diskImagePath := existingVM.DiskImage
	sshKeyPath := existingVM.SSHKeyPath

	// Verify SSH key exists
	if _, err := os.Stat(sshKeyPath); err != nil {
		// Try to find alternative SSH key
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		sshKeyPath = filepath.Join(homeDir, ".ssh", "id_ed25519")
		if _, err := os.Stat(sshKeyPath); err != nil {
			sshKeyPath = filepath.Join(homeDir, ".ssh", "id_rsa")
			if _, err := os.Stat(sshKeyPath); err != nil {
				return fmt.Errorf("no SSH private key found")
			}
		}
	}

	sshUser := existingVM.SSHUser
	if sshUser == "" {
		sshUser = getSSHUser()
	}

	// Create driver options
	// SSHPort is set to 0 to allow dynamic allocation by the driver
	vmType := vm.GetDefaultVMType()
	driverOpts := vm.VMOptions{
		Name:       vmName,
		DiskImage:  diskImagePath,
		CPUs:       vmStartCPUs,
		Memory:     vmStartMemory,
		SSHKeyPath: sshKeyPath,
		SSHUser:    sshUser,
		SSHPort:    0, // Dynamic allocation
		GUI:        vmStartGUI,
	}

	// Create platform-specific driver
	driver, err := vm.NewDriver(driverOpts, verbose)
	if err != nil {
		return fmt.Errorf("failed to create VM driver: %w", err)
	}

	// Start VM
	fmt.Printf("🚀 Starting VM with %s...\n", vmType.String())
	if err := driver.Start(ctx, driverOpts); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	// Wait for VM to be ready
	fmt.Println("⏳ Waiting for VM to boot...")
	if err := driver.WaitForReady(ctx); err != nil {
		_ = driver.Cleanup()
		return fmt.Errorf("VM failed to boot: %w", err)
	}
	fmt.Println("✅ VM is running")

	// Wait for SSH
	fmt.Println("⏳ Waiting for SSH to be available...")
	if err := driver.WaitForSSH(ctx); err != nil {
		fmt.Printf("⚠️  Warning: SSH not available: %v\n", err)
	} else {
		fmt.Println("✅ SSH connection established")
	}

	// Get SSH config from driver
	sshConfig := driver.GetSSHConfig()

	// Update VM info using driver
	updatedInfo := driver.ToVMInfo(vmName, existingVM.PipelineName, existingVM.PipelineFile, existingVM.ImageTag)

	if err := vm.SaveVMInfo(updatedInfo); err != nil {
		fmt.Printf("⚠️  Warning: Failed to save VM info: %v\n", err)
	}

	// Display SSH connection information
	fmt.Println()
	fmt.Println("SSH connection information:")
	fmt.Printf("  Host: %s\n", sshConfig.Host)
	fmt.Printf("  Port: %d\n", sshConfig.Port)
	fmt.Printf("  User: %s\n", sshConfig.User)
	fmt.Printf("  Key: %s\n", sshConfig.KeyPath)
	fmt.Println()
	fmt.Printf("To connect:\n")
	fmt.Printf("  bootc-man vm ssh %s\n", vmName)
	fmt.Println()
	fmt.Printf("To stop:\n")
	fmt.Printf("  bootc-man vm stop %s\n", vmName)

	return nil
}

// findDiskImageInArtifacts searches for disk images in the artifacts directory
func findDiskImageInArtifacts(baseDir string) string {
	artifactsDir := filepath.Join(baseDir, "output", "images")

	// Check common locations for disk images
	// First, try raw format (preferred for vfkit)
	rawPath := filepath.Join(artifactsDir, "image", "disk.raw")
	if _, err := os.Stat(rawPath); err == nil {
		return rawPath
	}

	// Try qcow2 format
	qcow2Path := filepath.Join(artifactsDir, "qcow2", "disk.qcow2")
	if _, err := os.Stat(qcow2Path); err == nil {
		return qcow2Path
	}

	// Search recursively for any raw or qcow2 file
	var foundPath string
	_ = filepath.Walk(artifactsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			if strings.HasSuffix(info.Name(), ".raw") {
				foundPath = path
				return filepath.SkipAll
			}
			if strings.HasSuffix(info.Name(), ".qcow2") && foundPath == "" {
				foundPath = path
			}
		}
		return nil
	})

	return foundPath
}

// copyDiskImageToVMs copies the source disk image to output/vms/<vmName>.raw
// If the file already exists, it is reused (no copy performed)
// Returns the path to the VM disk image
func copyDiskImageToVMs(srcPath, vmName string) (string, error) {
	// Get global VMs directory
	vmsDir, err := vm.GetVMsDir()
	if err != nil {
		return "", fmt.Errorf("failed to get VMs directory: %w", err)
	}
	if err := os.MkdirAll(vmsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create vms directory: %w", err)
	}

	// Destination path: ~/.local/share/bootc-man/vms/<vmName>.raw
	destPath := filepath.Join(vmsDir, fmt.Sprintf("%s.raw", vmName))

	// Check if destination already exists
	if _, err := os.Stat(destPath); err == nil {
		if verbose {
			fmt.Printf("Using existing VM disk image: %s\n", destPath)
		}
		return destPath, nil
	}

	// Copy the disk image
	if verbose {
		fmt.Printf("Copying disk image to VM directory...\n")
		fmt.Printf("  Source: %s\n", srcPath)
		fmt.Printf("  Dest:   %s\n", destPath)
	}

	// Open source file
	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	// Copy with progress indication for large files
	srcInfo, _ := src.Stat()
	if srcInfo != nil && srcInfo.Size() > 1024*1024*100 { // > 100MB
		fmt.Printf("Copying %.1f GB disk image (this may take a while)...\n", float64(srcInfo.Size())/(1024*1024*1024))
	}

	if _, err := io.Copy(dst, src); err != nil {
		os.Remove(destPath) // Clean up partial file
		return "", fmt.Errorf("failed to copy disk image: %w", err)
	}

	if verbose {
		fmt.Println("✅ Disk image copied")
	}

	return destPath, nil
}

// startVMWithDiskImage starts a new VM using only the disk image (no VM info required)
func startVMWithDiskImage(ctx context.Context, vmName, diskImagePath string) error {
	// Get SSH key path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	sshKeyPath := filepath.Join(homeDir, ".ssh", "id_ed25519")
	if _, err := os.Stat(sshKeyPath); err != nil {
		sshKeyPath = filepath.Join(homeDir, ".ssh", "id_rsa")
		if _, err := os.Stat(sshKeyPath); err != nil {
			return fmt.Errorf("no SSH private key found")
		}
	}

	// Copy disk image to global VMs directory if not already there
	// This allows the original image to remain unchanged and enables multiple VMs
	vmDiskPath, err := copyDiskImageToVMs(diskImagePath, vmName)
	if err != nil {
		return fmt.Errorf("failed to prepare VM disk image: %w", err)
	}

	sshUser := getSSHUser()
	vmType := vm.GetDefaultVMType()

	// Create driver options
	// SSHPort is set to 0 to allow dynamic allocation by the driver
	driverOpts := vm.VMOptions{
		Name:       vmName,
		DiskImage:  vmDiskPath,
		CPUs:       vmStartCPUs,
		Memory:     vmStartMemory,
		SSHKeyPath: sshKeyPath,
		SSHUser:    sshUser,
		SSHPort:    0, // Dynamic allocation
		GUI:        vmStartGUI,
	}

	// Create platform-specific driver
	driver, err := vm.NewDriver(driverOpts, verbose)
	if err != nil {
		return fmt.Errorf("failed to create VM driver: %w", err)
	}

	// Start VM
	fmt.Printf("🚀 Starting VM with %s...\n", vmType.String())
	if err := driver.Start(ctx, driverOpts); err != nil {
		return fmt.Errorf("failed to start VM: %w", err)
	}

	// Wait for VM to be ready
	fmt.Println("⏳ Waiting for VM to boot...")
	if err := driver.WaitForReady(ctx); err != nil {
		_ = driver.Cleanup()
		return fmt.Errorf("VM failed to boot: %w", err)
	}
	fmt.Println("✅ VM is running")

	// Wait for SSH
	fmt.Println("⏳ Waiting for SSH to be available...")
	if err := driver.WaitForSSH(ctx); err != nil {
		fmt.Printf("⚠️  Warning: SSH not available: %v\n", err)
	} else {
		fmt.Println("✅ SSH connection established")
	}

	// Get SSH config from driver
	sshConfig := driver.GetSSHConfig()

	// Create and save VM info using driver
	vmInfo := driver.ToVMInfo(vmName, "unknown", "", "")

	if err := vm.SaveVMInfo(vmInfo); err != nil {
		fmt.Printf("⚠️  Warning: Failed to save VM info: %v\n", err)
	}

	// Display SSH connection information
	fmt.Println()
	fmt.Println("SSH connection information:")
	fmt.Printf("  Host: %s\n", sshConfig.Host)
	fmt.Printf("  Port: %d\n", sshConfig.Port)
	fmt.Printf("  User: %s\n", sshConfig.User)
	fmt.Printf("  Key: %s\n", sshConfig.KeyPath)
	fmt.Println()
	fmt.Printf("To connect:\n")
	fmt.Printf("  bootc-man vm ssh %s\n", vmName)
	fmt.Println()
	fmt.Printf("To stop:\n")
	fmt.Printf("  bootc-man vm stop %s\n", vmName)

	return nil
}

// Note: findBaseDir was removed as it is currently unused.
// It can be restored if needed for future functionality.

// isVMRunning checks if a VM is actually running by checking the VM process
func isVMRunning(vmInfo *vm.VMInfo) bool {
	return vm.IsVMRunning(vmInfo)
}

// completeRunningVMNames returns completion candidates for running VM names
func completeRunningVMNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	vmInfos, err := vm.ListVMInfos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var runningVMNames []string
	for _, info := range vmInfos {
		if isVMRunning(info) {
			// Filter by toComplete prefix if provided
			if toComplete == "" || strings.HasPrefix(info.Name, toComplete) {
				runningVMNames = append(runningVMNames, info.Name)
			}
		}
	}

	return runningVMNames, cobra.ShellCompDirectiveNoFileComp
}

// completeVMNames returns completion candidates for all VM names
func completeVMNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	vmInfos, err := vm.ListVMInfos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var vmNames []string
	for _, info := range vmInfos {
		// Filter by toComplete prefix if provided
		if toComplete == "" || strings.HasPrefix(info.Name, toComplete) {
			vmNames = append(vmNames, info.Name)
		}
	}

	return vmNames, cobra.ShellCompDirectiveNoFileComp
}

// completeStartableVMNames returns completion candidates for VMs that can be started
// This includes stopped VMs with valid disk images
func completeStartableVMNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	vmInfos, err := vm.ListVMInfos()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var vmNames []string
	for _, info := range vmInfos {
		// Only include stopped VMs with valid disk images
		if !isVMRunning(info) {
			// Check if disk image exists
			if _, err := os.Stat(info.DiskImage); err == nil {
				// Filter by toComplete prefix if provided
				if toComplete == "" || strings.HasPrefix(info.Name, toComplete) {
					vmNames = append(vmNames, info.Name)
				}
			}
		}
	}

	return vmNames, cobra.ShellCompDirectiveNoFileComp
}

func runVMList(cmd *cobra.Command, args []string) error {
	// Dry-run mode
	if dryRun {
		fmt.Println("📋 Equivalent command (list VMs):")
		fmt.Println("   ls ~/.local/share/bootc-man/vms/*.json")
		fmt.Println()
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	vmInfos, err := vm.ListVMInfos()
	if err != nil {
		return fmt.Errorf("failed to list VMs: %w", err)
	}

	// Build VM list with status
	type VMListEntry struct {
		Name     string `json:"name"`
		State    string `json:"state"`
		Created  string `json:"created"`
		SSHUser  string `json:"sshUser"`
		SSHHost  string `json:"sshHost"`
		SSHPort  int    `json:"sshPort"`
		Pipeline string `json:"pipeline,omitempty"`
	}

	var entries []VMListEntry
	for _, info := range vmInfos {
		state := "Stopped"
		if isVMRunning(info) {
			state = "Running"
		}
		entries = append(entries, VMListEntry{
			Name:     info.Name,
			State:    state,
			Created:  info.Created.Format("2006-01-02T15:04:05Z07:00"),
			SSHUser:  info.SSHUser,
			SSHHost:  info.SSHHost,
			SSHPort:  info.SSHPort,
			Pipeline: info.PipelineName,
		})
	}

	// JSON output
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}

	// Table output
	if len(vmInfos) == 0 {
		fmt.Println("No VMs found")
		return nil
	}

	fmt.Println("VM List:")
	fmt.Println()
	fmt.Printf("%-30s %-15s %-20s %s\n", "NAME", "STATE", "CREATED", "SSH")
	fmt.Println(strings.Repeat("-", 80))

	for _, entry := range entries {
		sshInfo := fmt.Sprintf("%s@%s:%d", entry.SSHUser, entry.SSHHost, entry.SSHPort)
		created := entry.Created[:19] // Trim timezone for table display
		if len(entry.Created) >= 19 {
			created = strings.Replace(entry.Created[:19], "T", " ", 1)
		}
		fmt.Printf("%-30s %-15s %-20s %s\n", entry.Name, entry.State, created, sshInfo)
	}

	return nil
}

func runVMStatus(cmd *cobra.Command, args []string) error {
	var vmName string
	if len(args) > 0 {
		vmName = args[0]
	} else {
		// Try to get default VM name from pipeline file
		var err error
		vmName, err = getDefaultVMName(vmPipelineFile)
		if err != nil {
			return fmt.Errorf("VM name required: no bootc-ci.yaml found in current directory\n  Specify VM name: bootc-man vm status <name>\n  List available VMs: bootc-man vm list")
		}
	}

	// Dry-run mode
	if dryRun {
		fmt.Println("📋 Equivalent command (show VM status):")
		fmt.Printf("   cat ~/.local/share/bootc-man/vms/%s.json\n", vmName)
		fmt.Println()
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	vmInfo, err := vm.LoadVMInfo(vmName)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return err
	}

	// Helper function to check if process is running
	isProcessRunning := func(pid int) bool {
		if pid <= 0 {
			return false
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			return false
		}
		if err := process.Signal(os.Signal(syscall.Signal(0))); err != nil {
			return false
		}
		return true
	}

	// Get current VM state
	mainPID := vmInfo.ProcessID
	if mainPID == 0 {
		mainPID = vmInfo.VfkitPID // Fallback for old VM info format
	}
	vmRunning := isProcessRunning(mainPID)
	var currentState string
	if vmRunning {
		currentState = "Running"
	} else {
		currentState = "Stopped"
	}

	// Check gvproxy state (macOS specific)
	gvproxyRunning := isProcessRunning(vmInfo.GvproxyPID)

	// Determine VM type
	vmType := vmInfo.VMType
	if vmType == "" {
		vmType = config.BinaryVfkit // Default for old VM info format
	}

	// Display VM status
	fmt.Printf("VM: %s\n", vmInfo.Name)
	fmt.Printf("  Type: %s\n", vmType)
	fmt.Printf("  State: %s\n", currentState)
	fmt.Printf("  Pipeline: %s\n", vmInfo.PipelineName)
	fmt.Printf("  Image Tag: %s\n", vmInfo.ImageTag)
	fmt.Printf("  Disk Image: %s\n", vmInfo.DiskImage)
	fmt.Printf("  Created: %s\n", vmInfo.Created.Format("2006-01-02 15:04:05"))
	fmt.Println()

	// Show process status
	fmt.Println("Processes:")
	if vmRunning {
		fmt.Printf("  %s: Running (PID %d)\n", vmType, mainPID)
	} else {
		fmt.Printf("  %s: Stopped\n", vmType)
	}
	// Show gvproxy status (required for all platforms)
	if gvproxyRunning {
		fmt.Printf("  gvproxy: Running (PID %d)\n", vmInfo.GvproxyPID)
	} else {
		fmt.Printf("  gvproxy: Stopped ⚠️  (SSH not available)\n")
	}
	fmt.Println()

	fmt.Println("SSH Connection:")
	fmt.Printf("  Host: %s\n", vmInfo.SSHHost)
	fmt.Printf("  Port: %d\n", vmInfo.SSHPort)
	fmt.Printf("  User: %s\n", vmInfo.SSHUser)
	fmt.Printf("  Key: %s\n", vmInfo.SSHKeyPath)
	fmt.Println()

	// SSH is available only if both VM and gvproxy are running
	sshAvailable := vmRunning && gvproxyRunning
	if sshAvailable {
		fmt.Printf("To connect:\n")
		fmt.Printf("  ssh -i %s -p %d %s@%s\n", vmInfo.SSHKeyPath, vmInfo.SSHPort, vmInfo.SSHUser, vmInfo.SSHHost)
	} else if vmRunning && !gvproxyRunning {
		fmt.Println("⚠️  SSH is not available because gvproxy has stopped.")
		fmt.Println("   Restart the VM to restore SSH access:")
		fmt.Printf("     bootc-man vm stop %s\n", vmName)
		fmt.Printf("     bootc-man vm start %s\n", vmName)
	}

	return nil
}

func runVMStop(cmd *cobra.Command, args []string) error {
	var vmName string
	if len(args) > 0 {
		vmName = args[0]
	} else {
		// Try to get default VM name from pipeline file
		var err error
		vmName, err = getDefaultVMName(vmPipelineFile)
		if err != nil {
			return fmt.Errorf("VM name required: no bootc-ci.yaml found in current directory\n  Specify VM name: bootc-man vm stop <name>\n  List available VMs: bootc-man vm list")
		}
	}

	// Dry-run mode
	if dryRun {
		fmt.Println("📋 Equivalent command (stop VM):")
		fmt.Printf("   kill -SIGINT <vm-process-id>  # for VM: %s\n", vmName)
		fmt.Println()
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	vmInfo, err := vm.LoadVMInfo(vmName)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return err
	}

	// Helper function to stop a process gracefully
	stopProcess := func(pid int, name string) {
		if pid <= 0 {
			return
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			return
		}
		// Try graceful shutdown first
		if err := process.Signal(os.Interrupt); err == nil {
			// Wait for process to exit
			done := make(chan bool, 1)
			go func() {
				_, _ = process.Wait()
				done <- true
			}()
			select {
			case <-done:
				// Process exited
			case <-time.After(3 * time.Second):
				// Force kill if still running
				_ = process.Kill()
				_, _ = process.Wait()
			}
		} else {
			// If signal failed, try kill directly
			_ = process.Kill()
			_, _ = process.Wait()
		}
	}

	// Stop main VM process (use ProcessID first, fallback to VfkitPID for compatibility)
	mainPID := vmInfo.ProcessID
	if mainPID == 0 {
		mainPID = vmInfo.VfkitPID
	}
	stopProcess(mainPID, "VM")

	// Stop gvproxy (required for all platforms)
	stopProcess(vmInfo.GvproxyPID, config.BinaryGvproxy)

	// Update VM state
	vmInfo.State = "Stopped"
	if err := vm.SaveVMInfo(vmInfo); err != nil {
		fmt.Printf("⚠️  Warning: Failed to update VM state: %v\n", err)
	}

	fmt.Printf("✅ VM '%s' stopped\n", vmName)
	return nil
}

func runVMSSH(cmd *cobra.Command, args []string) error {
	var vmName string
	if len(args) > 0 {
		vmName = args[0]
	} else {
		// Try to get default VM name from pipeline file
		var err error
		vmName, err = getDefaultVMName(vmPipelineFile)
		if err != nil {
			return fmt.Errorf("VM name required: no bootc-ci.yaml found in current directory\n  Specify VM name: bootc-man vm ssh <name>\n  List available VMs: bootc-man vm list")
		}
	}

	// Dry-run mode
	if dryRun {
		fmt.Println("📋 Equivalent command (SSH to VM):")
		fmt.Printf("   ssh -i <key> -p <port> user@localhost  # for VM: %s\n", vmName)
		fmt.Println()
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	vmInfo, err := vm.LoadVMInfo(vmName)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return err
	}

	// Helper function to check if process is running
	isProcessRunning := func(pid int) bool {
		if pid <= 0 {
			return false
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			return false
		}
		if err := process.Signal(os.Signal(syscall.Signal(0))); err != nil {
			return false
		}
		return true
	}

	// Check if VM is running (use ProcessID first, fallback to VfkitPID)
	mainPID := vmInfo.ProcessID
	if mainPID == 0 {
		mainPID = vmInfo.VfkitPID
	}
	if !isProcessRunning(mainPID) {
		fmt.Printf("❌ VM '%s' is not running\n", vmName)
		return fmt.Errorf("VM is not running")
	}

	// Determine VM type
	vmType := vmInfo.VMType
	if vmType == "" {
		vmType = config.BinaryVfkit // Default for old VM info format
	}

	// Check if gvproxy is running (required for all platforms)
	gvproxyRunning := isProcessRunning(vmInfo.GvproxyPID)
	if !gvproxyRunning {
		fmt.Printf("❌ Network proxy (gvproxy) for VM '%s' is not running\n", vmName)
		fmt.Println("   SSH port forwarding is not available.")
		fmt.Println("   Please restart the VM:")
		fmt.Printf("     bootc-man vm stop %s\n", vmName)
		fmt.Printf("     bootc-man vm start %s\n", vmName)
		return fmt.Errorf("gvproxy is not running")
	}

	// For vfkit (macOS), set up port forwarding via gvproxy API
	// (QEMU sets up port forwarding during vm start)
	if vmType == config.BinaryVfkit {
		ctx := context.Background()
		vmIP := ""
		if vmInfo.LogFile != "" {
			logContent, err := os.ReadFile(vmInfo.LogFile)
			if err == nil && len(logContent) > 0 {
				vmIP = ci.ExtractVMIPFromLog(string(logContent))
			}
		}

		// Create gvproxy client to set up port forwarding
		gvproxy, err := ci.NewGvproxyClient(vmName, false)
		if err == nil {
			if vmIP != "" {
				if err := gvproxy.ExposePort(ctx, vmIP, 22); err != nil {
					_ = gvproxy.ExposePort(ctx, "192.168.127.2", 22)
				}
			} else {
				_ = gvproxy.ExposePort(ctx, "192.168.127.2", 22)
			}
		}
	}

	// Determine SSH user: --user flag > VM info > config default
	sshUser := vmInfo.SSHUser
	if vmSSHUser != "" {
		sshUser = vmSSHUser
	}
	if sshUser == "" {
		sshUser = getSSHUser()
	}

	// Build SSH arguments
	sshArgs := []string{
		"-i", vmInfo.SSHKeyPath,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-p", fmt.Sprintf("%d", vmInfo.SSHPort),
		fmt.Sprintf("%s@%s", sshUser, vmInfo.SSHHost),
	}

	// Display connection message
	if verbose {
		// Verbose mode: show detailed SSH connection information
		fmt.Println("SSH connection information:")
		fmt.Printf("  Host: %s\n", vmInfo.SSHHost)
		fmt.Printf("  Port: %d\n", vmInfo.SSHPort)
		fmt.Printf("  User: %s\n", sshUser)
		fmt.Printf("  Key: %s\n", vmInfo.SSHKeyPath)
		fmt.Println()
		fmt.Printf("To connect:\n")
		fmt.Printf("  ssh -i %s -p %d %s@%s\n", vmInfo.SSHKeyPath, vmInfo.SSHPort, sshUser, vmInfo.SSHHost)
		fmt.Println()
	} else {
		// Normal mode: show brief connection message like podman machine ssh
		fmt.Printf("Connecting to vm %s. To close connection, use `exit`\n", vmName)
	}

	sshCmd := exec.Command("ssh", sshArgs...)
	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	return sshCmd.Run()
}

func runVMRemove(cmd *cobra.Command, args []string) error {
	var vmName string
	if len(args) > 0 {
		vmName = args[0]
	} else {
		// Try to get default VM name from pipeline file
		var err error
		vmName, err = getDefaultVMName(vmPipelineFile)
		if err != nil {
			return fmt.Errorf("VM name required: no bootc-ci.yaml found in current directory\n  Specify VM name: bootc-man vm rm <name>\n  List available VMs: bootc-man vm list")
		}
	}

	// Dry-run mode
	if dryRun {
		fmt.Println("📋 Equivalent command (remove VM):")
		fmt.Printf("   rm ~/.local/share/bootc-man/vms/%s.json\n", vmName)
		fmt.Printf("   rm ~/.local/share/bootc-man/vms/%s.raw\n", vmName)
		fmt.Println()
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	vmInfo, err := vm.LoadVMInfo(vmName)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return err
	}

	// Collect files that will be deleted
	filesToDelete := []string{}

	// Get global VMs directory
	vmsDir, err := vm.GetVMsDir()
	if err != nil {
		return fmt.Errorf("failed to get VMs directory: %w", err)
	}

	// VM info file
	vmInfoFile := filepath.Join(vmsDir, fmt.Sprintf("%s.json", vmName))
	filesToDelete = append(filesToDelete, vmInfoFile)

	// VM disk image in global VMs directory
	vmDiskPath := filepath.Join(vmsDir, fmt.Sprintf("%s.raw", vmName))
	if _, err := os.Stat(vmDiskPath); err == nil {
		filesToDelete = append(filesToDelete, vmDiskPath)
	}

	// EFI store
	efiStorePath := filepath.Join(config.RuntimeDir(), fmt.Sprintf("bootc-man-%s-efi-store", vmName))
	if _, err := os.Stat(efiStorePath); err == nil {
		filesToDelete = append(filesToDelete, efiStorePath)
	}

	// Log file (if exists)
	if vmInfo.LogFile != "" {
		if _, err := os.Stat(vmInfo.LogFile); err == nil {
			filesToDelete = append(filesToDelete, vmInfo.LogFile)
		}
	}

	// Ask for confirmation unless --force is set
	if !vmRemoveForce {
		fmt.Println("The following files will be deleted:")
		fmt.Println()
		for _, file := range filesToDelete {
			fmt.Printf("  %s\n", file)
		}
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Are you sure you want to continue? [y/N] ")
		answer, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer == "" || answer[0] != 'y' {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Stop VM if running and not forced
	if !vmRemoveForce && vmInfo.State == "Running" {
		fmt.Printf("⚠️  VM '%s' is running. Stopping it first...\n", vmName)
		if err := runVMStop(cmd, args); err != nil {
			return fmt.Errorf("failed to stop VM: %w", err)
		}
	}

	// Stop processes if still running
	if vmInfo.VfkitPID > 0 {
		process, err := os.FindProcess(vmInfo.VfkitPID)
		if err == nil {
			_ = process.Kill()
			_, _ = process.Wait()
		}
	}

	if vmInfo.GvproxyPID > 0 {
		process, err := os.FindProcess(vmInfo.GvproxyPID)
		if err == nil {
			_ = process.Kill()
			_, _ = process.Wait()
		}
	}

	// Delete all files in the list (includes VM info file, disk image, EFI store, log file)
	for _, file := range filesToDelete {
		if err := os.RemoveAll(file); err != nil {
			if verbose {
				fmt.Printf("⚠️  Warning: failed to delete %s: %v\n", file, err)
			}
		}
	}

	fmt.Printf("✅ VM '%s' removed\n", vmName)
	return nil
}
