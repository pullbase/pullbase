// Package testutil provides comprehensive database setup and testing utilities for integration tests.
//
// The testutil package extracts common patterns used across integration tests in the pullbase project,
// providing a consistent and reusable way to set up test databases, create test fixtures, and perform
// common test operations.
//
// # Key Features
//
//   - PostgreSQL testcontainer setup with automatic cleanup
//   - Database migration handling with fallback schema creation
//   - Test data fixtures for users, environments, servers, and other entities
//   - Polling utilities to replace sleep patterns in asynchronous tests
//   - Helper functions for common test database operations
//   - Context management with appropriate timeouts
//   - Support for both isolated and shared test database instances
//
// # Basic Usage
//
// For a simple test that needs its own isolated database:
//
//	func TestMyFeature(t *testing.T) {
//	    testutil.SkipIfShort(t)
//
//	    tdb := testutil.SetupTestDB(t)
//	    repo := tdb.Repository()
//	    ctx := tdb.Context()
//
//	    // Create test data
//	    user := tdb.CreateTestUser("testuser")
//	    env := tdb.CreateTestEnvironment("test-env", "https://github.com/test/repo.git")
//
//	    // Your test logic here...
//	}
//
// For tests that can share a database instance:
//
//	func TestReadOnlyOperation(t *testing.T) {
//	    tdb := testutil.SharedTestDB(t)
//	    // ... test logic
//	}
//
// # Advanced Usage
//
// For tests requiring custom container configuration:
//
//	func TestWithCustomDB(t *testing.T) {
//	    config := testutil.ContainerConfig{
//	        Database: "mytest",
//	        Username: "myuser",
//	        Password: "mypass",
//	        Image:    "postgres:16-alpine",
//	    }
//	    tdb := testutil.SetupTestDBWithConfig(t, config)
//	    // ... test logic
//	}
//
// For tests with pre-seeded data:
//
//	func TestWithSeedData(t *testing.T) {
//	    tdb := testutil.SetupTestDB(t)
//	    fixtures := tdb.SeedDatabase() // Creates users, environments, servers, etc.
//
//	    users := fixtures["users"].(map[string]*testutil.TestUser)
//	    admin := users["admin"]
//	    // ... test logic using seeded data
//	}
//
// # Polling for Asynchronous Operations
//
// Instead of using time.Sleep(), use polling utilities:
//
//	// Wait for agent status to update
//	tdb.WaitForAgentStatus("server-id", "deployed")
//
//	// Wait for environment to reach active status
//	tdb.WaitForEnvironmentStatus(env.ID, "active")
//
//	// Custom polling condition
//	testutil.AssertEventuallyTrue(t, func() (bool, error) {
//	    count, err := repo.GetServerCount(ctx)
//	    return count == 5, err
//	}, testutil.PollOptions{
//	    Timeout: 10 * time.Second,
//	    Message: "server count to reach 5",
//	})
//
// # Database Schema Management
//
// The package automatically handles database migrations using the project's migration files.
// If migrations fail (e.g., during development), it falls back to creating a basic schema
// that supports most test operations.
//
// # Cleanup and Resource Management
//
// All database containers and connections are automatically cleaned up using Go's
// t.Cleanup() mechanism. No manual cleanup is required in most cases.
//
// # Performance Considerations
//
//   - Use SharedTestDB() for read-only tests to avoid container startup overhead
//   - Use isolated databases (SetupTestDB()) when tests modify data
//   - Consider using ResetTables() between tests instead of recreating containers
//   - Container startup time is ~2-5 seconds; plan test timeouts accordingly
//
// # Error Handling
//
// The package follows testing best practices by using t.Fatalf() for setup errors
// and providing helper functions like RequireNoError() and RequireError() for
// test assertions.
//
// # Compatibility
//
// This package is designed to work with:
//   - PostgreSQL 17 (default), but configurable
//   - The existing pullbase database schema and migrations
//   - Both sqlx.DB and sql.DB interfaces
//   - The project's repository patterns and model structures
package testutil
