package testutil

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pullbase/pullbase/server/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

// TestUser represents a test user fixture
type TestUser struct {
	*models.User
	PlainPassword string // Store plain password for login tests
}

// TestEnvironment represents a test environment fixture
type TestEnvironment struct {
	*models.Environment
}

// TestServer represents a test server fixture
type TestServer struct {
	*models.Server
}

// FixtureOptions holds options for creating test fixtures
type FixtureOptions struct {
	UserRole        string
	EnvironmentName string
	ServerName      string
	Timestamp       time.Time
}

// DefaultFixtureOptions returns default options for test fixtures
func DefaultFixtureOptions() FixtureOptions {
	return FixtureOptions{
		UserRole:        models.RoleUser,
		EnvironmentName: "test-env",
		ServerName:      "test-server",
		Timestamp:       time.Now().UTC(),
	}
}

// CreateTestUser creates a test user with the given username and options
func (tdb *TestDB) CreateTestUser(username string, opts ...FixtureOptions) *TestUser {
	tdb.t.Helper()

	options := DefaultFixtureOptions()
	if len(opts) > 0 {
		if opts[0].UserRole != "" {
			options.UserRole = opts[0].UserRole
		}
	}

	plainPassword := "testpass123"
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
	if err != nil {
		tdb.t.Fatalf("Failed to hash password: %v", err)
	}

	repo := tdb.Repository()
	ctx := tdb.Context()

	err = repo.CreateUser(ctx, username, string(hashedPassword), options.UserRole)
	if err != nil {
		tdb.t.Fatalf("Failed to create test user: %v", err)
	}

	user, err := repo.GetUser(ctx, username)
	if err != nil {
		tdb.t.Fatalf("Failed to retrieve created user: %v", err)
	}

	return &TestUser{
		User:          user,
		PlainPassword: plainPassword,
	}
}

// CreateTestEnvironment creates a test environment with the given name and options
func (tdb *TestDB) CreateTestEnvironment(name string, repoURL string, opts ...FixtureOptions) *TestEnvironment {
	tdb.t.Helper()

	env := &models.Environment{
		Name:           name,
		RepoURL:        repoURL,
		Provider:       models.ProviderGitHub,
		InstallationID: time.Now().UnixNano(),
		WebhookSecret:  "test-secret-" + name,
		WebhookURL:     "https://example.com/webhooks/" + name,
		Status:         string(models.StatusActive),
		AutoReconcile:  true,
	}

	repo := tdb.EnvironmentRepository()
	ctx := tdb.Context()

	err := repo.CreateEnvironment(ctx, env)
	if err != nil {
		tdb.t.Fatalf("Failed to create test environment: %v", err)
	}

	return &TestEnvironment{Environment: env}
}

// CreateTestServer creates a test server with the given name and environment
func (tdb *TestDB) CreateTestServer(name string, environmentID int64, opts ...FixtureOptions) *TestServer {
	tdb.t.Helper()

	repo := tdb.Repository()
	ctx := tdb.Context()

	server, err := repo.CreateServer(ctx, name, environmentID)
	if err != nil {
		tdb.t.Fatalf("Failed to create test server: %v", err)
	}

	return &TestServer{Server: server}
}

// CreateTestAgentStatus creates a test agent status for the given server
func (tdb *TestDB) CreateTestAgentStatus(serverID, commitHash, status string, isDrifted bool, errMsg *string) *models.AgentStatus {
	tdb.t.Helper()

	agentStatus := &models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   commitHash,
		IsDrifted:    isDrifted,
		Status:       status,
		ErrorMessage: errMsg,
		Timestamp:    time.Now().UTC(),
	}

	repo := tdb.Repository()
	ctx := tdb.Context()

	err := repo.CreateAgentStatus(ctx, agentStatus)
	if err != nil {
		tdb.t.Fatalf("Failed to create test agent status: %v", err)
	}

	return agentStatus
}

// CreateTestAuditLog creates a test audit log entry
func (tdb *TestDB) CreateTestAuditLog(userID *int, action, resourceType, resourceID string) *models.AuditLog {
	tdb.t.Helper()

	auditLog := &models.AuditLog{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      json.RawMessage(`{"test": true}`),
		IPAddress:    "127.0.0.1",
		Timestamp:    time.Now().UTC(),
	}

	repo := tdb.Repository()
	ctx := tdb.Context()

	err := repo.CreateAuditLog(ctx, auditLog)
	if err != nil {
		tdb.t.Fatalf("Failed to create test audit log: %v", err)
	}

	return auditLog
}

// CreateTestPull creates a test pull request
func (tdb *TestDB) CreateTestPull(id, title, status string) *models.Pull {
	tdb.t.Helper()

	pull := &models.Pull{
		ID:          id,
		Title:       title,
		Description: "Test pull request description",
		Status:      status,
	}

	repo := tdb.Repository()
	ctx := tdb.Context()

	err := repo.CreatePull(ctx, pull)
	if err != nil {
		tdb.t.Fatalf("Failed to create test pull: %v", err)
	}

	return pull
}

