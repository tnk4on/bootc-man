package main

import (
	"encoding/json"
	"testing"
)

func TestStatusCommandMetadata(t *testing.T) {
	if statusCmd.Use != "status" {
		t.Errorf("statusCmd.Use = %q, want %q", statusCmd.Use, "status")
	}

	if statusCmd.Short == "" {
		t.Error("statusCmd.Short should not be empty")
	}

	if statusCmd.Long == "" {
		t.Error("statusCmd.Long should not be empty")
	}
}

func TestServiceStatusSerialization(t *testing.T) {
	status := ServiceStatus{
		Name:    "registry",
		Status:  "running",
		Port:    5000,
		Message: "Healthy",
	}

	// Marshal to JSON
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("failed to marshal ServiceStatus: %v", err)
	}

	// Unmarshal back
	var decoded ServiceStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ServiceStatus: %v", err)
	}

	if decoded.Name != "registry" {
		t.Errorf("Name = %q, want %q", decoded.Name, "registry")
	}
	if decoded.Status != "running" {
		t.Errorf("Status = %q, want %q", decoded.Status, "running")
	}
	if decoded.Port != 5000 {
		t.Errorf("Port = %d, want %d", decoded.Port, 5000)
	}
}

func TestVMStatusSerialization(t *testing.T) {
	status := VMStatus{
		Name:     "test-vm",
		State:    "running",
		Pipeline: "my-pipeline",
		SSHHost:  "localhost",
		SSHPort:  2222,
		SSHUser:  "user",
		Message:  "Ready",
	}

	// Marshal to JSON
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("failed to marshal VMStatus: %v", err)
	}

	// Unmarshal back
	var decoded VMStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal VMStatus: %v", err)
	}

	if decoded.Name != "test-vm" {
		t.Errorf("Name = %q, want %q", decoded.Name, "test-vm")
	}
	if decoded.State != "running" {
		t.Errorf("State = %q, want %q", decoded.State, "running")
	}
	if decoded.SSHPort != 2222 {
		t.Errorf("SSHPort = %d, want %d", decoded.SSHPort, 2222)
	}
}

func TestOverallStatusSerialization(t *testing.T) {
	status := OverallStatus{
		Platform: "darwin/arm64",
		Services: []ServiceStatus{
			{Name: "registry", Status: "running", Port: 5000},
			{Name: "gui", Status: "stopped"},
		},
		VMs: []VMStatus{
			{Name: "test-vm", State: "running"},
		},
		Podman: PodmanStatus{
			Available: true,
			Version:   "4.5.0",
			Rootless:  true,
		},
		PodmanMachine: &PodmanMachineStatus{
			Running: true,
			Name:    "podman-machine-default",
			CPUs:    "4",
			Memory:  "8GB",
		},
		CITools: []CIToolStatus{
			{Name: "hadolint", Status: "available"},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("failed to marshal OverallStatus: %v", err)
	}

	// Unmarshal back
	var decoded OverallStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal OverallStatus: %v", err)
	}

	if decoded.Platform != "darwin/arm64" {
		t.Errorf("Platform = %q, want %q", decoded.Platform, "darwin/arm64")
	}
	if len(decoded.Services) != 2 {
		t.Errorf("len(Services) = %d, want %d", len(decoded.Services), 2)
	}
	if len(decoded.VMs) != 1 {
		t.Errorf("len(VMs) = %d, want %d", len(decoded.VMs), 1)
	}
	if !decoded.Podman.Available {
		t.Error("Podman.Available should be true")
	}
	if decoded.PodmanMachine == nil {
		t.Fatal("PodmanMachine should not be nil")
	}
	if !decoded.PodmanMachine.Running {
		t.Error("PodmanMachine.Running should be true")
	}
}

func TestPodmanStatusSerialization(t *testing.T) {
	status := PodmanStatus{
		Available: true,
		Version:   "4.8.0",
		Rootless:  false,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("failed to marshal PodmanStatus: %v", err)
	}

	var decoded PodmanStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal PodmanStatus: %v", err)
	}

	if !decoded.Available {
		t.Error("Available should be true")
	}
	if decoded.Version != "4.8.0" {
		t.Errorf("Version = %q, want %q", decoded.Version, "4.8.0")
	}
	if decoded.Rootless {
		t.Error("Rootless should be false")
	}
}

func TestPodmanMachineStatusSerialization(t *testing.T) {
	status := PodmanMachineStatus{
		Running: true,
		Name:    "podman-machine-default",
		CPUs:    "8",
		Memory:  "16GB",
		Disk:    "100GB",
		Rootful: "true",
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("failed to marshal PodmanMachineStatus: %v", err)
	}

	var decoded PodmanMachineStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal PodmanMachineStatus: %v", err)
	}

	if !decoded.Running {
		t.Error("Running should be true")
	}
	if decoded.Name != "podman-machine-default" {
		t.Errorf("Name = %q, want %q", decoded.Name, "podman-machine-default")
	}
	if decoded.CPUs != "8" {
		t.Errorf("CPUs = %q, want %q", decoded.CPUs, "8")
	}
}

func TestCIToolStatusSerialization(t *testing.T) {
	status := CIToolStatus{
		Name:       "trivy",
		Status:     "available",
		Image:      "aquasec/trivy:latest",
		Version:    "0.45.0",
		Privileged: false,
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("failed to marshal CIToolStatus: %v", err)
	}

	var decoded CIToolStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal CIToolStatus: %v", err)
	}

	if decoded.Name != "trivy" {
		t.Errorf("Name = %q, want %q", decoded.Name, "trivy")
	}
	if decoded.Status != "available" {
		t.Errorf("Status = %q, want %q", decoded.Status, "available")
	}
	if decoded.Image != "aquasec/trivy:latest" {
		t.Errorf("Image = %q, want %q", decoded.Image, "aquasec/trivy:latest")
	}
}
