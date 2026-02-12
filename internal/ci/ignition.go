package ci

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IgnitionConfig represents an Ignition configuration
type IgnitionConfig struct {
	Ignition struct {
		Version string `json:"version"`
	} `json:"ignition"`
	Passwd struct {
		Users []IgnitionUser `json:"users,omitempty"`
	} `json:"passwd,omitempty"`
	Storage struct {
		Files []IgnitionFile `json:"files,omitempty"`
	} `json:"storage,omitempty"`
}

// IgnitionUser represents a user in Ignition config
type IgnitionUser struct {
	Name              string   `json:"name"`
	SSHAuthorizedKeys []string `json:"sshAuthorizedKeys,omitempty"`
	UID               *int     `json:"uid,omitempty"`
}

// IgnitionFile represents a file in Ignition config
type IgnitionFile struct {
	Node struct {
		Path  string `json:"path"`
		User  struct {
			Name string `json:"name,omitempty"`
		} `json:"user,omitempty"`
		Group struct {
			Name string `json:"name,omitempty"`
		} `json:"group,omitempty"`
		Mode *int `json:"mode,omitempty"`
	} `json:"node"`
	FileEmbedded1 struct {
		Contents struct {
			Source string `json:"source"`
		} `json:"contents"`
	} `json:"fileEmbedded1"`
}

// GenerateIgnitionConfig generates an Ignition config file with SSH keys
func GenerateIgnitionConfig(sshPublicKey string, username string) (*IgnitionConfig, error) {
	// Read SSH public key from file or use provided string
	var sshKey string
	if sshPublicKey != "" {
		// Check if it's a file path
		if _, err := os.Stat(sshPublicKey); err == nil {
			data, err := os.ReadFile(sshPublicKey)
			if err != nil {
				return nil, fmt.Errorf("failed to read SSH key file: %w", err)
			}
			sshKey = strings.TrimSpace(string(data))
		} else {
			// Assume it's the key content itself
			sshKey = strings.TrimSpace(sshPublicKey)
		}
	} else {
		// Try to get SSH key from default location
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		
		// Try common SSH key locations
		sshKeyPaths := []string{
			filepath.Join(homeDir, ".ssh", "id_ed25519.pub"),
			filepath.Join(homeDir, ".ssh", "id_rsa.pub"),
			filepath.Join(homeDir, ".ssh", "id_ecdsa.pub"),
		}
		
		for _, keyPath := range sshKeyPaths {
			if data, err := os.ReadFile(keyPath); err == nil {
				sshKey = strings.TrimSpace(string(data))
				break
			}
		}
		
		if sshKey == "" {
			return nil, fmt.Errorf("no SSH public key found. Please specify one or ensure ~/.ssh/id_ed25519.pub exists")
		}
	}

	config := &IgnitionConfig{}
	config.Ignition.Version = "3.4.0"

	// Add user with SSH key
	uid := 0
	if username == "root" {
		uid = 0
	} else {
		// For non-root users, use a default UID
		uid = 1000
	}

	config.Passwd.Users = []IgnitionUser{
		{
			Name:              username,
			SSHAuthorizedKeys: []string{sshKey},
			UID:               &uid,
		},
	}

	return config, nil
}

// WriteIgnitionConfig writes an Ignition config to a file
func WriteIgnitionConfig(config *IgnitionConfig, path string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal Ignition config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write Ignition config: %w", err)
	}

	return nil
}

// GetSSHPublicKey gets the SSH public key from the default location
func GetSSHPublicKey() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Try user's SSH keys first
	sshKeyPaths := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519.pub"),
		filepath.Join(homeDir, ".ssh", "id_rsa.pub"),
		filepath.Join(homeDir, ".ssh", "id_ecdsa.pub"),
	}

	for _, keyPath := range sshKeyPaths {
		if data, err := os.ReadFile(keyPath); err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}

	return "", fmt.Errorf("no SSH public key found. Please ensure ~/.ssh/id_ed25519.pub or ~/.ssh/id_rsa.pub exists")
}

// ValidateIgnitionFile validates an Ignition config file using ignition-validate if available
func ValidateIgnitionFile(path string) error {
	// Try to use ignition-validate if available
	if _, err := exec.LookPath("ignition-validate"); err == nil {
		cmd := exec.Command("ignition-validate", path)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ignition config validation failed: %w", err)
		}
	}
	// If ignition-validate is not available, just check if file is valid JSON
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read Ignition config: %w", err)
	}
	
	var config IgnitionConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("invalid Ignition config JSON: %w", err)
	}
	
	return nil
}
