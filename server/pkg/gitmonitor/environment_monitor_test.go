//go:build integration
// +build integration

package gitmonitor

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/rollback"
	"github.com/pullbase/pullbase/server/pkg/testutil"
	"github.com/stretchr/testify/require"
)

func setupTestDatabase(t *testing.T) *testutil.TestDB {
	t.Helper()
	return testutil.SetupTestDB(t)
}

// MockServerRepository implements ServerRepository for testing
type MockServerRepository struct{}

func (m *MockServerRepository) GetServersByEnvironment(ctx context.Context, environmentID int64) ([]models.Server, error) {
	return []models.Server{}, nil
}

func (m *MockServerRepository) UpdateTargetCommitHash(ctx context.Context, serverName, commitHash string) error {
	return nil
}

type stubInstallationTokenProvider struct {
	token string
	err   error
}

func (s *stubInstallationTokenProvider) GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error) {
	if s.err != nil {
		return "", time.Time{}, s.err
	}
	return s.token, time.Now().Add(time.Hour), nil
}

func TestEnvironmentMonitor_AddEnvironment(t *testing.T) {
	tdb := setupTestDatabase(t)
	ctx := tdb.Context()

	// Test database connection
	tdb.WaitForDBConnection()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	encryptionKey := make([]byte, 32)
	for i := range encryptionKey {
		encryptionKey[i] = byte(i)
	}

	repo := tdb.EnvironmentRepository()
	mockServerRepo := &MockServerRepository{}
	router := NewWebhookRouter(logger, nil)
	webhookManager := NewWebhookManager(router, logger, repo)
	monitor := NewEnvironmentMonitor(webhookManager, logger, encryptionKey, repo, mockServerRepo, &stubInstallationTokenProvider{token: "dummy-token"})

	monitor.RegisterProvider(ProviderGitHub, NewGitHubProvider())

	env := &Environment{
		Name:           "test-env",
		RepoURL:        "https://github.com/testuser/testrepo",
		Provider:       ProviderGitHub,
		InstallationID: 1234,
		WebhookURL:     "https://example.com/webhooks/github",
		Status:         string(StatusPending),
	}

	err := monitor.AddEnvironment(ctx, env)

	// This will fail because we can't register webhooks without real credentials
	// but we can verify the environment was saved to database
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to register webhook")

	// Verify environment was saved to database
	savedEnv, err := repo.GetEnvironment(ctx, env.ID)
	require.NoError(t, err)
	require.NotNil(t, savedEnv)
	require.Equal(t, "test-env", savedEnv.Name)
	require.Equal(t, "https://github.com/testuser/testrepo", savedEnv.RepoURL)
	require.Equal(t, ProviderGitHub, savedEnv.Provider)
	require.Equal(t, int64(1234), savedEnv.InstallationID)
	require.NotEmpty(t, savedEnv.WebhookSecret)
}

