package testutil

import (
	"context"
	"io"

	"github.com/tnk4on/bootc-man/internal/bootc"
	"github.com/tnk4on/bootc-man/internal/podman"
)

// MockPodmanClient is a mock implementation of podman operations for testing.
type MockPodmanClient struct {
	// Function hooks for mocking
	RunFunc           func(ctx context.Context, opts podman.RunOptions) (string, error)
	StartFunc         func(ctx context.Context, name string) error
	StopFunc          func(ctx context.Context, name string) error
	RemoveFunc        func(ctx context.Context, name string, force bool) error
	ExistsFunc        func(ctx context.Context, name string) (bool, error)
	InspectFunc       func(ctx context.Context, name string) (*podman.ContainerInfo, error)
	LogsFunc          func(ctx context.Context, name string, follow bool) (io.ReadCloser, error)
	PullFunc          func(ctx context.Context, image string) error
	BuildFunc         func(ctx context.Context, opts podman.BuildOptions) error
	PushFunc          func(ctx context.Context, image string, tlsVerify bool) error
	VolumeExistsFunc  func(ctx context.Context, name string) (bool, error)
	VolumeRemoveFunc  func(ctx context.Context, name string, force bool) error
	ImagesFunc        func(ctx context.Context, bootcOnly bool) ([]podman.ImageInfo, error)
	ImageRemoveFunc   func(ctx context.Context, image string, force bool) error
	ImageInspectFunc  func(ctx context.Context, image string) (*podman.ImageInspectInfo, error)
	InfoFunc          func(ctx context.Context) (*podman.PodmanInfo, error)

	// Call tracking
	Calls []MockCall
}

// MockCall records a mock function call
type MockCall struct {
	Method string
	Args   []interface{}
}

func (m *MockPodmanClient) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

// Run mocks podman run
func (m *MockPodmanClient) Run(ctx context.Context, opts podman.RunOptions) (string, error) {
	m.recordCall("Run", opts)
	if m.RunFunc != nil {
		return m.RunFunc(ctx, opts)
	}
	return "mock-container-id", nil
}

// Start mocks podman start
func (m *MockPodmanClient) Start(ctx context.Context, name string) error {
	m.recordCall("Start", name)
	if m.StartFunc != nil {
		return m.StartFunc(ctx, name)
	}
	return nil
}

// Stop mocks podman stop
func (m *MockPodmanClient) Stop(ctx context.Context, name string) error {
	m.recordCall("Stop", name)
	if m.StopFunc != nil {
		return m.StopFunc(ctx, name)
	}
	return nil
}

// Remove mocks podman rm
func (m *MockPodmanClient) Remove(ctx context.Context, name string, force bool) error {
	m.recordCall("Remove", name, force)
	if m.RemoveFunc != nil {
		return m.RemoveFunc(ctx, name, force)
	}
	return nil
}

// Exists mocks podman container exists
func (m *MockPodmanClient) Exists(ctx context.Context, name string) (bool, error) {
	m.recordCall("Exists", name)
	if m.ExistsFunc != nil {
		return m.ExistsFunc(ctx, name)
	}
	return false, nil
}

// Inspect mocks podman inspect
func (m *MockPodmanClient) Inspect(ctx context.Context, name string) (*podman.ContainerInfo, error) {
	m.recordCall("Inspect", name)
	if m.InspectFunc != nil {
		return m.InspectFunc(ctx, name)
	}
	return &podman.ContainerInfo{
		ID:    "mock-id",
		Name:  name,
		Image: "mock-image",
		State: podman.ContainerState{Running: false},
	}, nil
}

// Logs mocks podman logs
func (m *MockPodmanClient) Logs(ctx context.Context, name string, follow bool) (io.ReadCloser, error) {
	m.recordCall("Logs", name, follow)
	if m.LogsFunc != nil {
		return m.LogsFunc(ctx, name, follow)
	}
	return io.NopCloser(&mockReader{}), nil
}

// Pull mocks podman pull
func (m *MockPodmanClient) Pull(ctx context.Context, image string) error {
	m.recordCall("Pull", image)
	if m.PullFunc != nil {
		return m.PullFunc(ctx, image)
	}
	return nil
}

// Build mocks podman build
func (m *MockPodmanClient) Build(ctx context.Context, opts podman.BuildOptions) error {
	m.recordCall("Build", opts)
	if m.BuildFunc != nil {
		return m.BuildFunc(ctx, opts)
	}
	return nil
}

// Push mocks podman push
func (m *MockPodmanClient) Push(ctx context.Context, image string, tlsVerify bool) error {
	m.recordCall("Push", image, tlsVerify)
	if m.PushFunc != nil {
		return m.PushFunc(ctx, image, tlsVerify)
	}
	return nil
}

