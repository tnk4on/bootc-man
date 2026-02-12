package ci

// PodmanMachineConfig represents recommended Podman Machine settings
type PodmanMachineConfig struct {
	CPUs    int
	Memory  int // MB
	Disk    int // GB
	Rootful bool
}

// RecommendedMachineConfig returns recommended settings for bootc CI
func RecommendedMachineConfig() PodmanMachineConfig {
	return PodmanMachineConfig{
		CPUs:    4,
		Memory:  8192,
		Disk:    100,
		Rootful: true,
	}
}

// MinimumMachineConfig returns minimum settings for bootc CI
func MinimumMachineConfig() PodmanMachineConfig {
	return PodmanMachineConfig{
		CPUs:    2,
		Memory:  4096,
		Disk:    50,
		Rootful: true,
	}
}
