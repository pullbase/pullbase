package models

import (
	"encoding/json"
	"time"
)

// User represents a system user with role-based access
type User struct {
	ID           int       `json:"id" db:"id"`
	Username     string    `json:"username" db:"username"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Role         string    `json:"role"`
	IsActive     bool      `json:"is_active" db:"is_active"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// Server represents a registered server configuration
type Server struct {
	ID               string    `json:"id" db:"id"`
	Name             string    `json:"name" db:"name"`
	TargetCommitHash *string   `json:"target_commit_hash,omitempty" db:"target_commit_hash"`
	AutoReconcile    bool      `json:"auto_reconcile" db:"auto_reconcile"`
	EnvironmentID    *int64    `json:"environment_id,omitempty" db:"environment_id"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// ServerWithEnvironment combines Server info with its Environment's Git configuration.
type ServerWithEnvironment struct {
	Server
	// Environment Git configuration
	RepoURL         string `db:"repo_url" json:"repo_url"`
	Branch          string `db:"branch" json:"branch"`
	DeployPath      string `db:"deploy_path" json:"deploy_path,omitempty"`
	EnvironmentName string `db:"environment_name" json:"environment_name"`
}

// ServerWithStatus combines Server info with its latest AgentStatus.
type ServerWithStatus struct {
	Server
	EnvironmentName  *string    `db:"environment_name" json:"environment_name,omitempty"`
	LastCommitHash   *string    `db:"last_commit_hash" json:"last_commit_hash,omitempty"`
	LastStatus       *string    `db:"last_status" json:"last_status,omitempty"`
	LastIsDrifted    *bool      `db:"last_is_drifted" json:"last_is_drifted,omitempty"`
	LastErrorMessage *string    `db:"last_error_message" json:"last_error_message,omitempty"`
	LastAgentVersion *string    `db:"last_agent_version" json:"last_agent_version,omitempty"`
	LastTimestamp    *time.Time `db:"last_timestamp" json:"last_timestamp,omitempty"`
}

// AgentToken represents an authentication token for agents
type AgentToken struct {
	ID              int        `json:"id" db:"id"`
	TokenHash       string     `json:"-" db:"token_hash"`
	ServerID        string     `json:"server_id" db:"server_id"`
	Description     string     `json:"description" db:"description"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	LastUsedAt      *time.Time `json:"last_used_at,omitempty" db:"last_used_at"`
	IsActive        bool       `json:"is_active" db:"is_active"`
	CreatedByUserID *int       `json:"created_by_user_id,omitempty" db:"created_by_user_id"`
}

type DriftItem struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Message  string `json:"message,omitempty"`
}

type DriftDetails struct {
	Packages []DriftItem `json:"packages,omitempty"`
	Services []DriftItem `json:"services,omitempty"`
	Files    []DriftItem `json:"files,omitempty"`
	Summary  string      `json:"summary,omitempty"`
}

type AgentStatus struct {
	ID           int              `json:"id" db:"id"`
	ServerID     string           `json:"server_id" db:"server_id"`
	CommitHash   string           `json:"commit_hash" db:"commit_hash"`
	IsDrifted    bool             `json:"is_drifted" db:"is_drifted"`
	Status       string           `json:"status" db:"status"`
	ErrorMessage *string          `json:"error_message,omitempty" db:"error_message"`
	AgentVersion *string          `json:"agent_version,omitempty" db:"agent_version"`
	DriftDetails *json.RawMessage `json:"drift_details,omitempty" db:"drift_details"`
	Timestamp    time.Time        `json:"timestamp" db:"agent_timestamp"`
	CreatedAt    time.Time        `json:"created_at" db:"created_at"`
}

