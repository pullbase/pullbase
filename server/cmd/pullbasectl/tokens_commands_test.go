package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func captureOutput(tb testing.TB, fn func()) string {
	tb.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestRunAuthLogin(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"token123","user":{"id":1,"username":"admin","role":"admin"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	original := newHTTPClient
	defer func() { newHTTPClient = original }()
	newHTTPClient = func(string, bool) (*http.Client, error) { return srv.Client(), nil }

	output := captureOutput(t, func() {
		err := runAuthLogin([]string{"--server-url", srv.URL, "--username", "admin", "--password", "secret"})
		if err != nil {
			t.Fatalf("runAuthLogin failed: %v", err)
		}
	})

	if !strings.Contains(output, "token123") {
		t.Fatalf("expected token in output, got %q", output)
	}
}

func TestRunTokensList(t *testing.T) {
	tokens := []agentToken{{ID: 1, ServerID: "srv-1", Description: "primary", CreatedAt: time.Unix(0, 0), IsActive: true}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/servers/srv-1/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer admin-token" {
			t.Fatalf("unexpected auth header: %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tokens)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	original := newHTTPClient
	defer func() { newHTTPClient = original }()
	newHTTPClient = func(string, bool) (*http.Client, error) { return srv.Client(), nil }

	output := captureOutput(t, func() {
		err := runTokensList([]string{
			"--server-url", srv.URL,
			"--server-id", "srv-1",
			"--admin-token", "admin-token",
		})
		if err != nil {
			t.Fatalf("runTokensList failed: %v", err)
		}
	})

	if !strings.Contains(output, "primary") {
		t.Fatalf("expected description in output, got %q", output)
	}
}

func TestRunTokensCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/servers/srv-1/tokens", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()
		if !bytes.Contains(body, []byte("\"description\":\"deploy\"")) {
			t.Fatalf("unexpected body: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":1,"server_id":"srv-1","description":"deploy","created_at":"2024-01-01T00:00:00Z","is_active":true,"token":"pbt_token","installation_info":{"instructions":"step","example_cmd":"cmd"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	original := newHTTPClient
	defer func() { newHTTPClient = original }()
	newHTTPClient = func(string, bool) (*http.Client, error) { return srv.Client(), nil }

	output := captureOutput(t, func() {
		err := runTokensCreate([]string{
			"--server-url", srv.URL,
			"--server-id", "srv-1",
			"--admin-token", "admin-token",
			"--description", "deploy",
		})
		if err != nil {
			t.Fatalf("runTokensCreate failed: %v", err)
		}
	})

	if !strings.Contains(output, "pbt_token") {
		t.Fatalf("expected token in output, got %q", output)
	}
}

func TestRunTokensRevoke(t *testing.T) {
	var called bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/servers/srv-1/tokens/10", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("expected DELETE, got %s", r.Method)
		}
		called = true
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	original := newHTTPClient
	defer func() { newHTTPClient = original }()
	newHTTPClient = func(string, bool) (*http.Client, error) { return srv.Client(), nil }

	err := runTokensRevoke([]string{
		"--server-url", srv.URL,
		"--server-id", "srv-1",
		"--token-id", "10",
		"--admin-token", "admin-token",
	})
	if err != nil {
		t.Fatalf("runTokensRevoke failed: %v", err)
	}
	if !called {
		t.Fatalf("expected revoke endpoint to be called")
	}
}

func TestRunGitHubAppStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/environments/5", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":5,"name":"prod","repo_url":"https://github.com/org/repo.git","installation_id":123,"status":"active","webhook_status":{"status":"active","retry_count":0}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	original := newHTTPClient
	defer func() { newHTTPClient = original }()
	newHTTPClient = func(string, bool) (*http.Client, error) { return srv.Client(), nil }

	output := captureOutput(t, func() {
		err := runGitHubAppStatus([]string{
			"--server-url", srv.URL,
			"--environment-id", "5",
			"--admin-token", "admin-token",
		})
		if err != nil {
			t.Fatalf("runGitHubAppStatus failed: %v", err)
		}
	})

	if !strings.Contains(output, "prod") {
		t.Fatalf("expected environment name in output, got %q", output)
	}
}

func TestRunUsersCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		defer r.Body.Close()
		if auth := r.Header.Get("Authorization"); auth != "Bearer admin-token" {
			t.Fatalf("unexpected auth header: %s", auth)
		}
		body, _ := io.ReadAll(r.Body)
		if !bytes.Contains(body, []byte(`"username":"new-operator"`)) {
			t.Fatalf("unexpected body: %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"user":{"id":7,"username":"new-operator","role":"user"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	originalClient := newHTTPClient
	defer func() { newHTTPClient = originalClient }()
	newHTTPClient = func(string, bool) (*http.Client, error) { return srv.Client(), nil }

	output := captureOutput(t, func() {
		err := runUsersCreate([]string{
			"--server-url", srv.URL,
			"--admin-token", "admin-token",
			"--new-username", "new-operator",
			"--new-password", "StrongPassword123",
		})
		if err != nil {
			t.Fatalf("runUsersCreate failed: %v", err)
		}
	})

	if !strings.Contains(output, "User created: new-operator") {
		t.Fatalf("expected success output, got %q", output)
	}
}

func TestRunUsersList(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if auth := r.Header.Get("Authorization"); auth != "Bearer admin-token" {
			t.Fatalf("unexpected auth header: %s", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"users":[{"id":1,"username":"testadmin","role":"admin"}],"total":1,"limit":100,"offset":0,"role":""}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	original := newHTTPClient
	defer func() { newHTTPClient = original }()
	newHTTPClient = func(string, bool) (*http.Client, error) { return srv.Client(), nil }

	output := captureOutput(t, func() {
		err := runUsersList([]string{
			"--server-url", srv.URL,
			"--admin-token", "admin-token",
		})
		if err != nil {
			t.Fatalf("runUsersList failed: %v", err)
		}
	})

	if !strings.Contains(output, "testadmin") {
		t.Fatalf("expected output to include username, got %q", output)
	}
}
