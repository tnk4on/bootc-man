package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tnk4on/bootc-man/internal/ci"
	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/podman"
	"github.com/tnk4on/bootc-man/internal/vm"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Display overall status of bootc-man services and VMs",
	Long: `Display the status of all bootc-man managed services and VMs.

This includes:
  - Registry service status
  - VM status (bootc-man vm)
  - System information`,
	RunE: runStatus,
}

type ServiceStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Port    int    `json:"port,omitempty"`
	Message string `json:"message,omitempty"`
}

type VMStatus struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Pipeline string `json:"pipeline,omitempty"`
	SSHHost  string `json:"sshHost,omitempty"`
	SSHPort  int    `json:"sshPort,omitempty"`
	SSHUser  string `json:"sshUser,omitempty"`
	Message  string `json:"message,omitempty"`
}

type OverallStatus struct {
	Platform      string               `json:"platform"`
	Services      []ServiceStatus      `json:"services"`
	VMs           []VMStatus           `json:"vms"`
	Podman        PodmanStatus         `json:"podman"`
	PodmanMachine *PodmanMachineStatus `json:"podmanMachine,omitempty"`
	CITools       []CIToolStatus       `json:"ciTools"`
}

type PodmanStatus struct {
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Rootless  bool   `json:"rootless"`
}

type PodmanMachineStatus struct {
	Running bool   `json:"running"`
	Name    string `json:"name,omitempty"`
	CPUs    string `json:"cpus,omitempty"`
	Memory  string `json:"memory,omitempty"`
	Disk    string `json:"disk,omitempty"`
	Rootful string `json:"rootful,omitempty"`
}

