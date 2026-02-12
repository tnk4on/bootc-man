package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/podman"
	"github.com/tnk4on/bootc-man/internal/registry"
)

// defaultSSHPubKey is a placeholder used when no SSH key is selected or found.
// This is NOT a real key - users must replace it with their own public key.
var defaultSSHPubKey = config.DefaultSSHPublicKeyPlaceholder

// Sample distro identifiers for init prompt
const (
	sampleFedora       = "fedora"
	sampleCentOSStream = "centos-stream"
	sampleRHEL         = "rhel"
)

// escapeSSHPubKeyForShell escapes the public key for use inside double quotes in a shell script.
func escapeSSHPubKeyForShell(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "\\", "\\\\")
	key = strings.ReplaceAll(key, "\"", "\\\"")
	key = strings.ReplaceAll(key, "\n", " ")
	return key
}

// sampleContainerfile returns the Containerfile content for the given distro, with SSH key and username injected.
func sampleContainerfile(distro, sshPublicKey, username string) string {
	escapedKey := escapeSSHPubKeyForShell(sshPublicKey)
	if escapedKey == "" {
		escapedKey = escapeSSHPubKeyForShell(defaultSSHPubKey)
	}

	runBlock := fmt.Sprintf(`RUN useradd -G wheel %s && \
    mkdir -m 0700 -p /home/%s/.ssh && \
    echo "%s" > /home/%s/.ssh/authorized_keys && \
    chmod 0600 /home/%s/.ssh/authorized_keys && \
    chown -R %s:%s /home/%s && \
    echo "%s ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/%s
`, username, username, escapedKey, username, username, username, username, username, username, username)

	var header string
	switch distro {
	case sampleFedora:
		header = `# Fedora bootc base image
FROM quay.io/fedora/fedora-bootc:latest

LABEL org.opencontainers.image.title="fedora-bootc-sample"
LABEL org.opencontainers.image.description="Sample bootc image (Fedora)"
LABEL org.opencontainers.image.version="1.0.0"

`
	case sampleCentOSStream:
		header = `# CentOS Stream bootc base image
FROM quay.io/centos-bootc/centos-bootc:stream10

LABEL org.opencontainers.image.title="centos-stream-bootc-sample"
LABEL org.opencontainers.image.description="Sample bootc image (CentOS Stream)"
LABEL org.opencontainers.image.version="1.0.0"

`
	case sampleRHEL:
		header = `# RHEL 10 bootc base image (requires authentication)
FROM registry.redhat.io/rhel10/rhel-bootc

LABEL org.opencontainers.image.title="rhel10-bootc-sample"
LABEL org.opencontainers.image.description="Sample bootc image (RHEL 10)"
LABEL org.opencontainers.image.version="1.0.0"

`
	default:
		return ""
	}

	return header + runBlock + `
RUN bootc container lint
`
}

// sampleBootcCI returns the bootc-ci.yaml content for the given distro.
func sampleBootcCI(distro, imageTag, pipelineName string) string {
	return fmt.Sprintf(`apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: %s
  description: "Sample bootc CI pipeline (%s)"

spec:
  source:
    containerfile: ./Containerfile
    context: .

  validate:
    containerfileLint:
      enabled: true
      requireBootcLint: true
      warnIfMissing: true
      failIfMissing: false
    configToml:
      enabled: false
    secretDetection:
      enabled: false

  build:
    imageTag: %s

  scan:
    vulnerability:
      enabled: false
      failOnVulnerability: false
    sbom:
      enabled: true
      tool: syft
      format: spdx-json

  convert:
    enabled: true
    insecureRegistries:
      - "host.containers.internal:5000"
    formats:
      - type: raw

  test:
    boot:
      enabled: true
      timeout: 30
      gui: true
      checks:
        - "sudo bootc status"

  release:
    registry: host.containers.internal:5000
    repository: %s
    tls: false
    tags:
      - "v1.0.0"
      - "latest"
    sign:
      enabled: false
`, pipelineName, distro, imageTag, pipelineName)
}

// sshKeyEntry holds path and content of an SSH public key
type sshKeyEntry struct {
	Path    string
	Content string
}

// discoverSSHKeys returns public keys found in ~/.ssh/*.pub
func discoverSSHKeys() ([]sshKeyEntry, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshDir := filepath.Join(home, ".ssh")
	entries, err := os.ReadDir(sshDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var keys []sshKeyEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pub") {
			continue
		}
		path := filepath.Join(sshDir, e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			logrus.Debugf("Skip reading %s: %v", path, err)
			continue
		}
		keys = append(keys, sshKeyEntry{Path: path, Content: string(content)})
	}
	return keys, nil
}

// promptSSHKeySelection lists keys and lets the user select one; returns content or default.
func promptSSHKeySelection(keys []sshKeyEntry) string {
	if len(keys) == 0 {
		fmt.Println("  No SSH public keys found in ~/.ssh")
		fmt.Println("  ⚠️  Using placeholder. Edit Containerfile to add your SSH key.")
		return defaultSSHPubKey
	}

	fmt.Println("  SSH public keys in ~/.ssh:")
	for i, k := range keys {
		fmt.Printf("    %d) %s\n", i+1, k.Path)
	}
	fmt.Printf("    %d) Use default (inject your key later)\n", len(keys)+1)
	fmt.Printf("  Select key [1]: ")

	choice, err := promptLine("1")
	if err != nil {
		return defaultSSHPubKey
	}

	for i, k := range keys {
		if choice == fmt.Sprintf("%d", i+1) {
			return strings.TrimSpace(k.Content)
		}
	}
	fmt.Println("  ⚠️  Using placeholder. Edit Containerfile to add your SSH key.")
	return defaultSSHPubKey
}