// AuditLog represents a security audit log entry
type AuditLog struct {
	ID           int             `json:"id" db:"id"`
	UserID       *int            `json:"user_id,omitempty" db:"user_id"`
	Action       string          `json:"action" db:"action"`
	ResourceType string          `json:"resource_type" db:"resource_type"`
	ResourceID   string          `json:"resource_id" db:"resource_id"`
	Details      json.RawMessage `json:"details,omitempty" db:"details"`
	IPAddress    string          `json:"ip_address" db:"ip_address"`
	Timestamp    time.Time       `json:"timestamp" db:"timestamp"`
}

// Pull represents a pull request in the system
type Pull struct {
	ID          string    `json:"id" db:"id"`
	Title       string    `json:"title" db:"title"`
	Description string    `json:"description" db:"description"`
	Status      string    `json:"status" db:"status"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// GitProvider represents different Git providers
type GitProvider string

const (
	ProviderGitHub GitProvider = "github"
)

type Environment struct {
	ID                     int64       `json:"id"`
	Name                   string      `json:"name"`
	RepoURL                string      `json:"repo_url"`
	Branch                 string      `json:"branch" db:"branch"`
	DeployPath             string      `json:"deploy_path" db:"deploy_path"`
	Provider               GitProvider `json:"provider"`
	InstallationID         int64       `json:"installation_id"`
	AppSlug                *string     `json:"app_slug,omitempty"`
	RepositoryID           *int64      `json:"repository_id,omitempty"`
	WebhookSecret          string      `json:"-"`
	WebhookID              *string     `json:"webhook_id"`
	WebhookURL             string      `json:"webhook_url"`
	Status                 string      `json:"status"`
	AutoReconcile          bool        `json:"auto_reconcile"`
	DeployedCommit         *string     `json:"deployed_commit,omitempty"`
	LastWebhookAt          *time.Time  `json:"last_webhook_at"`
	RetryCount             int         `json:"retry_count"`
	NotificationWebhookURL *string     `json:"notification_webhook_url,omitempty" db:"notification_webhook_url"`
	CreatedAt              time.Time   `json:"created_at"`
	UpdatedAt              time.Time   `json:"updated_at"`
}

// EnvironmentStatus represents the status of an environment
type EnvironmentStatus string

const (
	StatusPending EnvironmentStatus = "pending"
	StatusActive  EnvironmentStatus = "active"
	StatusError   EnvironmentStatus = "error"
)

// Role constants for RBAC
const (
	RoleAdmin  = "admin"
	RoleUser   = "user"
	RoleViewer = "viewer"
	RoleAgent  = "agent"
)

// Status constants for agent status
const (
	AgentStatusRunning      = "running"
	AgentStatusError        = "error"
	AgentStatusSyncing      = "syncing"
	AgentStatusDisconnected = "disconnected"
)

// RollbackEvent represents a rollback operation in the database
type RollbackEvent struct {
	ID            int64      `json:"id" db:"id"`
	EnvironmentID int64      `json:"environment_id" db:"environment_id"`
	FromCommit    string     `json:"from_commit" db:"from_commit"`
	ToCommit      string     `json:"to_commit" db:"to_commit"`
	InitiatedBy   string     `json:"initiated_by" db:"initiated_by"`
	Status        string     `json:"status" db:"status"` // pending, in_progress, completed, failed
	Reason        string     `json:"reason" db:"reason"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty" db:"completed_at"`
	ErrorMessage  *string    `json:"error_message,omitempty" db:"error_message"`
}

// Event represents a system event
type Event struct {
	ID            int64     `json:"id" db:"id"`
	EnvironmentID *int64    `json:"environment_id,omitempty" db:"environment_id"`
	ServerID      *int64    `json:"server_id,omitempty" db:"server_id"`
	EventType     string    `json:"event_type" db:"event_type"`
	Message       string    `json:"message" db:"message"`
	Timestamp     time.Time `json:"timestamp" db:"timestamp"`
}

// CommitInfo represents commit information for rollback selection
type CommitInfo struct {
	Hash      string    `json:"hash"`
	AppliedAt time.Time `json:"applied_at"`
	Message   string    `json:"message"`
}
