package gitmonitor

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/logging"
	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/rollback"
)

// EnvironmentMonitor monitors Git environments using webhooks
type EnvironmentMonitor struct {
	webhookManager  *WebhookManager
	providers       map[Provider]GitProvider
	logger          *logging.Logger
	encryptionKey   []byte
	repo            *database.EnvironmentRepository
	mainRepo        ServerRepository
	githubApp       InstallationTokenProvider
	rollbackService rollback.RollbackService
	mu              sync.RWMutex
	environments    map[int64]*Environment
}

// ServerRepository interface for server operations needed by the environment monitor
type ServerRepository interface {
	GetServersByEnvironment(ctx context.Context, environmentID int64) ([]models.Server, error)
	UpdateTargetCommitHash(ctx context.Context, serverName, commitHash string) error
}

// InstallationTokenProvider defines the minimal behavior required to obtain GitHub installation tokens.
type InstallationTokenProvider interface {
	GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error)
}

func (em *EnvironmentMonitor) fetchInstallationToken(ctx context.Context, env *Environment) (string, time.Time, error) {
	if em.githubApp == nil {
		return "", time.Time{}, fmt.Errorf("github app client is not configured")
	}
	return em.githubApp.GetInstallationToken(ctx, env.InstallationID)
}

// NewEnvironmentMonitor creates a new environment monitor
func NewEnvironmentMonitor(
	webhookManager *WebhookManager,
	logger *logging.Logger,
	encryptionKey []byte,
	repo *database.EnvironmentRepository,
	mainRepo ServerRepository,
	githubApp InstallationTokenProvider,
) *EnvironmentMonitor {
	return &EnvironmentMonitor{
		webhookManager: webhookManager,
		providers:      make(map[Provider]GitProvider),
		logger:         logger,
		encryptionKey:  encryptionKey,
		repo:           repo,
		mainRepo:       mainRepo,
		githubApp:      githubApp,
		environments:   make(map[int64]*Environment),
	}
}

// SetRollbackService sets the rollback service for integration
func (em *EnvironmentMonitor) SetRollbackService(rollbackService rollback.RollbackService) {
	em.rollbackService = rollbackService
}

// RegisterProvider registers a Git provider
func (em *EnvironmentMonitor) RegisterProvider(provider Provider, gitProvider GitProvider) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.providers[provider] = gitProvider
}

// AddEnvironment adds an environment for monitoring
func (em *EnvironmentMonitor) AddEnvironment(ctx context.Context, env *Environment) error {
	if env.InstallationID == 0 {
		return fmt.Errorf("installation id is required for github environments")
	}

	if env.Branch == "" {
		env.Branch = "main"
	}

	if env.DeployPath == "" {
		env.DeployPath = "config.yaml"
	}

	em.mu.Lock()
	defer em.mu.Unlock()

	_, exists := em.providers[env.Provider]
	if !exists {
		return fmt.Errorf("provider %s not supported", env.Provider)
	}

	if env.WebhookSecret == "" {
		secret, err := em.generateWebhookSecret()
		if err != nil {
			return fmt.Errorf("failed to generate webhook secret: %w", err)
		}
		env.WebhookSecret = secret
	}

	originalWebhookSecret := env.WebhookSecret

	encryptedSecret, err := em.encryptSecret(env.WebhookSecret)
	if err != nil {
		return fmt.Errorf("failed to encrypt webhook secret: %w", err)
	}
	env.WebhookSecret = encryptedSecret

	if err := em.repo.CreateEnvironment(ctx, env); err != nil {
		return fmt.Errorf("failed to save environment to database: %w", err)
	}

	em.environments[env.ID] = env

	token, _, err := em.fetchInstallationToken(ctx, env)
	if err != nil {
		em.logger.Error("Failed to obtain installation token",
			"environment_id", env.ID,
			"error", err)
		return fmt.Errorf("failed to obtain installation token: %w", err)
	}

	envForWebhook := *env
	envForWebhook.WebhookSecret = originalWebhookSecret

	if err := em.webhookManager.RegisterWebhook(ctx, &envForWebhook, token); err != nil {
		em.logger.Error("Failed to register webhook",
			"environment_id", env.ID,
			"error", err)
		return fmt.Errorf("failed to register webhook: %w", err)
	}

	em.logger.Info("Environment added for monitoring",
		"environment_id", env.ID,
		"name", env.Name,
		"provider", env.Provider,
		"repo_url", env.RepoURL)

	return nil
}