func TestEnvironmentMonitor_Encryption(t *testing.T) {
	tdb := setupTestDatabase(t)

	// Test database connection
	tdb.WaitForDBConnection()

	encryptionKey := make([]byte, 32)
	for i := range encryptionKey {
		encryptionKey[i] = byte(i)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := tdb.EnvironmentRepository()
	mockServerRepo := &MockServerRepository{}
	router := NewWebhookRouter(logger, nil)
	webhookManager := NewWebhookManager(router, logger, repo)
	monitor := NewEnvironmentMonitor(webhookManager, logger, encryptionKey, repo, mockServerRepo, &stubInstallationTokenProvider{token: "dummy-token"})

	originalSecret := "test-secret-123"

	encrypted, err := monitor.encryptSecret(originalSecret)
	require.NoError(t, err)
	require.NotEqual(t, originalSecret, encrypted)

	decrypted, err := monitor.decryptSecret(encrypted)
	require.NoError(t, err)
	require.Equal(t, originalSecret, decrypted)
}

func TestEnvironmentMonitor_GetEnvironmentByRepoURL(t *testing.T) {
	tdb := setupTestDatabase(t)
	ctx := tdb.Context()

	// Test database connection
	tdb.WaitForDBConnection()

	repo := tdb.EnvironmentRepository()

	encryptionKey := make([]byte, 32)
	for i := range encryptionKey {
		encryptionKey[i] = byte(i)
	}

	env := &Environment{
		Name:           "test-env",
		RepoURL:        "https://github.com/testuser/testrepo",
		Branch:         "main",
		DeployPath:     "config.yaml",
		Provider:       ProviderGitHub,
		InstallationID: 5678,
		WebhookSecret:  "test-secret",
		WebhookURL:     "https://example.com/webhooks/github",
		Status:         string(StatusPending),
	}

	// Save environment directly to database
	err := repo.CreateEnvironment(ctx, env)
	require.NoError(t, err)

	// Test lookup by repository URL and branch
	foundEnv, err := repo.GetEnvironmentByRepoURLAndBranch(ctx, "https://github.com/testuser/testrepo", "main")
	require.NoError(t, err)
	require.NotNil(t, foundEnv)
	require.Equal(t, env.Name, foundEnv.Name)
	require.Equal(t, env.RepoURL, foundEnv.RepoURL)

	// Test lookup for non-existent branch
	notFoundEnv, err := repo.GetEnvironmentByRepoURLAndBranch(ctx, "https://github.com/testuser/testrepo", "develop")
	require.NoError(t, err)
	require.Nil(t, notFoundEnv)

	// Test lookup for non-existent repository
	notFoundEnv, err = repo.GetEnvironmentByRepoURLAndBranch(ctx, "https://github.com/nonexistent/repo", "main")
	require.NoError(t, err)
	require.Nil(t, notFoundEnv)
}

func TestGitHubProvider_ParseRepoURL(t *testing.T) {
	provider := NewGitHubProvider()

	testCases := []struct {
		name     string
		repoURL  string
		expected struct {
			owner string
			repo  string
		}
		shouldErr bool
	}{
		{
			name:    "HTTPS URL",
			repoURL: "https://github.com/testuser/testrepo",
			expected: struct {
				owner string
				repo  string
			}{
				owner: "testuser",
				repo:  "testrepo",
			},
			shouldErr: false,
		},
		{
			name:    "HTTPS URL with .git",
			repoURL: "https://github.com/testuser/testrepo.git",
			expected: struct {
				owner string
				repo  string
			}{
				owner: "testuser",
				repo:  "testrepo",
			},
			shouldErr: false,
		},
		{
			name:    "SSH URL",
			repoURL: "git@github.com:testuser/testrepo.git",
			expected: struct {
				owner string
				repo  string
			}{
				owner: "testuser",
				repo:  "testrepo",
			},
			shouldErr: false,
		},
		{
			name:      "Invalid URL",
			repoURL:   "invalid-url",
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := provider.parseRepoURL(tc.repoURL)

			if tc.shouldErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected.owner, owner)
			require.Equal(t, tc.expected.repo, repo)
		})
	}
}

func TestWebhookStatus_UpdateStatus(t *testing.T) {
	tdb := setupTestDatabase(t)

	// Test database connection
	tdb.WaitForDBConnection()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repo := tdb.EnvironmentRepository()
	router := NewWebhookRouter(logger, nil)
	webhookManager := NewWebhookManager(router, logger, repo)

	webhookManager.updateStatus(1, ProviderGitHub, "active", "")

	status, exists := webhookManager.GetStatus(1)
	require.True(t, exists)
	require.Equal(t, "active", status.Status)
	require.Equal(t, int64(1), status.EnvironmentID)
}

