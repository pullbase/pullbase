package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/jmoiron/sqlx"
)

// EnvironmentRepository handles database operations for environments
type EnvironmentRepository struct {
	db      *sqlx.DB
	dialect Dialect
}

// NewEnvironmentRepository creates a new environment repository
func NewEnvironmentRepository(db *sqlx.DB, dialect Dialect) *EnvironmentRepository {
	if dialect == "" {
		dialect = DialectSQLite
	}
	return &EnvironmentRepository{db: db, dialect: dialect}
}

// Dialect returns the database dialect
func (r *EnvironmentRepository) Dialect() Dialect {
	return r.dialect
}

// Rebind converts query placeholders to the appropriate format for the dialect
func (r *EnvironmentRepository) Rebind(query string) string {
	return r.dialect.Rebind(query)
}

// SupportsReturning returns whether the dialect supports RETURNING clause
func (r *EnvironmentRepository) SupportsReturning() bool {
	return r.dialect.SupportsReturning()
}

// CreateEnvironment creates a new environment
func (r *EnvironmentRepository) CreateEnvironment(ctx context.Context, env *models.Environment) error {
	if env.Branch == "" {
		env.Branch = "main"
	}
	if env.DeployPath == "" {
		env.DeployPath = "config.yaml"
	}

	query := `
		INSERT INTO environments (name, repo_url, branch, deploy_path, provider, github_installation_id, github_app_slug, github_repository_id, webhook_secret, webhook_url, status, auto_reconcile)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`

	var appSlugValue interface{}
	if env.AppSlug != nil {
		appSlugValue = *env.AppSlug
	}

	var repoIDValue interface{}
	if env.RepositoryID != nil {
		repoIDValue = *env.RepositoryID
	}

	err := r.db.QueryRowxContext(ctx, query,
		env.Name,
		env.RepoURL,
		env.Branch,
		env.DeployPath,
		env.Provider,
		env.InstallationID,
		appSlugValue,
		repoIDValue,
		env.WebhookSecret,
		env.WebhookURL,
		env.Status,
		env.AutoReconcile,
	).Scan(&env.ID)

	if err != nil {
		return fmt.Errorf("failed to create environment: %w", err)
	}

	return nil
}

