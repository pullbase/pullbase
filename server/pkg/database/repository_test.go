//go:build integration
// +build integration

package database_test

import (
	"encoding/json"
	"testing"
	"time"

	db "github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB initializes a test database using testutil
func setupTestDB(t *testing.T) *testutil.TestDB {
	t.Helper()
	return testutil.SetupTestDB(t)
}

func TestRepository_PullOperations(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	pull := &models.Pull{
		ID:          "test-pull-1",
		Title:       "Test Pull Request",
		Description: "This is a test pull request",
		Status:      "pending",
	}

	err := repo.CreatePull(ctx, pull)
	require.NoError(t, err, "Failed to create pull")

	retrieved, err := repo.GetPull(ctx, pull.ID)
	require.NoError(t, err, "Failed to get pull")
	require.NotNil(t, retrieved, "Expected to find pull, got nil")
	assert.Equal(t, pull.Title, retrieved.Title)

	pulls, err := repo.ListPulls(ctx)
	require.NoError(t, err, "Failed to list pulls")
	assert.NotEmpty(t, pulls, "Expected to find pulls, got empty list")

	pull.Title = "Updated Title"
	pull.Status = "approved"
	err = repo.UpdatePull(ctx, pull)
	require.NoError(t, err, "Failed to update pull")

	updated, err := repo.GetPull(ctx, pull.ID)
	require.NoError(t, err, "Failed to get updated pull")
	assert.Equal(t, "Updated Title", updated.Title)

	err = repo.DeletePull(ctx, pull.ID)
	require.NoError(t, err, "Failed to delete pull")

	deletedPull, err := repo.GetPull(ctx, pull.ID)
	require.Error(t, err, "Expected an error when getting a deleted pull request")
	assert.ErrorIs(t, err, db.ErrNotFound, "Expected ErrNotFound after delete")
	require.Nil(t, deletedPull, "GetPull should return nil after delete")

	_, err = repo.GetPull(ctx, "totally-non-existent-pull")
	require.Error(t, err, "Expected an error for completely non-existent ID")
	assert.ErrorIs(t, err, db.ErrNotFound, "Expected ErrNotFound for non-existent ID")
}

func TestRepository_UserOperations(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	username := "testuser"
	password := "testpass"
	role := "user"

	err := repo.CreateUser(ctx, username, password, role)
	require.NoError(t, err, "Failed to create user")

	user, err := repo.GetUser(ctx, username)
	require.NoError(t, err, "Failed to get user")
	require.NotNil(t, user, "Expected to find user, got nil")
	assert.Equal(t, username, user.Username)
	assert.Equal(t, role, user.Role)
}

func TestRepository_ServerOperations(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	// Create a test environment first
	testEnv, err := repo.CreateEnvironment(ctx, "test-env", "https://github.com/test/repo.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	serverName := "Test Server One"

	createdServer, err := repo.CreateServer(ctx, serverName, testEnv.ID)
	require.NoError(t, err, "Failed to create server")
	require.NotNil(t, createdServer, "CreateServer returned nil server")
	serverID := createdServer.ID
	assert.Equal(t, serverName, createdServer.Name)
	assert.Equal(t, testEnv.ID, *createdServer.EnvironmentID)
	assert.Nil(t, createdServer.TargetCommitHash, "Expected TargetCommitHash to be nil on creation")

	retrievedServer, err := repo.GetServerByID(ctx, serverID)
	require.NoError(t, err, "Failed to get server by ID")
	require.NotNil(t, retrievedServer, "GetServerByID returned nil server")
	assert.Equal(t, serverName, retrievedServer.Name)
	assert.Equal(t, testEnv.ID, *retrievedServer.EnvironmentID)
	assert.Nil(t, retrievedServer.TargetCommitHash, "Expected TargetCommitHash to be nil when retrieved")

	_, err = repo.GetServerByID(ctx, "non-existent-server")
	require.Error(t, err, "Expected error for non-existent server")
	assert.ErrorIs(t, err, db.ErrNotFound, "Expected ErrNotFound for non-existent server")
}

func TestRepository_AgentStatusOperations(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	// Create a test environment first
	testEnv, err := repo.CreateEnvironment(ctx, "status-test-env", "https://github.com/test/status.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	// create server
	createdServer, err := repo.CreateServer(ctx, "Status Test Server", testEnv.ID)
	require.NoError(t, err, "Setup: Failed to create server for status test")
	serverID := createdServer.ID

	timestamp1 := time.Now().UTC().Truncate(time.Microsecond)
	status1 := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   "commit1",
		IsDrifted:    false,
		Status:       "synced",
		Timestamp:    timestamp1,
		ErrorMessage: nil,
	}
	err = repo.CreateAgentStatus(ctx, status1)
	require.NoError(t, err, "Failed to create agent status (1)")

	latestStatus, err := repo.GetLatestAgentStatus(ctx, serverID)
	require.NoError(t, err, "Failed to get latest agent status (1)")
	require.NotNil(t, latestStatus, "GetLatestAgentStatus returned nil status (1)")
	assert.Equal(t, "commit1", latestStatus.CommitHash)
	assert.Equal(t, timestamp1.Format(time.RFC3339), latestStatus.Timestamp.Format(time.RFC3339), "Timestamp mismatch")
	assert.Nil(t, latestStatus.ErrorMessage, "Expected nil ErrorMessage")

	// Ensure second timestamp is after the first
	timestamp2 := timestamp1.Add(time.Millisecond).UTC().Truncate(time.Microsecond)
	errMsg := "drift detected"
	status2 := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   "commit2",
		IsDrifted:    true,
		Status:       "drifted",
		Timestamp:    timestamp2,
		ErrorMessage: &errMsg,
	}
	err = repo.CreateAgentStatus(ctx, status2)
	require.NoError(t, err, "Failed to create agent status (2)")

	latestStatus, err = repo.GetLatestAgentStatus(ctx, serverID)
	require.NoError(t, err, "Failed to get latest agent status (2)")
	require.NotNil(t, latestStatus, "GetLatestAgentStatus returned nil status (2)")
	assert.Equal(t, "commit2", latestStatus.CommitHash)
	assert.Equal(t, timestamp2.Format(time.RFC3339), latestStatus.Timestamp.Format(time.RFC3339), "Timestamp mismatch")
	require.NotNil(t, latestStatus.ErrorMessage, "Expected non-nil ErrorMessage")
	assert.Equal(t, errMsg, *latestStatus.ErrorMessage)
}

func TestRepository_AuditLogOperations(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	err := repo.CreateUser(ctx, "audit-test-user", "password123", "admin")
	require.NoError(t, err, "Setup: Failed to create user for audit test")

	user, err := repo.GetUser(ctx, "audit-test-user")
	require.NoError(t, err, "Setup: Failed to get created user")

	timestamp1 := time.Now().UTC()
	log1 := &models.AuditLog{
		UserID:       &user.ID,
		Action:       "create",
		ResourceType: "server",
		ResourceID:   "server1",
		Details:      json.RawMessage(`{"name":"Test Server"}`),
		IPAddress:    "192.168.1.1",
		Timestamp:    timestamp1,
	}

	err = repo.CreateAuditLog(ctx, log1)
	require.NoError(t, err, "Failed to create audit log (1)")

	timestamp2 := time.Now().UTC()
	log2 := &models.AuditLog{
		UserID:       &user.ID,
		Action:       "update",
		ResourceType: "server",
		ResourceID:   "server1",
		Details:      json.RawMessage(`{"new_name":"Updated Server"}`),
		IPAddress:    "192.168.1.2",
		Timestamp:    timestamp2,
	}

	err = repo.CreateAuditLog(ctx, log2)
	require.NoError(t, err, "Failed to create audit log (2)")

	logs, err := repo.GetAuditLogs(ctx, 10, 0)
	require.NoError(t, err, "Failed to get audit logs")
	require.Len(t, logs, 2, "Expected 2 audit logs")

	assert.Equal(t, "update", logs[0].Action)
	assert.Equal(t, "create", logs[1].Action)
}

func TestRepository_UserOperations_Extended(t *testing.T) {
	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	initialUsername := "testuser-extended"
	initialPassword := "password123"
	initialRole := "user"

	err := repo.CreateUser(ctx, initialUsername, initialPassword, initialRole)
	require.NoError(t, err, "Failed to create initial user")

	createdUser, err := repo.GetUser(ctx, initialUsername)
	require.NoError(t, err, "Failed to retrieve created user")
	require.NotNil(t, createdUser, "Failed to retrieve created user")
	require.True(t, createdUser.IsActive, "Newly created user should be active")
	userID := createdUser.ID

	t.Run("GetUserByID_Active", func(t *testing.T) {
		retrievedUser, err := repo.GetUserByID(ctx, userID)
		require.NoError(t, err, "GetUserByID failed for active user")
		require.NotNil(t, retrievedUser, "GetUserByID returned nil user for active user")
		assert.Equal(t, initialUsername, retrievedUser.Username)
		assert.True(t, retrievedUser.IsActive)
	})

	t.Run("UpdateUser_RoleAndUsername", func(t *testing.T) {
		updatedUser := &models.User{
			ID:       userID,
			Username: "testuser-updated",
			Role:     "admin",
			IsActive: true,
		}
		err := repo.UpdateUser(ctx, updatedUser)
		require.NoError(t, err, "UpdateUser failed")

		retrievedUser, err := repo.GetUserByID(ctx, userID)
		require.NoError(t, err, "Failed to retrieve user after update")
		require.NotNil(t, retrievedUser, "Failed to retrieve user after update")
		assert.Equal(t, "testuser-updated", retrievedUser.Username)
		assert.Equal(t, "admin", retrievedUser.Role)
	})

	t.Run("UpdateUser_DuplicateUsername", func(t *testing.T) {
		err := repo.CreateUser(ctx, "existinguser", "pass", "user")
		require.NoError(t, err, "Setup failed: Could not create existing user for duplicate test")

		userToUpdate := &models.User{
			ID:       userID,
			Username: "existinguser",
			Role:     "admin",
		}
		err = repo.UpdateUser(ctx, userToUpdate)
		require.Error(t, err, "UpdateUser should fail for duplicate username")
		assert.ErrorIs(t, err, db.ErrConflict, "UpdateUser: expected ErrConflict for duplicate username")
	})

	t.Run("UpdateUserPassword", func(t *testing.T) {
		newPassword := "newSecurePassword"
		err = repo.UpdateUserPassword(ctx, userID, newPassword)
		require.NoError(t, err, "UpdateUserPassword failed")

		retrievedUser, err := repo.GetUserByID(ctx, userID)
		require.NoError(t, err, "Failed to retrieve user after password update")
		require.NotNil(t, retrievedUser, "Failed to retrieve user after password update")
		assert.True(t, db.CheckPassword(newPassword, retrievedUser.PasswordHash), "UpdateUserPassword: new password hash does not match")
	})

	t.Run("ListUsers_ActiveOnly", func(t *testing.T) {
		err := repo.CreateUser(ctx, "listuser2", "dummyhash", "viewer")
		require.NoError(t, err, "Failed to create second user for list test")

		users, total, err := repo.ListUsers(ctx, 10, 0, "")
		require.NoError(t, err, "ListUsers failed")

		expectedCount := 2
		existingCheck, _ := repo.GetUser(ctx, "existinguser")
		if existingCheck != nil {
			expectedCount = 3
		}

		assert.Len(t, users, expectedCount, "ListUsers: incorrect number of active users")
		assert.Equal(t, expectedCount, total, "ListUsers: incorrect total count for active users")

		foundUpdated := false
		foundListUser2 := false
		for _, u := range users {
			if u.ID == userID && u.Username == "testuser-updated" {
				foundUpdated = true
			}
			if u.Username == "listuser2" {
				foundListUser2 = true
			}
			assert.True(t, u.IsActive, "ListUsers: Found inactive user in list: %+v", u)
		}
		assert.True(t, foundUpdated, "ListUsers: did not find the updated user 'testuser-updated' in the list")
		assert.True(t, foundListUser2, "ListUsers: did not find the user 'listuser2' in the list")
	})

	t.Run("DeleteUser_Soft", func(t *testing.T) {
		err := repo.DeleteUser(ctx, userID)
		require.NoError(t, err, "DeleteUser (soft) failed")

		_, err = repo.GetUserByID(ctx, userID)
		require.Error(t, err, "Expected GetUserByID to fail after soft delete")
		assert.ErrorIs(t, err, db.ErrNotFound, "DeleteUser: expected GetUserByID to fail with ErrNotFound after soft delete")

		_, err = repo.GetUser(ctx, "testuser-updated")
		require.Error(t, err, "Expected GetUser to fail after soft delete")
		assert.ErrorIs(t, err, db.ErrNotFound, "DeleteUser: expected GetUser to fail with ErrNotFound after soft delete")

		usersAfterDelete, totalAfterDelete, err := repo.ListUsers(ctx, 10, 0, "")
		require.NoError(t, err, "Failed to list users after delete")
		foundDeleted := false
		for _, u := range usersAfterDelete {
			if u.ID == userID {
				foundDeleted = true
				break
			}
		}
		assert.False(t, foundDeleted, "DeleteUser: Soft-deleted user %d was still found in ListUsers result", userID)
		assert.Equal(t, len(usersAfterDelete), totalAfterDelete, "ListUsers: total mismatch after delete")
	})

	t.Run("UpdateUser_Inactive", func(t *testing.T) {
		// UserID was soft-deleted in the previous test
		inactiveUserUpdate := &models.User{
			ID:       userID,
			Username: "testuser-inactive-update",
			Role:     "inactive",
		}
		err := repo.UpdateUser(ctx, inactiveUserUpdate)
		require.Error(t, err, "Expected UpdateUser to fail for inactive user")
		assert.ErrorContains(t, err, "cannot update inactive user")
	})

	t.Run("UpdateUserPassword_Inactive", func(t *testing.T) {
		err := repo.UpdateUserPassword(ctx, userID, "newpassforinactive")
		require.Error(t, err, "Expected UpdateUserPassword to fail for inactive user")
		assert.ErrorContains(t, err, "cannot update password for inactive user")
	})

	t.Run("DeleteUser_AlreadyInactive", func(t *testing.T) {
		err := repo.DeleteUser(ctx, userID)
		require.Error(t, err, "Expected DeleteUser to fail for already inactive user")
		assert.ErrorContains(t, err, "not found for deletion")
	})
}

func TestRepository_ServerOperations_Extended(t *testing.T) {
	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	// Create a test environment first
	testEnv, err := repo.CreateEnvironment(ctx, "extended-test-env", "git@github.com:test/repo-ext.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	initialName := "Extended Test Server"

	createdServer, err := repo.CreateServer(ctx, initialName, testEnv.ID)
	require.NoError(t, err, "Failed to create initial server")
	require.NotNil(t, createdServer, "CreateServer returned nil server")
	serverID := createdServer.ID
	// Git configuration is inherited from environment, not stored on server

	t.Run("UpdateServer", func(t *testing.T) {
		updatedServer := &models.Server{
			ID:   serverID,
			Name: "Updated Server Name",
		}
		err := repo.UpdateServer(ctx, updatedServer)
		require.NoError(t, err, "UpdateServer failed")

		retrieved, err := repo.GetServerByID(ctx, serverID)
		require.NoError(t, err, "Failed to retrieve server after update")
		require.NotNil(t, retrieved, "Failed to retrieve server after update")
		assert.Equal(t, "Updated Server Name", retrieved.Name)
		assert.True(t, retrieved.UpdatedAt.After(retrieved.CreatedAt))
	})

	t.Run("ListServers", func(t *testing.T) {

		_, err = repo.CreateServer(ctx, "List Server 2", testEnv.ID)
		require.NoError(t, err, "Failed to create second server for list test")

		servers, err := repo.ListServers(ctx, 10, 0)
		require.NoError(t, err, "ListServers failed")
		assert.Len(t, servers, 2, "ListServers: expected 2 servers")

		found := false
		for _, s := range servers {
			if s.ID == serverID && s.Name == "Updated Server Name" {
				found = true
				break
			}
		}
		assert.True(t, found, "ListServers: did not find the updated server in the list")
	})

	t.Run("UpdateTargetCommitHash", func(t *testing.T) {
		targetCommit := "abc123456789def"
		updatedServer, err := repo.GetServerByID(ctx, serverID)
		require.NoError(t, err, "Failed to get server before updating target commit")
		err = repo.UpdateTargetCommitHash(ctx, updatedServer.Name, targetCommit)
		require.NoError(t, err, "UpdateTargetCommitHash failed")

		retrieved, err := repo.GetServerByID(ctx, serverID)
		require.NoError(t, err, "Failed to retrieve server after updating target commit hash")
		require.NotNil(t, retrieved, "Failed to retrieve server after updating target commit hash")
		assert.NotNil(t, retrieved.TargetCommitHash, "TargetCommitHash should not be nil after update")
		assert.Equal(t, targetCommit, *retrieved.TargetCommitHash, "TargetCommitHash not updated correctly")

		err = repo.UpdateTargetCommitHash(ctx, "non-existent-id", targetCommit)
		assert.Error(t, err, "Expected error when updating non-existent server")
		assert.ErrorIs(t, err, db.ErrNotFound, "Expected ErrNotFound when updating non-existent server")
	})

	t.Run("DeleteServer", func(t *testing.T) {
		err = repo.CreateAgentStatus(ctx, &models.AgentStatus{
			ServerID:   serverID,
			Status:     "running",
			CommitHash: "cascade123",
			Timestamp:  time.Now(),
		})
		require.NoError(t, err, "Failed to add agent status before server delete")

		err = repo.DeleteServer(ctx, serverID)
		require.NoError(t, err, "DeleteServer failed")

		_, err = repo.GetServerByID(ctx, serverID)
		require.Error(t, err, "Expected error getting deleted server")
		assert.ErrorIs(t, err, db.ErrNotFound, "DeleteServer: expected GetServerByID to fail with ErrNotFound after deletion")

		history, err := repo.GetAgentStatusHistory(ctx, serverID, 10, 0)
		require.NoError(t, err, "DeleteServer: failed to check agent status history after deletion")
		assert.Empty(t, history, "DeleteServer: expected agent status history to be empty after cascade delete")
	})
}

func TestRepository_AgentStatusOperations_History(t *testing.T) {
	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	// Create a test environment first
	testEnv, err := repo.CreateEnvironment(ctx, "history-test-env", "https://github.com/test/history.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	createdServer, err := repo.CreateServer(ctx, "History Test Server", testEnv.ID)
	require.NoError(t, err, "Failed to create server for history test")
	serverID := createdServer.ID

	errMsgFailed := "build failed"
	statuses := []*models.AgentStatus{
		{ServerID: serverID, Status: "deploying", CommitHash: "hist1", Timestamp: time.Now().UTC().Add(-3 * time.Minute)},
		{ServerID: serverID, Status: "success", CommitHash: "hist2", Timestamp: time.Now().UTC().Add(-2 * time.Minute)},
		{ServerID: serverID, Status: "deploying", CommitHash: "hist3", Timestamp: time.Now().UTC().Add(-1 * time.Minute)},
		{ServerID: serverID, Status: "failed", CommitHash: "hist3", ErrorMessage: &errMsgFailed, Timestamp: time.Now().UTC()},
	}
	for _, s := range statuses {
		err = repo.CreateAgentStatus(ctx, s)
		require.NoError(t, err, "Failed to add agent status %+v", s)
	}

	t.Run("GetAgentStatusHistory_All", func(t *testing.T) {
		history, err := repo.GetAgentStatusHistory(ctx, serverID, 10, 0)
		require.NoError(t, err, "GetAgentStatusHistory failed")
		require.Len(t, history, 4, "GetAgentStatusHistory: expected 4 status records")

		assert.Equal(t, "failed", history[0].Status)
		assert.Equal(t, "hist3", history[0].CommitHash)
		require.NotNil(t, history[0].ErrorMessage)
		assert.Equal(t, errMsgFailed, *history[0].ErrorMessage)

		assert.Equal(t, "deploying", history[1].Status)
		assert.Equal(t, "hist3", history[1].CommitHash)
		assert.Nil(t, history[1].ErrorMessage)

		assert.Equal(t, "success", history[2].Status)
		assert.Equal(t, "hist2", history[2].CommitHash)
		assert.Nil(t, history[2].ErrorMessage)

		assert.Equal(t, "deploying", history[3].Status)
		assert.Equal(t, "hist1", history[3].CommitHash)
		assert.Nil(t, history[3].ErrorMessage)
	})

	t.Run("GetAgentStatusHistory_Paginated", func(t *testing.T) {
		historyPage1, err := repo.GetAgentStatusHistory(ctx, serverID, 2, 0)
		require.NoError(t, err, "GetAgentStatusHistory (page 1) failed")
		require.Len(t, historyPage1, 2, "GetAgentStatusHistory (page 1): expected 2 status records")
		assert.Equal(t, "failed", historyPage1[0].Status)
		assert.Equal(t, "deploying", historyPage1[1].Status)
		assert.Equal(t, "hist3", historyPage1[1].CommitHash)

		historyPage2, err := repo.GetAgentStatusHistory(ctx, serverID, 2, 2)
		require.NoError(t, err, "GetAgentStatusHistory (page 2) failed")
		require.Len(t, historyPage2, 2, "GetAgentStatusHistory (page 2): expected 2 status records")
		assert.Equal(t, "success", historyPage2[0].Status)
		assert.Equal(t, "deploying", historyPage2[1].Status)
		assert.Equal(t, "hist1", historyPage2[1].CommitHash)
	})

	t.Run("GetAgentStatusHistory_NonExistentServer", func(t *testing.T) {
		history, err := repo.GetAgentStatusHistory(ctx, "non-existent-server-hist", 10, 0)
		require.NoError(t, err, "GetAgentStatusHistory for non-existent server failed unexpectedly")
		assert.Empty(t, history, "GetAgentStatusHistory for non-existent server expected empty slice")
	})
}

func TestRepository_ServerWithLatestStatus_NullHandling(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	// Create a test environment first
	testEnv, err := repo.CreateEnvironment(ctx, "null-test-env", "git@github.com:test/repo.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	// Create test servers
	servers := []struct {
		id           string
		name         string
		hasEmpty     bool
		targetCommit string
	}{
		{
			id:           "server-null-test-1",
			name:         "Server with Empty Status",
			hasEmpty:     true,
			targetCommit: "abcdef123456",
		},
		{
			id:           "server-null-test-2",
			name:         "Server with No Status",
			hasEmpty:     false,
			targetCommit: "",
		},
	}

	serverIDs := make(map[string]string)
	for _, s := range servers {
		createdServer, err := repo.CreateServer(ctx, s.name, testEnv.ID)
		require.NoError(t, err, "Failed to create test server %s", s.id)
		serverIDs[s.id] = createdServer.ID // Map test ID to actual UUID

		if s.targetCommit != "" {
			err = repo.UpdateTargetCommitHash(ctx, s.name, s.targetCommit) // Use server name instead of ID
			require.NoError(t, err, "Failed to set target commit hash for server %s", s.id)
		}

		if s.hasEmpty {
			err = repo.CreateAgentStatus(ctx, &models.AgentStatus{
				ServerID:   serverIDs[s.id],
				CommitHash: "",
				Status:     "",
				Timestamp:  time.Now(),
			})
			require.NoError(t, err, "Failed to create empty agent status for server %s", s.id)
		}
	}

	t.Run("ListServersWithLatestStatus", func(t *testing.T) {
		serverList, err := repo.ListServersWithLatestStatus(ctx, 10, 0, "recent")
		require.NoError(t, err, "ListServersWithLatestStatus failed")
		require.Len(t, serverList, 2, "Expected 2 servers in list")

		// Find and verify each server
		var foundServer1, foundServer2 bool
		for _, server := range serverList {
			switch server.ID {
			case serverIDs["server-null-test-1"]:
				// Server with empty strings should have nil pointers for status fields
				foundServer1 = true
				assert.Nil(t, server.LastCommitHash, "Expected nil LastCommitHash for empty string")
				assert.Nil(t, server.LastStatus, "Expected nil LastStatus for empty string")
				assert.NotNil(t, server.TargetCommitHash, "Expected non-nil TargetCommitHash")
				assert.Equal(t, "abcdef123456", *server.TargetCommitHash, "Incorrect TargetCommitHash value")
			case serverIDs["server-null-test-2"]:
				// Server with no status should have nil pointers
				foundServer2 = true
				assert.Nil(t, server.LastCommitHash, "Expected nil LastCommitHash for no status")
				assert.Nil(t, server.LastStatus, "Expected nil LastStatus for no status")
				assert.Nil(t, server.TargetCommitHash, "Expected nil TargetCommitHash")
			default:
				t.Errorf("Unexpected server ID in results: %s", server.ID)
			}
		}
		assert.True(t, foundServer1, "Did not find server 1 in results")
		assert.True(t, foundServer2, "Did not find server 2 in results")
	})

	t.Run("GetServerWithLatestStatusByID", func(t *testing.T) {
		server1, err := repo.GetServerWithLatestStatusByID(ctx, serverIDs["server-null-test-1"])
		require.NoError(t, err, "GetServerWithLatestStatusByID failed for server 1")
		assert.Nil(t, server1.LastCommitHash, "Expected nil LastCommitHash for empty string")
		assert.Nil(t, server1.LastStatus, "Expected nil LastStatus for empty string")
		assert.NotNil(t, server1.TargetCommitHash, "Expected non-nil TargetCommitHash")
		assert.Equal(t, "abcdef123456", *server1.TargetCommitHash, "Incorrect TargetCommitHash value")

		server2, err := repo.GetServerWithLatestStatusByID(ctx, serverIDs["server-null-test-2"])
		require.NoError(t, err, "GetServerWithLatestStatusByID failed for server 2")
		assert.Nil(t, server2.LastCommitHash, "Expected nil LastCommitHash for no status")
		assert.Nil(t, server2.LastStatus, "Expected nil LastStatus for no status")
		assert.Nil(t, server2.TargetCommitHash, "Expected nil TargetCommitHash")
	})
}

// TestRepository_AgentStatusComparison tests the error message comparison logic in CreateAgentStatus
func TestRepository_AgentStatusComparison(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	// Create a test environment first
	testEnv, err := repo.CreateEnvironment(ctx, "comparison-test-env", "https://github.com/test/comparison.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	createdServer, err := repo.CreateServer(ctx, "Error Comparison Test Server", testEnv.ID)
	require.NoError(t, err, "Setup: Failed to create server for error comparison test")
	serverID := createdServer.ID

	timestamp := time.Now().UTC().Truncate(time.Microsecond)
	initialStatus := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   "commit1",
		IsDrifted:    false,
		Status:       "synced",
		Timestamp:    timestamp,
		ErrorMessage: nil,
	}
	err = repo.CreateAgentStatus(ctx, initialStatus)
	require.NoError(t, err, "Failed to create initial agent status")
	require.NotZero(t, initialStatus.ID, "Initial status should be recorded with non-zero ID")

	timestamp = time.Now().UTC().Truncate(time.Microsecond)
	emptyMsg := ""
	sameStatusEmptyError := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   "commit1",
		IsDrifted:    false,
		Status:       "synced",
		Timestamp:    timestamp,
		ErrorMessage: &emptyMsg,
	}
	err = repo.CreateAgentStatus(ctx, sameStatusEmptyError)
	require.NoError(t, err, "Failed during same status with empty error test")
	require.Zero(t, sameStatusEmptyError.ID, "Empty string error should be treated same as nil - no new record")

	timestamp = time.Now().UTC().Truncate(time.Microsecond)
	errorMsg := "error detected"
	statusWithError := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   "commit1",
		IsDrifted:    false,
		Status:       "synced",
		Timestamp:    timestamp,
		ErrorMessage: &errorMsg,
	}
	err = repo.CreateAgentStatus(ctx, statusWithError)
	require.NoError(t, err, "Failed to create status with error message")
	require.NotZero(t, statusWithError.ID, "Status with new error should be recorded with non-zero ID")

	timestamp = time.Now().UTC().Truncate(time.Microsecond)
	statusWithNilError := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   "commit1",
		IsDrifted:    false,
		Status:       "synced",
		Timestamp:    timestamp,
		ErrorMessage: nil,
	}
	err = repo.CreateAgentStatus(ctx, statusWithNilError)
	require.NoError(t, err, "Failed to create status with nil error after error string")
	require.NotZero(t, statusWithNilError.ID, "Status with nil error after string should be recorded with non-zero ID")

	// Test case 5: Same status with empty error message - should be treated same as nil
	timestamp = time.Now().UTC().Truncate(time.Microsecond)
	emptyMsg = ""
	statusWithEmptyError := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   "commit1",
		IsDrifted:    false,
		Status:       "synced",
		Timestamp:    timestamp,
		ErrorMessage: &emptyMsg,
	}
	err = repo.CreateAgentStatus(ctx, statusWithEmptyError)
	require.NoError(t, err, "Failed to create status with empty error message")
	require.Zero(t, statusWithEmptyError.ID, "Empty error should be treated same as nil - no new record")

	timestamp = time.Now().UTC().Truncate(time.Microsecond)
	statusWithDrift := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   "commit1",
		IsDrifted:    true,
		Status:       "synced",
		Timestamp:    timestamp,
		ErrorMessage: nil,
	}
	err = repo.CreateAgentStatus(ctx, statusWithDrift)
	require.NoError(t, err, "Failed to create status with drift change")
	require.NotZero(t, statusWithDrift.ID, "Status with drift change should be recorded with non-zero ID")
}