type CIToolStatus struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Image      string `json:"image,omitempty"`
	Version    string `json:"version,omitempty"`
	Privileged bool   `json:"privileged,omitempty"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Dry-run mode: show commands that would be executed
	if dryRun {
		fmt.Println("📋 Equivalent commands (check status):")

		fmt.Println("   podman info --format json")
		if runtime.GOOS != "linux" {
			fmt.Println("   podman machine list --format json")
			fmt.Println("   podman machine inspect <name>")
		}
		fmt.Println("   podman inspect <registry-container>")
		if cfg != nil && cfg.Experimental {
			fmt.Println("   podman inspect <ci-container>")
			fmt.Println("   podman inspect <gui-container>")
		}
		fmt.Println("   podman image exists <tool-image>")
		fmt.Println()
		fmt.Println("(dry-run mode - command not executed)")
		return nil
	}

	status := OverallStatus{
		Platform: fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Services: []ServiceStatus{},
		VMs:      []VMStatus{},
		CITools:  []CIToolStatus{},
	}

	// Check Podman
	pm, err := podman.NewClient()
	if err != nil {
		status.Podman = PodmanStatus{
			Available: false,
		}
	} else {
		// Run independent checks in parallel to reduce total latency
		var wg sync.WaitGroup
		var podmanInfo *podman.PodmanInfo
		var podmanInfoErr error
		var machineStatus *PodmanMachineStatus

		// Podman info check
		wg.Add(1)
		go func() {
			defer wg.Done()
			podmanInfo, podmanInfoErr = pm.Info(ctx)
		}()

		// Podman Machine check (macOS/Windows only)
		if runtime.GOOS != "linux" {
			wg.Add(1)
			go func() {
				defer wg.Done()
				machineStatus = checkPodmanMachineStatus()
			}()
		}

		wg.Wait()

		if podmanInfoErr != nil {
			status.Podman = PodmanStatus{
				Available: false,
			}
		} else {
			status.Podman = PodmanStatus{
				Available: true,
				Version:   podmanInfo.Version,
				Rootless:  podmanInfo.Rootless,
			}
		}
		status.PodmanMachine = machineStatus
	}

	cfg := getConfig()

	// Check Registry (uses Inspect directly, skipping redundant Exists check)
	registryStatus := checkService(ctx, pm, cfg.Containers.RegistryName, cfg.Registry.Port)
	status.Services = append(status.Services, registryStatus)

	// Check CI and GUI (experimental only)
	if cfg.Experimental {
		ciServiceStatus := checkService(ctx, pm, cfg.Containers.CIName, cfg.CI.Port)
		status.Services = append(status.Services, ciServiceStatus)

		guiStatus := checkService(ctx, pm, cfg.Containers.GUIName, cfg.GUI.Port)
		status.Services = append(status.Services, guiStatus)
	}

	// Check VMs
	vmStatuses := checkVMs()
	status.VMs = vmStatuses

	// Check CI Tools (batch: single podman call instead of per-tool)
	status.CITools = checkCITools(ctx, pm)

	// Output
	if jsonOut {
		return outputJSON(status)
	}
	return outputTable(status)
}

// checkPodmanMachineStatus checks the status of Podman Machine (macOS/Windows)
func checkPodmanMachineStatus() *PodmanMachineStatus {
	running, name := checkPodmanMachineRunning()
	if !running {
		return &PodmanMachineStatus{
			Running: false,
		}
	}

	machineStatus := &PodmanMachineStatus{
		Running: true,
		Name:    strings.TrimSuffix(name, "*"),
	}

	// Get machine info
	info, err := getPodmanMachineInfo()
	if err == nil {
		machineStatus.CPUs = info["cpus"]
		machineStatus.Memory = info["memory"]
		machineStatus.Disk = info["disk"]
		machineStatus.Rootful = info["rootful"]
	}

	return machineStatus
}

// checkCITools checks the status of all CI tools
// Uses a single podman images call to batch-check all container tool images
func checkCITools(ctx context.Context, pm *podman.Client) []CIToolStatus {
	var tools []CIToolStatus

	// Batch: get all local images once with a single podman call,
	// then check each tool image in memory (instead of 6 separate subprocess calls)
	localImages := getLocalImageSet(ctx, pm)

	// Container-based tools
	for name, tool := range ci.CITools {
		status := "not pulled"
		if localImages[tool.Image] {
			status = "pulled"
		}
		tools = append(tools, CIToolStatus{
			Name:       name,
			Status:     status,
			Image:      tool.Image,
			Privileged: tool.Privileged,
		})
	}

	// Platform-specific VM tools
	switch runtime.GOOS {
	case "darwin":
		// vfkit
		vfkitStatus := CIToolStatus{Name: config.BinaryVfkit, Status: "not found"}
		if err := ci.CheckVfkitAvailable(); err == nil {
			vfkitStatus.Status = "installed"
			if version, err := ci.GetVfkitVersion(); err == nil {
				vfkitStatus.Version = version
			}
		}
		tools = append(tools, vfkitStatus)

		// gvproxy
		gvproxyStatus := CIToolStatus{Name: config.BinaryGvproxy, Status: "not found"}
		if err := ci.CheckGvproxyAvailable(); err == nil {
			gvproxyStatus.Status = "installed"
		}
		tools = append(tools, gvproxyStatus)

	case "linux":
		// QEMU/KVM (check qemu-kvm in /usr/libexec/ for RHEL/Fedora)
		qemuStatus := CIToolStatus{Name: "qemu-kvm", Status: "not found"}
		if _, err := exec.LookPath("qemu-kvm"); err == nil {
			qemuStatus.Status = "installed"
		} else if _, err := os.Stat("/usr/libexec/qemu-kvm"); err == nil {
			qemuStatus.Status = "installed"
		}
		tools = append(tools, qemuStatus)

		// gvproxy
		gvproxyStatus := CIToolStatus{Name: config.BinaryGvproxy, Status: "not found"}
		if err := ci.CheckGvproxyAvailable(); err == nil {
			gvproxyStatus.Status = "installed"
		}
		tools = append(tools, gvproxyStatus)
	}

	return tools
}

// getLocalImageSet returns a set of locally available image names.
// Uses a single "podman images" call instead of per-image "podman image exists".
func getLocalImageSet(ctx context.Context, pm *podman.Client) map[string]bool {
	imageSet := make(map[string]bool)
	if pm == nil {
		return imageSet
	}

	images, err := pm.Images(ctx, false)
	if err != nil {
		return imageSet
	}

	for _, img := range images {
		for _, name := range img.Names {
			imageSet[name] = true
		}
	}
	return imageSet
}

func checkVMs() []VMStatus {
	var vmStatuses []VMStatus

	// List all VMs from global directory
	vmInfos, err := vm.ListVMInfos()
	if err != nil {
		return vmStatuses
	}

	for _, info := range vmInfos {
		vs := VMStatus{
			Name:     info.Name,
			Pipeline: info.PipelineName,
			SSHHost:  info.SSHHost,
			SSHPort:  info.SSHPort,
			SSHUser:  info.SSHUser,
		}

		// Check actual VM state by verifying if vfkit process is running
		if info.VfkitPID > 0 {
			process, err := os.FindProcess(info.VfkitPID)
			if err == nil {
				// Try to send signal 0 to check if process exists (no-op signal)
				if err := process.Signal(os.Signal(syscall.Signal(0))); err == nil {
					vs.State = "running"
				} else {
					vs.State = "stopped"
				}
			} else {
				vs.State = "stopped"
			}
		} else {
			vs.State = "stopped"
		}

		vmStatuses = append(vmStatuses, vs)
	}

	return vmStatuses
}

// checkService checks a service container status using a single Inspect call.
// Previously used Exists() + Inspect() (2 subprocess calls); now uses only Inspect().
// If the container doesn't exist, Inspect returns an error which we handle as "not created".
func checkService(ctx context.Context, pm *podman.Client, name string, port int) ServiceStatus {
	ss := ServiceStatus{
		Name: name,
		Port: port,
	}

	if pm == nil {
		ss.Status = "unknown"
		ss.Message = "podman not available"
		return ss
	}

	// Single Inspect call replaces Exists + Inspect (saves one subprocess call per service)
	info, err := pm.Inspect(ctx, name)
	if err != nil {
		// Inspect failure means container doesn't exist or other error
		ss.Status = "not created"
		return ss
	}

	if info.State.Running {
		ss.Status = "running"
	} else {
		ss.Status = "stopped"
	}

	return ss
}

func outputJSON(status OverallStatus) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(status)
}

func outputTable(status OverallStatus) error {
	// Platform
	fmt.Printf("Platform: %s\n", status.Platform)

	// Podman status
	fmt.Println("\nPodman:")
	if status.Podman.Available {
		mode := "root"
		if status.Podman.Rootless {
			mode = "rootless"
		}
		fmt.Printf("  ✅ Version: %s (%s)\n", status.Podman.Version, mode)
	} else {
		fmt.Println("  ❌ not available")
	}

	// Podman Machine (macOS/Windows)
	if status.PodmanMachine != nil {
		fmt.Println("\nPodman Machine:")
		if status.PodmanMachine.Running {
			fmt.Printf("  ✅ Status: running (%s)\n", status.PodmanMachine.Name)
			if verbose {
				// Verbose: show each setting on its own line
				if status.PodmanMachine.CPUs != "" {
					fmt.Printf("  • CPUs: %s\n", status.PodmanMachine.CPUs)
					fmt.Printf("  • Memory: %s\n", status.PodmanMachine.Memory)
					fmt.Printf("  • Disk: %s\n", status.PodmanMachine.Disk)
					fmt.Printf("  • Rootful: %s\n", status.PodmanMachine.Rootful)
				}
				// Show recommended settings
				fmt.Println()
				fmt.Println("  📋 Recommended settings for bootc CI:")
				rec := ci.RecommendedMachineConfig()
				min := ci.MinimumMachineConfig()
				fmt.Printf("    • CPUs: %d (minimum: %d)\n", rec.CPUs, min.CPUs)
				fmt.Printf("    • Memory: %d MB (minimum: %d MB)\n", rec.Memory, min.Memory)
				fmt.Printf("    • Disk: %d GB (minimum: %d GB)\n", rec.Disk, min.Disk)
				fmt.Printf("    • Rootful: %v (required for bootc-image-builder)\n", rec.Rootful)
			} else if status.PodmanMachine.CPUs != "" {
				fmt.Printf("  • CPUs: %s, Memory: %s, Disk: %s, Rootful: %s\n",
					status.PodmanMachine.CPUs,
					status.PodmanMachine.Memory,
					status.PodmanMachine.Disk,
					status.PodmanMachine.Rootful)
			}
		} else {
			fmt.Println("  ❌ not running")
			fmt.Println("  Run: podman machine start")
		}
	}

	// Services
	fmt.Println("\nServices:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  NAME\tSTATUS\tPORT")
	for _, s := range status.Services {
		statusIcon := "⚪"
		switch s.Status {
		case "running":
			statusIcon = "✅"
		case "stopped":
			statusIcon = "⏸️"
		case "not created":
			statusIcon = "⚪"
		case "error":
			statusIcon = "❌"
		}
		portStr := "-"
		if s.Port > 0 {
			portStr = fmt.Sprintf("%d", s.Port)
		}
		fmt.Fprintf(w, "  %s %s\t%s\t%s\n", statusIcon, s.Name, s.Status, portStr)
	}
	w.Flush()

	// VMs
	fmt.Println("\nVMs:")
	if len(status.VMs) == 0 {
		fmt.Println("  No VMs found")
	} else {
		w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  NAME\tSTATE\tPIPELINE\tSSH")
		for _, v := range status.VMs {
			stateIcon := "⚪"
			if v.State == "running" {
				stateIcon = "✅"
			}
			pipeline := v.Pipeline
			if pipeline == "" {
				pipeline = "-"
			}
			sshInfo := "-"
			if v.SSHHost != "" && v.SSHPort > 0 {
				sshInfo = fmt.Sprintf("%s@%s:%d", v.SSHUser, v.SSHHost, v.SSHPort)
			}
			fmt.Fprintf(w, "  %s %s\t%s\t%s\t%s\n", stateIcon, v.Name, v.State, pipeline, sshInfo)
		}
		w.Flush()
	}

	// CI Tools
	if verbose {
		outputCIToolsVerbose(status.CITools)
	} else {
		outputCIToolsCompact(status.CITools)
	}

	return nil
}

// outputCIToolsCompact outputs CI tools in compact format
func outputCIToolsCompact(tools []CIToolStatus) {
	fmt.Println("\nCI Tools:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, tool := range tools {
		statusIcon := "⚪"
		statusText := tool.Status
		switch tool.Status {
		case "pulled", "installed":
			statusIcon = "✅"
		case "not pulled", "not found":
			statusIcon = "⚪"
		}
		extra := ""
		if tool.Version != "" {
			extra = fmt.Sprintf(" (%s)", tool.Version)
		}
		if tool.Privileged {
			extra += " [privileged]"
		}
		fmt.Fprintf(w, "  %s %s\t%s%s\n", statusIcon, tool.Name, statusText, extra)
	}
	w.Flush()
}

// outputCIToolsVerbose outputs CI tools grouped by stage with image names
func outputCIToolsVerbose(tools []CIToolStatus) {
	fmt.Println("\nCI Tools by Stage:")

	// Create lookup map
	toolMap := make(map[string]CIToolStatus)
	for _, t := range tools {
		toolMap[t.Name] = t
	}

	// Stage 1: Validate
	fmt.Println("\n  Stage 1: Validate")
	printToolVerbose(toolMap, "hadolint")

	// Stage 2: Build
	fmt.Println("\n  Stage 2: Build")
	fmt.Println("    • podman build (native)")

	// Stage 3: Scan
	fmt.Println("\n  Stage 3: Scan")
	printToolVerbose(toolMap, "trivy")
	printToolVerbose(toolMap, "syft")

	// Stage 4: Convert
	fmt.Println("\n  Stage 4: Convert")
	printToolVerbose(toolMap, "bootc-image-builder")

	// Stage 5: Test
	switch runtime.GOOS {
	case "darwin":
		fmt.Println("\n  Stage 5: Test (macOS - vfkit)")
		printToolVerbose(toolMap, config.BinaryVfkit)
		printToolVerbose(toolMap, config.BinaryGvproxy)
	case "linux":
		fmt.Println("\n  Stage 5: Test (Linux - QEMU/KVM)")
		printToolVerbose(toolMap, "qemu-kvm")
		printToolVerbose(toolMap, config.BinaryGvproxy)
	default:
		fmt.Println("\n  Stage 5: Test (unsupported platform)")
	}

	// Stage 6: Release
	fmt.Println("\n  Stage 6: Release")
	printToolVerbose(toolMap, "cosign")
	printToolVerbose(toolMap, "skopeo")
	fmt.Println("    • podman push (native)")
}

// printToolVerbose prints a single tool with verbose info
func printToolVerbose(toolMap map[string]CIToolStatus, name string) {
	tool, ok := toolMap[name]
	if !ok {
		fmt.Printf("    • %s: ⚪ not available\n", name)
		return
	}

	statusIcon := "⚪"
	switch tool.Status {
	case "pulled", "installed":
		statusIcon = "✅"
	}

	extra := ""
	if tool.Version != "" {
		extra = tool.Version
	} else if tool.Image != "" {
		extra = tool.Image
	}
	if tool.Privileged {
		extra += " [privileged]"
	}

	fmt.Printf("    • %s: %s %s %s\n", name, statusIcon, tool.Status, extra)
}
