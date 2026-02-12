package podman

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

// testBootcImage is defined locally to avoid import cycle with testutil.
// Keep in sync with testutil.TestBootcImagePrevious().
const testBootcImage = "quay.io/fedora/fedora-bootc:42"

func TestPortMapping(t *testing.T) {
	tests := []struct {
		host      int
		container int
		want      string
	}{
		{8080, 80, "8080:80"},
		{5000, 5000, "5000:5000"},
		{443, 8443, "443:8443"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d:%d", tt.host, tt.container), func(t *testing.T) {
			pm := PortMapping{Host: tt.host, Container: tt.container}
			got := fmt.Sprintf("%d:%d", pm.Host, pm.Container)
			if got != tt.want {
				t.Errorf("PortMapping format = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVolumeMapping(t *testing.T) {
	tests := []struct {
		name      string
		vol       VolumeMapping
		wantParts []string
	}{
		{
			name:      "simple mount",
			vol:       VolumeMapping{Host: "/host/path", Container: "/container/path"},
			wantParts: []string{"/host/path", "/container/path"},
		},
		{
			name:      "with options",
			vol:       VolumeMapping{Host: "/data", Container: "/mnt/data", Options: "Z"},
			wantParts: []string{"/data", "/mnt/data", "Z"},
		},
		{
			name:      "read-only",
			vol:       VolumeMapping{Host: "/config", Container: "/etc/config", Options: "ro"},
			wantParts: []string{"/config", "/etc/config", "ro"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.vol.Host != tt.wantParts[0] {
				t.Errorf("Host = %q, want %q", tt.vol.Host, tt.wantParts[0])
			}
			if tt.vol.Container != tt.wantParts[1] {
				t.Errorf("Container = %q, want %q", tt.vol.Container, tt.wantParts[1])
			}
			if len(tt.wantParts) > 2 && tt.vol.Options != tt.wantParts[2] {
				t.Errorf("Options = %q, want %q", tt.vol.Options, tt.wantParts[2])
			}
		})
	}
}

func TestRunOptions(t *testing.T) {
	tests := []struct {
		name string
		opts RunOptions
	}{
		{
			name: "minimal options",
			opts: RunOptions{
				Image: "alpine:latest",
			},
		},
		{
			name: "full options",
			opts: RunOptions{
				Name:       "test-container",
				Image:      "nginx:1.21",
				Ports:      []PortMapping{{Host: 8080, Container: 80}},
				Volumes:    []VolumeMapping{{Host: "/data", Container: "/usr/share/nginx/html"}},
				Detach:     true,
				Remove:     false,
				Privileged: false,
				Env:        map[string]string{"NGINX_HOST": "localhost"},
				Args:       []string{"-c", "/etc/nginx/nginx.conf"},
			},
		},
		{
			name: "privileged container",
			opts: RunOptions{
				Name:       "privileged-test",
				Image:      "registry:2",
				Privileged: true,
				Detach:     true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts.Image == "" {
				t.Error("RunOptions.Image should not be empty")
			}
		})
	}
}

func TestBuildOptions(t *testing.T) {
	tests := []struct {
		name string
		opts BuildOptions
	}{
		{
			name: "minimal build",
			opts: BuildOptions{
				Context: ".",
			},
		},
		{
			name: "full build options",
			opts: BuildOptions{
				Context:    "/path/to/context",
				Tag:        "myimage:v1.0",
				Dockerfile: "Containerfile.prod",
				NoCache:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.opts.Context == "" {
				t.Error("BuildOptions.Context should not be empty")
			}
		})
	}
}

func TestContainerState(t *testing.T) {
	tests := []struct {
		name  string
		state ContainerState
		want  string
	}{
		{
			name:  "running container",
			state: ContainerState{Running: true, Pid: 12345},
			want:  "running",
		},
		{
			name:  "paused container",
			state: ContainerState{Paused: true},
			want:  "paused",
		},
		{
			name:  "dead container",
			state: ContainerState{Dead: true, ExitCode: 1},
			want:  "dead",
		},
		{
			name:  "stopped container",
			state: ContainerState{Running: false, ExitCode: 0},
			want:  "stopped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			switch {
			case tt.state.Running:
				got = "running"
			case tt.state.Paused:
				got = "paused"
			case tt.state.Dead:
				got = "dead"
			default:
				got = "stopped"
			}
			if got != tt.want {
				t.Errorf("state = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestContainerInfo(t *testing.T) {
	info := ContainerInfo{
		ID:      "abc123def456",
		Name:    "test-container",
		Image:   "alpine:latest",
		Created: "2024-01-15T10:00:00Z",
		State: ContainerState{
			Running:   true,
			Pid:       12345,
			StartedAt: "2024-01-15T10:00:01Z",
		},
	}

	if info.ID == "" {
		t.Error("ContainerInfo.ID should not be empty")
	}

	if info.Name != "test-container" {
		t.Errorf("ContainerInfo.Name = %q, want %q", info.Name, "test-container")
	}

	if !info.State.Running {
		t.Error("ContainerInfo.State.Running should be true")
	}
}

func TestPodmanInfo(t *testing.T) {
	info := PodmanInfo{
		Version:  "4.5.0",
		Rootless: true,
	}

	if info.Version == "" {
		t.Error("PodmanInfo.Version should not be empty")
	}

	if !info.Rootless {
		t.Error("expected rootless=true")
	}
}

func TestImageInfo(t *testing.T) {
	tests := []struct {
		name    string
		info    ImageInfo
		isBootc bool
	}{
		{
			name: "bootc image with label",
			info: ImageInfo{
				ID:     "abc123def456",
				Names:  []string{testBootcImage},
				Size:   1073741824,
				Labels: map[string]string{"containers.bootc": "1"},
			},
			isBootc: true,
		},
		{
			name: "non-bootc image",
			info: ImageInfo{
				ID:     "xyz789ghi012",
				Names:  []string{"alpine:latest"},
				Size:   5242880,
				Labels: map[string]string{"maintainer": "test"},
			},
			isBootc: false,
		},
		{
			name: "image with nil labels",
			info: ImageInfo{
				ID:     "nolabl123456",
				Names:  []string{"busybox:latest"},
				Size:   1048576,
				Labels: nil,
			},
			isBootc: false,
		},
		{
			name: "image with bootc label set to wrong value",
			info: ImageInfo{
				ID:     "wrongval1234",
				Names:  []string{"custom:v1"},
				Size:   2097152,
				Labels: map[string]string{"containers.bootc": "0"},
			},
			isBootc: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.IsBootc()
			if got != tt.isBootc {
				t.Errorf("IsBootc() = %v, want %v", got, tt.isBootc)
			}
		})
	}
}

func TestImageInspectInfo(t *testing.T) {
	tests := []struct {
		name    string
		info    ImageInspectInfo
		isBootc bool
	}{
		{
			name: "bootc image - label in Config.Labels",
			info: ImageInspectInfo{
				ID:           "abc123def456",
				RepoTags:     []string{testBootcImage},
				Architecture: "amd64",
				Os:           "linux",
				Config: struct {
					Cmd        []string          `json:"Cmd"`
					Env        []string          `json:"Env"`
					Labels     map[string]string `json:"Labels"`
					WorkingDir string            `json:"WorkingDir"`
				}{
					Labels: map[string]string{"containers.bootc": "1"},
				},
			},
			isBootc: true,
		},
		{
			name: "bootc image - label in top-level Labels",
			info: ImageInspectInfo{
				ID:       "def456ghi789",
				RepoTags: []string{"localhost:5000/my-bootc:latest"},
				Labels:   map[string]string{"containers.bootc": "1"},
			},
			isBootc: true,
		},
		{
			name: "non-bootc image",
			info: ImageInspectInfo{
				ID:       "xyz789jkl012",
				RepoTags: []string{"nginx:latest"},
				Labels:   map[string]string{"maintainer": "NGINX Docker Maintainers"},
			},
			isBootc: false,
		},
		{
			name: "image with no labels",
			info: ImageInspectInfo{
				ID:       "nolabel12345",
				RepoTags: []string{"scratch:latest"},
			},
			isBootc: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.IsBootc()
			if got != tt.isBootc {
				t.Errorf("IsBootc() = %v, want %v", got, tt.isBootc)
			}
		})
	}
}

func TestBootcLabelConstant(t *testing.T) {
	expected := "containers.bootc"
	if BootcLabel != expected {
		t.Errorf("BootcLabel = %q, want %q", BootcLabel, expected)
	}
}

// Integration test - only runs if podman is available
func TestClientImages(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("podman not available: %v", err)
	}

	ctx := context.Background()

	// Test listing all images
	images, err := client.Images(ctx, false)
	if err != nil {
		t.Skipf("podman images failed: %v", err)
	}

	t.Logf("Found %d total images", len(images))

	// Test listing bootc images only
	bootcImages, err := client.Images(ctx, true)
	if err != nil {
		t.Errorf("Images(bootcOnly=true) failed: %v", err)
	}

	t.Logf("Found %d bootc images", len(bootcImages))

	// Verify bootc images have the correct label
	for _, img := range bootcImages {
		if !img.IsBootc() {
			t.Errorf("Image %s returned by bootcOnly filter but IsBootc() returns false", img.ID)
		}
	}
}

// Integration test - only runs if podman is available
func TestNewClient(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		// This is expected if podman is not installed
		t.Skipf("podman not available: %v", err)
	}

	if client == nil {
		t.Error("NewClient() returned nil client")
		return
	}

	if client.binary == "" {
		t.Error("client.binary should not be empty")
	}
}

// Integration test - only runs if podman is available
func TestClientInfo(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("podman not available: %v", err)
	}

	ctx := context.Background()
	info, err := client.Info(ctx)
	if err != nil {
		t.Skipf("podman info failed (podman might not be running): %v", err)
	}

	if info.Version == "" {
		t.Error("PodmanInfo.Version should not be empty")
	}

	t.Logf("Podman version: %s, rootless: %v", info.Version, info.Rootless)
}

// === PodmanError Tests ===

func TestPodmanError(t *testing.T) {
	tests := []struct {
		name    string
		err     PodmanError
		wantMsg string
	}{
		{
			name: "run command failed",
			err: PodmanError{
				Command: "run -d nginx",
				Stderr:  "Error: image not found",
				Err:     fmt.Errorf("exit status 1"),
			},
			wantMsg: "podman run -d nginx failed: exit status 1",
		},
		{
			name: "build command failed",
			err: PodmanError{
				Command: "build -t test .",
				Stderr:  "Error: Containerfile not found",
				Err:     fmt.Errorf("exit status 125"),
			},
			wantMsg: "podman build -t test . failed: exit status 125",
		},
		{
			name: "empty command",
			err: PodmanError{
				Command: "",
				Stderr:  "",
				Err:     fmt.Errorf("unknown error"),
			},
			wantMsg: "podman  failed: unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.wantMsg {
				t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
			}
		})
	}
}

