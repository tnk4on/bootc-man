// Package testutil provides testing utilities for bootc-man
package testutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TempDir creates a temporary directory for testing and returns a cleanup function.
// The directory is automatically cleaned up when the test finishes.
func TempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// WriteFile creates a file with the given content in the specified directory.
// Returns the full path to the created file.
func WriteFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
	return path
}

// CreateDir creates a directory at the specified path.
func CreateDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("failed to create directory %s: %v", path, err)
	}
}

// ReadFile reads a file and returns its content.
func ReadFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	return string(content)
}

// FileExists checks if a file exists.
func FileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// SetEnv sets an environment variable and returns a cleanup function.
// The original value is restored when the cleanup function is called.
func SetEnv(t *testing.T, key, value string) {
	t.Helper()
	originalValue, hadValue := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("failed to set env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadValue {
			os.Setenv(key, originalValue)
		} else {
			os.Unsetenv(key)
		}
	})
}

// UnsetEnv unsets an environment variable and returns a cleanup function.
// The original value is restored when the cleanup function is called.
func UnsetEnv(t *testing.T, key string) {
	t.Helper()
	originalValue, hadValue := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("failed to unset env %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadValue {
			os.Setenv(key, originalValue)
		}
	})
}

// Chdir changes the current working directory and returns a cleanup function.
// The original directory is restored when the cleanup function is called.
func Chdir(t *testing.T, dir string) {
	t.Helper()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change directory to %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("failed to restore directory to %s: %v", originalDir, err)
		}
	})
}

// AssertNoError fails the test if err is not nil.
func AssertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// AssertError fails the test if err is nil.
func AssertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// AssertEqual fails the test if got != want.
func AssertEqual(t *testing.T, got, want interface{}) {
	t.Helper()
	if got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// AssertContains fails the test if s does not contain substr.
func AssertContains(t *testing.T, s, substr string) {
	t.Helper()
	if len(substr) == 0 {
		return
	}
	if len(s) < len(substr) {
		t.Fatalf("string %q does not contain %q", s, substr)
		return
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return
		}
	}
	t.Fatalf("string %q does not contain %q", s, substr)
}

// WriteFileToPath writes content to a file at the given path.
// Creates parent directories if they don't exist.
// This is a standalone helper that doesn't require a testing.T.
func WriteFileToPath(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// NowUnixNano returns the current time in nanoseconds.
// Useful for generating unique identifiers in tests.
func NowUnixNano() int64 {
	return time.Now().UnixNano()
}
