// Package testutil provides shared database setup and test utilities for integration tests.
package testutil

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/logging"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	DefaultPostgresImage = "docker.io/postgres:17-alpine"

	DefaultMigrationPath = "file://migrations"

	DefaultTestTimeout = 3 * time.Minute

	DefaultStartupTimeout = 5 * time.Minute

	// FallbackSchemaVersion is the migration version the fallback schema represents.
	// Update this when adding new migrations.
	FallbackSchemaVersion = 22
)

type TestDB struct {
	*sqlx.DB
	Container testcontainers.Container
	Config    database.Config
	Dialect   database.Dialect
	cleanup   func()
	t         testing.TB
}

type ContainerConfig struct {
	Database       string
	Username       string
	Password       string
	Image          string
	StartupTimeout time.Duration
}

func DefaultContainerConfig() ContainerConfig {
	return ContainerConfig{
		Database:       "testdb",
		Username:       "testuser",
		Password:       "testpass",
		Image:          DefaultPostgresImage,
		StartupTimeout: DefaultStartupTimeout,
	}
}

func StartPostgresContainer(t testing.TB, config ContainerConfig) (*TestDB, error) {
	t.Helper()
	ctx := context.Background()

	if config.Image == "" {
		config.Image = DefaultPostgresImage
	}
	if config.StartupTimeout == 0 {
		config.StartupTimeout = DefaultStartupTimeout
	}

	container, err := postgres.Run(ctx,
		config.Image,
		postgres.WithDatabase(config.Database),
		postgres.WithUsername(config.Username),
		postgres.WithPassword(config.Password),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").
				WithStartupTimeout(config.StartupTimeout),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get container port: %w", err)
	}

	dbConfig := database.Config{
		Dialect:      database.DialectPostgres,
		Host:         host,
		Port:         port.Int(),
		User:         config.Username,
		Password:     config.Password,
		DatabaseName: config.Database,
		SSLMode:      "disable",
	}

	dbConn, dialect, err := database.New(dbConfig)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to connect to test database: %w", err)
	}

	cleanup := func() {
		if dbConn != nil {
			dbConn.Close()
		}
		if err := container.Terminate(ctx); err != nil {
			logging.Warn("could not terminate container", "error", err)
		}
	}

	testDB := &TestDB{
		DB:        dbConn,
		Container: container,
		Config:    dbConfig,
		Dialect:   dialect,
		cleanup:   cleanup,
		t:         t,
	}

	t.Cleanup(cleanup)

	return testDB, nil
}

func (tdb *TestDB) Close() {
	if tdb.cleanup != nil {
		tdb.cleanup()
	}
}

func (tdb *TestDB) MustMigrate(migrationPath string) {
	tdb.t.Helper()

	if migrationPath == "" {
		migrationPath = defaultMigrationPath()
	}
	tdb.t.Logf("using migration path: %s", migrationPath)

	ctx := context.Background()
	if err := database.InitSchema(ctx, tdb.DB, tdb.Dialect, migrationPath); err != nil {
		tdb.t.Logf("Migration failed: %v - falling back to manual schema", err)
		tdb.setupFallbackSchema()
	}
}

func defaultMigrationPath() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "file://migrations"
	}

	serverDir := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
	migrationsDir := filepath.Join(serverDir, "migrations")

	return "file://" + migrationsDir
}