func (r *EnvironmentRepository) GetEnvironment(ctx context.Context, id int64) (*models.Environment, error) {
	query := `
		SELECT id, name, repo_url, branch, deploy_path, provider, github_installation_id, github_app_slug, github_repository_id, webhook_secret, webhook_id, webhook_url, 
		       status, auto_reconcile, deployed_commit, last_webhook_at, retry_count, notification_webhook_url, created_at, updated_at
		FROM environments WHERE id = $1
	`

	env := &models.Environment{}
	var lastWebhookAt sql.NullTime
	var deployedCommit sql.NullString
	var appSlug sql.NullString
	var repoID sql.NullInt64
	var notificationWebhookURL sql.NullString

	err := r.db.QueryRowxContext(ctx, query, id).Scan(
		&env.ID,
		&env.Name,
		&env.RepoURL,
		&env.Branch,
		&env.DeployPath,
		&env.Provider,
		&env.InstallationID,
		&appSlug,
		&repoID,
		&env.WebhookSecret,
		&env.WebhookID,
		&env.WebhookURL,
		&env.Status,
		&env.AutoReconcile,
		&deployedCommit,
		&lastWebhookAt,
		&env.RetryCount,
		&notificationWebhookURL,
		&env.CreatedAt,
		&env.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	if deployedCommit.Valid {
		env.DeployedCommit = &deployedCommit.String
	}
	if lastWebhookAt.Valid {
		env.LastWebhookAt = &lastWebhookAt.Time
	}
	if appSlug.Valid {
		value := appSlug.String
		env.AppSlug = &value
	}
	if repoID.Valid {
		value := repoID.Int64
		env.RepositoryID = &value
	}
	if notificationWebhookURL.Valid {
		env.NotificationWebhookURL = &notificationWebhookURL.String
	}

	return env, nil
}

func (r *EnvironmentRepository) GetEnvironmentByRepoURLAndBranch(ctx context.Context, repoURL, branch string) (*models.Environment, error) {
	query := `
		SELECT id, name, repo_url, branch, deploy_path, provider, github_installation_id, github_app_slug, github_repository_id, webhook_secret, webhook_id, webhook_url, 
		       status, auto_reconcile, deployed_commit, last_webhook_at, retry_count, notification_webhook_url, created_at, updated_at
		FROM environments WHERE repo_url = $1 AND branch = $2
	`

	env := &models.Environment{}
	var lastWebhookAt sql.NullTime
	var deployedCommit sql.NullString
	var appSlug sql.NullString
	var repoID sql.NullInt64
	var notificationWebhookURL sql.NullString

	err := r.db.QueryRowxContext(ctx, query, repoURL, branch).Scan(
		&env.ID,
		&env.Name,
		&env.RepoURL,
		&env.Branch,
		&env.DeployPath,
		&env.Provider,
		&env.InstallationID,
		&appSlug,
		&repoID,
		&env.WebhookSecret,
		&env.WebhookID,
		&env.WebhookURL,
		&env.Status,
		&env.AutoReconcile,
		&deployedCommit,
		&lastWebhookAt,
		&env.RetryCount,
		&notificationWebhookURL,
		&env.CreatedAt,
		&env.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get environment by repo URL and branch: %w", err)
	}

	if deployedCommit.Valid {
		env.DeployedCommit = &deployedCommit.String
	}
	if lastWebhookAt.Valid {
		env.LastWebhookAt = &lastWebhookAt.Time
	}
	if appSlug.Valid {
		env.AppSlug = &appSlug.String
	}
	if repoID.Valid {
		env.RepositoryID = &repoID.Int64
	}
	if notificationWebhookURL.Valid {
		env.NotificationWebhookURL = &notificationWebhookURL.String
	}

	return env, nil
}

func (r *EnvironmentRepository) ListEnvironments(ctx context.Context) ([]*models.Environment, error) {
	query := `
		SELECT id, name, repo_url, branch, deploy_path, provider, github_installation_id, github_app_slug, github_repository_id, webhook_secret, webhook_id, webhook_url, 
		       status, auto_reconcile, deployed_commit, last_webhook_at, retry_count, notification_webhook_url, created_at, updated_at
		FROM environments ORDER BY created_at DESC
	`

	rows, err := r.db.QueryxContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	defer rows.Close()

	var environments []*models.Environment
	for rows.Next() {
		env := &models.Environment{}
		var lastWebhookAt sql.NullTime
		var deployedCommit sql.NullString
		var appSlug sql.NullString
		var repoID sql.NullInt64
		var notificationWebhookURL sql.NullString

		err := rows.Scan(
			&env.ID,
			&env.Name,
			&env.RepoURL,
			&env.Branch,
			&env.DeployPath,
			&env.Provider,
			&env.InstallationID,
			&appSlug,
			&repoID,
			&env.WebhookSecret,
			&env.WebhookID,
			&env.WebhookURL,
			&env.Status,
			&env.AutoReconcile,
			&deployedCommit,
			&lastWebhookAt,
			&env.RetryCount,
			&notificationWebhookURL,
			&env.CreatedAt,
			&env.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan environment: %w", err)
		}

		if deployedCommit.Valid {
			env.DeployedCommit = &deployedCommit.String
		}
		if lastWebhookAt.Valid {
			env.LastWebhookAt = &lastWebhookAt.Time
		}
		if appSlug.Valid {
			env.AppSlug = &appSlug.String
		}
		if repoID.Valid {
			env.RepositoryID = &repoID.Int64
		}
		if notificationWebhookURL.Valid {
			env.NotificationWebhookURL = &notificationWebhookURL.String
		}

		environments = append(environments, env)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating environments: %w", err)
	}

	return environments, nil
}

// UpdateWebhookID updates the webhook ID for an environment
func (r *EnvironmentRepository) UpdateWebhookID(ctx context.Context, environmentID int64, webhookID string) error {
	query := `UPDATE environments SET webhook_id = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`

	_, err := r.db.ExecContext(ctx, query, webhookID, environmentID)
	if err != nil {
		return fmt.Errorf("failed to update webhook ID: %w", err)
	}

	return nil
}

// UpdateWebhookStatus updates the webhook status and last webhook time
func (r *EnvironmentRepository) UpdateWebhookStatus(ctx context.Context, environmentID int64, status string, lastWebhookAt time.Time) error {
	query := `UPDATE environments SET status = $1, last_webhook_at = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3`

	_, err := r.db.ExecContext(ctx, query, status, lastWebhookAt, environmentID)
	if err != nil {
		return fmt.Errorf("failed to update webhook status: %w", err)
	}

	return nil
}

// UpdateRetryCount updates the retry count for an environment
func (r *EnvironmentRepository) UpdateRetryCount(ctx context.Context, environmentID int64, retryCount int) error {
	query := `UPDATE environments SET retry_count = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`

	_, err := r.db.ExecContext(ctx, query, retryCount, environmentID)
	if err != nil {
		return fmt.Errorf("failed to update retry count: %w", err)
	}

	return nil
}

// DeleteEnvironment deletes an environment
func (r *EnvironmentRepository) DeleteEnvironment(ctx context.Context, id int64) error {
	query := `DELETE FROM environments WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete environment: %w", err)
	}

	return nil
}

func (r *EnvironmentRepository) UpdateEnvironment(ctx context.Context, env *models.Environment) error {
	query := `UPDATE environments SET deployed_commit = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, env.DeployedCommit, env.ID)
	if err != nil {
		return fmt.Errorf("failed to update environment: %w", err)
	}
	return nil
}