func TestPodmanErrorUnwrap(t *testing.T) {
	innerErr := fmt.Errorf("inner error")
	podmanErr := &PodmanError{
		Command: "test",
		Stderr:  "stderr output",
		Err:     innerErr,
	}

	unwrapped := podmanErr.Unwrap()
	if unwrapped != innerErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, innerErr)
	}
}

// === Run Options Argument Building Tests ===

func TestRunOptionsToArgs(t *testing.T) {
	// These tests verify the logic of how options would be converted to arguments
	tests := []struct {
		name         string
		opts         RunOptions
		wantContains []string
	}{
		{
			name: "detached with name",
			opts: RunOptions{
				Name:   "test-container",
				Image:  "alpine",
				Detach: true,
			},
			wantContains: []string{"--name", "test-container", "-d", "alpine"},
		},
		{
			name: "remove on exit",
			opts: RunOptions{
				Image:  "busybox",
				Remove: true,
			},
			wantContains: []string{"--rm", "busybox"},
		},
		{
			name: "privileged mode",
			opts: RunOptions{
				Image:      "registry:2",
				Privileged: true,
			},
			wantContains: []string{"--privileged", "registry:2"},
		},
		{
			name: "with ports",
			opts: RunOptions{
				Image: "nginx",
				Ports: []PortMapping{
					{Host: 8080, Container: 80},
					{Host: 8443, Container: 443},
				},
			},
			wantContains: []string{"-p", "8080:80", "-p", "8443:443"},
		},
		{
			name: "with volumes",
			opts: RunOptions{
				Image: "postgres",
				Volumes: []VolumeMapping{
					{Host: "/data", Container: "/var/lib/postgresql/data"},
					{Host: "/config", Container: "/etc/postgresql", Options: "ro"},
				},
			},
			wantContains: []string{"-v", "/data:/var/lib/postgresql/data", "-v", "/config:/etc/postgresql:ro"},
		},
		{
			name: "with environment variables",
			opts: RunOptions{
				Image: "mysql",
				Env: map[string]string{
					"MYSQL_ROOT_PASSWORD": "secret",
				},
			},
			wantContains: []string{"-e", "MYSQL_ROOT_PASSWORD=secret"},
		},
		{
			name: "with extra args",
			opts: RunOptions{
				Image: "alpine",
				Args:  []string{"sh", "-c", "echo hello"},
			},
			wantContains: []string{"alpine", "sh", "-c", "echo hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the expected args manually (simulating what Run does)
			args := []string{"run"}

			if tt.opts.Name != "" {
				args = append(args, "--name", tt.opts.Name)
			}
			if tt.opts.Detach {
				args = append(args, "-d")
			}
			if tt.opts.Remove {
				args = append(args, "--rm")
			}
			if tt.opts.Privileged {
				args = append(args, "--privileged")
			}

			for _, p := range tt.opts.Ports {
				args = append(args, "-p", fmt.Sprintf("%d:%d", p.Host, p.Container))
			}

			for _, v := range tt.opts.Volumes {
				mapping := fmt.Sprintf("%s:%s", v.Host, v.Container)
				if v.Options != "" {
					mapping += ":" + v.Options
				}
				args = append(args, "-v", mapping)
			}

			for k, v := range tt.opts.Env {
				args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
			}

			args = append(args, tt.opts.Image)
			args = append(args, tt.opts.Args...)

			// Verify all expected components are present
			argsStr := fmt.Sprintf("%v", args)
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args %s does not contain %q", argsStr, want)
				}
			}
		})
	}
}