func (tdb *TestDB) setupFallbackSchema() {
	tdb.t.Helper()

	_, err := tdb.DB.Exec(`
		-- Create users table
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			role VARCHAR(50) NOT NULL,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		-- Create environments table
		CREATE TABLE IF NOT EXISTS environments (
			id SERIAL PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	repo_url TEXT NOT NULL,
	branch TEXT NOT NULL DEFAULT 'main',
	deploy_path TEXT NOT NULL DEFAULT 'config.yaml',
	provider TEXT NOT NULL CHECK (provider = 'github'),
	github_installation_id BIGINT NOT NULL DEFAULT 0,
	github_app_slug TEXT,
	github_repository_id BIGINT,
	webhook_secret TEXT NOT NULL,
	webhook_id TEXT,
	webhook_url TEXT NOT NULL,
	notification_webhook_url TEXT,
			status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'error', 'fallback')),
			auto_reconcile BOOLEAN NOT NULL DEFAULT true,
			deployed_commit TEXT,
			last_webhook_at TIMESTAMP,
			last_poll_at TIMESTAMP,
			retry_count INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		-- Create servers table
		CREATE TABLE IF NOT EXISTS servers (
			id VARCHAR(255) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			repo_url VARCHAR(1024) NOT NULL DEFAULT '',
			branch VARCHAR(255) NOT NULL DEFAULT 'main',
			deploy_path VARCHAR(1024) NOT NULL DEFAULT 'config.yaml',
			target_commit_hash VARCHAR(255),
			auto_reconcile BOOLEAN NOT NULL DEFAULT FALSE,
			environment_id INTEGER,
			deleted_at TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE SET NULL
		);

		-- Create agent_status table
		CREATE TABLE IF NOT EXISTS agent_status (
			id SERIAL PRIMARY KEY,
			server_id VARCHAR(255) NOT NULL,
			commit_hash VARCHAR(255) NOT NULL,
			is_drifted BOOLEAN NOT NULL DEFAULT FALSE,
			status VARCHAR(50) NOT NULL,
			error_message TEXT,
			agent_version TEXT,
			drift_details JSONB,
			agent_timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
		);

		-- Add missing columns if not present
		ALTER TABLE environments ADD COLUMN IF NOT EXISTS notification_webhook_url TEXT;


		-- Create agent_tokens table
		CREATE TABLE IF NOT EXISTS agent_tokens (
			id SERIAL PRIMARY KEY,
			token_hash VARCHAR(128) NOT NULL UNIQUE,
			server_id VARCHAR(255) NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
			description TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP,
			last_used_at TIMESTAMP,
			is_active BOOLEAN NOT NULL DEFAULT TRUE,
			created_by_user_id INTEGER REFERENCES users(id) ON DELETE SET NULL
		);

		-- Create audit_log table
		CREATE TABLE IF NOT EXISTS audit_log (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL,
			action VARCHAR(255) NOT NULL,
			resource_type VARCHAR(255) NOT NULL,
			resource_id VARCHAR(255) NOT NULL,
			details JSONB,
			ip_address VARCHAR(45),
			timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		-- Create pulls table
		CREATE TABLE IF NOT EXISTS pulls (
			id VARCHAR(255) PRIMARY KEY,
			title VARCHAR(255) NOT NULL,
			description TEXT,
			status VARCHAR(50) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		-- Create rollback_events table
		CREATE TABLE IF NOT EXISTS rollback_events (
			id SERIAL PRIMARY KEY,
			environment_id INTEGER NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
			from_commit VARCHAR(255) NOT NULL,
			to_commit VARCHAR(255) NOT NULL,
			initiated_by VARCHAR(255) NOT NULL,
			status VARCHAR(50) NOT NULL DEFAULT 'pending',
			reason TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at TIMESTAMP,
			error_message TEXT,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		-- Create events table
		CREATE TABLE IF NOT EXISTS events (
			id SERIAL PRIMARY KEY,
			environment_id INTEGER REFERENCES environments(id) ON DELETE CASCADE,
			server_id INTEGER,
			event_type VARCHAR(100) NOT NULL,
			message TEXT NOT NULL,
			timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		-- Create indexes
		CREATE INDEX IF NOT EXISTS idx_environments_repo_url ON environments(repo_url);
		CREATE INDEX IF NOT EXISTS idx_environments_provider ON environments(provider);
		CREATE INDEX IF NOT EXISTS idx_environments_status ON environments(status);
		CREATE INDEX IF NOT EXISTS idx_agent_status_server_id ON agent_status(server_id);
		CREATE INDEX IF NOT EXISTS idx_agent_status_timestamp ON agent_status(agent_timestamp DESC);
		CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON audit_log(user_id);
		CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp);
		CREATE INDEX IF NOT EXISTS idx_pulls_status ON pulls(status);
		CREATE INDEX IF NOT EXISTS idx_pulls_created_at ON pulls(created_at);

		-- Create trigger function for updated_at columns
		CREATE OR REPLACE FUNCTION update_updated_at_column()
		RETURNS TRIGGER AS $$
		BEGIN
			NEW.updated_at = CURRENT_TIMESTAMP;
			RETURN NEW;
		END;
		$$ language 'plpgsql';

		-- Create triggers
		DROP TRIGGER IF EXISTS update_environments_updated_at ON environments;
		CREATE TRIGGER update_environments_updated_at 
			BEFORE UPDATE ON environments
			FOR EACH ROW
			EXECUTE FUNCTION update_updated_at_column();

		DROP TRIGGER IF EXISTS update_servers_updated_at ON servers;
		CREATE TRIGGER update_servers_updated_at
			BEFORE UPDATE ON servers
			FOR EACH ROW
			EXECUTE FUNCTION update_updated_at_column();

		-- Create schema_migrations table for migration tracking
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT NOT NULL PRIMARY KEY,
			dirty BOOLEAN NOT NULL DEFAULT FALSE
		);
	`)
	if err != nil {
		tdb.t.Fatalf("Failed to create fallback schema: %v", err)
	}

	_, err = tdb.DB.Exec(
		"INSERT INTO schema_migrations (version, dirty) VALUES ($1, false) ON CONFLICT DO NOTHING",
		FallbackSchemaVersion,
	)
	if err != nil {
		tdb.t.Fatalf("Failed to insert schema version: %v", err)
	}
}

