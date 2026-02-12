package registry

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tnk4on/bootc-man/internal/config"
	"github.com/tnk4on/bootc-man/internal/podman"
)

// mockPodmanClient is a mock implementation for testing
type mockPodmanClient struct {
	existsFunc       func(ctx context.Context, name string) (bool, error)
	inspectFunc      func(ctx context.Context, name string) (*podman.ContainerInfo, error)
	runFunc          func(ctx context.Context, opts podman.RunOptions) (string, error)
	startFunc        func(ctx context.Context, name string) error
	stopFunc         func(ctx context.Context, name string) error
	removeFunc       func(ctx context.Context, name string, force bool) error
	logsFunc         func(ctx context.Context, name string, follow bool) (io.ReadCloser, error)
	volumeExistsFunc func(ctx context.Context, name string) (bool, error)
	volumeRemoveFunc func(ctx context.Context, name string, force bool) error
}

func (m *mockPodmanClient) Exists(ctx context.Context, name string) (bool, error) {
	if m.existsFunc != nil {
		return m.existsFunc(ctx, name)
	}
	return false, nil
}

func (m *mockPodmanClient) Inspect(ctx context.Context, name string) (*podman.ContainerInfo, error) {
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, name)
	}
	return &podman.ContainerInfo{State: podman.ContainerState{Running: false}}, nil
}

func (m *mockPodmanClient) Run(ctx context.Context, opts podman.RunOptions) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, opts)
	}
	return "container-id", nil
}

func (m *mockPodmanClient) Start(ctx context.Context, name string) error {
	if m.startFunc != nil {
		return m.startFunc(ctx, name)
	}
	return nil
}

func (m *mockPodmanClient) Stop(ctx context.Context, name string) error {
	if m.stopFunc != nil {
		return m.stopFunc(ctx, name)
	}
	return nil
}

func (m *mockPodmanClient) Remove(ctx context.Context, name string, force bool) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, name, force)
	}
	return nil
}

