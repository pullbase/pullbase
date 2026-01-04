//go:build integration
// +build integration

package testutil_test

import (
	"testing"

	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestUtilBasicSetup(t *testing.T) {
	testutil.SkipIfShort(t)

	// Basic database setup
	tdb := testutil.SetupTestDB(t)
	defer tdb.Close() // Optional since t.Cleanup handles this

	// Verify database connection
	err := tdb.DB.Ping()
	require.NoError(t, err)

	// Test repository access
	repo := tdb.Repository()
	require.NotNil(t, repo)

	envRepo := tdb.EnvironmentRepository()
	require.NotNil(t, envRepo)
}

func TestTestUtilCustomConfig(t *testing.T) {
	testutil.SkipIfShort(t)

	config := testutil.ContainerConfig{
		Database: "custom_testdb",
		Username: "custom_user",
		Password: "custom_pass",
	}

	tdb := testutil.SetupTestDBWithConfig(t, config)

	assert.Equal(t, "custom_testdb", tdb.Config.DatabaseName)
	assert.Equal(t, "custom_user", tdb.Config.User)
}

func TestTestUtilFixtures(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := testutil.SetupTestDB(t)

	// Create individual fixtures
	user := tdb.CreateTestUser("testuser")
	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, models.RoleUser, user.Role)
	assert.NotEmpty(t, user.PlainPassword)

	env := tdb.CreateTestEnvironment("test-env", "https://github.com/test/repo.git")
	assert.Equal(t, "test-env", env.Name)
	assert.Equal(t, "https://github.com/test/repo.git", env.RepoURL)

	server := tdb.CreateTestServer("test-server", env.ID)
	assert.Equal(t, "test-server", server.Name)
	assert.Equal(t, env.ID, *server.EnvironmentID)

	// Create agent status
	status := tdb.CreateTestAgentStatus(server.ID, "abc123", "running", false, nil)
	assert.Equal(t, server.ID, status.ServerID)
	assert.Equal(t, "abc123", status.CommitHash)
	assert.Equal(t, "running", status.Status)
	assert.False(t, status.IsDrifted)
}

func TestTestUtilBulkFixtures(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := testutil.SetupTestDB(t)

	// Create bulk fixtures
	users := tdb.CreateUserFixtures()
	assert.Len(t, users, 4) // admin, user, viewer, agent
	assert.Equal(t, models.RoleAdmin, users["admin"].Role)
	assert.Equal(t, models.RoleUser, users["user"].Role)
	assert.Equal(t, models.RoleViewer, users["viewer"].Role)
	assert.Equal(t, models.RoleAgent, users["agent"].Role)

	envs := tdb.CreateEnvironmentFixtures()
	assert.Len(t, envs, 3) // development, staging, production
	assert.Contains(t, envs, "development")
	assert.Contains(t, envs, "staging")
	assert.Contains(t, envs, "production")
}

func TestTestUtilSeedDatabase(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := testutil.SetupTestDB(t)

	// Seed with complete test data
	fixtures := tdb.SeedDatabase()

	users := fixtures["users"].(map[string]*testutil.TestUser)
	envs := fixtures["environments"].(map[string]*testutil.TestEnvironment)
	servers := fixtures["servers"].(map[string]*testutil.TestServer)

	assert.Len(t, users, 4)
	assert.Len(t, envs, 3)
	assert.Len(t, servers, 3)

	// Verify data was actually created
	repo := tdb.Repository()
	ctx := tdb.Context()

	allUsers, total, err := repo.ListUsers(ctx, 100, 0, "")
	require.NoError(t, err)
	assert.Len(t, allUsers, 4)
	assert.Equal(t, 4, total)
}

func TestTestUtilPolling(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := testutil.SetupTestDB(t)

	// Create test data
	env := tdb.CreateTestEnvironment("test-env", "https://github.com/test/repo.git")
	server := tdb.CreateTestServer("test-server", env.ID)

	// Create initial status
	tdb.CreateTestAgentStatus(server.ID, "abc123", "running", false, nil)

	// Test polling for agent status
	tdb.WaitForAgentStatus(server.ID, "running")

	// Test polling for specific counts
	tdb.WaitForServerCount(1)
	tdb.WaitForUserCount(0) // No users created in this test
	tdb.WaitForEnvironmentCount(1)
}

func TestTestUtilHelpers(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := testutil.SetupTestDB(t)

	// Test pointer helpers
	assert.Equal(t, "test", *testutil.StringPtr("test"))
	assert.Equal(t, 42, *testutil.IntPtr(42))
	assert.Equal(t, int64(42), *testutil.Int64Ptr(42))
	assert.True(t, *testutil.BoolPtr(true))
	assert.NotNil(t, testutil.NowPtr())
	assert.NotNil(t, testutil.UTCNowPtr())
	assert.NotNil(t, testutil.TruncatedNowPtr())

	// Test database URL
	url := tdb.DatabaseURL()
	assert.Contains(t, url, "postgres://")
	assert.Contains(t, url, tdb.Config.DatabaseName)

	// Test SQL helpers
	tdb.ExecSQL("CREATE TEMP TABLE test_table (id INT PRIMARY KEY, name TEXT)")
	tdb.ExecSQL("INSERT INTO test_table (id, name) VALUES ($1, $2)", 1, "test")

	var name string
	tdb.QueryRowSQL("SELECT name FROM test_table WHERE id = $1", 1).MustScan(&name)
	assert.Equal(t, "test", name)
}

func TestTestUtilResetTables(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := testutil.SetupTestDB(t)

	// Create some data
	tdb.CreateTestUser("user1")
	tdb.CreateTestEnvironment("env1", "https://github.com/test/repo1.git")

	// Verify data exists
	tdb.WaitForUserCount(1)
	tdb.WaitForEnvironmentCount(1)

	// Reset all tables
	err := tdb.ResetTables()
	require.NoError(t, err)

	// Verify data is gone
	tdb.WaitForUserCount(0)
	tdb.WaitForEnvironmentCount(0)
}

func TestTestUtilAssertEventually(t *testing.T) {
	testutil.SkipIfShort(t)

	tdb := testutil.SetupTestDB(t)

	// Create test user
	user := tdb.CreateTestUser("testuser")

	// Test AssertEventuallyEqual
	testutil.AssertEventuallyEqual(t, func() (string, error) {
		repo := tdb.Repository()
		ctx := tdb.Context()
		u, err := repo.GetUser(ctx, "testuser")
		if err != nil {
			return "", err
		}
		return u.Username, nil
	}, "testuser")

	// Test AssertEventuallyTrue
	testutil.AssertEventuallyTrue(t, func() (bool, error) {
		repo := tdb.Repository()
		ctx := tdb.Context()
		u, err := repo.GetUser(ctx, "testuser")
		if err != nil {
			return false, err
		}
		return u.ID == user.ID, nil
	})
}
