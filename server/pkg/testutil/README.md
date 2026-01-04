# TestUtil Package

The `testutil` package provides comprehensive database setup and testing utilities for integration tests in the pullbase project. It extracts common patterns used across existing integration tests and provides a consistent, reusable API for test database management.

## Features

- **PostgreSQL Testcontainer Setup**: Automated container lifecycle management with proper cleanup
- **Database Migration Handling**: Automatic migration execution with fallback schema creation
- **Test Data Fixtures**: Pre-built fixtures for users, environments, servers, and other entities
- **Polling Utilities**: Replace sleep patterns with robust polling for asynchronous operations
- **Helper Functions**: Common database operations, pointer helpers, and assertion utilities
- **Context Management**: Proper timeout handling for test operations
- **Shared vs Isolated Databases**: Support for both performance and isolation needs

## Quick Start

### Basic Usage

```go
func TestMyFeature(t *testing.T) {
    testutil.SkipIfShort(t)
    
    // Setup isolated test database
    tdb := testutil.SetupTestDB(t)
    repo := tdb.Repository()
    ctx := tdb.Context()
    
    // Create test data
    user := tdb.CreateTestUser("testuser")
    env := tdb.CreateTestEnvironment("test-env", "https://github.com/test/repo.git")
    server := tdb.CreateTestServer("test-server", env.ID)
    
    // Your test logic here...
    // Database cleanup is automatic via t.Cleanup()
}
```

### Shared Database for Performance

```go
func TestReadOnlyOperation(t *testing.T) {
    // Use shared database for read-only tests
    tdb := testutil.SharedTestDB(t)
    // ... test logic
}
```

## Core Functions

### Database Setup

| Function | Description |
|----------|-------------|
| `SetupTestDB(t)` | Creates isolated test database with default config |
| `SetupTestDBWithConfig(t, config)` | Creates test database with custom configuration |
| `SharedTestDB(t)` | Returns shared database instance for performance |
| `StartPostgresContainer(t, config)` | Low-level container creation |

### Migration and Schema

| Function | Description |
|----------|-------------|
| `MustMigrate(migrationPath)` | Runs migrations, falls back to manual schema |
| `ResetTables()` | Truncates all tables for test cleanup |

### Test Fixtures

#### Individual Fixtures
```go
// Create individual test entities
user := tdb.CreateTestUser("username")
env := tdb.CreateTestEnvironment("env-name", "repo-url")
server := tdb.CreateTestServer("server-name", environmentID)
status := tdb.CreateTestAgentStatus(serverID, "commit", "status", false, nil)
```

#### Bulk Fixtures
```go
// Create multiple entities at once
users := tdb.CreateUserFixtures()        // admin, user, viewer, agent
envs := tdb.CreateEnvironmentFixtures()  // development, staging, production
fixtures := tdb.SeedDatabase()           // Complete realistic dataset
```

### Polling and Async Testing

Replace `time.Sleep()` with robust polling:

```go
// Wait for specific conditions
tdb.WaitForAgentStatus("server-id", "deployed")
tdb.WaitForEnvironmentStatus(envID, "active") 
tdb.WaitForRollbackCompletion(rollbackID)

// Generic polling
testutil.AssertEventuallyEqual(t, func() (int, error) {
    return repo.GetServerCount(ctx)
}, 5, testutil.PollOptions{
    Timeout: 10 * time.Second,
    Message: "server count to reach 5",
})

// Custom conditions
testutil.AssertEventuallyTrue(t, func() (bool, error) {
    status, err := repo.GetLatestAgentStatus(ctx, serverID)
    return status != nil && status.Status == "deployed", err
})
```

### Helper Functions

```go
// Pointer helpers
userID := testutil.IntPtr(42)
name := testutil.StringPtr("test")
now := testutil.UTCNowPtr()

// Database helpers
tdb.ExecSQL("INSERT INTO test_table VALUES ($1)", value)
tdb.QueryRowSQL("SELECT name FROM users WHERE id = $1", id).MustScan(&name)

// Error handling helpers
testutil.RequireNoError(t, err, "operation should succeed")
testutil.RequireError(t, err, "operation should fail")

// Test skipping
testutil.SkipIfShort(t) // Skip integration tests in short mode
```

