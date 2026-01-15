package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/pullbase/pullbase/server/pkg/logging"
	"github.com/pullbase/pullbase/server/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("record already exists")
)

type Repository struct {
	*sqlx.DB
	dialect Dialect
}

type GitMonitorRepository interface {
	GetAllServers(ctx context.Context) ([]models.Server, error)
	GetServerByName(ctx context.Context, name string) (*models.Server, error)
	GetTargetCommitHash(ctx context.Context, serverName string) (string, error)
	UpdateTargetCommitHash(ctx context.Context, serverName, commitHash string) error
	CreateAgentStatus(ctx context.Context, status *models.AgentStatus) error
}

var _ GitMonitorRepository = (*Repository)(nil)

func NewRepository(db *sqlx.DB, dialect Dialect) *Repository {
	if dialect == "" {
		dialect = DialectSQLite
	}
	return &Repository{DB: db, dialect: dialect}
}

func (r *Repository) Dialect() Dialect {
	return r.dialect
}

func (r *Repository) Rebind(query string) string {
	return r.dialect.Rebind(query)
}

func (r *Repository) SupportsReturning() bool {
	return r.dialect.SupportsReturning()
}

// hashPassword securely hashes a password using bcrypt
func hashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CreateUser creates a new user record
func (r *Repository) CreateUser(ctx context.Context, username, password, role string) error {
	hashedPassword, err := hashPassword(password)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO users (username, password_hash, role)
		VALUES ($1, $2, $3)`

	_, err = r.DB.ExecContext(ctx, query, username, hashedPassword, role)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			return fmt.Errorf("username '%s' already exists: %w", username, ErrConflict)
		}
		return fmt.Errorf("failed to create user: %w", err)
	}
	return nil
}

// GetUser retrieves an active user by username
func (r *Repository) GetUser(ctx context.Context, username string) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, username, password_hash, role, is_active, created_at, updated_at
		FROM users
		WHERE username = $1 AND is_active = TRUE`

	err := r.DB.QueryRowxContext(ctx, query, username).StructScan(user)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("active user '%s' not found: %w", username, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get user '%s': %w", username, err)
	}

	return user, nil
}