// RemoveEnvironment removes an environment from monitoring
func (em *EnvironmentMonitor) RemoveEnvironment(ctx context.Context, environmentID int64) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	env, exists := em.environments[environmentID]
	if !exists {
		return fmt.Errorf("environment %d not found", environmentID)
	}

	token, _, err := em.fetchInstallationToken(ctx, env)
	if err != nil {
		em.logger.Error("Failed to obtain installation token for webhook removal",
			"environment_id", environmentID,
			"error", err)
	} else {
		envForWebhook := *env
		secret, decryptErr := em.decryptSecret(env.WebhookSecret)
		if decryptErr != nil {
			em.logger.Error("Failed to decrypt webhook secret during removal",
				"environment_id", environmentID,
				"error", decryptErr)
		} else {
			envForWebhook.WebhookSecret = secret
		}

		if err := em.webhookManager.UnregisterWebhook(ctx, &envForWebhook, token); err != nil {
			em.logger.Error("Failed to unregister webhook",
				"environment_id", environmentID,
				"error", err)
		}
	}

	if err := em.repo.DeleteEnvironment(ctx, environmentID); err != nil {
		return fmt.Errorf("failed to delete environment from database: %w", err)
	}

	delete(em.environments, environmentID)

	em.logger.Info("Environment removed from monitoring",
		"environment_id", environmentID,
		"name", env.Name)

	return nil
}

// GetEnvironment returns an environment by ID
func (em *EnvironmentMonitor) GetEnvironment(environmentID int64) (*Environment, bool) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	env, exists := em.environments[environmentID]
	if !exists {
		return nil, false
	}

	// Create a copy with decrypted webhook secret for use
	envCopy := *env
	if secret, err := em.decryptSecret(env.WebhookSecret); err == nil {
		envCopy.WebhookSecret = secret
	} else {
		em.logger.Error("Failed to decrypt webhook secret",
			"environment_id", environmentID,
			"error", err)
		envCopy.WebhookSecret = ""
	}

	return &envCopy, true
}

// GetEnvironmentByRepoURL returns an environment by repository URL and branch with encrypted secrets intact
// This is used for webhook signature validation where we need the encrypted webhook secret
func (em *EnvironmentMonitor) GetEnvironmentByRepoURL(ctx context.Context, repoURL, branch string) (*Environment, error) {
	return em.repo.GetEnvironmentByRepoURLAndBranch(ctx, repoURL, branch)
}

// GetProvider returns a registered git provider by name
func (em *EnvironmentMonitor) GetProvider(provider Provider) (GitProvider, bool) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	gitProvider, exists := em.providers[provider]
	return gitProvider, exists
}

// GetAllEnvironments returns all monitored environments
func (em *EnvironmentMonitor) GetAllEnvironments() []*Environment {
	em.mu.RLock()
	defer em.mu.RUnlock()

	environments := make([]*Environment, 0, len(em.environments))
	for _, env := range em.environments {
		envCopy := *env
		envCopy.WebhookSecret = ""
		environments = append(environments, &envCopy)
	}
	return environments
}

// LoadEnvironmentsFromDatabase loads all environments from the database
func (em *EnvironmentMonitor) LoadEnvironmentsFromDatabase(ctx context.Context) error {
	environments, err := em.repo.ListEnvironments(ctx)
	if err != nil {
		return fmt.Errorf("failed to load environments from database: %w", err)
	}

	em.mu.Lock()
	defer em.mu.Unlock()

	em.environments = make(map[int64]*Environment)
	for _, env := range environments {
		em.environments[env.ID] = env
	}

	em.logger.Info("Loaded environments from database", "count", len(environments))

	if err := em.webhookManager.InitializeWebhookStatusesFromDatabase(ctx); err != nil {
		em.logger.Error("Failed to initialize webhook statuses from database", "error", err)
		return fmt.Errorf("failed to initialize webhook statuses: %w", err)
	}

	return nil
}