func TestEnvironmentMonitor_WebhookRollbackIntegration(t *testing.T) {
	tdb := setupTestDatabase(t)
	ctx := tdb.Context()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	encryptionKey := make([]byte, 32)
	for i := range encryptionKey {
		encryptionKey[i] = byte(i)
	}

	repo := tdb.EnvironmentRepository()
	mockServerRepo := &MockServerRepository{}
	router := NewWebhookRouter(logger, nil)
	webhookManager := NewWebhookManager(router, logger, repo)
	monitor := NewEnvironmentMonitor(webhookManager, logger, encryptionKey, repo, mockServerRepo, &stubInstallationTokenProvider{token: "dummy-token"})

	// Create a mock rollback service
	mockRollbackService := &MockRollbackService{
		rollbackRequests: make(chan *rollback.RollbackRequest, 1),
	}
	monitor.SetRollbackService(mockRollbackService)

	// Add an environment
	env := &Environment{
		Name:           "test-env",
		RepoURL:        "https://github.com/testuser/testrepo",
		Branch:         "main",
		DeployPath:     "config.yaml",
		Provider:       ProviderGitHub,
		InstallationID: 91011,
		WebhookSecret:  "test-secret",
		WebhookURL:     "https://example.com/webhooks/github",
		Status:         string(StatusPending),
		DeployedCommit: stringPtr("abc123"),
	}

	// Save environment directly to database to avoid webhook registration
	err := repo.CreateEnvironment(ctx, env)
	require.NoError(t, err)

	savedEnv, err := repo.GetEnvironmentByRepoURLAndBranch(ctx, env.RepoURL, env.Branch)
	require.NoError(t, err)
	require.NotNil(t, savedEnv)

	savedEnv.DeployedCommit = stringPtr("abc123")
	err = repo.UpdateEnvironment(ctx, savedEnv)
	require.NoError(t, err)

	monitor.environments[savedEnv.ID] = savedEnv

	// Create a webhook event with rollback trigger
	event := &WebhookEvent{
		Provider:   ProviderGitHub,
		EventType:  "push",
		Repository: "https://github.com/testuser/testrepo",
		Branch:     "main",
		CommitHash: "def456",
		CommitMsg:  "feat: new feature [ROLLBACK]",
		Author:     "testuser",
		Timestamp:  time.Now(),
	}

	// Process the webhook event
	err = monitor.HandleWebhookEvent(ctx, event)
	require.NoError(t, err)

	// Wait for rollback request to be made
	select {
	case rollbackReq := <-mockRollbackService.rollbackRequests:
		require.Equal(t, savedEnv.ID, rollbackReq.EnvironmentID)
		require.Equal(t, "abc123", rollbackReq.ToCommit)
		require.Equal(t, "webhook-system", rollbackReq.InitiatedBy)
		require.Contains(t, rollbackReq.Reason, "Auto-rollback triggered by commit def456")
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for rollback request")
	}
}

// MockRollbackService is a mock implementation for testing
var _ rollback.RollbackService = (*MockRollbackService)(nil)

type MockRollbackService struct {
	rollbackRequests chan *rollback.RollbackRequest
}

func (m *MockRollbackService) InitiateRollback(ctx context.Context, req *rollback.RollbackRequest) (*rollback.RollbackResponse, error) {
	m.rollbackRequests <- req
	return &rollback.RollbackResponse{
		RollbackID: 1,
		Status:     "pending",
		Message:    "Rollback initiated successfully",
	}, nil
}

func (m *MockRollbackService) GetRollbackStatus(ctx context.Context, rollbackID int64) (*models.RollbackEvent, error) {
	return &models.RollbackEvent{
		ID:            rollbackID,
		EnvironmentID: 1,
		FromCommit:    "def456",
		ToCommit:      "abc123",
		Status:        "completed",
		InitiatedBy:   "webhook-system",
		Reason:        "Auto-rollback triggered",
		CreatedAt:     time.Now(),
	}, nil
}

func (m *MockRollbackService) ListRollbacks(ctx context.Context, environmentID int64, limit, offset int) ([]*models.RollbackEvent, error) {
	return []*models.RollbackEvent{}, nil
}

func (m *MockRollbackService) GetAvailableCommits(ctx context.Context, environmentID int64, limit int) ([]*models.CommitInfo, error) {
	return []*models.CommitInfo{}, nil
}

func (m *MockRollbackService) ValidateRollbackRequest(ctx context.Context, req *rollback.RollbackRequest) error {
	return nil
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}