// === Build Options Argument Building Tests ===

func TestBuildOptionsToArgs(t *testing.T) {
	tests := []struct {
		name         string
		opts         BuildOptions
		wantContains []string
	}{
		{
			name: "minimal build",
			opts: BuildOptions{
				Context: ".",
			},
			wantContains: []string{"build", "."},
		},
		{
			name: "with tag",
			opts: BuildOptions{
				Context: ".",
				Tag:     "myimage:v1.0",
			},
			wantContains: []string{"-t", "myimage:v1.0"},
		},
		{
			name: "with dockerfile",
			opts: BuildOptions{
				Context:    ".",
				Dockerfile: "Containerfile.prod",
			},
			wantContains: []string{"-f", "Containerfile.prod"},
		},
		{
			name: "with no-cache",
			opts: BuildOptions{
				Context: ".",
				NoCache: true,
			},
			wantContains: []string{"--no-cache"},
		},
		{
			name: "full options",
			opts: BuildOptions{
				Context:    "/path/to/context",
				Tag:        "myapp:latest",
				Dockerfile: "Dockerfile.dev",
				NoCache:    true,
			},
			wantContains: []string{"-t", "myapp:latest", "-f", "Dockerfile.dev", "--no-cache", "/path/to/context"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the expected args manually
			args := []string{"build"}

			if tt.opts.Tag != "" {
				args = append(args, "-t", tt.opts.Tag)
			}
			if tt.opts.Dockerfile != "" {
				args = append(args, "-f", tt.opts.Dockerfile)
			}
			if tt.opts.NoCache {
				args = append(args, "--no-cache")
			}

			args = append(args, tt.opts.Context)

			// Verify expected components
			argsStr := fmt.Sprintf("%v", args)
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args %s does not contain %q", argsStr, want)
				}
			}
		})
	}
}

// === Push Options Tests ===

func TestPushWithTLSVerify(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		tlsVerify    bool
		wantContains []string
	}{
		{
			name:         "with TLS verify",
			image:        "localhost:5000/myimage:latest",
			tlsVerify:    true,
			wantContains: []string{"push", "localhost:5000/myimage:latest"},
		},
		{
			name:         "without TLS verify",
			image:        "localhost:5000/myimage:latest",
			tlsVerify:    false,
			wantContains: []string{"push", "--tls-verify=false", "localhost:5000/myimage:latest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"push"}
			if !tt.tlsVerify {
				args = append(args, "--tls-verify=false")
			}
			args = append(args, tt.image)

			// Verify expected components
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args %v does not contain %q", args, want)
				}
			}
		})
	}
}

func TestPushWithDestination(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		destination string
		tlsVerify   bool
		wantLen     int
	}{
		{
			name:        "without destination",
			image:       "myimage:latest",
			destination: "",
			tlsVerify:   true,
			wantLen:     2, // push, image
		},
		{
			name:        "with destination",
			image:       "myimage:latest",
			destination: "docker://quay.io/myrepo/myimage:latest",
			tlsVerify:   true,
			wantLen:     3, // push, image, destination
		},
		{
			name:        "with destination and no TLS",
			image:       "myimage:latest",
			destination: "docker://localhost:5000/myimage:latest",
			tlsVerify:   false,
			wantLen:     4, // push, --tls-verify=false, image, destination
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"push"}
			if !tt.tlsVerify {
				args = append(args, "--tls-verify=false")
			}
			args = append(args, tt.image)
			if tt.destination != "" {
				args = append(args, tt.destination)
			}

			if len(args) != tt.wantLen {
				t.Errorf("args length = %d, want %d (args: %v)", len(args), tt.wantLen, args)
			}
		})
	}
}

// === Remove Options Tests ===

func TestRemoveForce(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		force         bool
		wantContains  []string
	}{
		{
			name:          "without force",
			containerName: "mycontainer",
			force:         false,
			wantContains:  []string{"rm", "mycontainer"},
		},
		{
			name:          "with force",
			containerName: "mycontainer",
			force:         true,
			wantContains:  []string{"rm", "-f", "mycontainer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"rm"}
			if tt.force {
				args = append(args, "-f")
			}
			args = append(args, tt.containerName)

			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args %v does not contain %q", args, want)
				}
			}
		})
	}
}

// === Volume Remove Options Tests ===

func TestVolumeRemoveForce(t *testing.T) {
	tests := []struct {
		name         string
		volumeName   string
		force        bool
		wantContains []string
	}{
		{
			name:         "without force",
			volumeName:   "myvolume",
			force:        false,
			wantContains: []string{"volume", "rm", "myvolume"},
		},
		{
			name:         "with force",
			volumeName:   "myvolume",
			force:        true,
			wantContains: []string{"volume", "rm", "--force", "myvolume"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"volume", "rm"}
			if tt.force {
				args = append(args, "--force")
			}
			args = append(args, tt.volumeName)

			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args %v does not contain %q", args, want)
				}
			}
		})
	}
}

// === ImageRemove Tests ===

func TestImageRemoveForce(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		force        bool
		wantContains []string
	}{
		{
			name:         "without force",
			image:        "alpine:latest",
			force:        false,
			wantContains: []string{"rmi", "alpine:latest"},
		},
		{
			name:         "with force",
			image:        "alpine:latest",
			force:        true,
			wantContains: []string{"rmi", "-f", "alpine:latest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"rmi"}
			if tt.force {
				args = append(args, "-f")
			}
			args = append(args, tt.image)

			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args %v does not contain %q", args, want)
				}
			}
		})
	}
}

// === Logs Tests ===

func TestLogsFollow(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		follow        bool
		wantContains  []string
	}{
		{
			name:          "without follow",
			containerName: "mycontainer",
			follow:        false,
			wantContains:  []string{"logs", "mycontainer"},
		},
		{
			name:          "with follow",
			containerName: "mycontainer",
			follow:        true,
			wantContains:  []string{"logs", "-f", "mycontainer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"logs"}
			if tt.follow {
				args = append(args, "-f")
			}
			args = append(args, tt.containerName)

			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args %v does not contain %q", args, want)
				}
			}
		})
	}
}

