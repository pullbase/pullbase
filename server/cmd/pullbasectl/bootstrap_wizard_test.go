package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pullbase/pullbase/server/pkg/githubapp"
)

type stubGitHubClient struct {
	tokenCalls int32
}

func (s *stubGitHubClient) GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error) {
	atomic.AddInt32(&s.tokenCalls, 1)
	return "ghs_stub", time.Now().Add(time.Hour), nil
}

func TestRunBootstrapWizardNonInteractive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/bootstrap/admin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"access_token":"token","user":{"id":1,"username":"admin","role":"admin"}}`))
	})

	var envPayload map[string]interface{}
	mux.HandleFunc("/api/v1/environments", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&envPayload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// prepare temp private key file
	tmpDir := t.TempDir()
	privateKeyPath := filepath.Join(tmpDir, "app.pem")
	if err := os.WriteFile(privateKeyPath, []byte("stub"), 0o600); err != nil {
		t.Fatalf("failed to write private key file: %v", err)
	}

	originalHTTPClient := newHTTPClient
	originalGitHubClient := newGitHubAppClient
	defer func() {
		newHTTPClient = originalHTTPClient
		newGitHubAppClient = originalGitHubClient
	}()

	newHTTPClient = func(string, bool) (*http.Client, error) {
		return srv.Client(), nil
	}

	stubClient := &stubGitHubClient{}
	newGitHubAppClient = func(cfg githubapp.Config) (installationTokenFetcher, error) {
		return stubClient, nil
	}

	args := []string{
		"--non-interactive",
		"--server-url", srv.URL,
		"--bootstrap-secret", "secret",
		"--admin-username", "admin-user",
		"--admin-password", "SuperSecretPass!",
		"--app-id", "1",
		"--private-key", privateKeyPath,
		"--installation-id", "2",
		"--environment-name", "prod",
		"--repo-url", "https://github.com/test/repo.git",
	}

	if err := runBootstrapWizard(args); err != nil {
		t.Fatalf("wizard failed: %v", err)
	}

	if stubClient.tokenCalls == 0 {
		t.Fatalf("expected github client to be used")
	}

	if envPayload == nil {
		t.Fatalf("expected environment payload to be sent")
	}

	if envPayload["name"] != "prod" {
		t.Fatalf("unexpected environment name: %v", envPayload["name"])
	}
}