func (r *EnvironmentRepository) UpdateEnvironmentDetails(
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
	query := `
		UPDATE environments 
		SET name = $1,
		    repo_url = $2,
		    branch = $3,
		    deploy_path = $4,
		    github_installation_id = $5,
		    github_app_slug = $6,
		    github_repository_id = $7,
		    webhook_secret = $8,
		    auto_reconcile = $9,
		    notification_webhook_url = $10,
		    updated_at = CURRENT_TIMESTAMP 
		WHERE id = $11`

	var appSlugValue interface{}
	if appSlug != nil {
		appSlugValue = *appSlug
	}

	var repoIDValue interface{}
	if repositoryID != nil {
		repoIDValue = *repositoryID
	}

	var notificationURLValue interface{}
	if notificationWebhookURL != nil {
		notificationURLValue = *notificationWebhookURL
	}

	_, err := r.db.ExecContext(ctx, query, name, repoURL, branch, deployPath, installationID, appSlugValue, repoIDValue, webhookSecret, autoReconcile, notificationURLValue, environmentID)
	if err != nil {
		return fmt.Errorf("failed to update environment details: %w", err)
	}
	return nil
}

// ToggleEnvironmentAutoReconcile toggles the auto_reconcile field for an environment
func (r *EnvironmentRepository) ToggleEnvironmentAutoReconcile(ctx context.Context, environmentID int64) (bool, error) {
	query := `
		UPDATE environments 
		SET auto_reconcile = NOT auto_reconcile,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
		RETURNING auto_reconcile`

	var newAutoReconcile bool
	err := r.db.QueryRowxContext(ctx, query, environmentID).Scan(&newAutoReconcile)
	if err != nil {
		return false, fmt.Errorf("failed to toggle auto_reconcile for environment %d: %w", environmentID, err)
	}

	return newAutoReconcile, nil
}