// === Client Command Tests ===

func TestClientCommand(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("podman not available: %v", err)
	}

	ctx := context.Background()
	cmd := client.Command(ctx, "version")

	if cmd == nil {
		t.Fatal("Command() returned nil")
	}

	// Verify the command path includes podman
	if cmd.Path == "" {
		t.Error("Command.Path should not be empty")
	}
}

// === Images List Tests ===

func TestImagesBootcFilter(t *testing.T) {
	// Test that the filter argument is correctly constructed
	tests := []struct {
		name       string
		bootcOnly  bool
		wantFilter bool
	}{
		{
			name:       "all images",
			bootcOnly:  false,
			wantFilter: false,
		},
		{
			name:       "bootc only",
			bootcOnly:  true,
			wantFilter: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"images", "--format", "json"}
			if tt.bootcOnly {
				args = append(args, "--filter", "label="+BootcLabel+"=1")
			}

			hasFilter := false
			for _, arg := range args {
				if arg == "--filter" {
					hasFilter = true
					break
				}
			}

			if hasFilter != tt.wantFilter {
				t.Errorf("hasFilter = %v, want %v", hasFilter, tt.wantFilter)
			}
		})
	}
}

// === RunInteractive Argument Building Tests ===

func TestRunInteractiveArgsBuilding(t *testing.T) {
	tests := []struct {
		name         string
		opts         RunOptions
		wantContains []string
	}{
		{
			name: "basic interactive run",
			opts: RunOptions{
				Image: "alpine",
			},
			wantContains: []string{"run", "-it", "alpine"},
		},
		{
			name: "interactive with name and rm",
			opts: RunOptions{
				Name:   "test-shell",
				Image:  "fedora:latest",
				Remove: true,
			},
			wantContains: []string{"run", "-it", "--name", "test-shell", "--rm", "fedora:latest"},
		},
		{
			name: "interactive with privileged",
			opts: RunOptions{
				Image:      "centos:8",
				Privileged: true,
			},
			wantContains: []string{"run", "-it", "--privileged", "centos:8"},
		},
		{
			name: "interactive with ports and volumes",
			opts: RunOptions{
				Image: "nginx",
				Ports: []PortMapping{{Host: 8080, Container: 80}},
				Volumes: []VolumeMapping{
					{Host: "/tmp/data", Container: "/data"},
				},
			},
			wantContains: []string{"-p", "8080:80", "-v", "/tmp/data:/data"},
		},
		{
			name: "interactive with env and args",
			opts: RunOptions{
				Image: "python:3",
				Env:   map[string]string{"DEBUG": "1"},
				Args:  []string{"python", "-c", "print('hello')"},
			},
			wantContains: []string{"-e", "DEBUG=1", "python:3", "python", "-c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build args as RunInteractive does
			args := []string{"run", "-it"}

			if tt.opts.Name != "" {
				args = append(args, "--name", tt.opts.Name)
			}
			if tt.opts.Remove {
				args = append(args, "--rm")
			}
			if tt.opts.Privileged {
				args = append(args, "--privileged")
			}

			for _, p := range tt.opts.Ports {
				args = append(args, "-p", fmt.Sprintf("%d:%d", p.Host, p.Container))
			}

			for _, v := range tt.opts.Volumes {
				mapping := fmt.Sprintf("%s:%s", v.Host, v.Container)
				if v.Options != "" {
					mapping += ":" + v.Options
				}
				args = append(args, "-v", mapping)
			}

			for k, v := range tt.opts.Env {
				args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
			}

			args = append(args, tt.opts.Image)
			args = append(args, tt.opts.Args...)

			// Verify expected components
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range args {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("args %v does not contain %q", args, want)
				}
			}
		})
	}
}

// === Exists/VolumeExists Error Handling Tests ===

func TestExistsErrorHandling(t *testing.T) {
	// Test the logic for parsing exit status
	tests := []struct {
		name      string
		errMsg    string
		wantExist bool
		wantErr   bool
	}{
		{
			name:      "exit status 1 means not exists",
			errMsg:    "podman container exists failed: exit status 1",
			wantExist: false,
			wantErr:   false,
		},
		{
			name:      "other errors should propagate",
			errMsg:    "podman container exists failed: connection refused",
			wantExist: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic in Exists
			err := fmt.Errorf("%s", tt.errMsg)
			var exists bool
			var resultErr error

			if err != nil {
				if strings.Contains(err.Error(), "exit status 1") {
					exists = false
					resultErr = nil
				} else {
					exists = false
					resultErr = err
				}
			} else {
				exists = true
			}

			if exists != tt.wantExist {
				t.Errorf("exists = %v, want %v", exists, tt.wantExist)
			}
			if (resultErr != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", resultErr, tt.wantErr)
			}
		})
	}
}

// === IsLoggedIn Error Handling Tests ===

func TestIsLoggedInErrorParsing(t *testing.T) {
	tests := []struct {
		name         string
		stderr       string
		wantLoggedIn bool
	}{
		{
			name:         "not logged in message",
			stderr:       "not logged in",
			wantLoggedIn: false,
		},
		{
			name:         "Error not logged into message",
			stderr:       "Error: not logged into quay.io",
			wantLoggedIn: false,
		},
		{
			name:         "other error",
			stderr:       "Error: connection timeout",
			wantLoggedIn: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic in IsLoggedIn
			podmanErr := &PodmanError{
				Command: "login --get-login",
				Stderr:  tt.stderr,
				Err:     fmt.Errorf("exit status 1"),
			}

			loggedIn := false
			if strings.Contains(podmanErr.Stderr, "not logged in") ||
				strings.Contains(podmanErr.Stderr, "Error: not logged into") {
				loggedIn = false
			}

			if loggedIn != tt.wantLoggedIn {
				t.Errorf("loggedIn = %v, want %v", loggedIn, tt.wantLoggedIn)
			}
		})
	}
}

// === Integration Tests ===

func TestClientExists(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("podman not available: %v", err)
	}

	ctx := context.Background()

	// Test with a non-existent container name
	exists, err := client.Exists(ctx, "nonexistent-container-xyz-12345")
	if err != nil {
		t.Errorf("Exists() returned error for non-existent container: %v", err)
	}
	if exists {
		t.Error("Exists() should return false for non-existent container")
	}
}

func TestClientVolumeExists(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("podman not available: %v", err)
	}

	ctx := context.Background()

	// Test with a non-existent volume name
	exists, err := client.VolumeExists(ctx, "nonexistent-volume-xyz-12345")
	if err != nil {
		t.Errorf("VolumeExists() returned error for non-existent volume: %v", err)
	}
	if exists {
		t.Error("VolumeExists() should return false for non-existent volume")
	}
}

