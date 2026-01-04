package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pullbase/pullbase/server/pkg/database"
)

func mockHealthyDBCheck() *database.HealthCheck {
	return &database.HealthCheck{
		Latency: 5 * time.Millisecond,
		Healthy: true,
		Error:   nil,
	}
}

func mockHealthyMigration() *database.MigrationStatus {
	return &database.MigrationStatus{
		Version: 21,
		Dirty:   false,
		Error:   nil,
	}
}

func TestHealthStatusConstants(t *testing.T) {
	t.Run("HealthStatusHealthy value", func(t *testing.T) {
		if HealthStatusHealthy != "healthy" {
			t.Errorf("expected HealthStatusHealthy to be 'healthy', got '%s'", HealthStatusHealthy)
		}
	})

	t.Run("HealthStatusDegraded value", func(t *testing.T) {
		if HealthStatusDegraded != "degraded" {
			t.Errorf("expected HealthStatusDegraded to be 'degraded', got '%s'", HealthStatusDegraded)
		}
	})

	t.Run("HealthStatusUnhealthy value", func(t *testing.T) {
		if HealthStatusUnhealthy != "unhealthy" {
			t.Errorf("expected HealthStatusUnhealthy to be 'unhealthy', got '%s'", HealthStatusUnhealthy)
		}
	})

	t.Run("HealthStatusUnknown value", func(t *testing.T) {
		if HealthStatusUnknown != "unknown" {
			t.Errorf("expected HealthStatusUnknown to be 'unknown', got '%s'", HealthStatusUnknown)
		}
	})
}

func TestHealthCheckResponseSerialization(t *testing.T) {
	t.Run("Healthy response serializes correctly", func(t *testing.T) {
		latency := int64(5)
		version := 21

		resp := HealthCheckResponse{
			Status:  HealthStatusHealthy,
			Service: "pullbase-server",
			Checks: map[string]CheckResult{
				"database": {
					Status:    HealthStatusHealthy,
					LatencyMs: &latency,
				},
				"migrations": {
					Status:  HealthStatusHealthy,
					Version: &version,
				},
			},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}

		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}

		if parsed["status"] != "healthy" {
			t.Errorf("expected status 'healthy', got '%v'", parsed["status"])
		}
		if parsed["service"] != "pullbase-server" {
			t.Errorf("expected service 'pullbase-server', got '%v'", parsed["service"])
		}

		checks, ok := parsed["checks"].(map[string]interface{})
		if !ok {
			t.Fatal("expected checks to be a map")
		}

		dbCheck, ok := checks["database"].(map[string]interface{})
		if !ok {
			t.Fatal("expected database check to be a map")
		}
		if dbCheck["status"] != "healthy" {
			t.Errorf("expected database status 'healthy', got '%v'", dbCheck["status"])
		}
	})

	t.Run("Degraded response serializes correctly", func(t *testing.T) {
		latency := int64(1500)

		resp := HealthCheckResponse{
			Status:  HealthStatusDegraded,
			Service: "pullbase-server",
			Checks: map[string]CheckResult{
				"database": {
					Status:    HealthStatusDegraded,
					LatencyMs: &latency,
				},
			},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}

		if !strings.Contains(string(data), `"status":"degraded"`) {
			t.Error("expected response to contain degraded status")
		}
	})

	t.Run("Unhealthy response with error serializes correctly", func(t *testing.T) {
		resp := HealthCheckResponse{
			Status:  HealthStatusUnhealthy,
			Service: "pullbase-server",
			Checks: map[string]CheckResult{
				"database": {
					Status: HealthStatusUnhealthy,
					Error:  "connection refused",
				},
			},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}

		if !strings.Contains(string(data), `"error":"connection refused"`) {
			t.Error("expected response to contain error message")
		}
	})
}

func TestHealthCheckConstants(t *testing.T) {
	t.Run("healthCheckTimeout is 5 seconds", func(t *testing.T) {
		if healthCheckTimeout != 5*time.Second {
			t.Errorf("expected healthCheckTimeout to be 5s, got %v", healthCheckTimeout)
		}
	})

	t.Run("dbLatencyDegradedThres is 1 second", func(t *testing.T) {
		if dbLatencyDegradedThres != 1*time.Second {
			t.Errorf("expected dbLatencyDegradedThres to be 1s, got %v", dbLatencyDegradedThres)
		}
	})
}

// mockRepository implements a minimal repository for testing health handlers
type mockRepository struct {
	healthCheck     *database.HealthCheck
	migrationStatus *database.MigrationStatus
}

func (m *mockRepository) CheckHealth(ctx context.Context) *database.HealthCheck {
	if m.healthCheck != nil {
		return m.healthCheck
	}
	return mockHealthyDBCheck()
}

func (m *mockRepository) GetMigrationStatus(ctx context.Context) *database.MigrationStatus {
	if m.migrationStatus != nil {
		return m.migrationStatus
	}
	return mockHealthyMigration()
}

