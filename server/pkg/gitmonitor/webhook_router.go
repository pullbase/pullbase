package gitmonitor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pullbase/pullbase/server/pkg/database"
)

// WebhookRouter handles incoming webhooks from Git providers
type WebhookRouter struct {
	providers map[Provider]GitProvider
	handlers  map[Provider]WebhookHandler
	logger    *slog.Logger
	monitor   *EnvironmentMonitor
	mu        sync.RWMutex
}

// WebhookHandler processes webhook events
type WebhookHandler interface {
	HandleWebhook(ctx context.Context, event *WebhookEvent) error
}

// NewWebhookRouter creates a new webhook router
func NewWebhookRouter(logger *slog.Logger, monitor *EnvironmentMonitor) *WebhookRouter {
	router := &WebhookRouter{
		providers: make(map[Provider]GitProvider),
		handlers:  make(map[Provider]WebhookHandler),
		logger:    logger,
		monitor:   monitor,
	}

	// Register default provider (GitHub only)
	router.RegisterProvider(ProviderGitHub, NewGitHubProvider())

	return router
}

// RegisterProvider registers a Git provider
func (wr *WebhookRouter) RegisterProvider(provider Provider, gitProvider GitProvider) {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	wr.providers[provider] = gitProvider
}

// RegisterHandler registers a webhook handler for a provider
func (wr *WebhookRouter) RegisterHandler(provider Provider, handler WebhookHandler) {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	wr.handlers[provider] = handler
}

// SetMonitor sets the environment monitor for the webhook router
func (wr *WebhookRouter) SetMonitor(monitor *EnvironmentMonitor) {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	wr.monitor = monitor
}

