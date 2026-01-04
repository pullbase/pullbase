package main

import (
	"strings"
	"testing"
)

func TestRunServersListValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing server-url",
			args:    []string{},
			wantErr: "--server-url is required",
		},
		{
			name:    "invalid output format",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--output", "xml"},
			wantErr: "--output must be 'table' or 'json'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runServersList(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunServersCreateValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing server-url",
			args:    []string{},
			wantErr: "--server-url is required",
		},
		{
			name:    "missing id",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test"},
			wantErr: "--id is required",
		},
		{
			name:    "missing name",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--id", "srv-1"},
			wantErr: "--name is required",
		},
		{
			name:    "invalid output format",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--id", "srv-1", "--name", "Server 1", "--output", "yaml"},
			wantErr: "--output must be 'table' or 'json'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runServersCreate(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunServersGetValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing server-url",
			args:    []string{},
			wantErr: "--server-url is required",
		},
		{
			name:    "missing id",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test"},
			wantErr: "--id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runServersGet(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunServersDeleteValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing server-url",
			args:    []string{},
			wantErr: "--server-url is required",
		},
		{
			name:    "missing id",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--force"},
			wantErr: "--id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runServersDelete(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunServersInstallScriptValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing server-url",
			args:    []string{},
			wantErr: "--server-url is required",
		},
		{
			name:    "missing id",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test"},
			wantErr: "--id is required",
		},
		{
			name:    "missing token",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--id", "srv-1"},
			wantErr: "--token is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runServersInstallScript(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
