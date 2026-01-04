package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/gitmonitor"
	"github.com/pullbase/pullbase/server/pkg/models"
)

// WebhookHandlers handles webhook-related HTTP requests
type WebhookHandlers struct {
	monitor *gitmonitor.EnvironmentMonitor
	logger  *slog.Logger
	audit   func(r *http.Request, action, resourceType, resourceID string, details interface{})
}

// NewWebhookHandlers creates new webhook handlers
func NewWebhookHandlers(monitor *gitmonitor.EnvironmentMonitor, logger *slog.Logger) *WebhookHandlers {
	return &WebhookHandlers{
		monitor: monitor,
		logger:  logger,
	}
}

// SetAuditRecorder allows wiring an audit recorder after initialization.
func (h *WebhookHandlers) SetAuditRecorder(recorder func(r *http.Request, action, resourceType, resourceID string, details interface{})) {
	h.audit = recorder
}

func (h *WebhookHandlers) recordAudit(r *http.Request, action, resourceType, resourceID string, details interface{}) {
	if h.audit != nil {
		h.audit(r, action, resourceType, resourceID, details)
	}
}

// GetEnvironmentMonitor returns the environment monitor for internal use
func (h *WebhookHandlers) GetEnvironmentMonitor() *gitmonitor.EnvironmentMonitor {
	return h.monitor
}

