# bootc-man

**bootc-man** (bootc manager) is a CLI tool that makes it easy to build, test, and verify [bootable containers](https://bootc-dev.github.io/bootc/) for *Image Mode* deployments.

## Features

- **Easy Local Registry Setup** — Start a local OCI registry with a single command to store and serve bootc images
- **CI Pipeline** — Define and run multi-stage image build, scan, convert, and test pipelines via YAML
- **Dry-Run & Verbose Mode** — Preview actual Podman commands before execution (see [Dry-Run & Verbose Mode](#dry-run--verbose-mode))
- **VM Boot Testing** — Convert bootc images to disk images, boot them in VMs, and verify via SSH (optional GUI console)
- **Remote bootc Operations** — Easy wrappers for bootc status, upgrade, switch, and rollback on running VMs
- **Single Binary** — All CI tools run as containers with no additional installation beyond Podman

## ⚠️ Disclaimer

**This tool is intended for development, verification, and testing purposes only. Do not use it in production environments.**

## Requirements

- **Podman** — Container build and execution
  - **macOS**: [Podman Desktop](https://podman-desktop.io/) with Podman Machine (see [Podman Machine Setup](#podman-machine-setup-macos) below)
  - **Linux**: Native Podman
  - **Windows**: Not yet supported
- **Go 1.24+** — Build-time only
- **VM hypervisor** — For CI test stage and `vm start`
  - **macOS**: [vfkit](https://github.com/crc-org/vfkit) v0.6.1+, [gvproxy](https://github.com/containers/gvisor-tap-vsock) v0.8.3+ (installed via `brew install bootc-man`)
  - **Linux**: QEMU/KVM, [gvproxy](https://github.com/containers/gvisor-tap-vsock) v0.8.3+ (`sudo dnf install gvisor-tap-vsock`)

> **Note:** The convert stage requires rootful Podman (Podman Machine on macOS). All other stages run in rootless mode.

### Disk Space (CI test stage)

| Path | Recommended | Usage |
|------|-------------|-------|
| `~/.local/share/bootc-man/` | 20 GB+ | Temporary disk image copies |
| `/var/tmp/` | 1 GB+ | Sockets, PID files, logs |

> **Note:** On Fedora/RHEL, `/tmp` is typically tmpfs (RAM-backed). bootc-man places large temporary files in `~/.local/share/bootc-man/tmp/` and small runtime files in `/var/tmp/bootc-man/` to avoid running out of space.

### Podman Machine Setup (macOS)

On macOS, a Podman Machine with rootful mode is required for CI stages that run privileged containers (e.g., `convert`).

```bash
podman machine init --cpus 4 --memory 8192 --disk-size 100
podman machine set --rootful
podman machine start
```

| Setting | Recommended | Minimum |
|---------|-------------|---------|
| CPUs | 4 | 2 |
| Memory | 8192 MB | 4096 MB |
| Disk | 100 GB | 50 GB |
| Rootful | Enabled | Enabled |

## Installation

### RPM (Fedora / RHEL / CentOS Stream)

```bash
sudo dnf copr enable tnk4on/bootc-man
sudo dnf install bootc-man
```

### Homebrew (macOS)

```bash
brew tap tnk4on/bootc-man
brew install bootc-man
```

### Build from Source

```bash
git clone https://github.com/tnk4on/bootc-man.git
cd bootc-man
make build
sudo make install
```

## Quick Start

```bash
# Initialize configuration (optionally generates a sample pipeline)
bootc-man init

# Verify CI environment (Podman, Podman Machine, etc.)
bootc-man ci check

# Run the full CI pipeline (validate → build → scan → convert → test → release)
bootc-man ci run

# Start a VM from the converted disk image
bootc-man vm start

# Connect to the VM via SSH
bootc-man vm ssh
```

## Commands

```
bootc-man
├── init                    # Initialize configuration and samples
├── status                  # Show overall status (--json)
├── version                 # Show version information (--json)
├── config                  # Configuration management
│   ├── show               # Display current configuration
│   ├── path               # Show config file path
│   └── edit               # Edit config file in $EDITOR
├── registry               # Local OCI registry management
│   ├── up                 # Start the registry container
│   ├── down               # Stop the registry container
│   ├── status             # Show registry status
│   ├── logs               # Display registry logs
│   └── rm                 # Remove registry (--force, --volumes)
├── ci                     # CI pipeline management
│   ├── check              # Validate pipeline and environment
│   ├── run [pipeline]     # Run pipeline stages
│   └── keygen             # Generate cosign key pair
├── container              # Container image helpers
│   ├── build              # Build a container image
│   ├── run                # Launch an interactive shell
│   └── image              # Image operations (list, rm, inspect)
├── vm                     # Virtual machine management
│   ├── start              # Start a VM
│   ├── list               # List VMs (--json)
│   ├── status             # Show VM status
│   ├── stop               # Stop a VM
│   ├── rm                 # Remove a VM (--force)
│   └── ssh                # Connect to a VM via SSH
├── remote                 # Remote bootc operations (via SSH)
│   ├── status             # Show bootc status
│   ├── upgrade            # Upgrade the booted image
│   ├── switch             # Switch to a different image
│   └── rollback           # Rollback to the previous image
└── completion             # Generate shell completions
    └── [bash|zsh|fish|powershell]
```

## Dry-Run & Verbose Mode

Most bootc-man commands accept `--dry-run` and `-v`/`--verbose` flags. These let you see the actual `podman` commands that bootc-man executes under the hood — useful for learning how bootc images and CI pipelines work.

For example, `registry up --dry-run` shows the equivalent `podman run` command:

```
$ bootc-man registry up --dry-run
📋 Equivalent command (run registry):
   podman run -d --name bootc-man-registry -p 5000:5000 \
     -v bootc-man-registry-data:/var/lib/registry docker.io/library/registry:2

(dry-run mode - command not executed)
```

The same applies to CI pipeline stages:

```bash
# Preview all CI stage commands without executing
bootc-man ci run --dry-run

# Run with verbose output to see every podman command in real time
bootc-man ci run -v
```

## VM GUI Window

On macOS and Linux desktop environments, you can display the VM console in a GUI window using the `--gui` flag. This is useful for observing the boot process and interacting with the VM directly:

```bash
# Start a VM with GUI console
bootc-man vm start --gui
```

The CI test stage also supports GUI mode via `bootc-ci.yaml`:

```yaml
test:
  boot:
    gui: true     # Show VM console window during test
```

## CI Pipeline

### Stages

| Stage | macOS | Linux | Description |
|-------|-------|-------|-------------|
| validate | ✅ | ✅ | Containerfile lint (hadolint container) |
| build | ✅ | ✅ | Container image build (`podman build`) |
| scan | ✅ | ✅ | Vulnerability scan & SBOM (trivy / syft containers) |
| convert | ✅ | ✅ | Disk image conversion (bootc-image-builder container) |
| test | ✅ (vfkit) | ✅ (QEMU/KVM) | VM boot test with SSH verification |
| release | ✅ | ✅ | Sign and push (cosign container) |

### Usage

```bash
# Run all stages
bootc-man ci run

# Run a single stage
bootc-man ci run --stage build

# Run multiple stages (comma-separated; executed in pipeline order)
bootc-man ci run --stage validate,build,scan

# Dry-run (show commands without executing)
bootc-man ci run --dry-run

# Specify a pipeline file
bootc-man ci run -f path/to/bootc-ci.yaml
```

### Pipeline Definition (`bootc-ci.yaml`)

The CI pipeline consists of 6 stages: **validate → build → scan → convert → test → release**. Pipelines are defined in YAML. `host.containers.internal` is a special hostname that resolves to the host machine from inside Podman Machine, allowing VMs to access the host's local registry.

```yaml
apiVersion: bootc-man/v1
kind: Pipeline
metadata:
  name: my-bootc-image

spec:
  source:
    containerfile: ./Containerfile
    context: .

  build:
    imageTag: host.containers.internal:5000/my-bootc:latest

  convert:
    insecureRegistries:
      - "host.containers.internal:5000"
    formats:
      - type: raw

  test:
    boot:
      enabled: true
      timeout: 60
      checks:
        - "sudo bootc status"
        - "cat /etc/os-release"
```

Run `bootc-man init` to generate a sample pipeline (Fedora, CentOS Stream, or RHEL) with a Containerfile and `bootc-ci.yaml` covering all 6 stages.

## Configuration

Configuration is loaded in the following order (later sources override earlier ones):

1. `/usr/share/bootc-man/config.yaml` — System defaults
2. `/etc/bootc-man/config.yaml` — System administrator overrides
3. `~/.config/bootc-man/config.yaml` — User settings
4. Environment variables (`BOOTCMAN_*`)
5. Command-line flags

### Example (`~/.config/bootc-man/config.yaml`)

```yaml
runtime:
  podman: auto

registry:
  port: 5000
  image: docker.io/library/registry:2

ci:
  # bootc-image-builder image (default: quay.io/centos-bootc/bootc-image-builder)
  bootc_image_builder: quay.io/centos-bootc/bootc-image-builder:latest

vm:
  ssh_user: user
  cpus: 2
  memory: 4096

ssh:
  key_path: .ssh/id_ed25519
  strict_host_key_checking: accept-new
```

## Development

```bash
make build      # Build
make test       # Run tests
make lint       # Run linter
```

## License

[Apache License 2.0](LICENSE)

## Related Links

- [bootc project](https://bootc-dev.github.io/bootc/)
- [Podman](https://podman.io/)
- [Fedora/CentOS bootc images](https://docs.fedoraproject.org/en-US/bootc/)
- [Image Mode for RHEL CI/CD Reference](https://gitlab.com/redhat/cop/rhel/rhel-image-mode-cicd)