func TestRepository_GetEnvironmentCommitHistory(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	// Insert test environment
	var envID int64
	err := tdb.DB.QueryRow(
		"INSERT INTO environments (name, repo_url, branch, deploy_path, provider, github_installation_id, webhook_secret, webhook_url, status) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active') RETURNING id",
		"test-env",
		"https://github.com/test/repo.git",
		"main",
		"config.yaml",
		"github",
		int64(12345),
		"test-secret",
		"http://test.com/webhook",
	).Scan(&envID)
	require.NoError(t, err, "Failed to create test environment")

	// Create a server associated with the environment
	testServer, err := repo.CreateServer(ctx, "Test Server", envID)
	require.NoError(t, err, "Failed to create test server")

	// Add some agent status entries with different commits
	statuses := []*models.AgentStatus{
		{
			ServerID:   testServer.ID,
			CommitHash: "abc123def456",
			Status:     "deployed",
			IsDrifted:  false,
			Timestamp:  time.Now().UTC().Add(-2 * time.Hour),
		},
		{
			ServerID:   testServer.ID,
			CommitHash: "def456ghi789",
			Status:     "deployed",
			IsDrifted:  false,
			Timestamp:  time.Now().UTC().Add(-1 * time.Hour),
		},
		{
			ServerID:   testServer.ID,
			CommitHash: "ghi789jkl012",
			Status:     "deployed",
			IsDrifted:  false,
			Timestamp:  time.Now().UTC(),
		},
	}

	for _, status := range statuses {
		err = repo.CreateAgentStatus(ctx, status)
		require.NoError(t, err, "Failed to create agent status")
	}

	// Test GetEnvironmentCommitHistory
	commits, err := repo.GetEnvironmentCommitHistory(ctx, envID, 10)
	require.NoError(t, err, "GetEnvironmentCommitHistory failed")

	// Should return commits in reverse chronological order (newest first)
	require.Len(t, commits, 3, "Expected 3 unique commits")

	// Verify the commits are returned in correct order (newest first)
	assert.Equal(t, "ghi789jkl012", commits[0].Hash, "Expected newest commit first")
	assert.Equal(t, "def456ghi789", commits[1].Hash, "Expected second newest commit")
	assert.Equal(t, "abc123def456", commits[2].Hash, "Expected oldest commit last")

	// Test with limit
	limitedCommits, err := repo.GetEnvironmentCommitHistory(ctx, envID, 2)
	require.NoError(t, err, "GetEnvironmentCommitHistory with limit failed")
	require.Len(t, limitedCommits, 2, "Expected only 2 commits with limit")

	// Test with non-existent environment
	emptyCommits, err := repo.GetEnvironmentCommitHistory(ctx, 99999, 10)
	require.NoError(t, err, "GetEnvironmentCommitHistory for non-existent environment should not error")
	assert.Empty(t, emptyCommits, "Expected no commits for non-existent environment")
}