// HandleWebhook handles incoming webhook requests
func (h *WebhookHandlers) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	parts := []string{}
	for _, part := range pathParts {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) != 2 || parts[0] != "webhooks" {
		http.Error(w, "Invalid webhook path", http.StatusBadRequest)
		return
	}
	provider := gitmonitor.Provider(parts[1])

	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "Unsupported content type", http.StatusUnsupportedMediaType)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if provider != gitmonitor.ProviderGitHub {
		http.Error(w, "Unsupported provider", http.StatusBadRequest)
		return
	}

	var payload struct {
		Repository struct {
			HTMLURL  string `json:"html_url"`
			CloneURL string `json:"clone_url"`
		} `json:"repository"`
	}

	repoURL := ""
	if err := json.Unmarshal(body, &payload); err == nil {
		if payload.Repository.CloneURL != "" {
			repoURL = payload.Repository.CloneURL
		} else {
			repoURL = payload.Repository.HTMLURL
		}
	}

	if repoURL == "" {
		http.Error(w, "Could not determine repository URL", http.StatusBadRequest)
		return
	}

	gitProvider, exists := h.monitor.GetProvider(provider)
	if !exists {
		h.logger.Error("Provider not found for webhook parsing", "provider", provider)
		http.Error(w, "Provider not supported", http.StatusInternalServerError)
		return
	}

	event, err := gitProvider.ParseWebhook(body)
	if err != nil {
		h.logger.Error("Failed to parse webhook payload",
			"provider", provider,
			"repo", repoURL,
			"error", err)
		// Continue with minimal event if parsing fails
		event = &gitmonitor.WebhookEvent{
			Provider:   provider,
			Repository: repoURL,
			RawPayload: body,
		}
	}

	if event.Repository != "" {
		repoURL = event.Repository
	}

	branch := event.Branch
	if branch == "" {
		branch = "main"
	}

	env, err := h.getEnvironmentByRepoURL(repoURL, branch, provider)
	if err != nil || env == nil {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	// Decrypt webhook secret for signature validation
	decryptedSecret, err := h.monitor.DecryptWebhookSecret(env.WebhookSecret)
	if err != nil {
		h.logger.Error("Failed to decrypt webhook secret",
			"provider", provider,
			"repo", repoURL,
			"error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Validate webhook signature
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" || decryptedSecret == "" {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	mac := hmac.New(sha256.New, []byte(decryptedSecret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		h.logger.Warn("Invalid webhook signature", "provider", provider, "repo", repoURL)
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	if event.Branch == "" {
		event.Branch = env.Branch
	}
	if event.Repository == "" {
		event.Repository = repoURL
	}

	// Process the webhook event synchronously to ensure errors are logged
	if err := h.monitor.HandleWebhookEvent(r.Context(), event); err != nil {
		h.logger.Error("Failed to handle webhook event",
			"provider", provider,
			"repo", repoURL,
			"error", err)
		// Still return success to GitHub to avoid retries, but log the error
	} else {
		h.logger.Info("Webhook event handled successfully",
			"provider", provider,
			"repo", repoURL,
			"commit", event.CommitHash,
			"branch", event.Branch)
	}

	w.WriteHeader(http.StatusOK)
}

// getEnvironmentByRepoURL gets environment by repo URL, branch, and provider
func (h *WebhookHandlers) getEnvironmentByRepoURL(repoURL, branch string, provider gitmonitor.Provider) (*models.Environment, error) {
	// Get environment from database directly to get encrypted webhook secret
	env, err := h.monitor.GetEnvironmentByRepoURL(context.Background(), repoURL, branch)
	if err != nil {
		return nil, err
	}

	if env == nil {
		return nil, fmt.Errorf("environment not found")
	}

	// Verify provider matches
	if env.Provider != models.GitProvider(provider) {
		return nil, fmt.Errorf("provider mismatch")
	}

	return env, nil
}

// CreateEnvironmentRequest represents a request to create an environment
type CreateEnvironmentRequest struct {
	Name           string             `json:"name"`
	RepoURL        string             `json:"repo_url"`
	Branch         string             `json:"branch"`
	DeployPath     string             `json:"deploy_path"`
	Provider       models.GitProvider `json:"provider"`
	InstallationID int64              `json:"installation_id"`
	AppSlug        *string            `json:"app_slug,omitempty"`
	RepositoryID   *int64             `json:"repository_id,omitempty"`
	WebhookSecret  string             `json:"webhook_secret,omitempty"`
}

// CreateEnvironment creates a new environment for monitoring
func (h *WebhookHandlers) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CreateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Provider == "" {
		req.Provider = models.ProviderGitHub
	}

	if req.Provider != models.ProviderGitHub {
		http.Error(w, "Only GitHub provider is supported", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.RepoURL == "" || req.InstallationID == 0 {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	branch := req.Branch
	if branch == "" {
		branch = "main"
	}

	deployPath := req.DeployPath
	if deployPath == "" {
		deployPath = "config.yaml"
	}

	// Create environment
	env := &models.Environment{
		ID:             0,
		Name:           req.Name,
		RepoURL:        req.RepoURL,
		Branch:         branch,
		DeployPath:     deployPath,
		Provider:       models.ProviderGitHub,
		InstallationID: req.InstallationID,
		AppSlug:        req.AppSlug,
		RepositoryID:   req.RepositoryID,
		WebhookSecret:  req.WebhookSecret,
		WebhookURL:     fmt.Sprintf("https://%s/webhooks/%s", r.Host, models.ProviderGitHub),
		Status:         string(models.StatusPending),
	}

	// Add environment to monitor
	if err := h.monitor.AddEnvironment(r.Context(), env); err != nil {
		h.logger.Error("Failed to add environment",
			"name", req.Name,
			"error", err)
		http.Error(w, fmt.Sprintf("Failed to add environment: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Environment created successfully",
		"environment": map[string]interface{}{
			"id":              env.ID,
			"name":            env.Name,
			"repo_url":        env.RepoURL,
			"branch":          env.Branch,
			"deploy_path":     env.DeployPath,
			"provider":        env.Provider,
			"installation_id": env.InstallationID,
			"status":          env.Status,
			"webhook_url":     env.WebhookURL,
		},
	})

	h.recordAudit(r, "create", "environment", fmt.Sprintf("%d", env.ID), map[string]interface{}{
		"repo_url":    env.RepoURL,
		"branch":      env.Branch,
		"deploy_path": env.DeployPath,
	})
}

// ListEnvironments returns all monitored environments
func (h *WebhookHandlers) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	environments, err := h.monitor.GetRepository().ListEnvironments(r.Context())
	if err != nil {
		h.logger.Error("Failed to list environments", "error", err)
		http.Error(w, "Failed to list environments", http.StatusInternalServerError)
		return
	}
	statuses := h.monitor.GetAllWebhookStatuses()
	sortKey := strings.ToLower(r.URL.Query().Get("sort"))

	// Create a map of statuses by environment ID
	statusMap := make(map[int64]*gitmonitor.WebhookStatus)
	for _, status := range statuses {
		statusMap[status.EnvironmentID] = status
	}

	getLastWebhook := func(env *models.Environment) time.Time {
		if status, ok := statusMap[env.ID]; ok {
			return status.LastWebhookAt
		}
		if env.LastWebhookAt != nil {
			return *env.LastWebhookAt
		}
		return env.UpdatedAt
	}

	slices.SortFunc(environments, func(a, b *models.Environment) int {
		switch sortKey {
		case "name":
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		case "webhook":
			ta := getLastWebhook(a)
			tb := getLastWebhook(b)
			if ta.After(tb) {
				return -1
			}
			if tb.After(ta) {
				return 1
			}
			return 0
		default:
			if a.UpdatedAt.After(b.UpdatedAt) {
				return -1
			}
			if b.UpdatedAt.After(a.UpdatedAt) {
				return 1
			}
			return 0
		}
	})

	// Combine environments with their statuses
	response := make([]map[string]interface{}, 0, len(environments))
	for _, env := range environments {
		envData := map[string]interface{}{
			"id":                     env.ID,
			"name":                   env.Name,
			"repo_url":               env.RepoURL,
			"branch":                 env.Branch,
			"deploy_path":            env.DeployPath,
			"provider":               env.Provider,
			"installation_id":        env.InstallationID,
			"app_slug":               env.AppSlug,
			"repository_id":          env.RepositoryID,
			"notification_webhook_url": env.NotificationWebhookURL,
			"status":                 env.Status,
			"auto_reconcile":         env.AutoReconcile,
			"deployed_commit":        env.DeployedCommit,
			"last_webhook_at":        env.LastWebhookAt,
			"retry_count":            env.RetryCount,
			"created_at":             env.CreatedAt,
			"updated_at":             env.UpdatedAt,
		}

		// Add webhook status if available
		if status, exists := statusMap[env.ID]; exists {
			envData["webhook_status"] = map[string]interface{}{
				"status":       status.Status,
				"last_webhook": status.LastWebhookAt,
				"retry_count":  status.RetryCount,
				"error":        status.Error,
			}
		}

		response = append(response, envData)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"environments": response,
	})
}

// GetEnvironment returns a specific environment
func (h *WebhookHandlers) GetEnvironment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment ID from URL parameter
	environmentIDStr := chi.URLParam(r, "environmentID")
	if environmentIDStr == "" {
		http.Error(w, "Environment ID is required", http.StatusBadRequest)
		return
	}

	environmentID, err := strconv.ParseInt(environmentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid environment ID", http.StatusBadRequest)
		return
	}

	foundEnv, err := h.monitor.GetRepository().GetEnvironment(r.Context(), environmentID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			http.Error(w, "Environment not found", http.StatusNotFound)
			return
		}
		h.logger.Error("Failed to load environment", "environment_id", environmentID, "error", err)
		http.Error(w, "Failed to load environment", http.StatusInternalServerError)
		return
	}

	webhookStatus, _ := h.monitor.GetWebhookStatus(environmentID)

	response := map[string]interface{}{
		"id":                     foundEnv.ID,
		"name":                   foundEnv.Name,
		"repo_url":               foundEnv.RepoURL,
		"branch":                 foundEnv.Branch,
		"deploy_path":            foundEnv.DeployPath,
		"provider":               foundEnv.Provider,
		"installation_id":        foundEnv.InstallationID,
		"app_slug":               foundEnv.AppSlug,
		"repository_id":          foundEnv.RepositoryID,
		"notification_webhook_url": foundEnv.NotificationWebhookURL,
		"status":                 foundEnv.Status,
		"auto_reconcile":         foundEnv.AutoReconcile,
		"deployed_commit":        foundEnv.DeployedCommit,
		"last_webhook_at":        foundEnv.LastWebhookAt,
		"retry_count":            foundEnv.RetryCount,
		"created_at":             foundEnv.CreatedAt,
		"updated_at":             foundEnv.UpdatedAt,
	}

	if webhookStatus != nil {
		response["webhook_status"] = map[string]interface{}{
			"status":       webhookStatus.Status,
			"last_webhook": webhookStatus.LastWebhookAt,
			"retry_count":  webhookStatus.RetryCount,
			"error":        webhookStatus.Error,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateEnvironmentRequest defines the payload for updating an environment
type UpdateEnvironmentRequest struct {
	Name                   string  `json:"name"`
	RepoURL                string  `json:"repo_url"`
	Branch                 string  `json:"branch"`
	DeployPath             string  `json:"deploy_path"`
	InstallationID         int64   `json:"installation_id"`
	AppSlug                *string `json:"app_slug,omitempty"`
	RepositoryID           *int64  `json:"repository_id,omitempty"`
	WebhookSecret          string  `json:"webhook_secret"`
	AutoReconcile          bool    `json:"auto_reconcile"`
	NotificationWebhookURL *string `json:"notification_webhook_url,omitempty"`
}

// UpdateEnvironment updates an environment's basic information
func (h *WebhookHandlers) UpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment ID from URL parameter
	environmentIDStr := chi.URLParam(r, "environmentID")
	if environmentIDStr == "" {
		http.Error(w, "Environment ID is required", http.StatusBadRequest)
		return
	}

	environmentID, err := strconv.ParseInt(environmentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid environment ID", http.StatusBadRequest)
		return
	}

	var req UpdateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.RepoURL == "" || req.InstallationID == 0 {
		http.Error(w, "Missing required fields: name, repo_url, installation_id", http.StatusBadRequest)
		return
	}

	// Check if environment exists
	allEnvironments := h.monitor.GetAllEnvironments()
	var foundEnv *gitmonitor.Environment
	for _, env := range allEnvironments {
		if env.ID == environmentID {
			foundEnv = env
			break
		}
	}

	if foundEnv == nil {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	repo := h.monitor.GetRepository()
	branch := req.Branch
	if branch == "" {
		branch = foundEnv.Branch
	}
	if branch == "" {
		branch = "main"
	}

	deployPath := req.DeployPath
	if deployPath == "" {
		deployPath = foundEnv.DeployPath
	}
	if deployPath == "" {
		deployPath = "config.yaml"
	}

	secretToPersist := foundEnv.WebhookSecret
	if strings.TrimSpace(req.WebhookSecret) != "" {
		encryptedSecret, err := h.monitor.EncryptWebhookSecret(strings.TrimSpace(req.WebhookSecret))
		if err != nil {
			h.logger.Error("Failed to encrypt webhook secret",
				"environment_id", environmentID,
				"error", err)
			http.Error(w, "Failed to encrypt webhook secret", http.StatusInternalServerError)
			return
		}
		secretToPersist = encryptedSecret
	}

	if err := repo.UpdateEnvironmentDetails(r.Context(), environmentID, req.Name, req.RepoURL, branch, deployPath, req.InstallationID, req.AppSlug, req.RepositoryID, secretToPersist, req.AutoReconcile, req.NotificationWebhookURL); err != nil {
		h.logger.Error("Failed to update environment",
			"environment_id", environmentID,
			"error", err)
		http.Error(w, fmt.Sprintf("Failed to update environment: %v", err), http.StatusInternalServerError)
		return
	}

	if err := h.monitor.LoadEnvironmentsFromDatabase(r.Context()); err != nil {
		h.logger.Warn("Failed to refresh environment cache after update",
			"environment_id", environmentID,
			"error", err)
	}

	h.recordAudit(r, "update", "environment", fmt.Sprintf("%d", environmentID), map[string]interface{}{
		"name":            req.Name,
		"repo_url":        req.RepoURL,
		"branch":          branch,
		"deploy_path":     deployPath,
		"installation_id": req.InstallationID,
	})

	h.logger.Info("Environment updated successfully",
		"environment_id", environmentID,
		"name", req.Name)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Environment updated successfully",
	})
}

// DeleteEnvironment removes an environment from monitoring
func (h *WebhookHandlers) DeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment ID from URL parameter
	environmentIDStr := chi.URLParam(r, "environmentID")
	if environmentIDStr == "" {
		http.Error(w, "Environment ID is required", http.StatusBadRequest)
		return
	}

	environmentID, err := strconv.ParseInt(environmentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid environment ID", http.StatusBadRequest)
		return
	}

	// Check if environment exists before attempting to delete
	allEnvironments := h.monitor.GetAllEnvironments()
	var foundEnv *gitmonitor.Environment
	for _, env := range allEnvironments {
		if env.ID == environmentID {
			foundEnv = env
			break
		}
	}

	if foundEnv == nil {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	if err := h.monitor.RemoveEnvironment(r.Context(), environmentID); err != nil {
		h.logger.Error("Failed to remove environment",
			"environment_id", environmentID,
			"error", err)
		http.Error(w, fmt.Sprintf("Failed to remove environment: %v", err), http.StatusInternalServerError)
		return
	}

	h.recordAudit(r, "delete", "environment", fmt.Sprintf("%d", environmentID), map[string]interface{}{
		"name":     foundEnv.Name,
		"repo_url": foundEnv.RepoURL,
		"branch":   foundEnv.Branch,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Environment removed successfully",
	})
}

// GetWebhookStatuses returns all webhook statuses
func (h *WebhookHandlers) GetWebhookStatuses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	statuses := h.monitor.GetAllWebhookStatuses()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"webhook_statuses": statuses,
	})
}

