package rollback

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"os"

	"github.com/pullbase/pullbase/server/pkg/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/pullbase/pullbase/server/pkg/models"
)

type MockRepository struct {
	mock.Mock
}

func (m *MockRepository) GetEnvironment(ctx context.Context, id int64) (*models.Environment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Environment), args.Error(1)
}

func (m *MockRepository) CreateRollbackEvent(ctx context.Context, event *models.RollbackEvent) error {
	args := m.Called(ctx, event)
	if args.Error(0) == nil {
		event.ID = 123
	}
	return args.Error(0)
}

func (m *MockRepository) UpdateRollbackEventStatus(ctx context.Context, id int64, status string, errorMsg *string) error {
	args := m.Called(ctx, id, status, errorMsg)
	return args.Error(0)
}

func (m *MockRepository) GetRollbackEvent(ctx context.Context, id int64) (*models.RollbackEvent, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.RollbackEvent), args.Error(1)
}

func (m *MockRepository) ListRollbackEvents(ctx context.Context, environmentID int64, limit, offset int) ([]*models.RollbackEvent, error) {
	args := m.Called(ctx, environmentID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.RollbackEvent), args.Error(1)
}

func (m *MockRepository) GetEnvironmentCommitHistory(ctx context.Context, environmentID int64, limit int) ([]*models.CommitInfo, error) {
	args := m.Called(ctx, environmentID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*models.CommitInfo), args.Error(1)
}

func (m *MockRepository) UpdateEnvironmentCommit(ctx context.Context, environmentID int64, commit string) error {
	args := m.Called(ctx, environmentID, commit)
	return args.Error(0)
}