func TestRepository_AgentVersionStorage(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	env, err := repo.CreateEnvironment(ctx, "version-test-env", "https://github.com/test/repo.git", "main", false)
	require.NoError(t, err, "Failed to create test environment")

	server, err := repo.CreateServer(ctx, "Version Test Server", env.ID)
	require.NoError(t, err, "Failed to create test server")

	t.Run("CreateAgentStatus stores agent version", func(t *testing.T) {
		agentVersion := "v1.2.3"
		status := &models.AgentStatus{
			ServerID:     server.ID,
			CommitHash:   "abc123",
			IsDrifted:    false,
			Status:       "Applied",
			Timestamp:    time.Now().UTC(),
			AgentVersion: &agentVersion,
		}

		err := repo.CreateAgentStatus(ctx, status)
		require.NoError(t, err, "CreateAgentStatus should succeed")
		require.NotZero(t, status.ID, "Status ID should be set")

		latestStatus, err := repo.GetLatestAgentStatus(ctx, server.ID)
		require.NoError(t, err, "GetLatestAgentStatus should succeed")
		require.NotNil(t, latestStatus.AgentVersion, "Agent version should not be nil")
		assert.Equal(t, agentVersion, *latestStatus.AgentVersion, "Agent version should match")
	})

	t.Run("CreateAgentStatus handles nil agent version", func(t *testing.T) {
		status := &models.AgentStatus{
			ServerID:     server.ID,
			CommitHash:   "def456",
			IsDrifted:    false,
			Status:       "Syncing",
			Timestamp:    time.Now().UTC().Add(time.Second),
			AgentVersion: nil,
		}

		err := repo.CreateAgentStatus(ctx, status)
		require.NoError(t, err, "CreateAgentStatus should succeed with nil version")

		latestStatus, err := repo.GetLatestAgentStatus(ctx, server.ID)
		require.NoError(t, err)
		assert.Nil(t, latestStatus.AgentVersion, "Agent version should be nil")
	})

	t.Run("CreateAgentStatus updates agent version", func(t *testing.T) {
		oldVersion := "v1.0.0"
		newVersion := "v2.0.0"

		status1 := &models.AgentStatus{
			ServerID:     server.ID,
			CommitHash:   "version1",
			IsDrifted:    false,
			Status:       "Applied",
			Timestamp:    time.Now().UTC().Add(2 * time.Second),
			AgentVersion: &oldVersion,
		}
		err := repo.CreateAgentStatus(ctx, status1)
		require.NoError(t, err)

		status2 := &models.AgentStatus{
			ServerID:     server.ID,
			CommitHash:   "version2",
			IsDrifted:    false,
			Status:       "Applied",
			Timestamp:    time.Now().UTC().Add(3 * time.Second),
			AgentVersion: &newVersion,
		}
		err = repo.CreateAgentStatus(ctx, status2)
		require.NoError(t, err)

		latestStatus, err := repo.GetLatestAgentStatus(ctx, server.ID)
		require.NoError(t, err)
		require.NotNil(t, latestStatus.AgentVersion)
		assert.Equal(t, newVersion, *latestStatus.AgentVersion, "Should have latest version")
	})
}