// VolumeExists mocks podman volume exists
func (m *MockPodmanClient) VolumeExists(ctx context.Context, name string) (bool, error) {
	m.recordCall("VolumeExists", name)
	if m.VolumeExistsFunc != nil {
		return m.VolumeExistsFunc(ctx, name)
	}
	return false, nil
}

// VolumeRemove mocks podman volume rm
func (m *MockPodmanClient) VolumeRemove(ctx context.Context, name string, force bool) error {
	m.recordCall("VolumeRemove", name, force)
	if m.VolumeRemoveFunc != nil {
		return m.VolumeRemoveFunc(ctx, name, force)
	}
	return nil
}

// Images mocks podman images
func (m *MockPodmanClient) Images(ctx context.Context, bootcOnly bool) ([]podman.ImageInfo, error) {
	m.recordCall("Images", bootcOnly)
	if m.ImagesFunc != nil {
		return m.ImagesFunc(ctx, bootcOnly)
	}
	return []podman.ImageInfo{}, nil
}

// ImageRemove mocks podman rmi
func (m *MockPodmanClient) ImageRemove(ctx context.Context, image string, force bool) error {
	m.recordCall("ImageRemove", image, force)
	if m.ImageRemoveFunc != nil {
		return m.ImageRemoveFunc(ctx, image, force)
	}
	return nil
}

// ImageInspect mocks podman image inspect
func (m *MockPodmanClient) ImageInspect(ctx context.Context, image string) (*podman.ImageInspectInfo, error) {
	m.recordCall("ImageInspect", image)
	if m.ImageInspectFunc != nil {
		return m.ImageInspectFunc(ctx, image)
	}
	return &podman.ImageInspectInfo{
		ID:     "mock-image-id",
		Labels: map[string]string{},
	}, nil
}

// Info mocks podman info
func (m *MockPodmanClient) Info(ctx context.Context) (*podman.PodmanInfo, error) {
	m.recordCall("Info")
	if m.InfoFunc != nil {
		return m.InfoFunc(ctx)
	}
	return &podman.PodmanInfo{
		Version:  "4.0.0",
		Rootless: true,
	}, nil
}

// mockReader is a simple io.Reader that returns empty data
type mockReader struct{}

func (r *mockReader) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

// MockBootcDriver is a mock implementation of bootc.Driver for testing.
type MockBootcDriver struct {
	// Function hooks for mocking
	UpgradeFunc  func(ctx context.Context, opts bootc.UpgradeOptions) error
	SwitchFunc   func(ctx context.Context, image string, opts bootc.SwitchOptions) error
	RollbackFunc func(ctx context.Context, opts bootc.RollbackOptions) error
	StatusFunc   func(ctx context.Context) (*bootc.Status, error)

	// Call tracking
	Calls []MockCall
}

func (m *MockBootcDriver) recordCall(method string, args ...interface{}) {
	m.Calls = append(m.Calls, MockCall{Method: method, Args: args})
}

// Upgrade mocks bootc upgrade
func (m *MockBootcDriver) Upgrade(ctx context.Context, opts bootc.UpgradeOptions) error {
	m.recordCall("Upgrade", opts)
	if m.UpgradeFunc != nil {
		return m.UpgradeFunc(ctx, opts)
	}
	return nil
}

// Switch mocks bootc switch
func (m *MockBootcDriver) Switch(ctx context.Context, image string, opts bootc.SwitchOptions) error {
	m.recordCall("Switch", image, opts)
	if m.SwitchFunc != nil {
		return m.SwitchFunc(ctx, image, opts)
	}
	return nil
}

// Rollback mocks bootc rollback
func (m *MockBootcDriver) Rollback(ctx context.Context, opts bootc.RollbackOptions) error {
	m.recordCall("Rollback", opts)
	if m.RollbackFunc != nil {
		return m.RollbackFunc(ctx, opts)
	}
	return nil
}

// Status mocks bootc status
func (m *MockBootcDriver) Status(ctx context.Context) (*bootc.Status, error) {
	m.recordCall("Status")
	if m.StatusFunc != nil {
		return m.StatusFunc(ctx)
	}
	return &bootc.Status{
		APIVersion: "org.containers.bootc/v1",
		Kind:       "BootcHost",
		Metadata:   bootc.Metadata{Name: "host"},
		Spec:       bootc.Spec{},
		Status: bootc.HostStatus{
			Booted: &bootc.BootEntry{
				Image: &bootc.ImageStatus{
					Image: bootc.ImageDetails{
						Image:     TestBootcImageCurrent(),
						Transport: "registry",
					},
					Version:     TestBootcImageTagCurrent + ".20240101.0",
					ImageDigest: "sha256:abc123",
				},
			},
		},
	}, nil
}

// Verify MockBootcDriver implements bootc.Driver
var _ bootc.Driver = (*MockBootcDriver)(nil)