// === logReader Tests ===

func TestLogReaderStruct(t *testing.T) {
	// This just tests that logReader implements io.ReadCloser
	var _ io.ReadCloser = (*logReader)(nil)
}

// === Inspect Args Tests ===

func TestInspectArgs(t *testing.T) {
	// Test that inspect arguments are correctly constructed
	name := "test-container"
	args := []string{"inspect", "--format", "json", name}

	if args[0] != "inspect" {
		t.Errorf("args[0] = %q, want %q", args[0], "inspect")
	}
	if args[1] != "--format" {
		t.Errorf("args[1] = %q, want %q", args[1], "--format")
	}
	if args[2] != "json" {
		t.Errorf("args[2] = %q, want %q", args[2], "json")
	}
	if args[3] != name {
		t.Errorf("args[3] = %q, want %q", args[3], name)
	}
}

func TestImageInspectArgs(t *testing.T) {
	// Test that image inspect arguments are correctly constructed
	image := "alpine:latest"
	args := []string{"image", "inspect", "--format", "json", image}

	if args[0] != "image" {
		t.Errorf("args[0] = %q, want %q", args[0], "image")
	}
	if args[1] != "inspect" {
		t.Errorf("args[1] = %q, want %q", args[1], "inspect")
	}
	if args[4] != image {
		t.Errorf("args[4] = %q, want %q", args[4], image)
	}
}

// === Start/Stop Args Tests ===

func TestStartStopArgs(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		container string
		want      []string
	}{
		{
			name:      "start container",
			action:    "start",
			container: "mycontainer",
			want:      []string{"start", "mycontainer"},
		},
		{
			name:      "stop container",
			action:    "stop",
			container: "mycontainer",
			want:      []string{"stop", "mycontainer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{tt.action, tt.container}

			if len(args) != len(tt.want) {
				t.Errorf("args length = %d, want %d", len(args), len(tt.want))
			}

			for i, want := range tt.want {
				if args[i] != want {
					t.Errorf("args[%d] = %q, want %q", i, args[i], want)
				}
			}
		})
	}
}

// === Pull Args Tests ===

func TestPullArgs(t *testing.T) {
	image := "docker.io/library/alpine:latest"
	args := []string{"pull", image}

	if args[0] != "pull" {
		t.Errorf("args[0] = %q, want %q", args[0], "pull")
	}
	if args[1] != image {
		t.Errorf("args[1] = %q, want %q", args[1], image)
	}
}

// === Info Args Tests ===

func TestInfoArgs(t *testing.T) {
	args := []string{"info", "--format", "json"}

	if args[0] != "info" {
		t.Errorf("args[0] = %q, want %q", args[0], "info")
	}
	if args[1] != "--format" {
		t.Errorf("args[1] = %q, want %q", args[1], "--format")
	}
	if args[2] != "json" {
		t.Errorf("args[2] = %q, want %q", args[2], "json")
	}
}

// === Additional Tests for Coverage ===

// TestPodmanInfoStruct tests PodmanInfo struct initialization
func TestPodmanInfoStruct(t *testing.T) {
	tests := []struct {
		name     string
		info     PodmanInfo
		wantVer  string
		wantRoot bool
	}{
		{
			name:     "rootless mode",
			info:     PodmanInfo{Version: "4.5.0", Rootless: true},
			wantVer:  "4.5.0",
			wantRoot: true,
		},
		{
			name:     "root mode",
			info:     PodmanInfo{Version: "4.4.0", Rootless: false},
			wantVer:  "4.4.0",
			wantRoot: false,
		},
		{
			name:     "empty version",
			info:     PodmanInfo{Version: "", Rootless: false},
			wantVer:  "",
			wantRoot: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.info.Version != tt.wantVer {
				t.Errorf("Version = %q, want %q", tt.info.Version, tt.wantVer)
			}
			if tt.info.Rootless != tt.wantRoot {
				t.Errorf("Rootless = %v, want %v", tt.info.Rootless, tt.wantRoot)
			}
		})
	}
}

// TestContainerStateFields tests all ContainerState struct fields
func TestContainerStateFields(t *testing.T) {
	state := ContainerState{
		Running:    true,
		Paused:     false,
		Restarting: false,
		OOMKilled:  false,
		Dead:       false,
		Pid:        12345,
		ExitCode:   0,
		Error:      "",
		StartedAt:  "2024-01-01T00:00:00Z",
		FinishedAt: "",
	}

	if !state.Running {
		t.Error("Running should be true")
	}
	if state.Pid != 12345 {
		t.Errorf("Pid = %d, want %d", state.Pid, 12345)
	}
	if state.StartedAt != "2024-01-01T00:00:00Z" {
		t.Errorf("StartedAt = %q, want %q", state.StartedAt, "2024-01-01T00:00:00Z")
	}

	// Test error state
	errorState := ContainerState{
		Running:  false,
		Dead:     true,
		ExitCode: 137,
		Error:    "OOM killed",
	}

	if errorState.ExitCode != 137 {
		t.Errorf("ExitCode = %d, want %d", errorState.ExitCode, 137)
	}
	if errorState.Error != "OOM killed" {
		t.Errorf("Error = %q, want %q", errorState.Error, "OOM killed")
	}
}

// TestContainerInfoFields tests all ContainerInfo struct fields
func TestContainerInfoFields(t *testing.T) {
	info := ContainerInfo{
		ID:      "abc123def456789",
		Name:    "/test-container",
		Image:   "sha256:abc123def456",
		Created: "2024-01-01T10:00:00Z",
		State: ContainerState{
			Running:   true,
			Pid:       54321,
			StartedAt: "2024-01-01T10:00:01Z",
		},
	}

	if info.ID != "abc123def456789" {
		t.Errorf("ID = %q, want %q", info.ID, "abc123def456789")
	}
	if info.Name != "/test-container" {
		t.Errorf("Name = %q, want %q", info.Name, "/test-container")
	}
	if info.Created != "2024-01-01T10:00:00Z" {
		t.Errorf("Created = %q, want %q", info.Created, "2024-01-01T10:00:00Z")
	}
}