// HandleWebhook handles incoming webhook requests
func (wr *WebhookRouter) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) != 2 || pathParts[0] != "webhooks" {
		http.Error(w, "Invalid webhook path", http.StatusBadRequest)
		return
	}

	providerStr := pathParts[1]
	provider := Provider(providerStr)

	wr.mu.RLock()
	gitProvider, exists := wr.providers[provider]
	handler, handlerExists := wr.handlers[provider]
	wr.mu.RUnlock()

	if !exists {
		wr.logger.Error("Unknown provider", "provider", provider)
		http.Error(w, "Unknown provider", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		wr.logger.Error("Failed to read request body", "error", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate webhook signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		signature = r.Header.Get("X-Hub-Signature")
	}

	// Parse webhook event first to get repository URL
	event, err := gitProvider.ParseWebhook(body)
	if err != nil {
		wr.logger.Error("Failed to parse webhook",
			"provider", provider,
			"error", err)
		http.Error(w, "Failed to parse webhook", http.StatusBadRequest)
		return
	}

	branch := event.Branch
	if branch == "" {
		branch = "main"
	}

	// Find environment by repository URL and branch to get the webhook secret
	env, err := wr.monitor.repo.GetEnvironmentByRepoURLAndBranch(ctx, event.Repository, branch)
	if err != nil {
		wr.logger.Error("Failed to find environment for repository",
			"repository", event.Repository,
			"branch", branch,
			"error", err)
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	if env == nil {
		wr.logger.Warn("No environment found for repository and branch",
			"repository", event.Repository,
			"branch", branch)
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	if env.Branch != branch {
		wr.logger.Info("Webhook branch does not match environment branch, ignoring",
			"environment_id", env.ID,
			"expected_branch", env.Branch,
			"event_branch", branch)
		http.Error(w, "Branch mismatch", http.StatusAccepted)
		return
	}

	if event.Branch == "" {
		event.Branch = branch
	}

	// Decrypt webhook secret for signature validation
	decryptedSecret, err := wr.monitor.decryptSecret(env.WebhookSecret)
	if err != nil {
		wr.logger.Error("Failed to decrypt webhook secret",
			"environment_id", env.ID,
			"error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := gitProvider.ValidateSignature(body, signature, decryptedSecret); err != nil {
		wr.logger.Error("Signature validation failed",
			"provider", provider,
			"environment_id", env.ID,
			"error", err,
			"signature", signature)
		http.Error(w, "Signature validation failed", http.StatusUnauthorized)
		return
	}

	// Log webhook event
	wr.logger.Info("Webhook received",
		"provider", provider,
		"environment_id", env.ID,
		"event_type", event.EventType,
		"repository", event.Repository,
		"branch", event.Branch,
		"commit", event.CommitHash,
		"author", event.Author)

	// Handle webhook event
	if handlerExists {
		if err := handler.HandleWebhook(ctx, event); err != nil {
			wr.logger.Error("Failed to handle webhook",
				"provider", provider,
				"environment_id", env.ID,
				"error", err)
			http.Error(w, "Failed to handle webhook", http.StatusInternalServerError)
			return
		}
	} else {
		wr.logger.Warn("No handler registered for provider", "provider", provider)
	}

	// Return success
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Webhook processed successfully"))
}

// WebhookStatus represents the status of a webhook
type WebhookStatus struct {
	EnvironmentID int64     `json:"environment_id"`
	Provider      Provider  `json:"provider"`
	Status        string    `json:"status"`
	LastWebhookAt time.Time `json:"last_webhook_at"`
	RetryCount    int       `json:"retry_count"`
	Error         string    `json:"error,omitempty"`
}

// WebhookManager manages webhook registration and status
type WebhookManager struct {
	router   *WebhookRouter
	logger   *slog.Logger
	repo     *database.EnvironmentRepository
	mu       sync.RWMutex
	statuses map[int64]*WebhookStatus
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager(router *WebhookRouter, logger *slog.Logger, repo *database.EnvironmentRepository) *WebhookManager {
	return &WebhookManager{
		router:   router,
		logger:   logger,
		repo:     repo,
		statuses: make(map[int64]*WebhookStatus),
	}
}

// InitializeStatusesFromDatabase initializes webhook statuses from database environment statuses
func (wm *WebhookManager) InitializeWebhookStatusesFromDatabase(ctx context.Context) error {
	environments, err := wm.repo.ListEnvironments(ctx)
	if err != nil {
		return fmt.Errorf("failed to list environments for status initialization: %w", err)
	}

	wm.mu.Lock()
	defer wm.mu.Unlock()

	for _, env := range environments {
		// Initialize webhook status based on environment status and last_webhook_at
		lastWebhookTime := time.Now()
		if env.LastWebhookAt != nil {
			lastWebhookTime = *env.LastWebhookAt
		}

		// Use database environment status as the initial webhook status
		status := env.Status
		if status == "" {
			status = "pending"
		}

		wm.statuses[env.ID] = &WebhookStatus{
			EnvironmentID: env.ID,
			Provider:      env.Provider,
			Status:        status,
			LastWebhookAt: lastWebhookTime,
			RetryCount:    env.RetryCount,
			Error:         "",
		}

		wm.logger.Debug("Initialized webhook status from database",
			"environment_id", env.ID,
			"status", status,
			"last_webhook_at", lastWebhookTime)
	}

	wm.logger.Info("Initialized webhook statuses from database", "count", len(environments))
	return nil
}

// RegisterWebhook registers a webhook for an environment
func (wm *WebhookManager) RegisterWebhook(ctx context.Context, env *Environment, accessToken string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Get provider
	provider, exists := wm.router.providers[env.Provider]
	if !exists {
		return fmt.Errorf("provider %s not supported", env.Provider)
	}

	// Register webhook with provider
	webhookID, err := provider.RegisterWebhook(ctx, env.RepoURL, env.WebhookURL, accessToken)
	if err != nil {
		wm.updateStatus(env.ID, env.Provider, "error", err.Error())
		return fmt.Errorf("failed to register webhook: %w", err)
	}

	// Update environment with webhook ID in database
	if err := wm.repo.UpdateWebhookID(ctx, env.ID, webhookID); err != nil {
		wm.logger.Error("Failed to update webhook ID in database",
			"environment_id", env.ID,
			"error", err)
		// Continue even if database update fails
	}

	// Update environment in memory
	env.WebhookID = &webhookID

	// Update status
	wm.updateStatus(env.ID, env.Provider, "active", "")

	wm.logger.Info("Webhook registered successfully",
		"environment_id", env.ID,
		"provider", env.Provider,
		"webhook_id", webhookID)

	return nil
}

// UnregisterWebhook removes a webhook for an environment
func (wm *WebhookManager) UnregisterWebhook(ctx context.Context, env *Environment, accessToken string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if env.WebhookID == nil || *env.WebhookID == "" {
		return nil
	}

	provider, exists := wm.router.providers[env.Provider]
	if !exists {
		return fmt.Errorf("provider %s not supported", env.Provider)
	}

	if err := provider.UnregisterWebhook(ctx, env.RepoURL, *env.WebhookID, accessToken); err != nil {
		return fmt.Errorf("failed to unregister webhook: %w", err)
	}

	wm.updateStatus(env.ID, env.Provider, "pending", "")

	wm.logger.Info("Webhook unregistered successfully",
		"environment_id", env.ID,
		"provider", env.Provider,
		"webhook_id", *env.WebhookID)

	return nil
}

func (wm *WebhookManager) GetStatus(environmentID int64) (*WebhookStatus, bool) {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	status, exists := wm.statuses[environmentID]
	return status, exists
}

func (wm *WebhookManager) GetAllStatuses() []*WebhookStatus {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	statuses := make([]*WebhookStatus, 0, len(wm.statuses))
	for _, status := range wm.statuses {
		statuses = append(statuses, status)
	}
	return statuses
}

func (wm *WebhookManager) updateStatus(environmentID int64, provider Provider, status string, error string) {
	now := time.Now()

	wm.statuses[environmentID] = &WebhookStatus{
		EnvironmentID: environmentID,
		Provider:      provider,
		Status:        status,
		LastWebhookAt: now,
		Error:         error,
	}
}

// HandleWebhookEvent handles a webhook event and updates status
func (wm *WebhookManager) HandleWebhookEvent(ctx context.Context, event *WebhookEvent) error {
	branch := event.Branch
	if branch == "" {
		branch = "main"
	}

	env, err := wm.repo.GetEnvironmentByRepoURLAndBranch(ctx, event.Repository, branch)
	if err != nil {
		return fmt.Errorf("failed to find environment for repository %s (branch %s): %w", event.Repository, branch, err)
	}

	if env == nil {
		wm.logger.Warn("No environment found for repository",
			"repository", event.Repository)
		return fmt.Errorf("no environment found for repository %s and branch %s", event.Repository, branch)
	}

	wm.logger.Info("Processing webhook event",
		"provider", event.Provider,
		"repository", event.Repository,
		"branch", branch,
		"environment_id", env.ID,
		"commit", event.CommitHash,
		"branch", event.Branch)

	// TODO: Trigger deployment or rollback logic based on event
	// This will integrate with the rollback service

	return nil
}