// CreateTestRollbackEvent creates a test rollback event
func (tdb *TestDB) CreateTestRollbackEvent(environmentID int64, fromCommit, toCommit, initiatedBy, status string) *models.RollbackEvent {
	tdb.t.Helper()

	rollbackEvent := &models.RollbackEvent{
		EnvironmentID: environmentID,
		FromCommit:    fromCommit,
		ToCommit:      toCommit,
		InitiatedBy:   initiatedBy,
		Status:        status,
		Reason:        "Test rollback",
		CreatedAt:     time.Now().UTC(),
	}

	ctx := tdb.Context()

	// Direct SQL insert since there might not be a CreateRollbackEvent method
	query := `
		INSERT INTO rollback_events (environment_id, from_commit, to_commit, initiated_by, status, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	err := tdb.DB.QueryRowContext(ctx, query,
		rollbackEvent.EnvironmentID,
		rollbackEvent.FromCommit,
		rollbackEvent.ToCommit,
		rollbackEvent.InitiatedBy,
		rollbackEvent.Status,
		rollbackEvent.Reason,
		rollbackEvent.CreatedAt,
	).Scan(&rollbackEvent.ID)

	if err != nil {
		tdb.t.Fatalf("Failed to create test rollback event: %v", err)
	}

	return rollbackEvent
}

// CreateUserFixtures creates multiple test users with different roles
func (tdb *TestDB) CreateUserFixtures() map[string]*TestUser {
	tdb.t.Helper()

	fixtures := make(map[string]*TestUser)

	fixtures["admin"] = tdb.CreateTestUser("test-admin", FixtureOptions{UserRole: models.RoleAdmin})
	fixtures["user"] = tdb.CreateTestUser("test-user", FixtureOptions{UserRole: models.RoleUser})
	fixtures["viewer"] = tdb.CreateTestUser("test-viewer", FixtureOptions{UserRole: models.RoleViewer})
	fixtures["agent"] = tdb.CreateTestUser("test-agent", FixtureOptions{UserRole: models.RoleAgent})

	return fixtures
}

// CreateEnvironmentFixtures creates multiple test environments
func (tdb *TestDB) CreateEnvironmentFixtures() map[string]*TestEnvironment {
	tdb.t.Helper()

	fixtures := make(map[string]*TestEnvironment)

	fixtures["development"] = tdb.CreateTestEnvironment("development", "https://github.com/test/dev-config.git")
	fixtures["staging"] = tdb.CreateTestEnvironment("staging", "https://github.com/test/staging-config.git")
	fixtures["production"] = tdb.CreateTestEnvironment("production", "https://github.com/test/prod-config.git")

	return fixtures
}

// CreateCompleteFixtures creates a complete set of test fixtures including users, environments, and servers
func (tdb *TestDB) CreateCompleteFixtures() map[string]interface{} {
	tdb.t.Helper()

	fixtures := make(map[string]interface{})

	// Create users
	fixtures["users"] = tdb.CreateUserFixtures()

	// Create environments
	envFixtures := tdb.CreateEnvironmentFixtures()
	fixtures["environments"] = envFixtures

	// Create servers for each environment
	serverFixtures := make(map[string]*TestServer)
	for envName, env := range envFixtures {
		serverName := fmt.Sprintf("%s-server", envName)
		serverFixtures[envName] = tdb.CreateTestServer(serverName, env.ID)
	}
	fixtures["servers"] = serverFixtures

	return fixtures
}

// SeedDatabase seeds the database with a realistic set of test data
func (tdb *TestDB) SeedDatabase() map[string]interface{} {
	tdb.t.Helper()

	fixtures := tdb.CreateCompleteFixtures()

	users := fixtures["users"].(map[string]*TestUser)
	envs := fixtures["environments"].(map[string]*TestEnvironment)
	servers := fixtures["servers"].(map[string]*TestServer)

	// Create some agent statuses
	tdb.CreateTestAgentStatus(servers["development"].ID, "abc123", "running", false, nil)
	tdb.CreateTestAgentStatus(servers["staging"].ID, "def456", "syncing", false, nil)

	errorMsg := "drift detected"
	tdb.CreateTestAgentStatus(servers["production"].ID, "ghi789", "error", true, &errorMsg)

	// Create some audit logs
	tdb.CreateTestAuditLog(&users["admin"].ID, "create", "server", servers["development"].ID)
	tdb.CreateTestAuditLog(&users["user"].ID, "update", "server", servers["staging"].ID)

	// Create some pull requests
	tdb.CreateTestPull("pr-001", "Feature: Add new functionality", "open")
	tdb.CreateTestPull("pr-002", "Fix: Critical bug fix", "merged")

	// Create some rollback events
	tdb.CreateTestRollbackEvent(envs["development"].ID, "abc123", "def456", users["admin"].Username, "completed")
	tdb.CreateTestRollbackEvent(envs["staging"].ID, "ghi789", "jkl012", users["user"].Username, "pending")

	return fixtures
}
