package vm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tnk4on/bootc-man/internal/config"
)

// VMInfo represents information about a VM
type VMInfo struct {
	Name         string    `json:"name"`         // VM名（<pipeline-name> または --nameで指定）
	PipelineName string    `json:"pipelineName"` // パイプライン名
	PipelineFile string    `json:"pipelineFile"` // パイプラインファイルのパス
	ImageTag     string    `json:"imageTag"`     // イメージタグ（build stageの成果物）
	DiskImage    string    `json:"diskImage"`    // ディスクイメージパス（convert stageの成果物）
	Created      time.Time `json:"created"`      // 作成日時
	SSHHost      string    `json:"sshHost"`      // SSH接続先ホスト（通常はlocalhost）
	SSHPort      int       `json:"sshPort"`      // SSH接続先ポート
	SSHUser      string    `json:"sshUser"`      // SSHユーザー名（通常は"user"）
	SSHKeyPath   string    `json:"sshKeyPath"`   // SSH秘密鍵パス
	LogFile      string    `json:"logFile"`      // シリアルコンソールログファイル
	State        string    `json:"state"`        // VM状態（Running, Stopped等）

	// Platform-specific fields
	VMType    string `json:"vmType"`    // VM種別（qemu, vfkit, hyperv）
	ProcessID int    `json:"processId"` // メインVMプロセスID

	// gvproxy related - used for all platforms
	GvproxySocket        string `json:"gvproxySocket,omitempty"`        // gvproxyソケットパス
	GvproxyServiceSocket string `json:"gvproxyServiceSocket,omitempty"` // gvproxy HTTP APIソケットパス
	GvproxyPID           int    `json:"gvproxyPid,omitempty"`           // gvproxyプロセスID

	// macOS (vfkit) specific - optional
	VfkitEndpoint string `json:"vfkitEndpoint,omitempty"` // vfkit RESTful endpoint
	VfkitPID      int    `json:"vfkitPid,omitempty"`      // vfkitプロセスID (deprecated, use ProcessID)

	// Linux (QEMU) specific - optional
	PIDFile string `json:"pidFile,omitempty"` // QEMUのPIDファイルパス
}

// PrerequisitesCheckResult represents the result of prerequisite checking
type PrerequisitesCheckResult struct {
	BuildCompleted   bool
	ConvertCompleted bool
	ImageTag         string
	DiskImagePath    string
	Errors           []string
}

// CheckPrerequisites checks if build and convert stages have been completed
// baseDir is the pipeline base directory (where bootc-ci.yaml is located)
func CheckPrerequisites(ctx context.Context, baseDir string, imageTag string) (*PrerequisitesCheckResult, error) {
	result := &PrerequisitesCheckResult{
		ImageTag: imageTag,
		Errors:   []string{},
	}

	// Check if image exists (build stage completed)
	// Check if image exists using podman image exists
	cmd := exec.CommandContext(ctx, "podman", "image", "exists", imageTag)
	if err := cmd.Run(); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Build stage not completed: image %s does not exist", imageTag))
		result.Errors = append(result.Errors, "  Run: bootc-man ci run --stage build")
	} else {
		result.BuildCompleted = true
	}

	// Check if disk image exists (convert stage completed)
	diskImagePath, err := FindDiskImageFile(baseDir, imageTag)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Convert stage not completed: %v", err))
		result.Errors = append(result.Errors, "  Run: bootc-man ci run --stage convert")
	} else {
		result.ConvertCompleted = true
		result.DiskImagePath = diskImagePath
	}

	return result, nil
}

// FindDiskImageFile finds the disk image file from convert stage artifacts
// Prefers raw format (for vfkit), falls back to qcow2
// baseDir is the pipeline base directory (where bootc-ci.yaml is located)
func FindDiskImageFile(baseDir string, imageTag string) (string, error) {
	artifactsDir := filepath.Join(baseDir, "output", "images")

	// Generate expected filename from image tag
	imageName := strings.ReplaceAll(imageTag, "/", "_")
	imageName = strings.ReplaceAll(imageName, ":", "_")

	// First, try to find raw file (preferred for vfkit)
	expectedRawFile := filepath.Join(artifactsDir, fmt.Sprintf("%s.raw", imageName))
	if _, err := os.Stat(expectedRawFile); err == nil {
		return expectedRawFile, nil
	}

	// Also check in image/ subdirectory (some convert outputs use this structure)
	expectedRawFileInImage := filepath.Join(artifactsDir, "image", "disk.raw")
	if _, err := os.Stat(expectedRawFileInImage); err == nil {
		return expectedRawFileInImage, nil
	}

	// Search recursively for raw files
	var foundRawFile string
	err := filepath.Walk(artifactsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".raw") {
			foundRawFile = path
			return filepath.SkipAll
		}
		return nil
	})

	if err == nil && foundRawFile != "" {
		return foundRawFile, nil
	}

	// Fallback to qcow2
	expectedQcow2File := filepath.Join(artifactsDir, fmt.Sprintf("%s.qcow2", imageName))
	if _, err := os.Stat(expectedQcow2File); err == nil {
		return expectedQcow2File, nil
	}

	// Search recursively for qcow2 files
	var foundQcow2File string
	err = filepath.Walk(artifactsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".qcow2") {
			foundQcow2File = path
			return filepath.SkipAll
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to search for disk image file: %w", err)
	}

	if foundQcow2File != "" {
		return foundQcow2File, nil
	}

	return "", fmt.Errorf("no disk image file (raw or qcow2) found in %s", artifactsDir)
}