func SetupTestDB(t testing.TB) *TestDB {
	t.Helper()

	config := DefaultContainerConfig()
	tdb, err := StartPostgresContainer(t, config)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}

	tdb.WaitForDBConnection(PollOptions{
		Timeout:  30 * time.Second,
		Interval: 100 * time.Millisecond,
		Message:  "database connection to be available",
	})

	tdb.MustMigrate("")
	return tdb
}

func SetupTestDBWithConfig(t testing.TB, config ContainerConfig) *TestDB {
	t.Helper()

	tdb, err := StartPostgresContainer(t, config)
	if err != nil {
		t.Fatalf("Failed to setup test database: %v", err)
	}

	tdb.WaitForDBConnection(PollOptions{
		Timeout:  30 * time.Second,
		Interval: 100 * time.Millisecond,
		Message:  "database connection to be available",
	})

	tdb.MustMigrate("")
	return tdb
}

func (tdb *TestDB) Repository() *database.Repository {
	return database.NewRepository(tdb.DB, tdb.Dialect)
}

func (tdb *TestDB) EnvironmentRepository() *database.EnvironmentRepository {
	return database.NewEnvironmentRepository(tdb.DB, tdb.Dialect)
}

func (tdb *TestDB) Context() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTestTimeout)
	tdb.t.Cleanup(cancel)
	return ctx
}

func (tdb *TestDB) ContextWithTimeout(timeout time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	tdb.t.Cleanup(cancel)
	return ctx
}

var testDBOnce sync.Once
var sharedTestDB *TestDB

func SharedTestDB(t testing.TB) *TestDB {
	t.Helper()

	testDBOnce.Do(func() {
		sharedTestDB = SetupTestDB(t)
	})

	return sharedTestDB
}

func (tdb *TestDB) ResetTables() error {
	_, err := tdb.DB.Exec(`
		TRUNCATE TABLE 
			audit_log,
			agent_status,
			agent_tokens,
			rollback_events,
			events,
			servers,
			environments,
			users,
			pulls
		RESTART IDENTITY CASCADE
	`)
	return err
}