func TestHealthCheckHTTPStatus(t *testing.T) {
	t.Run("Healthy should return 200", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := HealthCheckResponse{
				Status:  HealthStatusHealthy,
				Service: "pullbase-server",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		})

		req := httptest.NewRequest("GET", "/healthz", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("Degraded should return 200", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := HealthCheckResponse{
				Status:  HealthStatusDegraded,
				Service: "pullbase-server",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(resp)
		})

		req := httptest.NewRequest("GET", "/healthz", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
	})

	t.Run("Unhealthy should return 503", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := HealthCheckResponse{
				Status:  HealthStatusUnhealthy,
				Service: "pullbase-server",
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(resp)
		})

		req := httptest.NewRequest("GET", "/healthz", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", rr.Code)
		}
	})
}

func TestAllowGitTokenRequestBackoff(t *testing.T) {
	api := &API{}
	serverID := "server-1"
	now := time.Now()

	if !api.allowGitTokenRequest(serverID, now) {
		t.Fatalf("expected first request to be allowed")
	}

	if api.allowGitTokenRequest(serverID, now.Add(2*time.Second)) {
		t.Fatalf("expected second rapid request to be denied")
	}

	api.gitTokenMu.Lock()
	backoff := api.gitTokenBackoff[serverID]
	until := api.gitTokenCooldownUntil[serverID]
	api.gitTokenMu.Unlock()

	if backoff <= gitTokenCooldown {
		t.Fatalf("expected backoff to increase beyond base cooldown")
	}

	if until.Sub(now) <= gitTokenCooldown {
		t.Fatalf("expected cooldown window to extend when rate limited")
	}

	// After waiting beyond current backoff, the next request should be allowed again.
	future := until.Add(100 * time.Millisecond)
	if !api.allowGitTokenRequest(serverID, future) {
		t.Fatalf("expected request after cooldown to be allowed")
	}

	api.gitTokenMu.Lock()
	resetBackoff := api.gitTokenBackoff[serverID]
	api.gitTokenMu.Unlock()

	if resetBackoff != gitTokenCooldown {
		t.Fatalf("expected backoff to reset to base cooldown after successful request")
	}
}

func TestRenderInstallScript(t *testing.T) {
	api := &API{}

	t.Run("basic script generation", func(t *testing.T) {
		data := installScriptData{
			AgentVersion: "v1.0.0",
			ServerURL:    "https://pullbase.example.com",
			AgentToken:   "pbt_abc123",
			ServerID:     "web-01",
		}

		script := api.renderInstallScript(data)

		if len(script) == 0 {
			t.Fatal("expected non-empty script")
		}
		if !strings.Contains(script, "#!/bin/bash") {
			t.Error("script should start with shebang")
		}
		if !strings.Contains(script, `AGENT_VERSION="v1.0.0"`) {
			t.Error("script should contain agent version")
		}
		if !strings.Contains(script, `SERVER_URL="https://pullbase.example.com"`) {
			t.Error("script should contain server URL")
		}
		if !strings.Contains(script, `AGENT_TOKEN="pbt_abc123"`) {
			t.Error("script should contain agent token")
		}
		if !strings.Contains(script, `SERVER_ID="web-01"`) {
			t.Error("script should contain server ID")
		}
		if !strings.Contains(script, "systemctl") {
			t.Error("script should reference systemctl")
		}
		if !strings.Contains(script, "pullbase-agent.service") {
			t.Error("script should create systemd service")
		}
	})

	t.Run("script with CA cert", func(t *testing.T) {
		data := installScriptData{
			AgentVersion: "latest",
			ServerURL:    "https://localhost:8080",
			AgentToken:   "pbt_xyz789",
			ServerID:     "test-server",
			CACert:       "-----BEGIN CERTIFICATE-----\nMIIBxxx\n-----END CERTIFICATE-----",
		}

		script := api.renderInstallScript(data)

		if !strings.Contains(script, "ca.crt") {
			t.Error("script should reference CA cert file when CACert provided")
		}
		if !strings.Contains(script, "CA_CERT_PATH") {
			t.Error("script should set CA_CERT_PATH env var when CACert provided")
		}
		if !strings.Contains(script, "-----BEGIN CERTIFICATE-----") {
			t.Error("script should embed the CA certificate")
		}
	})

	t.Run("script without CA cert excludes cert handling", func(t *testing.T) {
		data := installScriptData{
			AgentVersion: "latest",
			ServerURL:    "http://localhost:8080",
			AgentToken:   "pbt_test",
			ServerID:     "no-tls-server",
			CACert:       "",
		}

		script := api.renderInstallScript(data)

		if strings.Contains(script, "{{if .CACert}}") {
			t.Error("script should not contain template markers")
		}
		if strings.Contains(script, "{{end}}") {
			t.Error("script should not contain template markers")
		}
	})

	t.Run("latest version uses releases/latest URL", func(t *testing.T) {
		data := installScriptData{
			AgentVersion: "latest",
			ServerURL:    "https://test.com",
			AgentToken:   "pbt_token",
			ServerID:     "srv",
		}

		script := api.renderInstallScript(data)

		if !strings.Contains(script, "releases/latest/download") {
			t.Error("latest version should use releases/latest URL pattern")
		}
	})
}
