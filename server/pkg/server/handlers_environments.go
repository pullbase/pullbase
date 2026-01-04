package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/notifications"
)

// EnvironmentHealthResponse provides health information for an environment
type EnvironmentHealthResponse struct {
	EnvironmentID    int64          `json:"environment_id"`
	EnvironmentName  string         `json:"environment_name"`
	Provider         string         `json:"provider"`
	WebhookStatus    any            `json:"webhook_status"`
	DeployedCommit   *string        `json:"deployed_commit"`
	LastWebhookAt    *time.Time     `json:"last_webhook_at"`
	GitTokenCooldown *time.Duration `json:"git_token_cooldown,omitempty"`
	GitTokenNext     *time.Time     `json:"git_token_next_allowed,omitempty"`
	GitTokenHistory  []tokenAttempt `json:"git_token_history"`
}

// GetEnvironmentHealth returns health status for all environments
//
//	@Summary		Get environment health
//	@Description	Returns health status for all environments including webhook status and git token rate limits
//	@Tags			Environments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	map[string]interface{}
//	@Failure		401	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/environments/health [get]
func (api *API) GetEnvironmentHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	envRepo := api.Repo.GetEnvironmentRepository()
	envs, err := envRepo.ListEnvironments(ctx)
	if err != nil {
		api.log().Error("failed to list environments for health check", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to load environment health")
		return
	}

	results := make([]EnvironmentHealthResponse, 0, len(envs))

	for _, env := range envs {
		resp := EnvironmentHealthResponse{
			EnvironmentID:   env.ID,
			EnvironmentName: env.Name,
			Provider:        string(env.Provider),
			WebhookStatus: map[string]any{
				"status":      env.Status,
				"retry_count": env.RetryCount,
			},
			DeployedCommit:  env.DeployedCommit,
			LastWebhookAt:   env.LastWebhookAt,
			GitTokenHistory: []tokenAttempt{},
		}

		servers, err := api.Repo.GetServersByEnvironment(ctx, env.ID)
		if err == nil {
			var next time.Time
			var cooldown time.Duration

			api.gitTokenMu.Lock()
			history := make([]tokenAttempt, 0)
			for _, srv := range servers {
				if until, ok := api.gitTokenCooldownUntil[srv.ID]; ok {
					if until.After(next) {
						next = until
					}
				}
				if backoff, ok := api.gitTokenBackoff[srv.ID]; ok && backoff > cooldown {
					cooldown = backoff
				}
				if hist, ok := api.gitTokenHistory[srv.ID]; ok {
					history = append(history, hist...)
				}
			}
			api.gitTokenMu.Unlock()

			if !next.IsZero() {
				resp.GitTokenNext = &next
			}
			if cooldown > 0 {
				resp.GitTokenCooldown = &cooldown
			}
			resp.GitTokenHistory = history
		}

		results = append(results, resp)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"environments": results,
	})
}

// TestWebhookHandler sends a test notification to the configured webhook URL
//
//	@Summary		Test webhook
//	@Description	Sends a test notification to the environment's configured webhook URL
//	@Tags			Webhooks
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			environmentID	path		int	true	"Environment ID"
//	@Success		200				{object}	map[string]string
//	@Failure		400				{object}	ErrorResponse
//	@Failure		401				{object}	ErrorResponse
//	@Failure		403				{object}	ErrorResponse
//	@Failure		404				{object}	ErrorResponse
//	@Failure		500				{object}	ErrorResponse
//	@Failure		502				{object}	ErrorResponse
//	@Router			/environments/{environmentID}/test-webhook [post]
func (api *API) TestWebhookHandler(w http.ResponseWriter, r *http.Request) {
	_, authorized := requireRole(r, models.RoleAdmin, models.RoleUser)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied")
		return
	}

	environmentIDStr := chi.URLParam(r, "environmentID")
	environmentID, err := strconv.ParseInt(environmentIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid environment ID")
		return
	}

	env, err := api.Repo.GetEnvironment(r.Context(), environmentID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Environment not found")
		} else {
			writeError(w, http.StatusInternalServerError, "Failed to get environment")
		}
		return
	}

	if env.NotificationWebhookURL == nil || *env.NotificationWebhookURL == "" {
		writeError(w, http.StatusBadRequest, "No notification webhook URL configured for this environment")
		return
	}

	if api.Notifications == nil {
		writeError(w, http.StatusInternalServerError, "Notification service not configured")
		return
	}

	payload := notifications.BuildTestPayload(env.ID, env.Name)
	err = api.Notifications.SendNotification(r.Context(), *env.NotificationWebhookURL, payload)
	if err != nil {
		api.log().Error("failed to send test notification", "url", *env.NotificationWebhookURL, "error", err)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Webhook test failed: %v", err))
		return
	}

	api.log().Info("successfully sent test notification", "url", *env.NotificationWebhookURL, "environment_id", env.ID)
	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Test notification sent successfully",
		"url":     *env.NotificationWebhookURL,
	})
}

// sendStatusNotification sends a webhook notification for drift or error events
func (api *API) sendStatusNotification(ctx context.Context, serverID string, payload AgentStatusPayload) {
	serverWithEnv, err := api.Repo.GetServerWithEnvironment(ctx, serverID)
	if err != nil {
		api.log().Warn("failed to get server environment for notifications", "server_id", serverID, "error", err)
		return
	}

	env, err := api.Repo.GetEnvironment(ctx, *serverWithEnv.EnvironmentID)
	if err != nil {
		api.log().Warn("failed to get environment for notifications", "environment_id", *serverWithEnv.EnvironmentID, "error", err)
		return
	}

	if env.NotificationWebhookURL == nil || *env.NotificationWebhookURL == "" {
		return
	}

	var webhookPayload notifications.WebhookPayload

	switch {
	case payload.Status == "error":
		errorMsg := "Apply failed"
		if payload.ErrorMessage != nil {
			errorMsg = *payload.ErrorMessage
		}
		webhookPayload = notifications.BuildApplyErrorPayload(
			env.ID,
			env.Name,
			serverID,
			serverWithEnv.Name,
			payload.CommitHash,
			errorMsg,
		)
	case payload.IsDrifted:
		webhookPayload = notifications.BuildDriftPayload(
			env.ID,
			env.Name,
			serverID,
			serverWithEnv.Name,
			payload.CommitHash,
			"Server configuration has drifted from desired state",
		)
	}

	api.Notifications.SendAsync(*env.NotificationWebhookURL, webhookPayload)
}