// ToggleEnvironmentAutoReconcile toggles auto-reconcile for an environment
func (h *WebhookHandlers) ToggleEnvironmentAutoReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment ID from URL parameter
	environmentIDStr := chi.URLParam(r, "environmentID")
	if environmentIDStr == "" {
		http.Error(w, "Environment ID is required", http.StatusBadRequest)
		return
	}

	environmentID, err := strconv.ParseInt(environmentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid environment ID", http.StatusBadRequest)
		return
	}

	// Check if environment exists
	allEnvironments := h.monitor.GetAllEnvironments()
	var foundEnv *gitmonitor.Environment
	for _, env := range allEnvironments {
		if env.ID == environmentID {
			foundEnv = env
			break
		}
	}

	if foundEnv == nil {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	// Toggle auto-reconcile in database
	repo := h.monitor.GetRepository()
	newAutoReconcile, err := repo.ToggleEnvironmentAutoReconcile(r.Context(), environmentID)
	if err != nil {
		h.logger.Error("Failed to toggle auto-reconcile for environment",
			"environment_id", environmentID,
			"error", err)
		http.Error(w, fmt.Sprintf("Failed to toggle auto-reconcile: %v", err), http.StatusInternalServerError)
		return
	}

	h.logger.Info("Environment auto-reconcile toggled",
		"environment_id", environmentID,
		"new_state", newAutoReconcile)

	h.recordAudit(r, "toggle_auto_reconcile", "environment", environmentIDStr, map[string]interface{}{
		"auto_reconcile": newAutoReconcile,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"auto_reconcile": newAutoReconcile,
		"message":        fmt.Sprintf("Auto-reconcile %s for environment", map[bool]string{true: "enabled", false: "disabled"}[newAutoReconcile]),
	})
}
