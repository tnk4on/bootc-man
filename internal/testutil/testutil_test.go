package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTempDir(t *testing.T) {
	dir := TempDir(t)
	if dir == "" {
		t.Fatal("TempDir returned empty string")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("TempDir directory does not exist: %s", dir)
	}
}

func TestWriteFile(t *testing.T) {
	dir := TempDir(t)
	content := "test content"
	path := WriteFile(t, dir, "test.txt", content)

	if !filepath.IsAbs(path) {
		t.Errorf("WriteFile returned non-absolute path: %s", path)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}
}

func TestWriteFileSubdirectory(t *testing.T) {
	dir := TempDir(t)
	content := "nested content"
	path := WriteFile(t, dir, "sub/dir/test.txt", content)

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}
}

func TestCreateDir(t *testing.T) {
	dir := TempDir(t)
	subDir := filepath.Join(dir, "subdir", "nested")

	CreateDir(t, subDir)

	info, err := os.Stat(subDir)
	if err != nil {
		t.Fatalf("CreateDir failed: %v", err)
	}

	if !info.IsDir() {
		t.Error("CreateDir did not create a directory")
	}
}

func TestReadFile(t *testing.T) {
	dir := TempDir(t)
	content := "read test"
	path := WriteFile(t, dir, "read.txt", content)

	got := ReadFile(t, path)
	if got != content {
		t.Errorf("ReadFile() = %q, want %q", got, content)
	}
}

func TestFileExists(t *testing.T) {
	dir := TempDir(t)
	path := WriteFile(t, dir, "exists.txt", "content")

	if !FileExists(t, path) {
		t.Error("FileExists() = false for existing file")
	}

	if FileExists(t, filepath.Join(dir, "nonexistent.txt")) {
		t.Error("FileExists() = true for nonexistent file")
	}
}

func TestSetEnv(t *testing.T) {
	key := "TESTUTIL_TEST_VAR"
	value := "test_value"

	// Ensure env is not set initially
	os.Unsetenv(key)

	SetEnv(t, key, value)

	got := os.Getenv(key)
	if got != value {
		t.Errorf("SetEnv: env = %q, want %q", got, value)
	}
}

func TestSetEnvPreservesOriginal(t *testing.T) {
	key := "TESTUTIL_PRESERVE_VAR"
	original := "original_value"
	newValue := "new_value"

	os.Setenv(key, original)
	defer os.Unsetenv(key)

	// Create a sub-test to use t.Cleanup
	t.Run("subtest", func(t *testing.T) {
		SetEnv(t, key, newValue)

		got := os.Getenv(key)
		if got != newValue {
			t.Errorf("SetEnv: env = %q, want %q", got, newValue)
		}
	})

	// After subtest, original should be restored
	got := os.Getenv(key)
	if got != original {
		t.Errorf("After cleanup: env = %q, want %q", got, original)
	}
}

func TestUnsetEnv(t *testing.T) {
	key := "TESTUTIL_UNSET_VAR"
	os.Setenv(key, "value")
	defer os.Unsetenv(key)

	t.Run("subtest", func(t *testing.T) {
		UnsetEnv(t, key)

		if val, ok := os.LookupEnv(key); ok {
			t.Errorf("UnsetEnv: env still set to %q", val)
		}
	})

	// After subtest, original should be restored
	if val, ok := os.LookupEnv(key); !ok || val != "value" {
		t.Errorf("After cleanup: env = %q, want %q", val, "value")
	}
}

func TestChdir(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}

	tempDir := TempDir(t)

	// Resolve symlinks for comparison (macOS /var -> /private/var)
	tempDirResolved, err := filepath.EvalSymlinks(tempDir)
	if err != nil {
		t.Fatalf("failed to resolve temp dir symlinks: %v", err)
	}

	t.Run("subtest", func(t *testing.T) {
		Chdir(t, tempDir)

		currentDir, err := os.Getwd()
		if err != nil {
			t.Fatalf("failed to get current directory: %v", err)
		}

		// Resolve symlinks for comparison
		currentDirResolved, err := filepath.EvalSymlinks(currentDir)
		if err != nil {
			t.Fatalf("failed to resolve current dir symlinks: %v", err)
		}

		if currentDirResolved != tempDirResolved {
			t.Errorf("Chdir: cwd = %q, want %q", currentDirResolved, tempDirResolved)
		}
	})

	// After subtest, original should be restored
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}

	if currentDir != originalDir {
		t.Errorf("After cleanup: cwd = %q, want %q", currentDir, originalDir)
	}
}

func TestAssertNoError(t *testing.T) {
	// This should not fail
	AssertNoError(t, nil)
}

func TestAssertError(t *testing.T) {
	// This should not fail
	AssertError(t, os.ErrNotExist)
}

func TestAssertEqual(t *testing.T) {
	// These should not fail
	AssertEqual(t, "a", "a")
	AssertEqual(t, 1, 1)
	AssertEqual(t, true, true)
}

func TestAssertContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
	}{
		{"substring at start", "hello world", "hello"},
		{"substring at end", "hello world", "world"},
		{"substring in middle", "hello world", "lo wo"},
		{"empty substring", "hello", ""},
		{"full string", "hello", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			AssertContains(t, tt.s, tt.substr)
		})
	}
}

func TestSamplePipelineYAML(t *testing.T) {
	yaml := SamplePipelineYAML()
	if yaml == "" {
		t.Error("SamplePipelineYAML returned empty string")
	}
	if !containsString(yaml, "apiVersion: bootc-man/v1") {
		t.Error("SamplePipelineYAML missing apiVersion")
	}
	if !containsString(yaml, "kind: Pipeline") {
		t.Error("SamplePipelineYAML missing kind")
	}
}

func TestSampleContainerfile(t *testing.T) {
	cf := SampleContainerfile()
	if cf == "" {
		t.Error("SampleContainerfile returned empty string")
	}
	if !containsString(cf, "FROM") {
		t.Error("SampleContainerfile missing FROM instruction")
	}
}

func TestSampleBootcStatusJSON(t *testing.T) {
	json := SampleBootcStatusJSON()
	if json == "" {
		t.Error("SampleBootcStatusJSON returned empty string")
	}
	if !containsString(json, "apiVersion") {
		t.Error("SampleBootcStatusJSON missing apiVersion")
	}
	if !containsString(json, "BootcHost") {
		t.Error("SampleBootcStatusJSON missing BootcHost kind")
	}
}

func TestSetupPipelineTestDir(t *testing.T) {
	dir := SetupPipelineTestDir(t)

	// Check Containerfile exists
	containerfilePath := filepath.Join(dir, "Containerfile")
	if !FileExists(t, containerfilePath) {
		t.Error("SetupPipelineTestDir did not create Containerfile")
	}

	// Check config.toml exists
	configPath := filepath.Join(dir, "config.toml")
	if !FileExists(t, configPath) {
		t.Error("SetupPipelineTestDir did not create config.toml")
	}
}

func TestSetupPipelineTestDirWithYAML(t *testing.T) {
	yaml := SamplePipelineYAML()
	dir := SetupPipelineTestDirWithYAML(t, yaml)

	// Check pipeline YAML exists
	yamlPath := filepath.Join(dir, "bootc-ci.yaml")
	if !FileExists(t, yamlPath) {
		t.Error("SetupPipelineTestDirWithYAML did not create bootc-ci.yaml")
	}

	// Verify content
	content := ReadFile(t, yamlPath)
	if content != yaml {
		t.Error("bootc-ci.yaml content does not match input")
	}
}

// containsString is a helper to check if a string contains a substring
func containsString(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
