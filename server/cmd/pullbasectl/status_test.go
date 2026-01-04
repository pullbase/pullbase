package main

import (
	"strings"
	"testing"
	"time"
)

func TestRunStatusValidation(t *testing.T) {
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
			name:    "missing selection flag",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test"},
			wantErr: "one of --server-id, --environment-id, or --all is required",
		},
		{
			name:    "multiple selection flags",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--server-id", "srv-1", "--all"},
			wantErr: "only one of --server-id, --environment-id, or --all can be specified",
		},
		{
			name:    "server-id and environment-id",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--server-id", "srv-1", "--environment-id", "1"},
			wantErr: "only one of --server-id, --environment-id, or --all can be specified",
		},
		{
			name:    "invalid output format",
			args:    []string{"--server-url", "https://example.com", "--admin-token", "test", "--all", "--output", "xml"},
			wantErr: "--output must be 'table' or 'json'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runStatus(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestFormatRelativeTime(t *testing.T) {
	tests := []struct {
		name     string
		timeDiff time.Duration
		want     string
	}{
		{
			name:     "just now",
			timeDiff: 30 * time.Second,
			want:     "just now",
		},
		{
			name:     "1 minute ago",
			timeDiff: 90 * time.Second,
			want:     "1 minute ago",
		},
		{
			name:     "5 minutes ago",
			timeDiff: 5 * time.Minute,
			want:     "5 minutes ago",
		},
		{
			name:     "1 hour ago",
			timeDiff: 90 * time.Minute,
			want:     "1 hour ago",
		},
		{
			name:     "3 hours ago",
			timeDiff: 3 * time.Hour,
			want:     "3 hours ago",
		},
		{
			name:     "1 day ago",
			timeDiff: 30 * time.Hour,
			want:     "1 day ago",
		},
		{
			name:     "5 days ago",
			timeDiff: 5 * 24 * time.Hour,
			want:     "5 days ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testTime := time.Now().Add(-tt.timeDiff)
			got := formatRelativeTime(testTime)
			if got != tt.want {
				t.Errorf("formatRelativeTime() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyStatus(t *testing.T) {
	tests := []struct {
		name        string
		entry       statusEntry
		wantHealthy int
		wantDrifted int
		wantError   int
		wantUnknown int
	}{
		{
			name: "applied not drifted",
			entry: statusEntry{
				Status:    strPtr("Applied"),
				IsDrifted: boolPtr(false),
			},
			wantHealthy: 1,
		},
		{
			name: "applied but drifted",
			entry: statusEntry{
				Status:    strPtr("Applied"),
				IsDrifted: boolPtr(true),
			},
			wantDrifted: 1,
		},
		{
			name: "in sync",
			entry: statusEntry{
				Status:    strPtr("In Sync"),
				IsDrifted: boolPtr(false),
			},
			wantHealthy: 1,
		},
		{
			name: "syncing",
			entry: statusEntry{
				Status: strPtr("Syncing"),
			},
			wantHealthy: 1,
		},
		{
			name: "error",
			entry: statusEntry{
				Status: strPtr("Error"),
			},
			wantError: 1,
		},
		{
			name: "failed",
			entry: statusEntry{
				Status: strPtr("Failed"),
			},
			wantError: 1,
		},
		{
			name: "drifted status",
			entry: statusEntry{
				Status: strPtr("Drifted"),
			},
			wantDrifted: 1,
		},
		{
			name: "drift detected",
			entry: statusEntry{
				Status: strPtr("Drift Detected"),
			},
			wantDrifted: 1,
		},
		{
			name: "nil status",
			entry: statusEntry{
				Status: nil,
			},
			wantUnknown: 1,
		},
		{
			name: "unknown status string",
			entry: statusEntry{
				Status: strPtr("SomeUnknownStatus"),
			},
			wantUnknown: 1,
		},
		{
			name: "dry-run in sync",
			entry: statusEntry{
				Status:    strPtr("Dry-Run: In Sync"),
				IsDrifted: boolPtr(false),
			},
			wantHealthy: 1,
		},
		{
			name: "dry-run drift detected",
			entry: statusEntry{
				Status: strPtr("Dry-Run: Drift Detected"),
			},
			wantDrifted: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &fleetStatusResponse{}
			classifyStatus(tt.entry, status)

			if status.HealthyCount != tt.wantHealthy {
				t.Errorf("HealthyCount = %d, want %d", status.HealthyCount, tt.wantHealthy)
			}
			if status.DriftedCount != tt.wantDrifted {
				t.Errorf("DriftedCount = %d, want %d", status.DriftedCount, tt.wantDrifted)
			}
			if status.ErrorCount != tt.wantError {
				t.Errorf("ErrorCount = %d, want %d", status.ErrorCount, tt.wantError)
			}
			if status.UnknownCount != tt.wantUnknown {
				t.Errorf("UnknownCount = %d, want %d", status.UnknownCount, tt.wantUnknown)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