func (m *mockPodmanClient) Logs(ctx context.Context, name string, follow bool) (io.ReadCloser, error) {
	if m.logsFunc != nil {
		return m.logsFunc(ctx, name, follow)
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func (m *mockPodmanClient) VolumeExists(ctx context.Context, name string) (bool, error) {
	if m.volumeExistsFunc != nil {
		return m.volumeExistsFunc(ctx, name)
	}
	return false, nil
}

func (m *mockPodmanClient) VolumeRemove(ctx context.Context, name string, force bool) error {
	if m.volumeRemoveFunc != nil {
		return m.volumeRemoveFunc(ctx, name, force)
	}
	return nil
}

// Note: podmanInterface was removed as it's not used in tests.
// The mockPodmanClient above provides the mock implementation directly.

func TestContainerName(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.ContainersConfig
		want string
	}{
		{
			name: "nil config returns default",
			cfg:  nil,
			want: config.ContainerNameRegistry,
		},
		{
			name: "empty registry name returns default",
			cfg:  &config.ContainersConfig{},
			want: config.ContainerNameRegistry,
		},
		{
			name: "custom registry name",
			cfg:  &config.ContainersConfig{RegistryName: "my-registry"},
			want: "my-registry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainerName(tt.cfg)
			if got != tt.want {
				t.Errorf("ContainerName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVolumeName(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.ContainersConfig
		want string
	}{
		{
			name: "nil config returns default",
			cfg:  nil,
			want: config.VolumeNameRegistryData,
		},
		{
			name: "empty volume name returns default",
			cfg:  &config.ContainersConfig{},
			want: config.VolumeNameRegistryData,
		},
		{
			name: "custom volume name",
			cfg:  &config.ContainersConfig{RegistryDataVolume: "my-volume"},
			want: "my-volume",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VolumeName(tt.cfg)
			if got != tt.want {
				t.Errorf("VolumeName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewService(t *testing.T) {
	regCfg := &config.RegistryConfig{
		Image: config.DefaultRegistryImage,
		Port:  5000,
	}
	contCfg := &config.ContainersConfig{
		RegistryName:       "test-registry",
		RegistryDataVolume: "test-volume",
	}

	svc := NewService(ServiceOptions{
		Config:           regCfg,
		ContainersConfig: contCfg,
		Verbose:          true,
		DryRun:           false,
	})

	if svc.GetContainerName() != "test-registry" {
		t.Errorf("GetContainerName() = %q, want %q", svc.GetContainerName(), "test-registry")
	}
	if svc.GetVolumeName() != "test-volume" {
		t.Errorf("GetVolumeName() = %q, want %q", svc.GetVolumeName(), "test-volume")
	}
	if svc.IsDryRun() != false {
		t.Error("IsDryRun() = true, want false")
	}
}

func TestGetRegistryURL(t *testing.T) {
	tests := []struct {
		name string
		port int
		want string
	}{
		{
			name: "default port",
			port: 5000,
			want: "localhost:5000",
		},
		{
			name: "custom port",
			port: 8080,
			want: "localhost:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &Service{
				config: &config.RegistryConfig{Port: tt.port},
			}
			got := svc.GetRegistryURL()
			if got != tt.want {
				t.Errorf("GetRegistryURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetDataDir(t *testing.T) {
	svc := &Service{}
	got := svc.GetDataDir("/data")
	want := "/data/registry"
	if got != want {
		t.Errorf("GetDataDir() = %q, want %q", got, want)
	}
}

func TestDryRunMode(t *testing.T) {
	svc := &Service{
		config:        &config.RegistryConfig{Port: config.DefaultRegistryPort, Image: config.DefaultRegistryImage},
		dryRun:        true,
		containerName: "test",
		volumeName:    "test-vol",
	}

	ctx := context.Background()

	// Test Up in dry-run mode
	result, err := svc.Up(ctx)
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if result == nil {
		t.Fatal("Up() result is nil")
	}

	// Test Down in dry-run mode
	downResult, err := svc.Down(ctx)
	if err != nil {
		t.Fatalf("Down() error = %v", err)
	}
	if downResult == nil {
		t.Fatal("Down() result is nil")
	}

	// Test Status in dry-run mode
	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.State != "(dry-run)" {
		t.Errorf("Status().State = %q, want %q", status.State, "(dry-run)")
	}

	// Test Logs in dry-run mode
	reader, err := svc.Logs(ctx, false)
	if err != nil {
		t.Fatalf("Logs() error = %v", err)
	}
	if reader != nil {
		t.Error("Logs() should return nil in dry-run mode")
	}

	// Test Remove in dry-run mode
	err = svc.Remove(ctx, true, true)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
}

func TestFormatPortError(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		port         int
		wantNil      bool
		wantContains string
	}{
		{
			name:    "nil error returns nil",
			err:     nil,
			port:    5000,
			wantNil: true,
		},
		{
			name: "address already in use",
			err: &podman.PodmanError{
				Command: "run",
				Stderr:  "Error: address already in use",
				Err:     errors.New("exit status 1"),
			},
			port:         5000,
			wantNil:      false,
			wantContains: "port 5000 is already in use",
		},
		{
			name: "bind address already in use",
			err: &podman.PodmanError{
				Command: "run",
				Stderr:  "Error: bind: address already in use",
				Err:     errors.New("exit status 1"),
			},
			port:         8080,
			wantNil:      false,
			wantContains: "port 8080 is already in use",
		},
		{
			name: "other podman error",
			err: &podman.PodmanError{
				Command: "run",
				Stderr:  "Error: some other error",
				Err:     errors.New("exit status 1"),
			},
			port:         5000,
			wantNil:      false,
			wantContains: "failed to execute podman command",
		},
		{
			name:         "non-podman error with address in use",
			err:          errors.New("address already in use"),
			port:         5000,
			wantNil:      false,
			wantContains: "port 5000 is already in use",
		},
		{
			name:         "generic error",
			err:          errors.New("some generic error"),
			port:         5000,
			wantNil:      false,
			wantContains: "some generic error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPortError(tt.err, tt.port)

			if tt.wantNil {
				if result != nil {
					t.Errorf("formatPortError() = %v, want nil", result)
				}
				return
			}

			if result == nil {
				t.Fatal("formatPortError() = nil, want error")
			}

			if !strings.Contains(result.Error(), tt.wantContains) {
				t.Errorf("formatPortError().Error() = %q, want to contain %q", result.Error(), tt.wantContains)
			}
		})
	}
}

func TestRegistryError(t *testing.T) {
	podmanErr := &podman.PodmanError{
		Command: "run",
		Stderr:  "test error",
		Err:     errors.New("exit status 1"),
	}

	regErr := &RegistryError{
		Message:     "test message",
		PodmanError: podmanErr,
	}

	if regErr.Error() != "test message" {
		t.Errorf("Error() = %q, want %q", regErr.Error(), "test message")
	}

	unwrapped := regErr.Unwrap()
	if unwrapped != podmanErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, podmanErr)
	}

	// Test with nil PodmanError
	regErr2 := &RegistryError{Message: "no podman error"}
	if regErr2.Unwrap() != nil {
		t.Errorf("Unwrap() = %v, want nil", regErr2.Unwrap())
	}
}

func TestStatusStates(t *testing.T) {
	tests := []struct {
		name      string
		exists    bool
		running   bool
		wantState string
	}{
		{
			name:      "not created",
			exists:    false,
			running:   false,
			wantState: "not created",
		},
		{
			name:      "stopped",
			exists:    true,
			running:   false,
			wantState: "stopped",
		},
		{
			name:      "running",
			exists:    true,
			running:   true,
			wantState: "running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockPodmanClient{
				existsFunc: func(ctx context.Context, name string) (bool, error) {
					return tt.exists, nil
				},
				inspectFunc: func(ctx context.Context, name string) (*podman.ContainerInfo, error) {
					return &podman.ContainerInfo{
						Image:   config.DefaultRegistryImage,
						Created: "2024-01-01T00:00:00Z",
						State:   podman.ContainerState{Running: tt.running},
					}, nil
				},
			}

			// We test the mock logic directly since Service uses *podman.Client
			ctx := context.Background()

			// Test exists check
			exists, _ := mock.Exists(ctx, "test")
			if exists != tt.exists {
				t.Errorf("exists = %v, want %v", exists, tt.exists)
			}

			if tt.exists {
				info, _ := mock.Inspect(ctx, "test")
				var state string
				if info.State.Running {
					state = "running"
				} else {
					state = "stopped"
				}
				if state != tt.wantState {
					t.Errorf("state = %q, want %q", state, tt.wantState)
				}
			}
		})
	}
}

// TestServiceOptions tests ServiceOptions initialization
func TestServiceOptions(t *testing.T) {
	regCfg := &config.RegistryConfig{
		Port:  5001,
		Image: "custom/registry:v1",
	}
	contCfg := &config.ContainersConfig{
		RegistryName:       "custom-registry",
		RegistryDataVolume: "custom-volume",
	}

	opts := ServiceOptions{
		Config:           regCfg,
		ContainersConfig: contCfg,
		Verbose:          true,
		DryRun:           true,
	}

	svc := NewService(opts)

	if svc.GetContainerName() != "custom-registry" {
		t.Errorf("GetContainerName() = %q, want %q", svc.GetContainerName(), "custom-registry")
	}
	if svc.GetVolumeName() != "custom-volume" {
		t.Errorf("GetVolumeName() = %q, want %q", svc.GetVolumeName(), "custom-volume")
	}
	if !svc.IsDryRun() {
		t.Error("IsDryRun() = false, want true")
	}
}

// TestNewServiceWithNilContainersConfig tests creating service with nil ContainersConfig
func TestNewServiceWithNilContainersConfig(t *testing.T) {
	regCfg := &config.RegistryConfig{
		Port:  5000,
		Image: config.DefaultRegistryImage,
	}

	svc := NewService(ServiceOptions{
		Config:           regCfg,
		ContainersConfig: nil, // nil config should use defaults
	})

	if svc.GetContainerName() != config.ContainerNameRegistry {
		t.Errorf("GetContainerName() = %q, want default %q", svc.GetContainerName(), config.ContainerNameRegistry)
	}
	if svc.GetVolumeName() != config.VolumeNameRegistryData {
		t.Errorf("GetVolumeName() = %q, want default %q", svc.GetVolumeName(), config.VolumeNameRegistryData)
	}
}

// TestUpResultFields tests UpResult struct fields
func TestUpResultFields(t *testing.T) {
	result := &UpResult{
		AlreadyRunning: true,
	}

	if !result.AlreadyRunning {
		t.Error("UpResult.AlreadyRunning should be true")
	}

	result2 := &UpResult{
		AlreadyRunning: false,
	}

	if result2.AlreadyRunning {
		t.Error("UpResult.AlreadyRunning should be false")
	}
}

// TestDownResultFields tests DownResult struct fields
func TestDownResultFields(t *testing.T) {
	result := &DownResult{
		AlreadyStopped: true,
		NotCreated:     false,
	}

	if !result.AlreadyStopped {
		t.Error("DownResult.AlreadyStopped should be true")
	}
	if result.NotCreated {
		t.Error("DownResult.NotCreated should be false")
	}

	result2 := &DownResult{
		AlreadyStopped: false,
		NotCreated:     true,
	}

	if result2.AlreadyStopped {
		t.Error("DownResult.AlreadyStopped should be false")
	}
	if !result2.NotCreated {
		t.Error("DownResult.NotCreated should be true")
	}
}

// TestStatusFields tests Status struct fields
func TestStatusFields(t *testing.T) {
	status := &Status{
		State:   "running",
		Port:    5000,
		Image:   "registry:2",
		Created: "2024-01-01T00:00:00Z",
	}

	if status.State != "running" {
		t.Errorf("Status.State = %q, want %q", status.State, "running")
	}
	if status.Port != 5000 {
		t.Errorf("Status.Port = %d, want %d", status.Port, 5000)
	}
	if status.Image != "registry:2" {
		t.Errorf("Status.Image = %q, want %q", status.Image, "registry:2")
	}
	if status.Created != "2024-01-01T00:00:00Z" {
		t.Errorf("Status.Created = %q, want %q", status.Created, "2024-01-01T00:00:00Z")
	}
}

// TestMockPodmanClientDefaults tests mock client default behavior
func TestMockPodmanClientDefaults(t *testing.T) {
	mock := &mockPodmanClient{}
	ctx := context.Background()

	// Test default Exists
	exists, err := mock.Exists(ctx, "test")
	if err != nil {
		t.Errorf("Exists() error = %v", err)
	}
	if exists {
		t.Error("Exists() = true, want false by default")
	}

	// Test default Inspect
	info, err := mock.Inspect(ctx, "test")
	if err != nil {
		t.Errorf("Inspect() error = %v", err)
	}
	if info == nil {
		t.Fatal("Inspect() = nil")
	}
	if info.State.Running {
		t.Error("Inspect().State.Running = true, want false by default")
	}

	// Test default Run
	id, err := mock.Run(ctx, podman.RunOptions{})
	if err != nil {
		t.Errorf("Run() error = %v", err)
	}
	if id != "container-id" {
		t.Errorf("Run() = %q, want %q", id, "container-id")
	}

	// Test default Start
	if err := mock.Start(ctx, "test"); err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Test default Stop
	if err := mock.Stop(ctx, "test"); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Test default Remove
	if err := mock.Remove(ctx, "test", false); err != nil {
		t.Errorf("Remove() error = %v", err)
	}

	// Test default Logs
	reader, err := mock.Logs(ctx, "test", false)
	if err != nil {
		t.Errorf("Logs() error = %v", err)
	}
	if reader == nil {
		t.Error("Logs() = nil")
	} else {
		reader.Close()
	}

	// Test default VolumeExists
	volExists, err := mock.VolumeExists(ctx, "test")
	if err != nil {
		t.Errorf("VolumeExists() error = %v", err)
	}
	if volExists {
		t.Error("VolumeExists() = true, want false by default")
	}

	// Test default VolumeRemove
	if err := mock.VolumeRemove(ctx, "test", false); err != nil {
		t.Errorf("VolumeRemove() error = %v", err)
	}
}

// TestMockPodmanClientCustomFunctions tests mock client with custom functions
func TestMockPodmanClientCustomFunctions(t *testing.T) {
	expectedError := errors.New("test error")

	mock := &mockPodmanClient{
		existsFunc: func(ctx context.Context, name string) (bool, error) {
			return true, nil
		},
		inspectFunc: func(ctx context.Context, name string) (*podman.ContainerInfo, error) {
			return &podman.ContainerInfo{
				Image:   "custom-image",
				Created: "2024-12-31T23:59:59Z",
				State:   podman.ContainerState{Running: true},
			}, nil
		},
		runFunc: func(ctx context.Context, opts podman.RunOptions) (string, error) {
			return "custom-id", nil
		},
		startFunc: func(ctx context.Context, name string) error {
			return expectedError
		},
		stopFunc: func(ctx context.Context, name string) error {
			return expectedError
		},
		removeFunc: func(ctx context.Context, name string, force bool) error {
			if force {
				return nil
			}
			return expectedError
		},
		logsFunc: func(ctx context.Context, name string, follow bool) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("test logs")), nil
		},
		volumeExistsFunc: func(ctx context.Context, name string) (bool, error) {
			return true, nil
		},
		volumeRemoveFunc: func(ctx context.Context, name string, force bool) error {
			return nil
		},
	}

	ctx := context.Background()

	// Test custom Exists
	exists, _ := mock.Exists(ctx, "test")
	if !exists {
		t.Error("Exists() = false, want true")
	}

	// Test custom Inspect
	info, _ := mock.Inspect(ctx, "test")
	if info.Image != "custom-image" {
		t.Errorf("Inspect().Image = %q, want %q", info.Image, "custom-image")
	}
	if !info.State.Running {
		t.Error("Inspect().State.Running = false, want true")
	}

	// Test custom Run
	id, _ := mock.Run(ctx, podman.RunOptions{})
	if id != "custom-id" {
		t.Errorf("Run() = %q, want %q", id, "custom-id")
	}

	// Test custom Start (returns error)
	if err := mock.Start(ctx, "test"); err != expectedError {
		t.Errorf("Start() error = %v, want %v", err, expectedError)
	}

	// Test custom Stop (returns error)
	if err := mock.Stop(ctx, "test"); err != expectedError {
		t.Errorf("Stop() error = %v, want %v", err, expectedError)
	}

	// Test custom Remove (force=false returns error)
	if err := mock.Remove(ctx, "test", false); err != expectedError {
		t.Errorf("Remove(force=false) error = %v, want %v", err, expectedError)
	}
	// Test custom Remove (force=true returns nil)
	if err := mock.Remove(ctx, "test", true); err != nil {
		t.Errorf("Remove(force=true) error = %v, want nil", err)
	}

	// Test custom Logs
	reader, _ := mock.Logs(ctx, "test", false)
	if reader == nil {
		t.Fatal("Logs() = nil")
	}
	data, _ := io.ReadAll(reader)
	reader.Close()
	if string(data) != "test logs" {
		t.Errorf("Logs() content = %q, want %q", string(data), "test logs")
	}

	// Test custom VolumeExists
	volExists, _ := mock.VolumeExists(ctx, "test")
	if !volExists {
		t.Error("VolumeExists() = false, want true")
	}
}

// TestDryRunModeWithVerbose tests dry-run mode with verbose flag
func TestDryRunModeWithVerbose(t *testing.T) {
	svc := &Service{
		config:        &config.RegistryConfig{Port: 5000, Image: config.DefaultRegistryImage},
		dryRun:        true,
		verbose:       true,
		containerName: "test-container",
		volumeName:    "test-volume",
	}

	ctx := context.Background()

	// Test Up
	upResult, err := svc.Up(ctx)
	if err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if upResult == nil {
		t.Fatal("Up() result is nil")
	}
	if upResult.AlreadyRunning {
		t.Error("Up() AlreadyRunning should be false in dry-run")
	}

	// Test Down
	downResult, err := svc.Down(ctx)
	if err != nil {
		t.Fatalf("Down() error = %v", err)
	}
	if downResult == nil {
		t.Fatal("Down() result is nil")
	}

	// Test Remove with volume
	err = svc.Remove(ctx, true, true)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Test Remove without volume
	err = svc.Remove(ctx, false, false)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
}

// TestVerboseModeShowCommand tests verbose mode shows commands
func TestVerboseModeShowCommand(t *testing.T) {
	svc := &Service{
		config:        &config.RegistryConfig{Port: 5000, Image: config.DefaultRegistryImage},
		dryRun:        false,
		verbose:       true,
		containerName: "test-container",
		volumeName:    "test-volume",
	}

	// showCommand should not panic when called
	svc.showCommand("test", "echo hello")
}

// TestStatusPortFromConfig tests that Status uses port from config
func TestStatusPortFromConfig(t *testing.T) {
	svc := &Service{
		config:        &config.RegistryConfig{Port: 8888},
		dryRun:        true,
		containerName: "test",
	}

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	if status.Port != 8888 {
		t.Errorf("Status().Port = %d, want %d", status.Port, 8888)
	}
}

// TestRegistryErrorWithNilPodmanError tests RegistryError.Unwrap with nil PodmanError
func TestRegistryErrorWithNilPodmanError(t *testing.T) {
	regErr := &RegistryError{
		Message:     "test error message",
		PodmanError: nil,
	}

	if regErr.Error() != "test error message" {
		t.Errorf("Error() = %q, want %q", regErr.Error(), "test error message")
	}

	if regErr.Unwrap() != nil {
		t.Error("Unwrap() should return nil when PodmanError is nil")
	}
}

// TestFormatPortErrorWithBindAddressInUse tests formatPortError with "bind: address already in use"
func TestFormatPortErrorWithBindAddressInUse(t *testing.T) {
	err := &podman.PodmanError{
		Command: "run",
		Stderr:  "Error: bind: address already in use",
		Err:     errors.New("exit status 1"),
	}

	result := formatPortError(err, 5000)
	if result == nil {
		t.Fatal("formatPortError() = nil")
	}

	var regErr *RegistryError
	if !errors.As(result, &regErr) {
		t.Error("result should be *RegistryError")
	}

	if !strings.Contains(result.Error(), "port 5000 is already in use") {
		t.Errorf("error message = %q, should contain 'port 5000 is already in use'", result.Error())
	}
}

// TestGetDataDirVariousPaths tests GetDataDir with various paths
func TestGetDataDirVariousPaths(t *testing.T) {
	tests := []struct {
		name     string
		dataRoot string
		want     string
	}{
		{
			name:     "simple path",
			dataRoot: "/data",
			want:     "/data/registry",
		},
		{
			name:     "home path",
			dataRoot: "/home/user/.local/share/bootc-man",
			want:     "/home/user/.local/share/bootc-man/registry",
		},
		{
			name:     "var path",
			dataRoot: "/var/lib/bootc-man",
			want:     "/var/lib/bootc-man/registry",
		},
		{
			name:     "current directory",
			dataRoot: ".",
			want:     "registry",
		},
	}

	svc := &Service{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.GetDataDir(tt.dataRoot)
			if got != tt.want {
				t.Errorf("GetDataDir(%q) = %q, want %q", tt.dataRoot, got, tt.want)
			}
		})
	}
}

// TestGetRegistryURLVariousPorts tests GetRegistryURL with various ports
func TestGetRegistryURLVariousPorts(t *testing.T) {
	tests := []struct {
		port int
		want string
	}{
		{5000, "localhost:5000"},
		{5001, "localhost:5001"},
		{8080, "localhost:8080"},
		{443, "localhost:443"},
		{80, "localhost:80"},
		{65535, "localhost:65535"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			svc := &Service{
				config: &config.RegistryConfig{Port: tt.port},
			}
			got := svc.GetRegistryURL()
			if got != tt.want {
				t.Errorf("GetRegistryURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLogsFollowFlag tests Logs with follow flag variations in dry-run mode
func TestLogsFollowFlag(t *testing.T) {
	svc := &Service{
		config:        &config.RegistryConfig{Port: 5000},
		dryRun:        true,
		containerName: "test",
	}

	ctx := context.Background()

	// Test with follow=true
	reader, err := svc.Logs(ctx, true)
	if err != nil {
		t.Fatalf("Logs(follow=true) error = %v", err)
	}
	if reader != nil {
		t.Error("Logs() should return nil reader in dry-run mode")
	}

	// Test with follow=false
	reader, err = svc.Logs(ctx, false)
	if err != nil {
		t.Fatalf("Logs(follow=false) error = %v", err)
	}
	if reader != nil {
		t.Error("Logs() should return nil reader in dry-run mode")
	}
}