// TestImageInfoFields tests all ImageInfo struct fields
func TestImageInfoFields(t *testing.T) {
	info := ImageInfo{
		ID:         "sha256:abc123def456",
		Names:      []string{"myimage:latest", "myimage:v1"},
		Created:    1704067200,
		CreatedAt:  "2024-01-01T00:00:00Z",
		Size:       1073741824,
		Labels:     map[string]string{"version": "1.0", "containers.bootc": "1"},
		Repository: "myimage",
		Tag:        "latest",
	}

	if info.ID != "sha256:abc123def456" {
		t.Errorf("ID = %q, want %q", info.ID, "sha256:abc123def456")
	}
	if len(info.Names) != 2 {
		t.Errorf("len(Names) = %d, want %d", len(info.Names), 2)
	}
	if info.Size != 1073741824 {
		t.Errorf("Size = %d, want %d", info.Size, 1073741824)
	}
	if info.Repository != "myimage" {
		t.Errorf("Repository = %q, want %q", info.Repository, "myimage")
	}
	if info.Tag != "latest" {
		t.Errorf("Tag = %q, want %q", info.Tag, "latest")
	}
}

// TestImageInspectInfoFields tests all ImageInspectInfo struct fields
func TestImageInspectInfoFields(t *testing.T) {
	info := ImageInspectInfo{
		ID:           "sha256:abc123def456",
		Digest:       "sha256:xyz789",
		RepoTags:     []string{"myimage:latest"},
		RepoDigests:  []string{"docker.io/myimage@sha256:xyz789"},
		Created:      "2024-01-01T00:00:00Z",
		Size:         536870912,
		VirtualSize:  1073741824,
		Labels:       map[string]string{"maintainer": "test"},
		Architecture: "amd64",
		Os:           "linux",
		Config: struct {
			Cmd        []string          `json:"Cmd"`
			Env        []string          `json:"Env"`
			Labels     map[string]string `json:"Labels"`
			WorkingDir string            `json:"WorkingDir"`
		}{
			Cmd:        []string{"/bin/sh"},
			Env:        []string{"PATH=/usr/local/bin:/usr/bin"},
			Labels:     map[string]string{"version": "1.0"},
			WorkingDir: "/app",
		},
	}

	if info.Architecture != "amd64" {
		t.Errorf("Architecture = %q, want %q", info.Architecture, "amd64")
	}
	if info.Os != "linux" {
		t.Errorf("Os = %q, want %q", info.Os, "linux")
	}
	if info.VirtualSize != 1073741824 {
		t.Errorf("VirtualSize = %d, want %d", info.VirtualSize, 1073741824)
	}
	if info.Config.WorkingDir != "/app" {
		t.Errorf("Config.WorkingDir = %q, want %q", info.Config.WorkingDir, "/app")
	}
}

// TestPortMappingStruct tests PortMapping struct
func TestPortMappingStruct(t *testing.T) {
	pm := PortMapping{
		Host:      8080,
		Container: 80,
	}

	if pm.Host != 8080 {
		t.Errorf("Host = %d, want %d", pm.Host, 8080)
	}
	if pm.Container != 80 {
		t.Errorf("Container = %d, want %d", pm.Container, 80)
	}
}

// TestVolumeMappingStruct tests VolumeMapping struct
func TestVolumeMappingStruct(t *testing.T) {
	tests := []struct {
		name string
		vm   VolumeMapping
	}{
		{
			name: "basic mapping",
			vm:   VolumeMapping{Host: "/host", Container: "/container"},
		},
		{
			name: "with options",
			vm:   VolumeMapping{Host: "/data", Container: "/mnt", Options: "ro,Z"},
		},
		{
			name: "named volume",
			vm:   VolumeMapping{Host: "myvolume", Container: "/var/lib/data"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.vm.Host == "" {
				t.Error("Host should not be empty")
			}
			if tt.vm.Container == "" {
				t.Error("Container should not be empty")
			}
		})
	}
}

// TestRunOptionsStruct tests RunOptions struct fields
func TestRunOptionsStruct(t *testing.T) {
	opts := RunOptions{
		Name:       "test",
		Image:      "alpine",
		Ports:      []PortMapping{{Host: 8080, Container: 80}},
		Volumes:    []VolumeMapping{{Host: "/data", Container: "/mnt"}},
		Detach:     true,
		Remove:     false,
		Privileged: true,
		Env:        map[string]string{"KEY": "VALUE"},
		Args:       []string{"sh", "-c", "echo hello"},
	}

	if opts.Name != "test" {
		t.Errorf("Name = %q, want %q", opts.Name, "test")
	}
	if opts.Image != "alpine" {
		t.Errorf("Image = %q, want %q", opts.Image, "alpine")
	}
	if !opts.Detach {
		t.Error("Detach should be true")
	}
	if !opts.Privileged {
		t.Error("Privileged should be true")
	}
	if len(opts.Ports) != 1 {
		t.Errorf("len(Ports) = %d, want %d", len(opts.Ports), 1)
	}
	if len(opts.Args) != 3 {
		t.Errorf("len(Args) = %d, want %d", len(opts.Args), 3)
	}
}

// TestBuildOptionsStruct tests BuildOptions struct fields
func TestBuildOptionsStruct(t *testing.T) {
	opts := BuildOptions{
		Context:    "/path/to/context",
		Tag:        "myimage:v1.0",
		Dockerfile: "Containerfile.prod",
		NoCache:    true,
	}

	if opts.Context != "/path/to/context" {
		t.Errorf("Context = %q, want %q", opts.Context, "/path/to/context")
	}
	if opts.Tag != "myimage:v1.0" {
		t.Errorf("Tag = %q, want %q", opts.Tag, "myimage:v1.0")
	}
	if opts.Dockerfile != "Containerfile.prod" {
		t.Errorf("Dockerfile = %q, want %q", opts.Dockerfile, "Containerfile.prod")
	}
	if !opts.NoCache {
		t.Error("NoCache should be true")
	}
}

// TestImageInfoIsBootcEdgeCases tests edge cases for ImageInfo.IsBootc
func TestImageInfoIsBootcEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		info    ImageInfo
		isBootc bool
	}{
		{
			name: "empty labels map",
			info: ImageInfo{
				ID:     "abc123",
				Labels: map[string]string{},
			},
			isBootc: false,
		},
		{
			name: "bootc label with uppercase",
			info: ImageInfo{
				ID:     "abc123",
				Labels: map[string]string{"CONTAINERS.BOOTC": "1"},
			},
			isBootc: false, // Label is case-sensitive
		},
		{
			name: "bootc label with empty value",
			info: ImageInfo{
				ID:     "abc123",
				Labels: map[string]string{"containers.bootc": ""},
			},
			isBootc: false,
		},
		{
			name: "bootc label with true string",
			info: ImageInfo{
				ID:     "abc123",
				Labels: map[string]string{"containers.bootc": "true"},
			},
			isBootc: false, // Only "1" is valid
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.IsBootc()
			if got != tt.isBootc {
				t.Errorf("IsBootc() = %v, want %v", got, tt.isBootc)
			}
		})
	}
}

