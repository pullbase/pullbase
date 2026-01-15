package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/pullbase/pullbase/server/pkg/database"
)

// PollCondition represents a condition function that returns true when the condition is met
type PollCondition func() (bool, error)

// PollOptions holds configuration for polling operations
type PollOptions struct {
	Timeout  time.Duration
	Interval time.Duration
	Message  string
}

// DefaultPollOptions returns sensible defaults for polling
func DefaultPollOptions() PollOptions {
	return PollOptions{
		Timeout:  5 * time.Second,
		Interval: 50 * time.Millisecond,
		Message:  "condition not met",
	}
}

// Poll repeatedly calls the condition function until it returns true or timeout is reached
func Poll(t testing.TB, condition PollCondition, opts ...PollOptions) {
	t.Helper()

	options := DefaultPollOptions()
	if len(opts) > 0 {
		if opts[0].Timeout > 0 {
			options.Timeout = opts[0].Timeout
		}
		if opts[0].Interval > 0 {
			options.Interval = opts[0].Interval
		}
		if opts[0].Message != "" {
			options.Message = opts[0].Message
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), options.Timeout)
	defer cancel()

	ticker := time.NewTicker(options.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for condition: %s (waited %v)", options.Message, options.Timeout)
		case <-ticker.C:
			ok, err := condition()
			if err != nil {
				t.Fatalf("Error checking condition: %v", err)
			}
			if ok {
				return
			}
		}
	}
}

// WaitForAgentStatus waits for an agent status to be created for the given server
func (tdb *TestDB) WaitForAgentStatus(serverID string, expectedStatus string, opts ...PollOptions) {
	tdb.t.Helper()

	options := DefaultPollOptions()
	if len(opts) > 0 {
		options = opts[0]
	}
	if options.Message == "" {
		options.Message = fmt.Sprintf("agent status '%s' for server '%s'", expectedStatus, serverID)
	}

	condition := func() (bool, error) {
		repo := tdb.Repository()
		ctx := tdb.Context()

		status, err := repo.GetLatestAgentStatus(ctx, serverID)
		if err != nil {
			return false, err
		}

		return status != nil && status.Status == expectedStatus, nil
	}

	Poll(tdb.t, condition, options)
}

// WaitForRollbackCompletion waits for a rollback to complete
func (tdb *TestDB) WaitForRollbackCompletion(rollbackID int64, opts ...PollOptions) {
	tdb.t.Helper()

	options := DefaultPollOptions()
	if len(opts) > 0 {
		options = opts[0]
	}
	if options.Message == "" {
		options.Message = fmt.Sprintf("rollback %d to complete", rollbackID)
	}

	condition := func() (bool, error) {
		ctx := tdb.Context()

		var status string
		query := "SELECT status FROM rollback_events WHERE id = $1"
		err := tdb.DB.QueryRowContext(ctx, query, rollbackID).Scan(&status)
		if err != nil {
			return false, err
		}

		return status == "completed" || status == "failed", nil
	}

	Poll(tdb.t, condition, options)
}

// WaitForEnvironmentStatus waits for an environment to reach a specific status
func (tdb *TestDB) WaitForEnvironmentStatus(environmentID int64, expectedStatus string, opts ...PollOptions) {
	tdb.t.Helper()

	options := DefaultPollOptions()
	if len(opts) > 0 {
		options = opts[0]
	}
	if options.Message == "" {
		options.Message = fmt.Sprintf("environment %d to reach status '%s'", environmentID, expectedStatus)
	}

	condition := func() (bool, error) {
		repo := tdb.EnvironmentRepository()
		ctx := tdb.Context()

		env, err := repo.GetEnvironment(ctx, environmentID)
		if err != nil {
			return false, err
		}

		return env != nil && env.Status == expectedStatus, nil
	}

	Poll(tdb.t, condition, options)
}

// AssertEventuallyEqual polls a getter function until it returns the expected value
func AssertEventuallyEqual[T comparable](t testing.TB, getter func() (T, error), expected T, opts ...PollOptions) {
	t.Helper()

	options := DefaultPollOptions()
	if len(opts) > 0 {
		options = opts[0]
	}
	if options.Message == "" {
		options.Message = fmt.Sprintf("value to equal %v", expected)
	}

	condition := func() (bool, error) {
		actual, err := getter()
		if err != nil {
			return false, err
		}
		return actual == expected, nil
	}

	Poll(t, condition, options)
}

// AssertEventuallyTrue polls a condition function until it returns true
func AssertEventuallyTrue(t testing.TB, condition func() (bool, error), opts ...PollOptions) {
	t.Helper()
	Poll(t, condition, opts...)
}

// WaitForTableCount waits for a table to have a specific row count
func (tdb *TestDB) WaitForTableCount(tableName string, expectedCount int, opts ...PollOptions) {
	tdb.t.Helper()

	options := DefaultPollOptions()
	if len(opts) > 0 {
		options = opts[0]
	}
	if options.Message == "" {
		options.Message = fmt.Sprintf("table '%s' to have %d rows", tableName, expectedCount)
	}

	condition := func() (bool, error) {
		ctx := tdb.Context()

		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
		err := tdb.DB.QueryRowContext(ctx, query).Scan(&count)
		if err != nil {
			return false, err
		}

		return count == expectedCount, nil
	}

	Poll(tdb.t, condition, options)
}

