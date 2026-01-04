//go:build integration
// +build integration

package server_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pullbase/pullbase/server/pkg/gitmonitor"
	"github.com/pullbase/pullbase/server/pkg/models"
	server "github.com/pullbase/pullbase/server/pkg/server"
	"github.com/pullbase/pullbase/server/pkg/testutil"
	"github.com/stretchr/testify/require"
	"log/slog"
)

type recordingServerRepo struct {
	lastCommit string
}

func (r *recordingServerRepo) GetServersByEnvironment(ctx context.Context, environmentID int64) ([]models.Server, error) {
	return []models.Server{{Name: "test-server"}}, nil
}

func (r *recordingServerRepo) UpdateTargetCommitHash(ctx context.Context, serverName, commitHash string) error {
	r.lastCommit = commitHash
	return nil
}

func setupWebhookHandler(t *testing.T) (*server.WebhookHandlers, *recordingServerRepo, string, *gitmonitor.EnvironmentMonitor) {
	t.Helper()

	tdb := testutil.SetupTestDB(t)
	ctx := tdb.Context()

	envRepo := tdb.EnvironmentRepository()

	encryptionKey := make([]byte, 32)
	for i := range encryptionKey {
		encryptionKey[i] = byte(i)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	recordingRepo := &recordingServerRepo{}
	router := gitmonitor.NewWebhookRouter(logger, nil)
	webhookManager := gitmonitor.NewWebhookManager(router, logger, envRepo)
	monitor := gitmonitor.NewEnvironmentMonitor(webhookManager, logger, encryptionKey, envRepo, recordingRepo, nil)
	router.SetMonitor(monitor)
	monitor.RegisterProvider(gitmonitor.ProviderGitHub, gitmonitor.NewGitHubProvider())

	secret := "webhook-secret-value"

	env := &models.Environment{
		Name:           "webhook-test-env",
		RepoURL:        "https://github.com/example/configs.git",
		Branch:         "main",
		DeployPath:     "config.yaml",
		Provider:       models.ProviderGitHub,
		InstallationID: 12345,
		WebhookSecret:  secret,
		WebhookURL:     "https://pullbase.test/webhooks/github",
		Status:         string(models.StatusActive),
		AutoReconcile:  true,
	}

	err := envRepo.CreateEnvironment(ctx, env)
	require.NoError(t, err, "failed to create test environment")

	err = monitor.LoadEnvironmentsFromDatabase(ctx)
	require.NoError(t, err, "failed to load environments into monitor cache")

	handlers := server.NewWebhookHandlers(monitor, logger)
	handlers.SetAuditRecorder(func(*http.Request, string, string, string, interface{}) {})

	return handlers, recordingRepo, secret, monitor
}

func buildGitHubPayload(branchRef, commitHash string) []byte {
	payload := map[string]any{
		"ref": branchRef,
		"repository": map[string]any{
			"full_name": "example/configs",
			"clone_url": "https://github.com/example/configs.git",
			"html_url":  "https://github.com/example/configs",
		},
		"head_commit": map[string]any{
			"id":      commitHash,
			"message": "test commit",
			"author": map[string]any{
				"name":  "tester",
				"email": "tester@example.com",
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}
	data, _ := json.Marshal(payload)
	return data
}

func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookHandlers_HandleWebhook_Success(t *testing.T) {
	handlers, recordingRepo, secret, _ := setupWebhookHandler(t)

	payload := buildGitHubPayload("refs/heads/main", "abc123def456")
	signature := signPayload(secret, payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handlers.HandleWebhook(rr, req)

	res := rr.Result()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, "abc123def456", recordingRepo.lastCommit)
}

func TestWebhookHandlers_HandleWebhook_BranchMismatch(t *testing.T) {
	handlers, recordingRepo, secret, _ := setupWebhookHandler(t)

	payload := buildGitHubPayload("refs/heads/feature", "abc123def456")
	signature := signPayload(secret, payload)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", signature)

	rr := httptest.NewRecorder()
	handlers.HandleWebhook(rr, req)

	res := rr.Result()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
	require.Empty(t, recordingRepo.lastCommit)
}

func TestWebhookHandlers_HandleWebhook_InvalidSignature(t *testing.T) {
	handlers, recordingRepo, _, _ := setupWebhookHandler(t)

	payload := buildGitHubPayload("refs/heads/main", "abc123def456")

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")

	rr := httptest.NewRecorder()
	handlers.HandleWebhook(rr, req)

	res := rr.Result()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
	require.Empty(t, recordingRepo.lastCommit)
}