// HasActiveAdmin reports whether an active admin user already exists.
func (r *Repository) HasActiveAdmin(ctx context.Context) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM users WHERE role = $1 AND is_active = TRUE)`

	var exists bool
	if err := r.DB.QueryRowxContext(ctx, query, models.RoleAdmin).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check for existing admin user: %w", err)
	}
	return exists, nil
}

// CreateServer creates a new server record in the database.
// It returns the newly created server model or an error.
func (r *Repository) CreateServer(ctx context.Context, name string, environmentID int64) (*models.Server, error) {
	return r.CreateServerWithID(ctx, "", name, environmentID)
}

// CreateServerWithID creates a new server record in the database with a specific ID.
// If serverID is empty, a random UUID will be generated.
func (r *Repository) CreateServerWithID(ctx context.Context, serverID, name string, environmentID int64) (*models.Server, error) {
	var server models.Server

	// Generate a UUID for the new server if not provided
	if serverID == "" {
		generatedID, err := uuid.NewRandom()
		if err != nil {
			return nil, fmt.Errorf("failed to generate server ID: %w", err)
		}
		serverID = generatedID.String()
	}

	if name == "" {
		return nil, fmt.Errorf("server name is required")
	}

	var sqlEnvironmentID sql.NullInt64
	if environmentID > 0 {
		sqlEnvironmentID = sql.NullInt64{Int64: environmentID, Valid: true}

		// Verify environment exists
		_, err := r.GetEnvironment(ctx, environmentID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch environment %d: %w", environmentID, err)
		}
	} else {
		return nil, fmt.Errorf("environment ID is required for server creation")
	}

	query := `
        INSERT INTO servers (id, name, environment_id, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $4)
        RETURNING id, name, target_commit_hash, auto_reconcile, environment_id, created_at, updated_at
    `
	err := r.DB.QueryRowxContext(ctx, query,
		serverID, name, sqlEnvironmentID, time.Now(),
	).StructScan(&server)

	if err != nil {
		if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
			if strings.Contains(pgErr.Constraint, "servers_name_key") {
				return nil, fmt.Errorf("server name '%s' already exists: %w", name, ErrConflict)
			}
			// Handle other potential unique constraints if necessary
			return nil, fmt.Errorf("duplicate key error: %w", ErrConflict)
		}
		return nil, fmt.Errorf("failed to create server '%s': %w", name, err)
	}
	logging.Info("server created successfully", "name", server.Name, "id", server.ID, "environment_id", environmentID)
	return &server, nil
}

// GetServerByID retrieves a server by its ID.
func (r *Repository) GetServerByID(ctx context.Context, id string) (*models.Server, error) {
	var server models.Server
	query := `
        SELECT id, name, target_commit_hash, auto_reconcile, environment_id, created_at, updated_at
        FROM servers WHERE id = $1
    `
	err := r.DB.GetContext(ctx, &server, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get server by ID %s: %w", id, err)
	}
	return &server, nil
}

// GetServerByName retrieves a server by its name.
func (r *Repository) GetServerByName(ctx context.Context, name string) (*models.Server, error) {
	var server models.Server
	query := `
        SELECT id, name, target_commit_hash, auto_reconcile, environment_id, created_at, updated_at
        FROM servers WHERE name = $1
    `
	err := r.DB.GetContext(ctx, &server, query, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get server by name '%s': %w", name, err)
	}
	return &server, nil
}

// UpdateTargetCommitHash updates the target commit hash for a server specified by name.
func (r *Repository) UpdateTargetCommitHash(ctx context.Context, serverName, commitHash string) error {
	// Allow empty commitHash
	var sqlCommitHash sql.NullString
	if commitHash != "" {
		sqlCommitHash = sql.NullString{String: commitHash, Valid: true}
	} else {
		sqlCommitHash = sql.NullString{Valid: false} // Set to NULL in DB if empty string provided
	}

	query := `UPDATE servers SET target_commit_hash = $1, updated_at = NOW() WHERE name = $2`
	result, err := r.DB.ExecContext(ctx, query, sqlCommitHash, serverName)
	if err != nil {
		return fmt.Errorf("failed to update target commit hash for server '%s': %w", serverName, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logging.Warn("error checking affected rows after updating commit hash", "server_name", serverName, "error", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("server name '%s' not found for updating commit hash: %w", serverName, ErrNotFound)
	}

	logging.Debug("updated target commit hash", "server_name", serverName, "commit_hash", commitHash)
	return nil
}

// GetTargetCommitHash retrieves the target commit hash for a server by its name.
func (r *Repository) GetTargetCommitHash(ctx context.Context, serverName string) (string, error) {
	var commitHash sql.NullString
	query := `SELECT target_commit_hash FROM servers WHERE name = $1`
	err := r.DB.QueryRowContext(ctx, query, serverName).Scan(&commitHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("server name '%s' not found: %w", serverName, ErrNotFound)
		}
		return "", fmt.Errorf("failed to query target commit hash for server '%s': %w", serverName, err)
	}

	if !commitHash.Valid {
		return "", nil
	}

	return commitHash.String, nil
}

// Agent Status Repository Methods

// CreateAgentStatus inserts a new agent status record.
func (r *Repository) CreateAgentStatus(ctx context.Context, status *models.AgentStatus) error {
	latestStatus, err := r.GetLatestAgentStatus(ctx, status.ServerID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("error checking latest agent status for server %s: %w", status.ServerID, err)
	}

	if latestStatus != nil {
		statusChanged := latestStatus.Status != status.Status
		driftChanged := latestStatus.IsDrifted != status.IsDrifted
		commitChanged := latestStatus.CommitHash != status.CommitHash

		// Determine if error message has changed, treating nil and empty string as equivalent
		errorChanged := false

		// Helper function to get the effective error message (empty string if nil)
		getEffectiveErrorMessage := func(errMsgPtr *string) string {
			if errMsgPtr == nil {
				return ""
			}
			return *errMsgPtr
		}

		latestErrorMsg := getEffectiveErrorMessage(latestStatus.ErrorMessage)
		currentErrorMsg := getEffectiveErrorMessage(status.ErrorMessage)
		errorChanged = latestErrorMsg != currentErrorMsg

		if !statusChanged && !driftChanged && !commitChanged && !errorChanged {
			return nil
		}
	}

	if err := status.PrepareDriftDetailsRaw(); err != nil {
		return fmt.Errorf("error preparing drift details for server %s: %w", status.ServerID, err)
	}

	query := `
        INSERT INTO agent_status
          (server_id, commit_hash, is_drifted, status, error_message, agent_version, drift_details, agent_timestamp)
        VALUES
          (:server_id, :commit_hash, :is_drifted, :status, :error_message, :agent_version, :drift_details, :agent_timestamp)
        RETURNING id
    `

	stmt, err := r.DB.PrepareNamedContext(ctx, query)
	if err != nil {
		return fmt.Errorf("error preparing statement for agent status for server %s: %w", status.ServerID, err)
	}
	defer stmt.Close()

	err = stmt.QueryRowxContext(ctx, status).Scan(&status.ID)
	if err != nil {
		return fmt.Errorf("error inserting agent status for server %s: %w", status.ServerID, err)
	}
	return nil
}

// GetLatestAgentStatus retrieves the most recent status for a given server ID.
func (r *Repository) GetLatestAgentStatus(ctx context.Context, serverID string) (*models.AgentStatus, error) {
	status := &models.AgentStatus{}
	// Order by agent_timestamp first, then ID for consistency
	query := `
        SELECT * FROM agent_status 
        WHERE server_id = $1 
        ORDER BY agent_timestamp DESC, id DESC 
        LIMIT 1
    `
	err := r.DB.GetContext(ctx, status, query, serverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("error getting latest agent status for server %s: %w", serverID, err)
	}
	if err := status.LoadDriftDetails(); err != nil {
		return nil, fmt.Errorf("error loading drift details for server %s: %w", serverID, err)
	}
	return status, nil
}

// GetAgentStatusHistory retrieves paginated status history for a given server ID.
func (r *Repository) GetAgentStatusHistory(ctx context.Context, serverID string, limit, offset int) ([]models.AgentStatus, error) {
	var statuses []models.AgentStatus
	query := `
        SELECT * FROM agent_status
        WHERE server_id = $1
        ORDER BY agent_timestamp DESC, id DESC
        LIMIT $2 OFFSET $3
    `
	err := r.DB.SelectContext(ctx, &statuses, query, serverID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("error getting agent status history for server %s: %w", serverID, err)
	}
	for i := range statuses {
		if err := statuses[i].LoadDriftDetails(); err != nil {
			return nil, fmt.Errorf("error loading drift details for server %s: %w", serverID, err)
		}
	}
	return statuses, nil
}

// CountAgentStatusHistory returns the total number of status history entries for a server.
func (r *Repository) CountAgentStatusHistory(ctx context.Context, serverID string) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM agent_status WHERE server_id = $1`
	err := r.DB.GetContext(ctx, &count, query, serverID)
	if err != nil {
		return 0, fmt.Errorf("failed to count status history for server %s: %w", serverID, err)
	}
	return count, nil
}

