package gitmonitor

import (
	"context"
	"time"

	"github.com/pullbase/pullbase/server/pkg/models"
)

// Provider represents a Git provider (currently GitHub-only)
type Provider = models.GitProvider

const (
	ProviderGitHub = models.ProviderGitHub
)

// WebhookEvent represents a Git webhook event
type WebhookEvent struct {
	Provider   Provider
	EventType  string
	Repository string
	Branch     string
	CommitHash string
	CommitMsg  string
	Author     string
	Timestamp  time.Time
	RawPayload []byte
}

// CommitInfo represents commit information
type CommitInfo struct {
	Hash      string
	Message   string
	Author    string
	Timestamp time.Time
	Branch    string
}

// GitProvider interface for different Git providers
type GitProvider interface {
	ValidateSignature(payload []byte, signature string, secret string) error

	ParseWebhook(payload []byte) (*WebhookEvent, error)

	RegisterWebhook(ctx context.Context, repoURL, webhookURL, token string) (string, error)

	UnregisterWebhook(ctx context.Context, repoURL, webhookID, token string) error

	GetCommitInfo(ctx context.Context, repoURL, commitHash, token string) (*CommitInfo, error)

	CheckoutCommit(ctx context.Context, repoURL, commitHash, token string) error
	CommitExists(ctx context.Context, repoURL, commitHash, token string) (bool, error)

	GetProvider() Provider
}

// Environment represents a Git environment for monitoring
type Environment = models.Environment

// EnvironmentStatus represents the status of an environment
type EnvironmentStatus = models.EnvironmentStatus

const (
	StatusPending = models.StatusPending
	StatusActive  = models.StatusActive
	StatusError   = models.StatusError
)