// GetVMsDir returns the global VMs directory path
// On macOS/Linux: ~/.local/share/bootc-man/vms/
// On Windows: %APPDATA%/bootc-man/vms/
func GetVMsDir() (string, error) {
	var baseDir string
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable not set")
		}
		baseDir = filepath.Join(appData, "bootc-man")
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".local", "share", "bootc-man")
	}
	return filepath.Join(baseDir, "vms"), nil
}

// SaveVMInfo saves VM information to a JSON file in global VMs directory
func SaveVMInfo(vmInfo *VMInfo) error {
	vmsDir, err := GetVMsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(vmsDir, 0755); err != nil {
		return fmt.Errorf("failed to create vms directory: %w", err)
	}

	vmInfoFile := filepath.Join(vmsDir, fmt.Sprintf("%s.json", vmInfo.Name))
	data, err := json.MarshalIndent(vmInfo, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal VM info: %w", err)
	}

	if err := os.WriteFile(vmInfoFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write VM info file: %w", err)
	}

	return nil
}

// LoadVMInfo loads VM information from a JSON file in global VMs directory
func LoadVMInfo(name string) (*VMInfo, error) {
	vmsDir, err := GetVMsDir()
	if err != nil {
		return nil, err
	}
	vmInfoFile := filepath.Join(vmsDir, fmt.Sprintf("%s.json", name))

	data, err := os.ReadFile(vmInfoFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("VM '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to read VM info file: %w", err)
	}

	var vmInfo VMInfo
	if err := json.Unmarshal(data, &vmInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal VM info: %w", err)
	}

	return &vmInfo, nil
}

// ListVMInfos lists all VM information files from global VMs directory
func ListVMInfos() ([]*VMInfo, error) {
	vmsDir, err := GetVMsDir()
	if err != nil {
		return nil, err
	}

	// Check if directory exists
	if _, err := os.Stat(vmsDir); os.IsNotExist(err) {
		return []*VMInfo{}, nil
	}

	var vmInfos []*VMInfo
	err = filepath.Walk(vmsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".json") {
			// Extract VM name from filename
			vmName := strings.TrimSuffix(info.Name(), ".json")
			vmInfo, err := LoadVMInfo(vmName)
			if err != nil {
				// Skip corrupted files
				return nil
			}
			vmInfos = append(vmInfos, vmInfo)
		}
		return nil
	})

	return vmInfos, err
}

// DeleteVMInfo deletes VM information file from global VMs directory
func DeleteVMInfo(name string) error {
	vmsDir, err := GetVMsDir()
	if err != nil {
		return err
	}
	vmInfoFile := filepath.Join(vmsDir, fmt.Sprintf("%s.json", name))

	if err := os.Remove(vmInfoFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("VM '%s' not found", name)
		}
		return fmt.Errorf("failed to delete VM info file: %w", err)
	}

	return nil
}

// SanitizeVMName sanitizes a VM name to meet requirements
func SanitizeVMName(name string) string {
	maxLen := 30
	if len(name) > maxLen {
		name = name[:maxLen]
	}

	// Remove invalid characters (alphanumeric and hyphens only)
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}

	return result.String()
}

// GenerateImageTag generates an image tag from pipeline name
// customTag is an optional custom image tag (if empty, generates from pipelineName)
func GenerateImageTag(pipelineName string, customTag string) string {
	// Use custom tag if specified
	if customTag != "" {
		return customTag
	}
	// Otherwise, generate from pipeline name
	name := strings.ToLower(pipelineName)
	name = strings.ReplaceAll(name, " ", "-")
	return fmt.Sprintf("localhost/bootc-man-%s:latest", name)
}

// CheckVfkitAvailable checks if vfkit is available (required for macOS)
func CheckVfkitAvailable() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("vfkit is only available on macOS")
	}
	if _, err := exec.LookPath(config.BinaryVfkit); err != nil {
		return fmt.Errorf("vfkit is not installed. Install it from https://github.com/crc-org/vfkit")
	}
	return nil
}