## Configuration

### Container Configuration

```go
config := testutil.ContainerConfig{
    Database:       "custom_testdb",
    Username:       "custom_user",
    Password:       "custom_pass", 
    Image:          "postgres:16-alpine",
    StartupTimeout: 2 * time.Minute,
}
tdb := testutil.SetupTestDBWithConfig(t, config)
```

### Polling Configuration

```go
opts := testutil.PollOptions{
    Timeout:  30 * time.Second,
    Interval: 100 * time.Millisecond,
    Message:  "custom condition description",
}
testutil.Poll(t, condition, opts)
```

## Repository Integration

The package integrates seamlessly with existing repository patterns:

```go
tdb := testutil.SetupTestDB(t)

// Main repository
repo := tdb.Repository()
users, total, err := repo.ListUsers(ctx, 10, 0, "")

// Environment repository  
envRepo := tdb.EnvironmentRepository()
env, err := envRepo.GetEnvironment(ctx, envID)

// Both support the same models and interfaces
```

## Performance Considerations

- **Container Startup**: ~2-5 seconds per container
- **Use SharedTestDB()**: For read-only tests to avoid startup overhead
- **Use ResetTables()**: Instead of recreating containers between tests
- **Parallel Tests**: Each gets its own isolated container when using `SetupTestDB()`

## Migration Compatibility

The package uses the project's existing migration files located at `server/migrations/`. If migrations fail (common during development), it automatically falls back to creating a compatible schema that supports most test operations.

## Error Handling

Following Go testing best practices:
- Setup errors use `t.Fatalf()` for immediate test failure
- Helper functions provide `RequireNoError()` and `RequireError()` for assertions
- Polling functions fail tests on timeout with descriptive messages

## Example: Converting Existing Tests

### Before (Manual Setup)
```go
func TestServerOperations(t *testing.T) {
    // Manual container setup
    container, err := postgres.Run(ctx, "postgres:17-alpine", ...)
    if err != nil {
        t.Fatalf("Failed to start container: %v", err)
    }
    defer container.Terminate(ctx)
    
    // Manual connection setup
    host, _ := container.Host(ctx)
    port, _ := container.MappedPort(ctx, "5432")
    db, err := sqlx.Open("postgres", fmt.Sprintf("host=%s port=%d...", host, port))
    // ... more setup
    
    // Manual schema creation
    _, err = db.Exec("CREATE TABLE ...")
    
    // Manual test data creation
    _, err = db.Exec("INSERT INTO users ...")
    
    // Test with sleep
    time.Sleep(100 * time.Millisecond)
    // ... test logic
}
```

### After (Using TestUtil)
```go
func TestServerOperations(t *testing.T) {
    testutil.SkipIfShort(t)
    
    tdb := testutil.SetupTestDB(t)
    repo := tdb.Repository()
    
    // Test data creation
    user := tdb.CreateTestUser("testuser")
    
    // Test with polling
    tdb.WaitForUserCount(1)
    // ... test logic
}
```

## Best Practices

1. **Use `SkipIfShort(t)`** at the beginning of integration tests
2. **Use `SharedTestDB()`** for read-only tests to improve performance
3. **Use polling instead of sleep** for asynchronous operations
4. **Create isolated databases** for tests that modify data
5. **Use fixtures** instead of manual SQL for test data creation
6. **Prefer repository methods** over direct SQL when possible
7. **Use descriptive poll messages** for easier debugging

## Compatibility

- **PostgreSQL**: 17 (default), configurable to other versions
- **Go Modules**: Uses existing project dependencies
- **Testing**: Compatible with `go test`, `testify`, and other testing frameworks
- **Interfaces**: Supports both `sqlx.DB` and `sql.DB` where needed
- **Models**: Uses existing `models` package types
