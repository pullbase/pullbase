//go:build integration
// +build integration

package rollback_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"log/slog"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/rollback"
	"github.com/pullbase/pullbase/server/pkg/server"
	"github.com/pullbase/pullbase/server/pkg/testutil"
)

func TestRollbackIntegration(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := testutil.SetupTestDB(t)
	ctx := tdb.Context()

	envRepo := tdb.EnvironmentRepository()
	mainRepo := tdb.Repository()

	env := tdb.CreateTestEnvironment("test-env", "https://github.com/test/config.git")

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockGit := &MockGitMonitor{}
	rollbackService := rollback.NewService(mainRepo, mockGit, logger)
	rollbackHandlers := server.NewRollbackHandlers(rollbackService)

	r := chi.NewRouter()
	r.Post("/api/v1/environments/{id}/rollback", rollbackHandlers.InitiateRollback)
	r.Get("/api/v1/environments/{id}/rollbacks", rollbackHandlers.ListRollbacks)
	r.Get("/api/v1/environments/{id}/commits", rollbackHandlers.GetAvailableCommits)
	r.Get("/api/v1/rollbacks/{id}", rollbackHandlers.GetRollbackStatus)

	t.Run("successful rollback workflow", func(t *testing.T) {
		mockGit.On("CommitExists", mock.Anything, env.RepoURL, "previous123").Return(true, nil)
		mockGit.On("CheckoutCommit", mock.Anything, env.RepoURL, "previous123").Return(nil)

		rollbackReq := rollback.RollbackRequest{
			ToCommit:    "previous123",
			Reason:      "Integration test rollback",
			InitiatedBy: "test-user",
		}

		reqBody, err := json.Marshal(rollbackReq)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/api/v1/environments/1/rollback", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Logf("Unexpected status code: %d", w.Code)
			t.Logf("Response body: %s", w.Body.String())
		}
		assert.Equal(t, http.StatusAccepted, w.Code)

		var response rollback.RollbackResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "pending", response.Status)
		assert.Greater(t, response.RollbackID, int64(0))

		// Wait for async rollback to complete using polling
		tdb.WaitForRollbackCompletion(response.RollbackID)

		statusReq := httptest.NewRequest("GET", "/api/v1/rollbacks/"+strconv.FormatInt(response.RollbackID, 10), nil)
		statusW := httptest.NewRecorder()

		r.ServeHTTP(statusW, statusReq)

		assert.Equal(t, http.StatusOK, statusW.Code)

		var rollbackEvent models.RollbackEvent
		err = json.Unmarshal(statusW.Body.Bytes(), &rollbackEvent)
		require.NoError(t, err)

		assert.Contains(t, []string{"completed", "failed"}, rollbackEvent.Status)

		updatedEnv, err := envRepo.GetEnvironment(ctx, env.ID)
		require.NoError(t, err)
		assert.NotNil(t, updatedEnv)
	})

	t.Run("rollback to non-existent commit should fail", func(t *testing.T) {
		mockGit.On("CommitExists", mock.Anything, env.RepoURL, "nonexistent").Return(false, nil)

		rollbackReq := rollback.RollbackRequest{
			ToCommit:    "nonexistent",
			Reason:      "Test failure case",
			InitiatedBy: "test-user",
		}

		reqBody, err := json.Marshal(rollbackReq)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/api/v1/environments/1/rollback", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "does not exist in repository")
	})

	t.Run("list rollback history", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/environments/1/rollbacks", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		rollbacks, ok := response["rollbacks"].([]interface{})
		require.True(t, ok)
		assert.GreaterOrEqual(t, len(rollbacks), 1)
	})

	t.Run("get available commits", func(t *testing.T) {
		// Create a server associated with the environment
		testServer := tdb.CreateTestServer("Integration Test Server", env.ID)

		// Create agent status entries with commit history
		tdb.CreateTestAgentStatus(testServer.ID, "test123commit456", "deployed", false, nil)

		req := httptest.NewRequest("GET", "/api/v1/environments/1/commits", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		commits, ok := response["commits"].([]interface{})
		require.True(t, ok)
		assert.GreaterOrEqual(t, len(commits), 1)
	})
}

// MockGitMonitor implements the rollback.GitMonitor interface for testing
type MockGitMonitor struct {
	mock.Mock
}

func (m *MockGitMonitor) CommitExists(ctx context.Context, repoURL, commit string) (bool, error) {
	args := m.Called(ctx, repoURL, commit)
	return args.Bool(0), args.Error(1)
}

func (m *MockGitMonitor) CheckoutCommit(ctx context.Context, repoURL, commit string) error {
	args := m.Called(ctx, repoURL, commit)
	return args.Error(0)
}