// CommitExists checks if a commit exists in the specified environment
func (em *EnvironmentMonitor) CommitExists(ctx context.Context, environmentID int64, commitHash string) (bool, error) {
	env, exists := em.GetEnvironment(environmentID)
	if !exists {
		return false, fmt.Errorf("environment %d not found", environmentID)
	}

	provider, exists := em.providers[env.Provider]
	if !exists {
		return false, fmt.Errorf("provider %s not supported", env.Provider)
	}

	token, _, err := em.fetchInstallationToken(ctx, env)
	if err != nil {
		return false, fmt.Errorf("failed to obtain installation token: %w", err)
	}

	return provider.CommitExists(ctx, env.RepoURL, commitHash, token)
}

// CheckoutCommit checks out a specific commit in the environment
func (em *EnvironmentMonitor) CheckoutCommit(ctx context.Context, environmentID int64, commitHash string) error {
	env, exists := em.GetEnvironment(environmentID)
	if !exists {
		return fmt.Errorf("environment %d not found", environmentID)
	}

	provider, exists := em.providers[env.Provider]
	if !exists {
		return fmt.Errorf("provider %s not supported", env.Provider)
	}

	token, _, err := em.fetchInstallationToken(ctx, env)
	if err != nil {
		return fmt.Errorf("failed to obtain installation token: %w", err)
	}

	return provider.CheckoutCommit(ctx, env.RepoURL, commitHash, token)
}

// GetCommitInfo retrieves commit information
func (em *EnvironmentMonitor) GetCommitInfo(ctx context.Context, environmentID int64, commitHash string) (*CommitInfo, error) {
	env, exists := em.GetEnvironment(environmentID)
	if !exists {
		return nil, fmt.Errorf("environment %d not found", environmentID)
	}

	provider, exists := em.providers[env.Provider]
	if !exists {
		return nil, fmt.Errorf("provider %s not supported", env.Provider)
	}

	token, _, err := em.fetchInstallationToken(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain installation token: %w", err)
	}

	return provider.GetCommitInfo(ctx, env.RepoURL, commitHash, token)
}

// GetWebhookStatus returns the webhook status for an environment
func (em *EnvironmentMonitor) GetWebhookStatus(environmentID int64) (*WebhookStatus, bool) {
	return em.webhookManager.GetStatus(environmentID)
}

// GetAllWebhookStatuses returns all webhook statuses
func (em *EnvironmentMonitor) GetAllWebhookStatuses() []*WebhookStatus {
	return em.webhookManager.GetAllStatuses()
}

// GetRepository returns the environment repository
func (em *EnvironmentMonitor) GetRepository() *database.EnvironmentRepository {
	return em.repo
}

