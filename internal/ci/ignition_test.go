package ci

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tnk4on/bootc-man/internal/testutil"
)

func TestIgnitionConfigSerialization(t *testing.T) {
	// Create a sample config
	uid := 1000
	config := &IgnitionConfig{}
	config.Ignition.Version = "3.4.0"
	config.Passwd.Users = []IgnitionUser{
		{
			Name:              "testuser",
			SSHAuthorizedKeys: []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... test@example.com"},
			UID:               &uid,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("failed to marshal IgnitionConfig: %v", err)
	}

	// Unmarshal back
	var decoded IgnitionConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal IgnitionConfig: %v", err)
	}

	// Verify fields
	if decoded.Ignition.Version != "3.4.0" {
		t.Errorf("Ignition.Version = %q, want %q", decoded.Ignition.Version, "3.4.0")
	}

	if len(decoded.Passwd.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(decoded.Passwd.Users))
	}

	user := decoded.Passwd.Users[0]
	if user.Name != "testuser" {
		t.Errorf("User.Name = %q, want %q", user.Name, "testuser")
	}
	if len(user.SSHAuthorizedKeys) != 1 {
		t.Errorf("expected 1 SSH key, got %d", len(user.SSHAuthorizedKeys))
	}
	if user.UID == nil || *user.UID != 1000 {
		t.Errorf("User.UID = %v, want 1000", user.UID)
	}
}

func TestGenerateIgnitionConfigWithProvidedKey(t *testing.T) {
	// Test with a directly provided SSH key string
	sshKey := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAITestKeyContent test@example.com"

	tests := []struct {
		name     string
		username string
		wantUID  int
	}{
		{
			name:     "root user",
			username: "root",
			wantUID:  0,
		},
		{
			name:     "regular user",
			username: "user",
			wantUID:  1000,
		},
		{
			name:     "custom username",
			username: "admin",
			wantUID:  1000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := GenerateIgnitionConfig(sshKey, tt.username)
			if err != nil {
				t.Fatalf("GenerateIgnitionConfig failed: %v", err)
			}

			if config.Ignition.Version != "3.4.0" {
				t.Errorf("Ignition.Version = %q, want %q", config.Ignition.Version, "3.4.0")
			}

			if len(config.Passwd.Users) != 1 {
				t.Fatalf("expected 1 user, got %d", len(config.Passwd.Users))
			}

			user := config.Passwd.Users[0]
			if user.Name != tt.username {
				t.Errorf("User.Name = %q, want %q", user.Name, tt.username)
			}
			if len(user.SSHAuthorizedKeys) != 1 {
				t.Errorf("expected 1 SSH key, got %d", len(user.SSHAuthorizedKeys))
			}
			if user.SSHAuthorizedKeys[0] != sshKey {
				t.Errorf("User.SSHAuthorizedKeys[0] = %q, want %q", user.SSHAuthorizedKeys[0], sshKey)
			}
			if user.UID == nil || *user.UID != tt.wantUID {
				t.Errorf("User.UID = %v, want %d", user.UID, tt.wantUID)
			}
		})
	}
}

func TestGenerateIgnitionConfigFromFile(t *testing.T) {
	// Create a temp directory with an SSH key file
	dir := testutil.TempDir(t)
	sshKeyContent := "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIFileKeyContent test@example.com"
	keyPath := testutil.WriteFile(t, dir, "id_ed25519.pub", sshKeyContent)

	config, err := GenerateIgnitionConfig(keyPath, "user")
	if err != nil {
		t.Fatalf("GenerateIgnitionConfig failed: %v", err)
	}

	if len(config.Passwd.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(config.Passwd.Users))
	}

	user := config.Passwd.Users[0]
	if user.SSHAuthorizedKeys[0] != sshKeyContent {
		t.Errorf("SSH key content mismatch: got %q, want %q", user.SSHAuthorizedKeys[0], sshKeyContent)
	}
}

func TestWriteIgnitionConfig(t *testing.T) {
	dir := testutil.TempDir(t)

	uid := 1000
	config := &IgnitionConfig{}
	config.Ignition.Version = "3.4.0"
	config.Passwd.Users = []IgnitionUser{
		{
			Name:              "testuser",
			SSHAuthorizedKeys: []string{"ssh-key-content"},
			UID:               &uid,
		},
	}

	outputPath := filepath.Join(dir, "config.ign")

	err := WriteIgnitionConfig(config, outputPath)
	if err != nil {
		t.Fatalf("WriteIgnitionConfig failed: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("Ignition config file was not created")
	}

	// Read and verify contents
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	var decoded IgnitionConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal output file: %v", err)
	}

	if decoded.Ignition.Version != "3.4.0" {
		t.Errorf("Ignition.Version = %q, want %q", decoded.Ignition.Version, "3.4.0")
	}
}

func TestValidateIgnitionFile(t *testing.T) {
	dir := testutil.TempDir(t)

	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid ignition config",
			content: `{
				"ignition": {"version": "3.4.0"},
				"passwd": {
					"users": [
						{
							"name": "user",
							"sshAuthorizedKeys": ["ssh-key"]
						}
					]
				}
			}`,
			wantErr: false,
		},
		{
			name:    "invalid JSON",
			content: `{not valid json}`,
			wantErr: true,
		},
		{
			name: "minimal valid config",
			content: `{
				"ignition": {"version": "3.4.0"}
			}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := testutil.WriteFile(t, dir, tt.name+".ign", tt.content)

			err := ValidateIgnitionFile(path)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateIgnitionFileNotFound(t *testing.T) {
	err := ValidateIgnitionFile("/nonexistent/path/config.ign")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestIgnitionUserSerialization(t *testing.T) {
	uid := 1000
	user := IgnitionUser{
		Name:              "testuser",
		SSHAuthorizedKeys: []string{"key1", "key2"},
		UID:               &uid,
	}

	data, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("failed to marshal IgnitionUser: %v", err)
	}

	var decoded IgnitionUser
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal IgnitionUser: %v", err)
	}

	if decoded.Name != "testuser" {
		t.Errorf("Name = %q, want %q", decoded.Name, "testuser")
	}
	if len(decoded.SSHAuthorizedKeys) != 2 {
		t.Errorf("expected 2 SSH keys, got %d", len(decoded.SSHAuthorizedKeys))
	}
	if decoded.UID == nil || *decoded.UID != 1000 {
		t.Errorf("UID = %v, want 1000", decoded.UID)
	}
}

func TestIgnitionFileSerialization(t *testing.T) {
	mode := 0644
	file := IgnitionFile{}
	file.Node.Path = "/etc/hostname"
	file.Node.User.Name = "root"
	file.Node.Group.Name = "root"
	file.Node.Mode = &mode
	file.FileEmbedded1.Contents.Source = "data:,testhost"

	data, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("failed to marshal IgnitionFile: %v", err)
	}

	var decoded IgnitionFile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal IgnitionFile: %v", err)
	}

	if decoded.Node.Path != "/etc/hostname" {
		t.Errorf("Node.Path = %q, want %q", decoded.Node.Path, "/etc/hostname")
	}
	if decoded.FileEmbedded1.Contents.Source != "data:,testhost" {
		t.Errorf("Contents.Source = %q, want %q", decoded.FileEmbedded1.Contents.Source, "data:,testhost")
	}
}