// promptUsername asks for the VM login username; default is "user".
func promptUsername(defaultUser string) string {
	if defaultUser == "" {
		defaultUser = "user"
	}
	fmt.Printf("  Username for VM login [%s]: ", defaultUser)
	s, err := promptLine(defaultUser)
	if err != nil {
		return defaultUser
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return defaultUser
	}
	return s
}

// writeSample creates the sample pipeline directory and files in outputDir (current directory).
func writeSample(outputDir, distro, sshPublicKey, username string) error {
	var pipelineName, imageTag string
	switch distro {
	case sampleFedora:
		pipelineName = "fedora-bootc"
		imageTag = "localhost/fedora-bootc:latest"
	case sampleCentOSStream:
		pipelineName = "centos-stream-bootc"
		imageTag = "localhost/centos-stream-bootc:latest"
	case sampleRHEL:
		pipelineName = "rhel10-bootc"
		imageTag = "localhost/rhel10-bootc:latest"
	default:
		return fmt.Errorf("unknown distro: %s", distro)
	}

	dir := filepath.Join(outputDir, pipelineName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create sample directory: %w", err)
	}

	containerfile := sampleContainerfile(distro, sshPublicKey, username)
	if err := os.WriteFile(filepath.Join(dir, config.DefaultContainerfileName), []byte(containerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Containerfile: %w", err)
	}

	bootcCI := sampleBootcCI(distro, imageTag, pipelineName)
	if err := os.WriteFile(filepath.Join(dir, config.DefaultPipelineFileName), []byte(bootcCI), 0644); err != nil {
		return fmt.Errorf("failed to write bootc-ci.yaml: %w", err)
	}

	logrus.Debugf("Created sample pipeline: %s", dir)
	return nil
}

// isStdinTerminal returns true if stdin is a terminal (interactive).
func isStdinTerminal() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// promptLine reads a line from stdin, trimmed. Empty input returns defaultVal.
func promptLine(defaultVal string) (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		s := strings.TrimSpace(scanner.Text())
		if s == "" {
			return defaultVal, nil
		}
		return s, nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return defaultVal, nil
}

// runSamplePrompt asks the user to choose a sample (Fedora / CentOS Stream / RHEL / None) and creates it in the current directory.
func runSamplePrompt() (string, error) {
	if !isStdinTerminal() {
		logrus.Debug("stdin is not a terminal, skipping sample prompt")
		return "", nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	fmt.Println("\nCreate sample pipeline?")
	fmt.Println("  1) Fedora")
	fmt.Println("  2) CentOS Stream")
	fmt.Println("  3) RHEL")
	fmt.Print("  4) None (skip) [4]: ")

	choice, err := promptLine("4")
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	var distro string
	switch choice {
	case "1":
		distro = sampleFedora
	case "2":
		distro = sampleCentOSStream
	case "3":
		distro = sampleRHEL
	case "4", "":
		fmt.Println("  Skipping sample creation.")
		return "", nil
	default:
		fmt.Printf("  Unknown option %q, skipping.\n", choice)
		return "", nil
	}

	// Select SSH key from ~/.ssh
	keys, err := discoverSSHKeys()
	if err != nil {
		return "", fmt.Errorf("failed to discover SSH keys: %w", err)
	}
	sshPublicKey := promptSSHKeySelection(keys)

	// Username for VM login (default: user)
	username := promptUsername("user")

	if err := writeSample(cwd, distro, sshPublicKey, username); err != nil {
		return "", err
	}

	var name string
	switch distro {
	case sampleFedora:
		name = "fedora-bootc"
	case sampleCentOSStream:
		name = "centos-stream-bootc"
	case sampleRHEL:
		name = "rhel10-bootc"
	default:
		name = distro
	}
	fmt.Printf("  Created sample: %s (user: %s)\n", filepath.Join(cwd, name), username)
	return name, nil
}

// runRegistryPrompt asks whether to start the registry and starts it if yes.
func runRegistryPrompt(configPath string) error {
	if !isStdinTerminal() {
		logrus.Debug("stdin is not a terminal, skipping registry prompt")
		return nil
	}

	fmt.Print("\nStart registry now? [Y/n]: ")
	answer, err := promptLine("y")
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	answer = strings.ToLower(answer)
	if answer != "y" && answer != "yes" {
		fmt.Println("  Skipping registry start.")
		return nil
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config for registry: %w", err)
	}

	pm, err := podman.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create podman client: %w", err)
	}

	svc := registry.NewService(registry.ServiceOptions{
		Config:           &cfg.Registry,
		ContainersConfig: &cfg.Containers,
		Podman:           pm,
		Verbose:          verbose,
		DryRun:           dryRun,
	})

	fmt.Println("Starting registry service...")
	ctx := context.Background()
	result, err := svc.Up(ctx)
	if err != nil {
		return fmt.Errorf("failed to start registry: %w", err)
	}

	if result.AlreadyRunning {
		fmt.Printf("  Registry is already running on port %d\n", cfg.Registry.Port)
	} else {
		fmt.Printf("  Registry started on port %d\n", cfg.Registry.Port)
	}
	fmt.Printf("  Push images to: localhost:%d/<image>:<tag>\n", cfg.Registry.Port)
	return nil
}