// Audit Log Repository Methods

func (r *Repository) CreateAuditLog(ctx context.Context, log *models.AuditLog) error {
	var details interface{}
	if len(log.Details) > 0 {
		details = log.Details
	} else {
		details = nil
	}
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO audit_log (user_id, action, resource_type, resource_id, details, ip_address, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		log.UserID, log.Action, log.ResourceType, log.ResourceID,
		details, log.IPAddress, log.Timestamp,
	)
	return err
}

func (r *Repository) GetAuditLogs(ctx context.Context, limit, offset int) ([]*models.AuditLog, error) {
	rows, err := r.DB.QueryxContext(ctx,
		`SELECT id, user_id, action, resource_type, resource_id, details, ip_address, timestamp
		FROM audit_log
		ORDER BY timestamp DESC
		LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*models.AuditLog
	for rows.Next() {
		var log models.AuditLog
		err := rows.StructScan(&log)
		if err != nil {
			return nil, err
		}
		logs = append(logs, &log)
	}
	return logs, rows.Err()
}

// CreatePull creates a new pull request record
func (r *Repository) CreatePull(ctx context.Context, pull *models.Pull) error {
	query := `
		INSERT INTO pulls (id, title, description, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	err := r.DB.QueryRowxContext(ctx, query,
		pull.ID,
		pull.Title,
		pull.Description,
		pull.Status,
		time.Now(),
		time.Now(),
	).Scan(&pull.ID)

	if err != nil {
		return err
	}

	return nil
}

// GetPull retrieves a pull request by ID
func (r *Repository) GetPull(ctx context.Context, id string) (*models.Pull, error) {
	pull := &models.Pull{}
	query := `
		SELECT id, title, description, status, created_at, updated_at
		FROM pulls
		WHERE id = $1`

	err := r.DB.QueryRowxContext(ctx, query, id).StructScan(pull)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("pull request with id %s not found: %w", id, ErrNotFound)
		}
		return nil, err
	}

	return pull, nil
}