func TestRepository_ServerWithStatusAgentVersion(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := setupTestDB(t)
	repo := tdb.Repository()
	ctx := tdb.Context()

	env, err := repo.CreateEnvironment(ctx, "status-version-env", "https://github.com/test/repo.git", "main", false)
	require.NoError(t, err)

	server1, err := repo.CreateServer(ctx, "Server With Version", env.ID)
	require.NoError(t, err)

	server2, err := repo.CreateServer(ctx, "Server Without Version", env.ID)
	require.NoError(t, err)

	agentVersion := "v3.1.4"
	err = repo.CreateAgentStatus(ctx, &models.AgentStatus{
		ServerID:     server1.ID,
		CommitHash:   "commit1",
		IsDrifted:    false,
		Status:       "Applied",
		Timestamp:    time.Now().UTC(),
		AgentVersion: &agentVersion,
	})
	require.NoError(t, err)

	err = repo.CreateAgentStatus(ctx, &models.AgentStatus{
		ServerID:     server2.ID,
		CommitHash:   "commit2",
		IsDrifted:    false,
		Status:       "Applied",
		Timestamp:    time.Now().UTC(),
		AgentVersion: nil,
	})
	require.NoError(t, err)

	t.Run("GetServerWithLatestStatusByID returns agent version", func(t *testing.T) {
		serverWithStatus, err := repo.GetServerWithLatestStatusByID(ctx, server1.ID)
		require.NoError(t, err)
		require.NotNil(t, serverWithStatus.LastAgentVersion, "LastAgentVersion should not be nil")
		assert.Equal(t, agentVersion, *serverWithStatus.LastAgentVersion, "Should return correct agent version")
	})

	t.Run("GetServerWithLatestStatusByID handles nil agent version", func(t *testing.T) {
		serverWithStatus, err := repo.GetServerWithLatestStatusByID(ctx, server2.ID)
		require.NoError(t, err)
		assert.Nil(t, serverWithStatus.LastAgentVersion, "LastAgentVersion should be nil for server without version")
	})

	t.Run("ListServersWithLatestStatus returns agent versions", func(t *testing.T) {
		servers, err := repo.ListServersWithLatestStatus(ctx, 10, 0, "")
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(servers), 2, "Should have at least 2 servers")

		var foundServer1, foundServer2 bool
		for _, s := range servers {
			if s.ID == server1.ID {
				foundServer1 = true
				require.NotNil(t, s.LastAgentVersion, "Server1 should have agent version")
				assert.Equal(t, agentVersion, *s.LastAgentVersion)
			}
			if s.ID == server2.ID {
				foundServer2 = true
				assert.Nil(t, s.LastAgentVersion, "Server2 should not have agent version")
			}
		}
		assert.True(t, foundServer1, "Should find server1 in list")
		assert.True(t, foundServer2, "Should find server2 in list")
	})
}