func (m *MockRepository) CreateEvent(ctx context.Context, event *models.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

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

func TestService_InitiateRollback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		request        *RollbackRequest
		setupMocks     func(*MockRepository, *MockGitMonitor, *sync.WaitGroup)
		expectError    bool
		errorMsg       string
		validateResult func(t *testing.T, repo *MockRepository, git *MockGitMonitor, wg *sync.WaitGroup)
	}{
		{
			name: "successful rollback",
			request: &RollbackRequest{
				EnvironmentID: 1,
				ToCommit:      "abc123456789012345678901234567890123456",
				Reason:        "Test rollback",
				InitiatedBy:   "admin",
			},
			setupMocks: func(repo *MockRepository, git *MockGitMonitor, wg *sync.WaitGroup) {
				env := &models.Environment{
					ID:             1,
					Name:           "test-env",
					RepoURL:        "https://github.com/test/config.git",
					DeployedCommit: stringPtr("def4567890123456789012345678901234567890"),
				}
				repo.On("GetEnvironment", mock.Anything, int64(1)).Return(env, nil)
				git.On("CommitExists", mock.Anything, env.RepoURL, "abc123456789012345678901234567890123456").Return(true, nil)
				repo.On("CreateRollbackEvent", mock.Anything, mock.AnythingOfType("*models.RollbackEvent")).Run(func(args mock.Arguments) {
					event := args.Get(1).(*models.RollbackEvent)
					event.ID = 123
				}).Return(nil)
				repo.On("UpdateRollbackEventStatus", mock.Anything, int64(123), "in_progress", (*string)(nil)).Return(nil)
				git.On("CheckoutCommit", mock.Anything, env.RepoURL, "abc123456789012345678901234567890123456").Return(nil)
				repo.On("UpdateEnvironmentCommit", mock.Anything, int64(1), "abc123456789012345678901234567890123456").Return(nil)
				repo.On("CreateEvent", mock.Anything, mock.AnythingOfType("*models.Event")).Return(nil)
				repo.On("UpdateRollbackEventStatus", mock.Anything, int64(123), "completed", (*string)(nil)).Run(func(args mock.Arguments) {
					wg.Done()
				}).Return(nil)
			},
			expectError: false,
			validateResult: func(t *testing.T, repo *MockRepository, git *MockGitMonitor, wg *sync.WaitGroup) {
				wg.Wait()
			},
		},
		{
			name: "environment not found",
			request: &RollbackRequest{
				EnvironmentID: 999,
				ToCommit:      "abc123456789012345678901234567890123456",
				Reason:        "Test rollback",
				InitiatedBy:   "admin",
			},
			setupMocks: func(repo *MockRepository, git *MockGitMonitor, wg *sync.WaitGroup) {
				repo.On("GetEnvironment", mock.Anything, int64(999)).Return(nil, fmt.Errorf("environment not found"))
			},
			expectError: true,
			errorMsg:    "environment not found",
		},
		{
			name: "commit does not exist",
			request: &RollbackRequest{
				EnvironmentID: 1,
				ToCommit:      "nonexistent",
				Reason:        "Test rollback",
				InitiatedBy:   "admin",
			},
			setupMocks: func(repo *MockRepository, git *MockGitMonitor, wg *sync.WaitGroup) {
				env := &models.Environment{
					ID:             1,
					Name:           "test-env",
					RepoURL:        "https://github.com/test/config.git",
					DeployedCommit: stringPtr("def4567890123456789012345678901234567890"),
				}
				repo.On("GetEnvironment", mock.Anything, int64(1)).Return(env, nil)
				git.On("CommitExists", mock.Anything, env.RepoURL, "nonexistent").Return(false, nil)
			},
			expectError: true,
			errorMsg:    "does not exist in repository",
		},
		{
			name: "already at target commit",
			request: &RollbackRequest{
				EnvironmentID: 1,
				ToCommit:      "same123456789012345678901234567890123456",
				Reason:        "Test rollback",
				InitiatedBy:   "admin",
			},
			setupMocks: func(repo *MockRepository, git *MockGitMonitor, wg *sync.WaitGroup) {
				env := &models.Environment{
					ID:             1,
					Name:           "test-env",
					RepoURL:        "https://github.com/test/config.git",
					DeployedCommit: stringPtr("same123456789012345678901234567890123456"),
				}
				repo.On("GetEnvironment", mock.Anything, int64(1)).Return(env, nil)
			},
			expectError: true,
			errorMsg:    "already at commit",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockRepo := new(MockRepository)
			mockGit := new(MockGitMonitor)
			logger := logging.NewLogger(logging.Options{Format: "text", Output: os.Stdout})

			var wg sync.WaitGroup
			if tt.name == "successful rollback" {
				wg.Add(1)
				tt.setupMocks(mockRepo, mockGit, &wg)
			} else {
				tt.setupMocks(mockRepo, mockGit, nil)
			}

			service := NewService(mockRepo, mockGit, logger)
			response, err := service.InitiateRollback(context.Background(), tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
				assert.Nil(t, response)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.Equal(t, "pending", response.Status)
				assert.Greater(t, response.RollbackID, int64(0))
			}

			if tt.validateResult != nil {
				if tt.name == "successful rollback" {
					tt.validateResult(t, mockRepo, mockGit, &wg)
				} else {
					tt.validateResult(t, mockRepo, mockGit, nil)
				}
			}

			mockRepo.AssertExpectations(t)
			mockGit.AssertExpectations(t)
		})
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