// ListPulls retrieves all pull requests
func (r *Repository) ListPulls(ctx context.Context) ([]*models.Pull, error) {
	query := `
		SELECT id, title, description, status, created_at, updated_at
		FROM pulls
		ORDER BY created_at DESC`

	rows, err := r.DB.QueryxContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pulls []*models.Pull
	for rows.Next() {
		pull := &models.Pull{}
		err := rows.StructScan(pull)
		if err != nil {
			return nil, err
		}
		pulls = append(pulls, pull)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return pulls, nil
}

// UpdatePull updates a pull request
func (r *Repository) UpdatePull(ctx context.Context, pull *models.Pull) error {
	query := `
		UPDATE pulls
		SET title = $1, description = $2, status = $3, updated_at = $4
		WHERE id = $5`

	result, err := r.DB.ExecContext(ctx, query,
		pull.Title,
		pull.Description,
		pull.Status,
		time.Now(),
		pull.ID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// DeletePull deletes a pull request
func (r *Repository) DeletePull(ctx context.Context, id string) error {
	query := `DELETE FROM pulls WHERE id = $1`

	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, username, password_hash, role, is_active, created_at, updated_at
		FROM users
		WHERE id = $1 AND is_active = TRUE`

	err := r.DB.QueryRowxContext(ctx, query, id).StructScan(user)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("active user with id %d not found: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get user by id %d: %w", id, err)
	}

	return user, nil
}

// UpdateUser updates non-sensitive user details.
func (r *Repository) UpdateUser(ctx context.Context, user *models.User) error {
	query := `UPDATE users SET username = $1, role = $2, updated_at = $3 WHERE id = $4 AND is_active = TRUE`
	result, err := r.DB.ExecContext(ctx, query, user.Username, user.Role, time.Now(), user.ID)
	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") {
			return fmt.Errorf("cannot update username for user %d, username '%s' already exists: %w", user.ID, user.Username, ErrConflict)
		}
		return fmt.Errorf("failed to update user %d: %w", user.ID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logging.Warn("could not get rows affected after updating user", "user_id", user.ID, "error", err)
	} else if rowsAffected == 0 {
		var exists bool
		checkQuery := `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`
		_ = r.DB.QueryRowxContext(ctx, checkQuery, user.ID).Scan(&exists)
		if exists {
			return fmt.Errorf("cannot update inactive user %d", user.ID)
		}
		return fmt.Errorf("active user with id %d not found for update", user.ID)
	}
	return nil
}

// UpdateUserPassword specifically updates the password hash.
func (r *Repository) UpdateUserPassword(ctx context.Context, userID int, newPassword string) error {
	newPasswordHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password for user %d: %w", userID, err)
	}
	query := `UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3 AND is_active = TRUE`
	result, err := r.DB.ExecContext(ctx, query, newPasswordHash, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to update password for user %d: %w", userID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logging.Warn("could not get rows affected after updating password", "user_id", userID, "error", err)
	} else if rowsAffected == 0 {
		var exists bool
		checkQuery := `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`
		_ = r.DB.QueryRowxContext(ctx, checkQuery, userID).Scan(&exists)
		if exists {
			return fmt.Errorf("cannot update password for inactive user %d", userID)
		}
		return fmt.Errorf("active user with id %d not found for password update", userID)
	}
	return nil
}

func (r *Repository) ListUsers(ctx context.Context, limit, offset int, roleFilter string) ([]*models.User, int, error) {
	whereClause := "WHERE is_active = TRUE"
	args := []interface{}{}

	if roleFilter != "" {
		whereClause += fmt.Sprintf(" AND role = $%d", len(args)+1)
		args = append(args, roleFilter)
	}

	query := fmt.Sprintf(`
		SELECT id, username, password_hash, role, is_active, created_at, updated_at
		FROM users
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereClause, len(args)+1, len(args)+2)

	argsWithPagination := append(append([]interface{}{}, args...), limit, offset)

	rows, err := r.DB.QueryxContext(ctx, query, argsWithPagination...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list active users: %w", err)
	}
	defer rows.Close()

	users := []*models.User{}
	for rows.Next() {
		user := &models.User{}
		err := rows.StructScan(user)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan active user row: %w", err)
		}
		users = append(users, user)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating active user rows: %w", err)
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM users %s`, whereClause)
	var total int
	if err := r.DB.QueryRowxContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count active users: %w", err)
	}

	return users, total, nil
}

// DeleteUser performs a soft delete by setting is_active to false.
// Note: Since IsActive is not in the model, this method needs rethinking or the model needs updating.
// For now, let's implement a HARD delete as soft delete isn't supported by the current model.
func (r *Repository) DeleteUser(ctx context.Context, id int) error {
	query := `UPDATE users SET is_active = FALSE, updated_at = $1 WHERE id = $2 AND is_active = TRUE`
	result, err := r.DB.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to soft delete user %d: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for user soft delete %d: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("active user with id %d not found for deletion", id)
	}
	return nil
}

// --- Server Methods ---

func (r *Repository) UpdateServer(ctx context.Context, server *models.Server) error {
	query := `UPDATE servers SET name = $1, auto_reconcile = $2, updated_at = $3 WHERE id = $4`
	result, err := r.DB.ExecContext(ctx, query, server.Name, server.AutoReconcile, time.Now(), server.ID)
	if err != nil {
		return fmt.Errorf("failed to update server %s: %w", server.ID, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		logging.Warn("could not get rows affected after updating server", "server_id", server.ID, "error", err)
	} else if rowsAffected == 0 {
		return fmt.Errorf("server with id %s not found for update: %w", server.ID, ErrNotFound)
	}
	return nil
}

// ListServers retrieves a paginated list of servers.
func (r *Repository) ListServers(ctx context.Context, limit, offset int) ([]models.Server, error) {
	var servers []models.Server
	query := `
        SELECT id, name, target_commit_hash, auto_reconcile, environment_id, created_at, updated_at
        FROM servers
        ORDER BY name ASC
        LIMIT $1 OFFSET $2
    `
	err := r.DB.SelectContext(ctx, &servers, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}
	return servers, nil
}

// ListServersWithLatestStatus retrieves servers along with their latest status.
func (r *Repository) ListServersWithLatestStatus(ctx context.Context, limit, offset int, sortOption string) ([]models.ServerWithStatus, error) {
	orderClause := "ORDER BY COALESCE(latest_status.agent_timestamp, s.updated_at) DESC"
	switch strings.ToLower(sortOption) {
	case "name":
		orderClause = "ORDER BY s.name ASC, s.id ASC"
	case "status":
		orderClause = "ORDER BY COALESCE(latest_status.status, '') ASC, s.name ASC"
	}

	query := fmt.Sprintf(`
        SELECT
            s.id,
            s.name,
            s.target_commit_hash,
            s.auto_reconcile,
            s.environment_id,
            e.name as environment_name,
            s.created_at,
            s.updated_at,
            NULLIF(latest_status.commit_hash, '') AS last_commit_hash,
            NULLIF(latest_status.status, '') AS last_status,
            latest_status.is_drifted AS last_is_drifted,
            NULLIF(latest_status.error_message, '') AS last_error_message,
            NULLIF(latest_status.agent_version, '') AS last_agent_version,
            latest_status.agent_timestamp AS last_timestamp
        FROM
            servers s
        LEFT JOIN environments e ON s.environment_id = e.id
        LEFT JOIN LATERAL (
            SELECT
                id, commit_hash, status, is_drifted, error_message, agent_version, agent_timestamp
            FROM
                agent_status
            WHERE
                server_id = s.id
            ORDER BY
                agent_timestamp DESC, id DESC
            LIMIT 1
        ) latest_status ON true
        %s
        LIMIT $1 OFFSET $2
    `, orderClause)

	servers := []models.ServerWithStatus{}
	err := r.DB.SelectContext(ctx, &servers, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list servers with latest status: %w", err)
	}

	return servers, nil
}

// CountActiveAdmins returns the number of active admin users.
func (r *Repository) CountActiveAdmins(ctx context.Context) (int, error) {
	const query = `SELECT COUNT(*) FROM users WHERE role = $1 AND is_active = TRUE`
	var count int
	if err := r.DB.QueryRowxContext(ctx, query, models.RoleAdmin).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count admin users: %w", err)
	}
	return count, nil
}

// GetServerWithLatestStatusByID retrieves a single server joined with its most recent status.
func (r *Repository) GetServerWithLatestStatusByID(ctx context.Context, id string) (*models.ServerWithStatus, error) {
	query := `
        SELECT
            s.id,
            s.name,
            s.target_commit_hash,
            s.auto_reconcile,
            s.environment_id,
            e.name as environment_name,
            s.created_at,
            s.updated_at,
            NULLIF(latest_status.commit_hash, '') AS last_commit_hash,
            NULLIF(latest_status.status, '') AS last_status,
            latest_status.is_drifted AS last_is_drifted,
            NULLIF(latest_status.error_message, '') AS last_error_message,
            NULLIF(latest_status.agent_version, '') AS last_agent_version,
            latest_status.agent_timestamp AS last_timestamp
        FROM
            servers s
        LEFT JOIN environments e ON s.environment_id = e.id
        LEFT JOIN LATERAL (
            SELECT
                id, commit_hash, status, is_drifted, error_message, agent_version, agent_timestamp
            FROM
                agent_status
            WHERE
                server_id = s.id
            ORDER BY
                agent_timestamp DESC, id DESC
            LIMIT 1
        ) latest_status ON true
        WHERE s.id = $1
    `

	var server models.ServerWithStatus
	err := r.DB.GetContext(ctx, &server, query, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get server %s with latest status: %w", id, err)
	}

	return &server, nil
}

// CountServers returns the total number of servers in the database.
func (r *Repository) CountServers(ctx context.Context) (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM servers`
	err := r.DB.GetContext(ctx, &count, query)
	if err != nil {
		return 0, fmt.Errorf("failed to count servers: %w", err)
	}
	return count, nil
}

func (r *Repository) DeleteServer(ctx context.Context, id string) error {
	// First delete related agent_status records to avoid FK constraint issues
	_, err := r.DB.ExecContext(ctx, `DELETE FROM agent_status WHERE server_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete agent status records for server %s: %w", id, err)
	}

	// Now delete the server
	query := `DELETE FROM servers WHERE id = $1`
	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete server %s: %w", id, err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected for server delete %s: %w", id, err)
	}
	if rowsAffected == 0 {
		return fmt.Errorf("server with id %s not found for deletion: %w", id, ErrNotFound)
	}
	return nil
}

// ToggleServerAutoReconcile toggles the auto_reconcile field for a server
func (r *Repository) ToggleServerAutoReconcile(ctx context.Context, serverID string) (bool, error) {
	query := `
		UPDATE servers 
		SET auto_reconcile = NOT auto_reconcile,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING auto_reconcile`

	var newValue bool
	err := r.DB.QueryRowContext(ctx, query, serverID).Scan(&newValue)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("server %s not found: %w", serverID, ErrNotFound)
		}
		return false, fmt.Errorf("failed to toggle auto_reconcile for server %s: %w", serverID, err)
	}

	return newValue, nil
}

// GetAllServers retrieves all servers, primarily for GitMonitor initialization.
func (r *Repository) GetAllServers(ctx context.Context) ([]models.Server, error) {
	var servers []models.Server
	query := `
        SELECT id, name, target_commit_hash, auto_reconcile, environment_id, created_at, updated_at
        FROM servers
        ORDER BY name ASC
    `
	err := r.DB.SelectContext(ctx, &servers, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all servers: %w", err)
	}
	return servers, nil
}

// Rollback Repository Methods

// CreateRollbackEvent creates a new rollback event record
func (r *Repository) CreateRollbackEvent(ctx context.Context, event *models.RollbackEvent) error {
	query := `
		INSERT INTO rollback_events (environment_id, from_commit, to_commit, initiated_by, status, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`

	err := r.DB.QueryRowContext(ctx, query,
		event.EnvironmentID,
		event.FromCommit,
		event.ToCommit,
		event.InitiatedBy,
		event.Status,
		event.Reason,
		event.CreatedAt,
	).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("failed to create rollback event: %w", err)
	}

	return nil
}

// UpdateRollbackEventStatus updates the status of a rollback event
func (r *Repository) UpdateRollbackEventStatus(ctx context.Context, id int64, status string, errorMsg *string) error {
	var query string
	var args []interface{}

	if status == "completed" || status == "failed" {
		now := time.Now()
		query = `UPDATE rollback_events SET status = $1, completed_at = $2, error_message = $3, updated_at = $4 WHERE id = $5`
		args = []interface{}{status, now, errorMsg, now, id}
	} else {
		query = `UPDATE rollback_events SET status = $1, updated_at = $2 WHERE id = $3`
		args = []interface{}{status, time.Now(), id}
	}

	result, err := r.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update rollback event status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("rollback event with id %d not found", id)
	}

	return nil
}

// GetRollbackEvent retrieves a rollback event by ID
func (r *Repository) GetRollbackEvent(ctx context.Context, id int64) (*models.RollbackEvent, error) {
	query := `
		SELECT id, environment_id, from_commit, to_commit, initiated_by, status, reason, 
		       created_at, completed_at, error_message
		FROM rollback_events 
		WHERE id = $1`

	event := &models.RollbackEvent{}
	err := r.DB.QueryRowContext(ctx, query, id).Scan(
		&event.ID,
		&event.EnvironmentID,
		&event.FromCommit,
		&event.ToCommit,
		&event.InitiatedBy,
		&event.Status,
		&event.Reason,
		&event.CreatedAt,
		&event.CompletedAt,
		&event.ErrorMessage,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("rollback event with id %d not found", id)
		}
		return nil, fmt.Errorf("failed to get rollback event: %w", err)
	}

	return event, nil
}

// ListRollbackEvents retrieves rollback events for an environment
func (r *Repository) ListRollbackEvents(ctx context.Context, environmentID int64, limit, offset int) ([]*models.RollbackEvent, error) {
	query := `
		SELECT id, environment_id, from_commit, to_commit, initiated_by, status, reason, 
		       created_at, completed_at, error_message
		FROM rollback_events 
		WHERE environment_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.DB.QueryContext(ctx, query, environmentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list rollback events: %w", err)
	}
	defer rows.Close()

	var events []*models.RollbackEvent
	for rows.Next() {
		event := &models.RollbackEvent{}
		err := rows.Scan(
			&event.ID,
			&event.EnvironmentID,
			&event.FromCommit,
			&event.ToCommit,
			&event.InitiatedBy,
			&event.Status,
			&event.Reason,
			&event.CreatedAt,
			&event.CompletedAt,
			&event.ErrorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rollback event: %w", err)
		}
		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate rollback events: %w", err)
	}

	return events, nil
}

// GetEnvironmentCommitHistory retrieves commit history for an environment
func (r *Repository) GetEnvironmentCommitHistory(ctx context.Context, environmentID int64, limit int) ([]*models.CommitInfo, error) {
	query := `
		SELECT 
			ast.commit_hash as hash, 
			MAX(ast.agent_timestamp) as applied_at,
			'Deployed commit ' || SUBSTRING(ast.commit_hash, 1, 8) || ' (last deployed to ' || 
			(SELECT s2.name FROM servers s2 
			 INNER JOIN agent_status ast2 ON s2.id = ast2.server_id 
			 WHERE ast2.commit_hash = ast.commit_hash AND s2.environment_id = $1 
			 ORDER BY ast2.agent_timestamp DESC LIMIT 1) || ')' as message
		FROM agent_status ast
		INNER JOIN servers s ON ast.server_id = s.id
		WHERE s.environment_id = $1 
			AND ast.commit_hash IS NOT NULL 
			AND ast.commit_hash != ''
		GROUP BY ast.commit_hash
		ORDER BY MAX(ast.agent_timestamp) DESC
		LIMIT $2`

	rows, err := r.DB.QueryContext(ctx, query, environmentID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get environment commit history: %w", err)
	}
	defer rows.Close()

	var commits []*models.CommitInfo
	for rows.Next() {
		commit := &models.CommitInfo{}
		err := rows.Scan(&commit.Hash, &commit.AppliedAt, &commit.Message)
		if err != nil {
			return nil, fmt.Errorf("failed to scan commit info: %w", err)
		}
		commits = append(commits, commit)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate commit history: %w", err)
	}

	return commits, nil
}

// UpdateEnvironmentCommit updates the environment's deployed commit
func (r *Repository) UpdateEnvironmentCommit(ctx context.Context, environmentID int64, commit string) error {
	query := `UPDATE environments SET deployed_commit = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	result, err := r.DB.ExecContext(ctx, query, commit, environmentID)
	if err != nil {
		return fmt.Errorf("failed to update environment commit: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("environment with id %d not found", environmentID)
	}

	return nil
}

// CreateEvent creates a new event record
func (r *Repository) CreateEvent(ctx context.Context, event *models.Event) error {
	query := `
		INSERT INTO events (environment_id, server_id, event_type, message, timestamp)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`

	return r.DB.QueryRowxContext(ctx, query,
		event.EnvironmentID,
		event.ServerID,
		event.EventType,
		event.Message,
		event.Timestamp,
	).Scan(&event.ID)
}

// GetEnvironment delegates to the EnvironmentRepository for rollback compatibility
func (r *Repository) GetEnvironment(ctx context.Context, id int64) (*models.Environment, error) {
	envRepo := NewEnvironmentRepository(r.DB, r.dialect)
	return envRepo.GetEnvironment(ctx, id)
}

// GetEnvironmentRepository returns a reusable environment repository wrapper
func (r *Repository) GetEnvironmentRepository() *EnvironmentRepository {
	return NewEnvironmentRepository(r.DB, r.dialect)
}

// ListEnvironments retrieves environments with pagination
func (r *Repository) ListEnvironments(ctx context.Context, limit, offset int) ([]*models.Environment, int, error) {
	envRepo := NewEnvironmentRepository(r.DB, r.dialect)

	allEnvs, err := envRepo.ListEnvironments(ctx)
	if err != nil {
		return nil, 0, err
	}

	total := len(allEnvs)

	start := offset
	end := offset + limit
	if start >= total {
		return []*models.Environment{}, total, nil
	}
	if end > total {
		end = total
	}

	return allEnvs[start:end], total, nil
}

// CreateEnvironment creates a new environment
func (r *Repository) CreateEnvironment(ctx context.Context, name, repoURL, branch string, autoReconcile bool) (*models.Environment, error) {
	envRepo := NewEnvironmentRepository(r.DB, r.dialect)

	env := &models.Environment{
		Name:           name,
		RepoURL:        repoURL,
		Branch:         branch,
		DeployPath:     "config.yaml",
		Provider:       models.ProviderGitHub,
		InstallationID: time.Now().UnixNano(),
		WebhookSecret:  uuid.NewString(),
		WebhookURL:     fmt.Sprintf("https://example.com/webhooks/%s", name),
		Status:         string(models.StatusPending),
		AutoReconcile:  autoReconcile,
	}

	err := envRepo.CreateEnvironment(ctx, env)
	if err != nil {
		return nil, err
	}

	return env, nil
}

func (r *Repository) UpdateEnvironmentDetails(
	ctx context.Context,
	environmentID int64,
	name string,
	repoURL string,
	branch string,
	deployPath string,
	installationID int64,
	appSlug *string,
	repositoryID *int64,
	webhookSecret string,
	autoReconcile bool,
	notificationWebhookURL *string,
) error {
	envRepo := NewEnvironmentRepository(r.DB, r.dialect)
	return envRepo.UpdateEnvironmentDetails(ctx, environmentID, name, repoURL, branch, deployPath, installationID, appSlug, repositoryID, webhookSecret, autoReconcile, notificationWebhookURL)
}

// GetServersByEnvironment retrieves all servers associated with a specific environment
func (r *Repository) GetServersByEnvironment(ctx context.Context, environmentID int64) ([]models.Server, error) {
	query := `
		SELECT id, name, target_commit_hash, auto_reconcile, environment_id, created_at, updated_at
		FROM servers 
		WHERE environment_id = $1
		ORDER BY created_at ASC`

	rows, err := r.DB.QueryxContext(ctx, query, environmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers by environment: %w", err)
	}
	defer rows.Close()

	var servers []models.Server
	for rows.Next() {
		var server models.Server
		err := rows.StructScan(&server)
		if err != nil {
			return nil, fmt.Errorf("failed to scan server: %w", err)
		}
		servers = append(servers, server)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate servers: %w", err)
	}

	return servers, nil
}

// GetServerWithEnvironment retrieves a server with its environment's Git configuration
func (r *Repository) GetServerWithEnvironment(ctx context.Context, serverID string) (*models.ServerWithEnvironment, error) {
	query := `
			SELECT 
				s.id, s.name, s.target_commit_hash, s.auto_reconcile, s.environment_id, s.created_at, s.updated_at,
				e.repo_url, e.branch, e.deploy_path, e.name as environment_name
		FROM servers s
		INNER JOIN environments e ON s.environment_id = e.id
		WHERE s.id = $1`

	var server models.ServerWithEnvironment
	err := r.DB.GetContext(ctx, &server, query, serverID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get server with environment %s: %w", serverID, err)
	}

	return &server, nil
}

// IsServerInEnvironment checks if a server belongs to a specific environment
func (r *Repository) IsServerInEnvironment(ctx context.Context, serverID string, environmentID int64) (bool, error) {
	query := `SELECT COUNT(*) FROM servers WHERE id = $1 AND environment_id = $2`

	var count int
	err := r.DB.QueryRowContext(ctx, query, serverID, environmentID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check server-environment relationship: %w", err)
	}

	return count > 0, nil
}

// GetServerEnvironmentID returns the environment ID for a given server, if any
func (r *Repository) GetServerEnvironmentID(ctx context.Context, serverID string) (*int64, error) {
	query := `SELECT environment_id FROM servers WHERE id = $1`

	var environmentID sql.NullInt64
	err := r.DB.QueryRowContext(ctx, query, serverID).Scan(&environmentID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get server environment ID: %w", err)
	}

	if environmentID.Valid {
		return &environmentID.Int64, nil
	}

	return nil, nil
}

// --- Agent Token Methods ---

// CreateAgentToken creates a new agent token
func (r *Repository) CreateAgentToken(ctx context.Context, tokenHash, serverID, description string, expiresAt *time.Time, createdByUserID *int) (*models.AgentToken, error) {
	query := `
		INSERT INTO agent_tokens (token_hash, server_id, description, expires_at, created_by_user_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, token_hash, server_id, description, created_at, expires_at, last_used_at, is_active, created_by_user_id
	`

	var token models.AgentToken
	err := r.DB.QueryRowxContext(ctx, query, tokenHash, serverID, description, expiresAt, createdByUserID).StructScan(&token)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent token: %w", err)
	}

	return &token, nil
}

// GetAgentTokenByHash retrieves an agent token by its hash
func (r *Repository) GetAgentTokenByHash(ctx context.Context, tokenHash string) (*models.AgentToken, error) {
	query := `
		SELECT id, token_hash, server_id, description, created_at, expires_at, last_used_at, is_active, created_by_user_id
		FROM agent_tokens
		WHERE token_hash = $1 AND is_active = TRUE
	`

	var token models.AgentToken
	err := r.DB.QueryRowxContext(ctx, query, tokenHash).StructScan(&token)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get agent token: %w", err)
	}

	// Check if token is expired
	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, ErrNotFound // Treat expired tokens as not found
	}

	return &token, nil
}

// UpdateAgentTokenLastUsed updates the last_used_at timestamp for a token
func (r *Repository) UpdateAgentTokenLastUsed(ctx context.Context, tokenID int) error {
	query := `UPDATE agent_tokens SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1`

	result, err := r.DB.ExecContext(ctx, query, tokenID)
	if err != nil {
		return fmt.Errorf("failed to update token last used timestamp: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// ListAgentTokensByServer retrieves all active agent tokens for a server
func (r *Repository) ListAgentTokensByServer(ctx context.Context, serverID string) ([]*models.AgentToken, error) {
	query := `
		SELECT id, token_hash, server_id, description, created_at, expires_at, last_used_at, is_active, created_by_user_id
		FROM agent_tokens
		WHERE server_id = $1 AND is_active = TRUE
		ORDER BY created_at DESC
	`

	rows, err := r.DB.QueryxContext(ctx, query, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*models.AgentToken
	for rows.Next() {
		var token models.AgentToken
		err := rows.StructScan(&token)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent token: %w", err)
		}
		tokens = append(tokens, &token)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate agent tokens: %w", err)
	}

	return tokens, nil
}

// DeactivateAgentToken deactivates an agent token
func (r *Repository) DeactivateAgentToken(ctx context.Context, tokenID int) error {
	query := `UPDATE agent_tokens SET is_active = FALSE WHERE id = $1`

	result, err := r.DB.ExecContext(ctx, query, tokenID)
	if err != nil {
		return fmt.Errorf("failed to deactivate agent token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// DeleteAgentToken permanently deletes an agent token
func (r *Repository) DeleteAgentToken(ctx context.Context, tokenID int) error {
	query := `DELETE FROM agent_tokens WHERE id = $1`

	result, err := r.DB.ExecContext(ctx, query, tokenID)
	if err != nil {
		return fmt.Errorf("failed to delete agent token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// ExpiringToken represents an agent token with additional context for expiration warnings
type ExpiringToken struct {
	models.AgentToken
	ServerName      string `db:"server_name" json:"server_name"`
	EnvironmentID   *int64 `db:"environment_id" json:"environment_id,omitempty"`
	EnvironmentName string `db:"environment_name" json:"environment_name,omitempty"`
	DaysUntilExpiry int    `db:"days_until_expiry" json:"days_until_expiry"`
}

// GetExpiringTokens retrieves all active tokens expiring within the specified number of days
func (r *Repository) GetExpiringTokens(ctx context.Context, days int) ([]*ExpiringToken, error) {
	now := time.Now()
	cutoff := now.Add(time.Duration(days) * 24 * time.Hour)
	query := `
		SELECT 
			t.id, t.token_hash, t.server_id, t.description, t.created_at, 
			t.expires_at, t.last_used_at, t.is_active, t.created_by_user_id,
			s.name as server_name,
			s.environment_id,
			COALESCE(e.name, '') as environment_name
		FROM agent_tokens t
		INNER JOIN servers s ON t.server_id = s.id
		LEFT JOIN environments e ON s.environment_id = e.id
		WHERE t.is_active = TRUE 
			AND t.expires_at IS NOT NULL
			AND t.expires_at > CURRENT_TIMESTAMP
			AND t.expires_at <= ?
		ORDER BY t.expires_at ASC
	`

	rows, err := r.DB.QueryxContext(ctx, r.Rebind(query), cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to get expiring tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*ExpiringToken
	for rows.Next() {
		var token ExpiringToken
		err := rows.StructScan(&token)
		if err != nil {
			return nil, fmt.Errorf("failed to scan expiring token: %w", err)
		}
		if token.ExpiresAt != nil {
			token.DaysUntilExpiry = int(token.ExpiresAt.Sub(now).Hours() / 24)
		}
		tokens = append(tokens, &token)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate expiring tokens: %w", err)
	}

	return tokens, nil
}

// CountExpiringTokens returns the count of tokens expiring within the specified number of days
func (r *Repository) CountExpiringTokens(ctx context.Context, days int) (int, error) {
	cutoff := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	query := `
		SELECT COUNT(*)
		FROM agent_tokens
		WHERE is_active = TRUE 
			AND expires_at IS NOT NULL
			AND expires_at > CURRENT_TIMESTAMP
			AND expires_at <= ?
	`

	var count int
	err := r.DB.GetContext(ctx, &count, r.Rebind(query), cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to count expiring tokens: %w", err)
	}

	return count, nil
}

// --- Health Check Methods ---

type HealthCheck struct {
	Latency time.Duration
	Healthy bool
	Error   error
}

// CheckHealth performs a database connectivity check and measures latency.
func (r *Repository) CheckHealth(ctx context.Context) *HealthCheck {
	start := time.Now()
	err := r.DB.PingContext(ctx)
	return &HealthCheck{
		Latency: time.Since(start),
		Healthy: err == nil,
		Error:   err,
	}
}

type MigrationStatus struct {
	Version int
	Dirty   bool
	Error   error
}

// GetMigrationStatus retrieves the current migration version and dirty state.
func (r *Repository) GetMigrationStatus(ctx context.Context) *MigrationStatus {
	var version int
	var dirty bool

	err := r.DB.QueryRowContext(ctx,
		"SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").
		Scan(&version, &dirty)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &MigrationStatus{
				Version: 0,
				Dirty:   false,
				Error:   nil,
			}
		}
		return &MigrationStatus{
			Version: 0,
			Dirty:   false,
			Error:   fmt.Errorf("failed to get migration status: %w", err),
		}
	}

	return &MigrationStatus{
		Version: version,
		Dirty:   dirty,
		Error:   nil,
	}
}