// WaitForServerCount waits for a specific number of servers to exist
func (tdb *TestDB) WaitForServerCount(expectedCount int, opts ...PollOptions) {
	tdb.t.Helper()
	tdb.WaitForTableCount("servers", expectedCount, opts...)
}

// WaitForUserCount waits for a specific number of users to exist
func (tdb *TestDB) WaitForUserCount(expectedCount int, opts ...PollOptions) {
	tdb.t.Helper()
	tdb.WaitForTableCount("users", expectedCount, opts...)
}

// WaitForEnvironmentCount waits for a specific number of environments to exist
func (tdb *TestDB) WaitForEnvironmentCount(expectedCount int, opts ...PollOptions) {
	tdb.t.Helper()
	tdb.WaitForTableCount("environments", expectedCount, opts...)
}

// StringPtr returns a pointer to the given string
func StringPtr(s string) *string {
	return &s
}

// IntPtr returns a pointer to the given int
func IntPtr(i int) *int {
	return &i
}

// Int64Ptr returns a pointer to the given int64
func Int64Ptr(i int64) *int64 {
	return &i
}

// BoolPtr returns a pointer to the given bool
func BoolPtr(b bool) *bool {
	return &b
}

// TimePtr returns a pointer to the given time
func TimePtr(t time.Time) *time.Time {
	return &t
}

// NowPtr returns a pointer to the current time
func NowPtr() *time.Time {
	now := time.Now()
	return &now
}

// UTCNowPtr returns a pointer to the current UTC time
func UTCNowPtr() *time.Time {
	now := time.Now().UTC()
	return &now
}

// TruncatedNowPtr returns a pointer to the current time truncated to microseconds (useful for DB comparisons)
func TruncatedNowPtr() *time.Time {
	now := time.Now().UTC().Truncate(time.Microsecond)
	return &now
}

// DatabaseURL returns the database connection URL for the test database
func (tdb *TestDB) DatabaseURL() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		tdb.Config.User,
		tdb.Config.Password,
		tdb.Config.Host,
		tdb.Config.Port,
		tdb.Config.DatabaseName,
		tdb.Config.SSLMode,
	)
}

// ExecSQL executes arbitrary SQL against the test database
func (tdb *TestDB) ExecSQL(query string, args ...interface{}) {
	tdb.t.Helper()

	ctx := tdb.Context()
	_, err := tdb.DB.ExecContext(ctx, query, args...)
	if err != nil {
		tdb.t.Fatalf("Failed to execute SQL query: %v\nQuery: %s", err, query)
	}
}

// QueryRowSQL executes a query that returns a single row
func (tdb *TestDB) QueryRowSQL(query string, args ...interface{}) *TestRow {
	tdb.t.Helper()

	ctx := tdb.Context()
	row := tdb.DB.QueryRowContext(ctx, query, args...)

	return &TestRow{
		row: row,
		t:   tdb.t,
	}
}

// TestRow wraps sql.Row with test-friendly methods
type TestRow struct {
	row interface {
		Scan(dest ...interface{}) error
	}
	t testing.TB
}

// MustScan scans the row and fails the test if there's an error
func (tr *TestRow) MustScan(dest ...interface{}) {
	tr.t.Helper()

	err := tr.row.Scan(dest...)
	if err != nil {
		tr.t.Fatalf("Failed to scan SQL row: %v", err)
	}
}

// WaitForDBConnection waits for the database to be available
func (tdb *TestDB) WaitForDBConnection(opts ...PollOptions) {
	tdb.t.Helper()

	options := DefaultPollOptions()
	if len(opts) > 0 {
		options = opts[0]
	}
	if options.Message == "" {
		options.Message = "database connection to be available"
	}

	condition := func() (bool, error) {
		ctx := tdb.ContextWithTimeout(1 * time.Second)
		err := tdb.DB.PingContext(ctx)
		return err == nil, nil
	}

	Poll(tdb.t, condition, options)
}

// SkipIfShort skips the test if testing.Short() is true
func SkipIfShort(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
}

// RequireNoError fails the test if err is not nil, providing helpful context
func RequireNoError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected no error but got: %v. %s", err, fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...))
		} else {
			t.Fatalf("Expected no error but got: %v", err)
		}
	}
}

// RequireError fails the test if err is nil
func RequireError(t testing.TB, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("Expected an error but got nil. %s", fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...))
		} else {
			t.Fatalf("Expected an error but got nil")
		}
	}
}

// UseFastBcrypt sets a lower bcrypt cost for faster test execution.
func UseFastBcrypt(t testing.TB) {
	t.Helper()
	oldCost := database.BcryptCost
	database.BcryptCost = 4
	t.Cleanup(func() {
		database.BcryptCost = oldCost
	})
}
