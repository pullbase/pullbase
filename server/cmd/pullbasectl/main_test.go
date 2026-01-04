package main

import (
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
    return f(req)
}

func TestRunAuthBootstrapAdmin_MissingServerURL(t *testing.T) {
    err := runAuthBootstrapAdmin([]string{"--username", "admin", "--password", "password123", "--bootstrap-secret", "secret"})
    if err == nil {
        t.Fatalf("expected error when server URL is missing")
    }
}

func TestRunAuthBootstrapAdmin_UsesSecretFileAndHTTPClient(t *testing.T) {
    tmpDir := t.TempDir()
    secretFile := filepath.Join(tmpDir, "bootstrap-secret.txt")
    if err := os.WriteFile(secretFile, []byte("test-secret"), 0o600); err != nil {
        t.Fatalf("failed to write secret file: %v", err)
    }

    originalFactory := newHTTPClient
    defer func() { newHTTPClient = originalFactory }()

    var capturedBody string
    newHTTPClient = func(ca string, insecure bool) (*http.Client, error) {
        return &http.Client{
            Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
                if req.URL.String() != "https://example.com/api/v1/bootstrap/admin" {
                    t.Fatalf("unexpected request URL: %s", req.URL.String())
                }
                bodyBytes, err := io.ReadAll(req.Body)
                if err != nil {
                    t.Fatalf("failed to read request body: %v", err)
                }
                capturedBody = string(bodyBytes)
                resp := &http.Response{
                    StatusCode: http.StatusCreated,
                    Body:       io.NopCloser(strings.NewReader(`{"access_token":"token","user":{"id":1,"username":"admin","role":"admin"}}`)),
                    Header:     make(http.Header),
                }
                return resp, nil
            }),
        }, nil
    }

    err := runAuthBootstrapAdmin([]string{
        "--server-url", "https://example.com",
        "--bootstrap-secret-file", secretFile,
        "--username", "admin",
        "--password", "strongpassword123",
    })
    if err != nil {
        t.Fatalf("expected success, got error: %v", err)
    }

    if !strings.Contains(capturedBody, "test-secret") {
        t.Fatalf("bootstrap secret not sent in request body: %s", capturedBody)
    }
    if !strings.Contains(capturedBody, "strongpassword123") {
        t.Fatalf("password not sent in request body: %s", capturedBody)
    }
}

func TestLoadSensitiveValueRejectsInsecurePermissions(t *testing.T) {
    tmpDir := t.TempDir()
    path := filepath.Join(tmpDir, "secret.txt")
    if err := os.WriteFile(path, []byte("secret"), 0o644); err != nil {
        t.Fatalf("failed to write secret file: %v", err)
    }
    if _, err := loadSensitiveValue(path); err == nil {
        t.Fatalf("expected error for world-readable file")
    }
}

func TestBuildHTTPClientInvalidCA(t *testing.T) {
    tmpDir := t.TempDir()
    caPath := filepath.Join(tmpDir, "bad.pem")
    if err := os.WriteFile(caPath, []byte("invalid"), 0o600); err != nil {
        t.Fatalf("failed to write CA file: %v", err)
    }

    if _, err := buildHTTPClient(caPath, false); err == nil {
        t.Fatalf("expected error for invalid CA bundle")
    }
}