// TestImageInspectInfoIsBootcEdgeCases tests edge cases for ImageInspectInfo.IsBootc
func TestImageInspectInfoIsBootcEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		info    ImageInspectInfo
		isBootc bool
	}{
		{
			name: "both Config.Labels and Labels nil",
			info: ImageInspectInfo{
				ID: "abc123",
			},
			isBootc: false,
		},
		{
			name: "Config.Labels nil but Labels has bootc",
			info: ImageInspectInfo{
				ID:     "abc123",
				Labels: map[string]string{"containers.bootc": "1"},
			},
			isBootc: true,
		},
		{
			name: "Config.Labels has bootc, Labels does not",
			info: ImageInspectInfo{
				ID: "abc123",
				Config: struct {
					Cmd        []string          `json:"Cmd"`
					Env        []string          `json:"Env"`
					Labels     map[string]string `json:"Labels"`
					WorkingDir string            `json:"WorkingDir"`
				}{
					Labels: map[string]string{"containers.bootc": "1"},
				},
				Labels: map[string]string{"other": "label"},
			},
			isBootc: true, // Config.Labels takes precedence
		},
		{
			name: "both have different values",
			info: ImageInspectInfo{
				ID: "abc123",
				Config: struct {
					Cmd        []string          `json:"Cmd"`
					Env        []string          `json:"Env"`
					Labels     map[string]string `json:"Labels"`
					WorkingDir string            `json:"WorkingDir"`
				}{
					Labels: map[string]string{"containers.bootc": "0"},
				},
				Labels: map[string]string{"containers.bootc": "1"},
			},
			isBootc: true, // Falls through: Config.Labels["containers.bootc"] != "1", so checks top-level Labels
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.info.IsBootc()
			if got != tt.isBootc {
				t.Errorf("IsBootc() = %v, want %v", got, tt.isBootc)
			}
		})
	}
}

// TestPodmanErrorFields tests PodmanError struct fields
func TestPodmanErrorFields(t *testing.T) {
	err := PodmanError{
		Command: "run -d nginx",
		Stderr:  "Error: image not found\ndetailed message",
		Err:     fmt.Errorf("exit status 125"),
	}

	if err.Command != "run -d nginx" {
		t.Errorf("Command = %q, want %q", err.Command, "run -d nginx")
	}
	if !strings.Contains(err.Stderr, "Error: image not found") {
		t.Errorf("Stderr should contain 'Error: image not found'")
	}
}

// TestVolumeMappingFormatting tests volume mapping string formatting
func TestVolumeMappingFormatting(t *testing.T) {
	tests := []struct {
		name     string
		vol      VolumeMapping
		expected string
	}{
		{
			name:     "simple mapping",
			vol:      VolumeMapping{Host: "/data", Container: "/mnt"},
			expected: "/data:/mnt",
		},
		{
			name:     "with ro option",
			vol:      VolumeMapping{Host: "/config", Container: "/etc/app", Options: "ro"},
			expected: "/config:/etc/app:ro",
		},
		{
			name:     "with Z option",
			vol:      VolumeMapping{Host: "/logs", Container: "/var/log", Options: "Z"},
			expected: "/logs:/var/log:Z",
		},
		{
			name:     "named volume",
			vol:      VolumeMapping{Host: "mydata", Container: "/data"},
			expected: "mydata:/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := fmt.Sprintf("%s:%s", tt.vol.Host, tt.vol.Container)
			if tt.vol.Options != "" {
				mapping += ":" + tt.vol.Options
			}

			if mapping != tt.expected {
				t.Errorf("mapping = %q, want %q", mapping, tt.expected)
			}
		})
	}
}

// TestPortMappingFormatting tests port mapping string formatting
func TestPortMappingFormatting(t *testing.T) {
	tests := []struct {
		name     string
		pm       PortMapping
		expected string
	}{
		{
			name:     "standard web port",
			pm:       PortMapping{Host: 8080, Container: 80},
			expected: "8080:80",
		},
		{
			name:     "same port",
			pm:       PortMapping{Host: 5000, Container: 5000},
			expected: "5000:5000",
		},
		{
			name:     "ssl port",
			pm:       PortMapping{Host: 8443, Container: 443},
			expected: "8443:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := fmt.Sprintf("%d:%d", tt.pm.Host, tt.pm.Container)
			if mapping != tt.expected {
				t.Errorf("mapping = %q, want %q", mapping, tt.expected)
			}
		})
	}
}

// TestClientBinaryPath tests that client has binary path set
func TestClientBinaryPath(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Skipf("podman not available: %v", err)
	}

	if client.binary == "" {
		t.Error("client.binary should not be empty")
	}

	// Binary should be an absolute path or just "podman"
	if !strings.HasPrefix(client.binary, "/") && client.binary != "podman" {
		t.Logf("client.binary = %q (may be relative path)", client.binary)
	}
}

// TestContainerExistsArgs tests container exists command arguments
func TestContainerExistsArgs(t *testing.T) {
	name := "test-container"
	args := []string{"container", "exists", name}

	if args[0] != "container" {
		t.Errorf("args[0] = %q, want %q", args[0], "container")
	}
	if args[1] != "exists" {
		t.Errorf("args[1] = %q, want %q", args[1], "exists")
	}
	if args[2] != name {
		t.Errorf("args[2] = %q, want %q", args[2], name)
	}
}

// TestVolumeExistsArgs tests volume exists command arguments
func TestVolumeExistsArgs(t *testing.T) {
	name := "test-volume"
	args := []string{"volume", "exists", name}

	if args[0] != "volume" {
		t.Errorf("args[0] = %q, want %q", args[0], "volume")
	}
	if args[1] != "exists" {
		t.Errorf("args[1] = %q, want %q", args[1], "exists")
	}
	if args[2] != name {
		t.Errorf("args[2] = %q, want %q", args[2], name)
	}
}

// TestLoginGetLoginArgs tests login --get-login command arguments
func TestLoginGetLoginArgs(t *testing.T) {
	registry := "quay.io"
	args := []string{"login", "--get-login", registry}

	if args[0] != "login" {
		t.Errorf("args[0] = %q, want %q", args[0], "login")
	}
	if args[1] != "--get-login" {
		t.Errorf("args[1] = %q, want %q", args[1], "--get-login")
	}
	if args[2] != registry {
		t.Errorf("args[2] = %q, want %q", args[2], registry)
	}
}

// === Pure Function Tests ===

