package main

import (
	"strings"
	"testing"
)

func TestRunEnvironmentsListValidation(t *testing.T) {
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
			err := runEnvironmentsList(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunEnvironmentsCreateValidation(t *testing.T) {
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
			name:    "missing name",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test"},
			wantErr: "--name is required",
		},
		{
			name:    "missing repo-url",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--name", "production"},
			wantErr: "--repo-url is required",
		},
		{
			name:    "invalid output format",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--name", "production", "--repo-url", "https://github.com/org/repo", "--output", "yaml"},
			wantErr: "--output must be 'table' or 'json'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runEnvironmentsCreate(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunEnvironmentsGetValidation(t *testing.T) {
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
			name:    "invalid id zero",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--id", "0"},
			wantErr: "--id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runEnvironmentsGet(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunEnvironmentsDeleteValidation(t *testing.T) {
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
			err := runEnvironmentsDelete(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunEnvironmentsRollbackValidation(t *testing.T) {
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
			name:    "missing commit",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--id", "1"},
			wantErr: "--commit is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runEnvironmentsRollback(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRunEnvironmentsRollbackListValidation(t *testing.T) {
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
			err := runEnvironmentsRollbackList(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
