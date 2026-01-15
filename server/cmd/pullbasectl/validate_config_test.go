package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunValidateConfig(t *testing.T) {
	t.Run("requires file flag", func(t *testing.T) {
		err := runValidateConfig([]string{})
		if err == nil {
			t.Error("expected error when --file not provided")
		}
		if !strings.Contains(err.Error(), "--file is required") {
			t.Errorf("expected '--file is required' error, got: %v", err)
		}
	})

	t.Run("rejects invalid output format", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(tmpFile, []byte("packages: []"), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := runValidateConfig([]string{"--file", tmpFile, "--output", "xml"})
		if err == nil {
			t.Error("expected error for invalid output format")
		}
		if !strings.Contains(err.Error(), "--output must be") {
			t.Errorf("expected output format error, got: %v", err)
		}
	})

	t.Run("valid config succeeds", func(t *testing.T) {
		config := `
serverMetadata:
  name: test
packages:
  - name: nginx
    state: present
services:
  - name: nginx
    state: running
`
		tmpFile := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(tmpFile, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := runValidateConfig([]string{"--file", tmpFile})
		if err != nil {
			t.Errorf("expected valid config to succeed, got: %v", err)
		}
	})

	t.Run("invalid config fails with validation error", func(t *testing.T) {
		config := `
packages:
  - name: nginx
    state: invalid-state
`
		tmpFile := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(tmpFile, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := runValidateConfig([]string{"--file", tmpFile})
		if err == nil {
			t.Error("expected error for invalid config")
		}
		if !strings.Contains(err.Error(), "validation failed") {
			t.Errorf("expected 'validation failed' error, got: %v", err)
		}
	})

	t.Run("nonexistent file fails", func(t *testing.T) {
		err := runValidateConfig([]string{"--file", "/nonexistent/path/config.yaml"})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
		if !strings.Contains(err.Error(), "failed to read file") {
			t.Errorf("expected 'failed to read file' error, got: %v", err)
		}
	})

	t.Run("json output format accepted", func(t *testing.T) {
		config := `
packages:
  - name: nginx
    state: present
`
		tmpFile := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(tmpFile, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := runValidateConfig([]string{"--file", tmpFile, "--output", "json"})
		if err != nil {
			t.Errorf("expected json output to work, got: %v", err)
		}
	})

	t.Run("table output format accepted", func(t *testing.T) {
		config := `
packages:
  - name: nginx
    state: present
`
		tmpFile := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(tmpFile, []byte(config), 0644); err != nil {
			t.Fatalf("failed to write temp file: %v", err)
		}

		err := runValidateConfig([]string{"--file", tmpFile, "--output", "table"})
		if err != nil {
			t.Errorf("expected table output to work, got: %v", err)
		}
	})
}
