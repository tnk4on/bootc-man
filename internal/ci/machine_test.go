package ci

import "testing"

func TestRecommendedMachineConfig(t *testing.T) {
	cfg := RecommendedMachineConfig()

	if cfg.CPUs != 4 {
		t.Errorf("RecommendedMachineConfig().CPUs = %d, want 4", cfg.CPUs)
	}
	if cfg.Memory != 8192 {
		t.Errorf("RecommendedMachineConfig().Memory = %d, want 8192", cfg.Memory)
	}
	if cfg.Disk != 100 {
		t.Errorf("RecommendedMachineConfig().Disk = %d, want 100", cfg.Disk)
	}
	if !cfg.Rootful {
		t.Errorf("RecommendedMachineConfig().Rootful = false, want true")
	}
}

func TestMinimumMachineConfig(t *testing.T) {
	cfg := MinimumMachineConfig()

	if cfg.CPUs != 2 {
		t.Errorf("MinimumMachineConfig().CPUs = %d, want 2", cfg.CPUs)
	}
	if cfg.Memory != 4096 {
		t.Errorf("MinimumMachineConfig().Memory = %d, want 4096", cfg.Memory)
	}
	if cfg.Disk != 50 {
		t.Errorf("MinimumMachineConfig().Disk = %d, want 50", cfg.Disk)
	}
	if !cfg.Rootful {
		t.Errorf("MinimumMachineConfig().Rootful = false, want true")
	}
}

func TestPodmanMachineConfigStruct(t *testing.T) {
	// Test struct field access
	cfg := PodmanMachineConfig{
		CPUs:    8,
		Memory:  16384,
		Disk:    200,
		Rootful: false,
	}

	if cfg.CPUs != 8 {
		t.Errorf("cfg.CPUs = %d, want 8", cfg.CPUs)
	}
	if cfg.Memory != 16384 {
		t.Errorf("cfg.Memory = %d, want 16384", cfg.Memory)
	}
	if cfg.Disk != 200 {
		t.Errorf("cfg.Disk = %d, want 200", cfg.Disk)
	}
	if cfg.Rootful {
		t.Errorf("cfg.Rootful = true, want false")
	}
}