func TestService_ValidateRollbackRequest(t *testing.T) {
	t.Parallel()

	service := NewService(nil, nil, nil)

	tests := []struct {
		name        string
		request     *RollbackRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			request: &RollbackRequest{
				EnvironmentID: 1,
				ToCommit:      "abc123456789012345678901234567890123456",
				InitiatedBy:   "admin",
			},
			expectError: false,
		},
		{
			name: "invalid environment ID",
			request: &RollbackRequest{
				EnvironmentID: 0,
				ToCommit:      "abc123",
				InitiatedBy:   "admin",
			},
			expectError: true,
			errorMsg:    "invalid environment ID",
		},
		{
			name: "empty commit hash",
			request: &RollbackRequest{
				EnvironmentID: 1,
				ToCommit:      "",
				InitiatedBy:   "admin",
			},
			expectError: true,
			errorMsg:    "target commit cannot be empty",
		},
		{
			name: "empty initiated by",
			request: &RollbackRequest{
				EnvironmentID: 1,
				ToCommit:      "abc123",
				InitiatedBy:   "",
			},
			expectError: true,
			errorMsg:    "initiated_by cannot be empty",
		},
		{
			name: "commit hash too short",
			request: &RollbackRequest{
				EnvironmentID: 1,
				ToCommit:      "abc12",
				InitiatedBy:   "admin",
			},
			expectError: true,
			errorMsg:    "invalid commit hash format",
		},
		{
			name: "commit hash too long",
			request: &RollbackRequest{
				EnvironmentID: 1,
				ToCommit:      "abc12345678901234567890123456789012345678901234567890",
				InitiatedBy:   "admin",
			},
			expectError: true,
			errorMsg:    "invalid commit hash format",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := service.ValidateRollbackRequest(context.Background(), tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_GetRollbackStatus(t *testing.T) {
	t.Parallel()

	mockRepo := new(MockRepository)
	logger := logging.NewLogger(logging.Options{Format: "text", Output: os.Stdout})
	service := NewService(mockRepo, nil, logger)

	expectedEvent := &models.RollbackEvent{
		ID:            123,
		EnvironmentID: 1,
		FromCommit:    "def456",
		ToCommit:      "abc123",
		InitiatedBy:   "admin",
		Status:        "completed",
		Reason:        "Emergency rollback",
		CreatedAt:     time.Now(),
	}

	mockRepo.On("GetRollbackEvent", mock.Anything, int64(123)).Return(expectedEvent, nil)

	result, err := service.GetRollbackStatus(context.Background(), 123)

	assert.NoError(t, err)
	assert.Equal(t, expectedEvent, result)
	mockRepo.AssertExpectations(t)
}

func TestService_ListRollbacks(t *testing.T) {
	t.Parallel()

	mockRepo := new(MockRepository)
	logger := logging.NewLogger(logging.Options{Format: "text", Output: os.Stdout})
	service := NewService(mockRepo, nil, logger)

	expectedRollbacks := []*models.RollbackEvent{
		{
			ID:            123,
			EnvironmentID: 1,
			FromCommit:    "def456",
			ToCommit:      "abc123",
			Status:        "completed",
			CreatedAt:     time.Now(),
		},
		{
			ID:            124,
			EnvironmentID: 1,
			FromCommit:    "ghi789",
			ToCommit:      "def456",
			Status:        "pending",
			CreatedAt:     time.Now().Add(-time.Hour),
		},
	}

	mockRepo.On("ListRollbackEvents", mock.Anything, int64(1), 10, 0).Return(expectedRollbacks, nil)

	result, err := service.ListRollbacks(context.Background(), 1, 10, 0)

	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, expectedRollbacks, result)
	mockRepo.AssertExpectations(t)
}

func TestService_GetAvailableCommits(t *testing.T) {
	t.Parallel()

	mockRepo := new(MockRepository)
	logger := logging.NewLogger(logging.Options{Format: "text", Output: os.Stdout})
	service := NewService(mockRepo, nil, logger)

	expectedCommits := []*models.CommitInfo{
		{
			Hash:      "abc123",
			AppliedAt: time.Now(),
			Message:   "Latest deployment",
		},
		{
			Hash:      "def456",
			AppliedAt: time.Now().Add(-time.Hour),
			Message:   "Previous deployment",
		},
	}

	mockRepo.On("GetEnvironmentCommitHistory", mock.Anything, int64(1), 20).Return(expectedCommits, nil)

	result, err := service.GetAvailableCommits(context.Background(), 1, 20)

	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, expectedCommits, result)
	mockRepo.AssertExpectations(t)
}