// HandleWebhookEvent processes a webhook event
func (em *EnvironmentMonitor) HandleWebhookEvent(ctx context.Context, event *WebhookEvent) error {
	em.logger.Info("Processing webhook event",
		"provider", event.Provider,
		"repository", event.Repository,
		"commit", event.CommitHash,
		"branch", event.Branch)

	// Find environment by repository URL
	env, err := em.repo.GetEnvironmentByRepoURLAndBranch(ctx, event.Repository, event.Branch)
	if err != nil {
		return fmt.Errorf("failed to find environment for repository %s (branch %s): %w", event.Repository, event.Branch, err)
	}

	if env == nil {
		em.logger.Warn("No environment found for repository and branch",
			"repository", event.Repository,
			"branch", event.Branch)
		return fmt.Errorf("no environment found for repository %s and branch %s", event.Repository, event.Branch)
	}

	if event.Branch == "" {
		event.Branch = env.Branch
	}

	if event.Branch != env.Branch {
		em.logger.Info("Skipping webhook event due to branch mismatch",
			"environment_id", env.ID,
			"expected_branch", env.Branch,
			"event_branch", event.Branch)
		return nil
	}

	// Update webhook status
	now := time.Now()
	if err := em.repo.UpdateWebhookStatus(ctx, env.ID, "active", now); err != nil {
		em.logger.Error("Failed to update webhook status",
			"environment_id", env.ID,
			"error", err)
	}

	// Store the previous deployed commit before updating
	previousCommit := env.DeployedCommit

	env.DeployedCommit = &event.CommitHash
	if err := em.repo.UpdateEnvironment(ctx, env); err != nil {
		em.logger.Error("Failed to update deployed commit",
			"environment_id", env.ID,
			"commit", event.CommitHash,
			"error", err)
	} else {
		em.logger.Info("Updated deployed commit",
			"environment_id", env.ID,
			"commit", event.CommitHash)
	}

	// Update target_commit_hash for all servers associated with this environment
	if err := em.updateServersTargetCommit(ctx, env.ID, event.CommitHash); err != nil {
		em.logger.Error("Failed to update servers target commit",
			"environment_id", env.ID,
			"commit", event.CommitHash,
			"error", err)
	}

	em.webhookManager.updateStatus(env.ID, env.Provider, "active", "")

	if em.rollbackService != nil && event.EventType == "push" {
		if previousCommit != nil && *previousCommit != event.CommitHash {
			em.logger.Info("New commit detected, checking if rollback is needed",
				"environment_id", env.ID,
				"previous_commit", *previousCommit,
				"new_commit", event.CommitHash)

			// Check if commit message contains rollback triggers
			if event.CommitMsg != "" && containsRollbackTrigger(event.CommitMsg) {
				em.logger.Info("Rollback trigger detected in commit message",
					"environment_id", env.ID,
					"commit", event.CommitHash,
					"message", event.CommitMsg)

				// Trigger rollback to the previous commit
				rollbackReq := &rollback.RollbackRequest{
					EnvironmentID: env.ID,
					ToCommit:      *previousCommit,
					Reason:        fmt.Sprintf("Auto-rollback triggered by commit %s: %s", event.CommitHash, event.CommitMsg),
					InitiatedBy:   "webhook-system",
				}

				go func() {
					if _, err := em.rollbackService.InitiateRollback(context.Background(), rollbackReq); err != nil {
						em.logger.Error("Failed to initiate auto-rollback",
							"environment_id", env.ID,
							"error", err)
					} else {
						em.logger.Info("Auto-rollback initiated successfully",
							"environment_id", env.ID,
							"to_commit", *previousCommit)
					}
				}()
			}
		}
	}

	em.logger.Info("Webhook event processed",
		"environment_id", env.ID,
		"environment_name", env.Name,
		"commit", event.CommitHash)

	return nil
}

// updateServersTargetCommit updates the target_commit_hash for all servers associated with an environment
func (em *EnvironmentMonitor) updateServersTargetCommit(ctx context.Context, environmentID int64, commitHash string) error {
	// Get all servers associated with this environment
	servers, err := em.mainRepo.GetServersByEnvironment(ctx, environmentID)
	if err != nil {
		return fmt.Errorf("failed to get servers for environment %d: %w", environmentID, err)
	}

	// Update target_commit_hash for each server
	for _, server := range servers {
		if err := em.mainRepo.UpdateTargetCommitHash(ctx, server.Name, commitHash); err != nil {
			em.logger.Error("Failed to update target commit hash for server",
				"environment_id", environmentID,
				"server_id", server.ID,
				"server_name", server.Name,
				"commit", commitHash,
				"error", err)
			continue
		}

		em.logger.Info("Updated target commit hash for server",
			"environment_id", environmentID,
			"server_id", server.ID,
			"server_name", server.Name,
			"commit", commitHash)
	}

	if len(servers) > 0 {
		em.logger.Info("Updated target commit hash for environment servers",
			"environment_id", environmentID,
			"servers_count", len(servers),
			"commit", commitHash)
	}

	return nil
}