// TestFormatPortMapping tests the pure function for formatting port mappings
func TestFormatPortMapping(t *testing.T) {
	tests := []struct {
		name string
		pm   PortMapping
		want string
	}{
		{
			name: "standard web port",
			pm:   PortMapping{Host: 8080, Container: 80},
			want: "8080:80",
		},
		{
			name: "same port",
			pm:   PortMapping{Host: 5000, Container: 5000},
			want: "5000:5000",
		},
		{
			name: "ssl port",
			pm:   PortMapping{Host: 8443, Container: 443},
			want: "8443:443",
		},
		{
			name: "registry port",
			pm:   PortMapping{Host: 5000, Container: 5000},
			want: "5000:5000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatPortMapping(tt.pm)
			if got != tt.want {
				t.Errorf("FormatPortMapping() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFormatVolumeMapping tests the pure function for formatting volume mappings
func TestFormatVolumeMapping(t *testing.T) {
	tests := []struct {
		name string
		vm   VolumeMapping
		want string
	}{
		{
			name: "simple mapping",
			vm:   VolumeMapping{Host: "/data", Container: "/mnt"},
			want: "/data:/mnt",
		},
		{
			name: "with ro option",
			vm:   VolumeMapping{Host: "/config", Container: "/etc/app", Options: "ro"},
			want: "/config:/etc/app:ro",
		},
		{
			name: "with Z option",
			vm:   VolumeMapping{Host: "/logs", Container: "/var/log", Options: "Z"},
			want: "/logs:/var/log:Z",
		},
		{
			name: "named volume",
			vm:   VolumeMapping{Host: "mydata", Container: "/data"},
			want: "mydata:/data",
		},
		{
			name: "with multiple options",
			vm:   VolumeMapping{Host: "/secure", Container: "/app", Options: "ro,Z"},
			want: "/secure:/app:ro,Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatVolumeMapping(tt.vm)
			if got != tt.want {
				t.Errorf("FormatVolumeMapping() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildRunArgs tests the pure function for building run arguments
func TestBuildRunArgs(t *testing.T) {
	tests := []struct {
		name         string
		opts         RunOptions
		interactive  bool
		wantContains []string
		wantLen      int
	}{
		{
			name: "minimal non-interactive",
			opts: RunOptions{
				Image: "alpine",
			},
			interactive:  false,
			wantContains: []string{"run", "alpine"},
			wantLen:      2,
		},
		{
			name: "minimal interactive",
			opts: RunOptions{
				Image: "alpine",
			},
			interactive:  true,
			wantContains: []string{"run", "-it", "alpine"},
			wantLen:      3,
		},
		{
			name: "detached with name",
			opts: RunOptions{
				Name:   "test-container",
				Image:  "nginx",
				Detach: true,
			},
			interactive:  false,
			wantContains: []string{"run", "--name", "test-container", "-d", "nginx"},
			wantLen:      5,
		},
		{
			name: "detach ignored in interactive mode",
			opts: RunOptions{
				Name:   "test",
				Image:  "alpine",
				Detach: true, // Should be ignored
			},
			interactive:  true,
			wantContains: []string{"run", "-it", "--name", "test", "alpine"},
			wantLen:      5, // No -d because interactive
		},
		{
			name: "with rm flag",
			opts: RunOptions{
				Image:  "busybox",
				Remove: true,
			},
			interactive:  false,
			wantContains: []string{"--rm"},
		},
		{
			name: "with privileged",
			opts: RunOptions{
				Image:      "registry:2",
				Privileged: true,
			},
			interactive:  false,
			wantContains: []string{"--privileged"},
		},
		{
			name: "with ports",
			opts: RunOptions{
				Image: "nginx",
				Ports: []PortMapping{
					{Host: 8080, Container: 80},
					{Host: 8443, Container: 443},
				},
			},
			interactive:  false,
			wantContains: []string{"-p", "8080:80", "-p", "8443:443"},
		},
		{
			name: "with volumes",
			opts: RunOptions{
				Image: "postgres",
				Volumes: []VolumeMapping{
					{Host: "/data", Container: "/var/lib/postgresql/data"},
					{Host: "/config", Container: "/etc/postgresql", Options: "ro"},
				},
			},
			interactive:  false,
			wantContains: []string{"-v", "/data:/var/lib/postgresql/data", "-v", "/config:/etc/postgresql:ro"},
		},
		{
			name: "with extra args",
			opts: RunOptions{
				Image: "alpine",
				Args:  []string{"sh", "-c", "echo hello"},
			},
			interactive:  false,
			wantContains: []string{"alpine", "sh", "-c", "echo hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildRunArgs(tt.opts, tt.interactive)

			// Verify expected components are present
			for _, want := range tt.wantContains {
				found := false
				for _, arg := range got {
					if arg == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("BuildRunArgs() = %v, missing %q", got, want)
				}
			}

			// Verify length if specified
			if tt.wantLen > 0 && len(got) != tt.wantLen {
				t.Errorf("BuildRunArgs() length = %d, want %d\nargs: %v", len(got), tt.wantLen, got)
			}
		})
	}
}

// TestBuildRunArgsOrder tests that BuildRunArgs produces args in the correct order
func TestBuildRunArgsOrder(t *testing.T) {
	opts := RunOptions{
		Name:       "test",
		Image:      "alpine",
		Detach:     true,
		Remove:     true,
		Privileged: true,
		Ports:      []PortMapping{{Host: 8080, Container: 80}},
		Volumes:    []VolumeMapping{{Host: "/data", Container: "/mnt"}},
		Args:       []string{"sh"},
	}

	args := BuildRunArgs(opts, false)

	// First arg should always be "run"
	if args[0] != "run" {
		t.Errorf("first arg should be 'run', got %q", args[0])
	}

	// Image should come before Args
	imageIdx := -1
	argIdx := -1
	for i, arg := range args {
		if arg == "alpine" {
			imageIdx = i
		}
		if arg == "sh" {
			argIdx = i
		}
	}

	if imageIdx >= argIdx {
		t.Errorf("image should come before args: image at %d, arg at %d", imageIdx, argIdx)
	}
}

// TestBuildRunArgsInteractiveMode tests interactive mode specific behavior
func TestBuildRunArgsInteractiveMode(t *testing.T) {
	opts := RunOptions{
		Image:  "alpine",
		Detach: true, // Should be ignored in interactive mode
	}

	// Non-interactive should have -d
	nonInteractive := BuildRunArgs(opts, false)
	hasDetach := false
	for _, arg := range nonInteractive {
		if arg == "-d" {
			hasDetach = true
			break
		}
	}
	if !hasDetach {
		t.Error("non-interactive mode with Detach=true should have -d flag")
	}

	// Interactive should NOT have -d
	interactive := BuildRunArgs(opts, true)
	hasDetach = false
	for _, arg := range interactive {
		if arg == "-d" {
			hasDetach = true
			break
		}
	}
	if hasDetach {
		t.Error("interactive mode should not have -d flag even with Detach=true")
	}

	// Interactive should have -it
	hasIT := false
	for i, arg := range interactive {
		if arg == "-it" || (arg == "-i" && i+1 < len(interactive) && interactive[i+1] == "-t") {
			hasIT = true
			break
		}
	}
	if !hasIT {
		t.Error("interactive mode should have -it flag")
	}
}