// containsRollbackTrigger checks if a commit message contains rollback triggers
func containsRollbackTrigger(message string) bool {
	triggers := []string{"[ROLLBACK]", "[REVERT]", "rollback", "revert", "emergency"}
	messageLower := strings.ToLower(message)
	for _, trigger := range triggers {
		if strings.Contains(messageLower, strings.ToLower(trigger)) {
			return true
		}
	}
	return false
}

// generateWebhookSecret generates a random webhook secret
func (em *EnvironmentMonitor) generateWebhookSecret() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// encryptSecret encrypts a secret value for secure storage
func (em *EnvironmentMonitor) encryptSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	block, err := aes.NewCipher(em.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to create nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(value), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptSecret decrypts an encrypted secret value
func (em *EnvironmentMonitor) decryptSecret(encryptedValue string) (string, error) {
	if encryptedValue == "" {
		return "", nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encryptedValue)
	if err != nil {
		// Treat value as already in plaintext (legacy data)
		return encryptedValue, nil
	}

	block, err := aes.NewCipher(em.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// Fall back to original cipher text string for legacy plaintext values
		return encryptedValue, nil
	}

	return string(plaintext), nil
}

// DecryptWebhookSecret decrypts a webhook secret for public use
func (em *EnvironmentMonitor) DecryptWebhookSecret(encryptedSecret string) (string, error) {
	return em.decryptSecret(encryptedSecret)
}

// EncryptWebhookSecret encrypts a webhook secret for storage in the database.
func (em *EnvironmentMonitor) EncryptWebhookSecret(secret string) (string, error) {
	return em.encryptSecret(secret)
}

// RetryFailedWebhooks retries webhook registration for failed environments
func (em *EnvironmentMonitor) RetryFailedWebhooks(ctx context.Context) {
	em.mu.RLock()
	environments := make([]*Environment, 0, len(em.environments))
	for _, env := range em.environments {
		environments = append(environments, env)
	}
	em.mu.RUnlock()

	for _, env := range environments {
		status, exists := em.webhookManager.GetStatus(env.ID)
		if !exists || status.Status != "error" {
			continue
		}

		if status.RetryCount >= 3 {
			em.logger.Warn("Max retries reached for environment, switching to fallback",
				"environment_id", env.ID,
				"retry_count", status.RetryCount)

			// TODO: Switch to polling mode
			em.webhookManager.updateStatus(env.ID, env.Provider, "fallback", "Max retries reached")
			continue
		}

		em.logger.Info("Retrying webhook registration",
			"environment_id", env.ID,
			"retry_count", status.RetryCount+1)

		token, _, err := em.fetchInstallationToken(ctx, env)
		if err != nil {
			em.logger.Error("Failed to obtain installation token for retry",
				"environment_id", env.ID,
				"error", err)
			status.RetryCount++
			em.webhookManager.updateStatus(env.ID, env.Provider, "error", err.Error())
			continue
		}

		decryptedSecret, err := em.decryptSecret(env.WebhookSecret)
		if err != nil {
			em.logger.Error("Failed to decrypt webhook secret for retry",
				"environment_id", env.ID,
				"error", err)
			status.RetryCount++
			em.webhookManager.updateStatus(env.ID, env.Provider, "error", err.Error())
			continue
		}

		// Create a copy with decrypted values for webhook registration
		envForWebhook := *env
		envForWebhook.WebhookSecret = decryptedSecret

		if err := em.webhookManager.RegisterWebhook(ctx, &envForWebhook, token); err != nil {
			em.logger.Error("Webhook retry failed",
				"environment_id", env.ID,
				"error", err)
			status.RetryCount++
			em.webhookManager.updateStatus(env.ID, env.Provider, "error", err.Error())
		}
	}
}

// StartRetryWorker starts a background worker to retry failed webhooks
func (em *EnvironmentMonitor) StartRetryWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			em.RetryFailedWebhooks(ctx)
		}
	}
}

// HandleWebhook implements the WebhookHandler interface
func (em *EnvironmentMonitor) HandleWebhook(ctx context.Context, event *WebhookEvent) error {
	return em.HandleWebhookEvent(ctx, event)
}
