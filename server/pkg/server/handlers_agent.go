package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/pullbase/pullbase/server/pkg/apierrors"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/logging"
	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/token"
)

const (
	gitTokenCooldown    = 5 * time.Second
	gitTokenMaxCooldown = 5 * time.Minute
)

// AgentTokenKey is the key used to store agent token info in the request context
const AgentTokenKey ContextKey = "agentToken"

// ServerInfoResponse defines the structure for server info responses to agents
type ServerInfoResponse struct {
	RepoURL          string `json:"repo_url"`
	Branch           string `json:"branch"`
	DeployPath       string `json:"deploy_path"`
	TargetCommitHash string `json:"target_commit_hash,omitempty"`
	AutoReconcile    bool   `json:"auto_reconcile"`
}

// AgentGitTokenResponse contains Git credential metadata returned to agents.
type AgentGitTokenResponse struct {
	Token          string             `json:"token"`
	ExpiresAt      time.Time          `json:"expires_at"`
	RepoURL        string             `json:"repo_url"`
	Provider       models.GitProvider `json:"provider"`
	InstallationID int64              `json:"installation_id"`
	Authentication string             `json:"authentication"`
	RepositoryID   *int64             `json:"repository_id,omitempty"`
	AppSlug        *string            `json:"app_slug,omitempty"`
}

type AgentStatusPayload struct {
	CommitHash   string               `json:"commit_hash"`
	IsDrifted    bool                 `json:"is_drifted"`
	Status       string               `json:"status"`
	ErrorMessage *string              `json:"error_message,omitempty"`
	AgentVersion *string              `json:"agent_version,omitempty"`
	DriftDetails *models.DriftDetails `json:"drift_details,omitempty"`
}

// tokenAttempt records a single git token request attempt
type tokenAttempt struct {
	Timestamp time.Time `json:"timestamp"`
	Allowed   bool      `json:"allowed"`
	Message   string    `json:"message"`
}

// AgentAuthMiddleware creates a middleware handler that validates agent tokens
func AgentAuthMiddleware(repo *database.Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAPIError(w, apierrors.Unauthorized("Authorization header with Bearer token required"))
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
				writeAPIError(w, apierrors.Unauthorized("Invalid Authorization header format (must be Bearer token)"))
				return
			}

			tokenString := parts[1]

			// Validate token format first
			if !token.ValidateTokenFormat(tokenString) {
				writeAPIError(w, apierrors.Unauthorized("Invalid token format"))
				return
			}

			// Hash the token to look it up in the database
			tokenHash := token.HashToken(tokenString)

			// Get token from database
			agentToken, err := repo.GetAgentTokenByHash(r.Context(), tokenHash)
			if err != nil {
				if errors.Is(err, database.ErrNotFound) {
					writeAPIError(w, apierrors.Unauthorized("Invalid or expired token"))
					return
				}
				logging.Error("failed to validate agent token", "error", err)
				writeAPIError(w, apierrors.Internal("Failed to validate token"))
				return
			}

			// Update last used timestamp (async, don't block on this)
			go func() {
				ctx := context.Background()
				if err := repo.UpdateAgentTokenLastUsed(ctx, agentToken.ID); err != nil {
					logging.Warn("failed to update token last used timestamp", "token_id", agentToken.ID, "error", err)
				}
			}()

			ctx := context.WithValue(r.Context(), AgentTokenKey, agentToken)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAgentToken retrieves agent token from the request context
func GetAgentToken(ctx context.Context) (*models.AgentToken, bool) {
	token, ok := ctx.Value(AgentTokenKey).(*models.AgentToken)
	return token, ok
}

// GetServerInfoHandler provides configuration details for a specific agent/server.
//
//	@Summary		Get server info (JWT)
//	@Description	Returns repository configuration for a server. Used by agents for initial setup.
//	@Tags			Agent
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string	true	"Server ID"
//	@Success		200			{object}	ServerInfoResponse
//	@Failure		400			{object}	apierrors.Error
//	@Failure		401			{object}	apierrors.Error
//	@Failure		403			{object}	apierrors.Error
//	@Failure		404			{object}	apierrors.Error
//	@Failure		500			{object}	apierrors.Error
//	@Router			/servers/{serverID}/info [get]
func (api *API) GetServerInfoHandler(w http.ResponseWriter, r *http.Request) {
	// Authentication is optional for this endpoint (to support agent initialization)
	// but if present, we enforce environment-level authorization
	claims, _ := GetUserClaims(r.Context())

	// If we have an Authorization header but invalid/expired token, check manually
	authHeader := r.Header.Get("Authorization")
	var authError error
	if authHeader != "" && claims == nil {
		// Token was provided but invalid - validate manually to give proper error
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			_, authError = api.Auth.ValidateToken(parts[1])
			if authError != nil {
				writeAPIError(w, apierrors.Unauthorized("Invalid or expired token"))
				return
			}
		}
	}

	chiCtx := chi.RouteContext(r.Context())
	serverID := chiCtx.URLParam("serverID")
	api.log().Debug("server info request received", "server_id", serverID, "authenticated", claims != nil)

	if serverID == "" {
		api.log().Error("failed to extract serverID from URL", "path", r.URL.Path)
		writeAPIError(w, apierrors.BadRequest("Server ID could not be extracted from URL path"))
		return
	}

	// If authenticated and has environment scope, enforce environment-level authorization
	if claims != nil && claims.EnvironmentID != nil {
		isInEnvironment, err := api.Repo.IsServerInEnvironment(r.Context(), serverID, *claims.EnvironmentID)
		if err != nil {
			api.log().Error("failed to check server-environment relationship",
				"username", claims.Username, "server_id", serverID, "environment_id", *claims.EnvironmentID, "error", err)
			writeAPIError(w, apierrors.Internal("Failed to validate server access"))
			return
		}

		if !isInEnvironment {
			api.log().Warn("user attempted to access server info not in their environment",
				"username", claims.Username, "environment_id", *claims.EnvironmentID, "server_id", serverID)
			writeAPIError(w, apierrors.Forbidden("Permission denied to access this server's information"))
			return
		}
	}

	serverWithEnv, err := api.Repo.GetServerWithEnvironment(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api.log().Info("server not found", "server_id", serverID)
			writeAPIError(w, apierrors.NotFound("Server", serverID))
		} else {
			api.log().Error("failed to get server info", "server_id", serverID, "error", err)
			writeAPIError(w, apierrors.Internal("Failed to retrieve server information"))
		}
		return
	}

	resp := ServerInfoResponse{
		RepoURL:          serverWithEnv.RepoURL,
		Branch:           serverWithEnv.Branch,
		DeployPath:       serverWithEnv.DeployPath,
		TargetCommitHash: "",
		AutoReconcile:    serverWithEnv.AutoReconcile,
	}

	// Include target commit hash if available
	if serverWithEnv.TargetCommitHash != nil {
		resp.TargetCommitHash = *serverWithEnv.TargetCommitHash
	}

	api.log().Debug("successfully retrieved server info", "server_id", serverID)
	writeJSON(w, http.StatusOK, resp)
}

// UpdateAgentStatusHandler handles incoming status updates from agents (JWT auth).
//
//	@Summary		Update agent status (JWT)
//	@Description	Receives status updates from agents including drift detection and errors
//	@Tags			Agent
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			serverID	path		string				true	"Server ID"
//	@Param			request		body		AgentStatusPayload	true	"Status update payload"
//	@Success		200			{object}	map[string]string
//	@Failure		400			{object}	apierrors.Error
//	@Failure		401			{object}	apierrors.Error
//	@Failure		403			{object}	apierrors.Error
//	@Failure		404			{object}	apierrors.Error
//	@Failure		500			{object}	apierrors.Error
//	@Router			/servers/{serverID}/status [post]
func (api *API) UpdateAgentStatusHandler(w http.ResponseWriter, r *http.Request) {

	claims, ok := GetUserClaims(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, apierrors.Unauthorized("Authorization header or session cookie required"))
		return
	}

	serverID := chi.URLParam(r, "serverID")
	var payload AgentStatusPayload

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.log().Error("bad request for status update", "server_id", serverID, "error", err)
		writeAPIError(w, apierrors.BadRequestf("Invalid status payload: %s", err.Error()))
		return
	}
	defer r.Body.Close()

	// Ensure agents can only update status for servers in their assigned environment
	if claims.EnvironmentID != nil {
		// Check if the server belongs to the agent's environment
		isInEnvironment, err := api.Repo.IsServerInEnvironment(r.Context(), serverID, *claims.EnvironmentID)
		if err != nil {
			api.log().Error("failed to check server-environment relationship",
				"username", claims.Username, "server_id", serverID, "environment_id", *claims.EnvironmentID, "error", err)
			writeAPIError(w, apierrors.Internal("Failed to validate server access"))
			return
		}

		if !isInEnvironment {
			api.log().Warn("user attempted to update status for server not in their environment",
				"username", claims.Username, "environment_id", *claims.EnvironmentID, "server_id", serverID)
			writeAPIError(w, apierrors.Forbidden("Permission denied to update status for this server"))
			return
		}
	}

	logAttrs := []any{
		"server_id", serverID,
		"commit_hash", payload.CommitHash,
		"status", payload.Status,
		"is_drifted", payload.IsDrifted,
	}
	if payload.ErrorMessage != nil {
		logAttrs = append(logAttrs, "error_message", *payload.ErrorMessage)
	}
	api.log().Info("status update received", logAttrs...)

	status := models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   payload.CommitHash,
		IsDrifted:    payload.IsDrifted,
		Status:       payload.Status,
		Timestamp:    time.Now().UTC(),
		ErrorMessage: payload.ErrorMessage,
		AgentVersion: payload.AgentVersion,
		DriftDetails: payload.DriftDetails,
	}

	// Store status in database using the RENAMED method
	if err := api.Repo.CreateAgentStatus(r.Context(), &status); err != nil {
		api.log().Error("failed to store agent status", "server_id", serverID, "error", err)
		if errors.Is(err, database.ErrNotFound) {
			writeAPIError(w, apierrors.NotFoundf("Server %s not registered", serverID))
		} else {
			writeAPIError(w, apierrors.Internal("Failed to store agent status"))
		}
		return

	} else if status.ID == 0 {
		api.log().Debug("no changes detected, skipping history entry", "server_id", serverID)
	} else {
		api.log().Info("stored new status entry", "server_id", serverID, "status_id", status.ID)
	}

	if api.Notifications != nil && (payload.IsDrifted || payload.Status == "error") {
		api.sendStatusNotification(r.Context(), serverID, payload)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

// AgentGetServerInfoHandler provides configuration details for agents using token auth
//
//	@Summary		Get server info (Agent Token)
//	@Description	Returns repository configuration for a server. Authenticated via agent token.
//	@Tags			Agent
//	@Accept			json
//	@Produce		json
//	@Security		AgentAuth
//	@Success		200	{object}	ServerInfoResponse
//	@Failure		401	{object}	apierrors.Error
//	@Failure		404	{object}	apierrors.Error
//	@Failure		500	{object}	apierrors.Error
//	@Router			/agent/server-info [get]
func (api *API) AgentGetServerInfoHandler(w http.ResponseWriter, r *http.Request) {
	agentToken, ok := GetAgentToken(r.Context())
	if !ok || agentToken == nil {
		writeAPIError(w, apierrors.Unauthorized("Agent token required"))
		return
	}

	serverID := agentToken.ServerID
	api.log().Debug("agent server info request", "server_id", serverID)

	serverWithEnv, err := api.Repo.GetServerWithEnvironment(r.Context(), serverID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			api.log().Info("agent server not found", "server_id", serverID)
			writeAPIError(w, apierrors.NotFound("Server", serverID))
		} else {
			api.log().Error("failed to get agent server info", "server_id", serverID, "error", err)
			writeAPIError(w, apierrors.Internal("Failed to retrieve server information"))
		}
		return
	}

	resp := ServerInfoResponse{
		RepoURL:          serverWithEnv.RepoURL,
		Branch:           serverWithEnv.Branch,
		DeployPath:       serverWithEnv.DeployPath,
		TargetCommitHash: "",
		AutoReconcile:    serverWithEnv.AutoReconcile,
	}

	if serverWithEnv.TargetCommitHash != nil {
		resp.TargetCommitHash = *serverWithEnv.TargetCommitHash
	}

	api.log().Debug("successfully retrieved agent server info", "server_id", serverID)
	writeJSON(w, http.StatusOK, resp)
}

// AgentRequestGitTokenHandler returns GitHub installation credentials to authenticated agents.
//
//	@Summary		Request Git token
//	@Description	Returns a GitHub installation token for repository access. Rate limited.
//	@Tags			Agent
//	@Accept			json
//	@Produce		json
//	@Security		AgentAuth
//	@Success		200	{object}	AgentGitTokenResponse
//	@Failure		400	{object}	apierrors.Error
//	@Failure		401	{object}	apierrors.Error
//	@Failure		429	{object}	apierrors.Error
//	@Failure		500	{object}	apierrors.Error
//	@Failure		501	{object}	apierrors.Error
//	@Router			/agent/git-token [get]
func (api *API) AgentRequestGitTokenHandler(w http.ResponseWriter, r *http.Request) {
	if api.TokenProvider == nil {
		writeAPIError(w, apierrors.NotImplemented("Git credential distribution is not configured"))
		return
	}

	agentToken, ok := GetAgentToken(r.Context())
	if !ok || agentToken == nil {
		writeAPIError(w, apierrors.Unauthorized("Agent token required"))
		return
	}

	server, err := api.Repo.GetServerByID(r.Context(), agentToken.ServerID)
	if err != nil {
		api.log().Error("failed to load server for git token", "server_id", agentToken.ServerID, "error", err)
		writeAPIError(w, apierrors.Internal("Failed to load server"))
		return
	}

	if server.EnvironmentID == nil {
		writeAPIError(w, apierrors.BadRequest("Server is not assigned to an environment"))
		return
	}

	if !api.allowGitTokenRequest(server.ID, time.Now()) {
		writeAPIError(w, apierrors.TooManyRequests("Git token requests are being rate limited"))
		return
	}

	env, err := api.Repo.GetEnvironment(r.Context(), *server.EnvironmentID)
	if err != nil {
		api.log().Error("failed to load environment for git token", "environment_id", *server.EnvironmentID, "server_id", agentToken.ServerID, "error", err)
		writeAPIError(w, apierrors.Internal("Failed to load environment"))
		return
	}

	if env.Provider != models.ProviderGitHub {
		writeAPIError(w, apierrors.BadRequest("Git credential distribution is only supported for GitHub environments"))
		return
	}

	if env.InstallationID == 0 {
		writeAPIError(w, apierrors.BadRequest("Environment is missing GitHub installation configuration"))
		return
	}

	tokenValue, expiresAt, err := api.TokenProvider.GetInstallationToken(r.Context(), env.InstallationID)
	if err != nil {
		api.log().Error("failed to obtain installation token", "environment_id", env.ID, "error", err)
		writeAPIError(w, apierrors.Internal("Failed to obtain installation token"))
		return
	}

	response := AgentGitTokenResponse{
		Token:          tokenValue,
		ExpiresAt:      expiresAt,
		RepoURL:        env.RepoURL,
		Provider:       env.Provider,
		InstallationID: env.InstallationID,
		Authentication: "github_app_installation_token",
		RepositoryID:   env.RepositoryID,
		AppSlug:        env.AppSlug,
	}

	api.log().Info("git token issued", "environment_id", env.ID, "installation_id", env.InstallationID, "server_id", server.ID, "expires_at", expiresAt.Format(time.RFC3339))
	api.RecordAuditLog(r, "issue_git_token", "environment", fmt.Sprintf("%d", env.ID), map[string]interface{}{
		"server_id":       server.ID,
		"installation_id": env.InstallationID,
		"expires_at":      expiresAt,
	})
	writeJSON(w, http.StatusOK, response)
}

func (api *API) allowGitTokenRequest(serverID string, now time.Time) bool {
	api.gitTokenMu.Lock()
	defer api.gitTokenMu.Unlock()

	if api.gitTokenCooldownUntil == nil {
		api.gitTokenCooldownUntil = make(map[string]time.Time)
	}
	if api.gitTokenBackoff == nil {
		api.gitTokenBackoff = make(map[string]time.Duration)
	}
	if api.gitTokenHistory == nil {
		api.gitTokenHistory = make(map[string][]tokenAttempt)
	}

	if until, ok := api.gitTokenCooldownUntil[serverID]; ok && now.Before(until) {
		backoff := api.gitTokenBackoff[serverID]
		if backoff == 0 {
			backoff = gitTokenCooldown
		}
		backoff *= 2
		if backoff > gitTokenMaxCooldown {
			backoff = gitTokenMaxCooldown
		}
		api.gitTokenBackoff[serverID] = backoff
		api.gitTokenCooldownUntil[serverID] = now.Add(backoff)
		api.log().Warn("rate limiting git token request", "server_id", serverID, "backoff", backoff)
		history := append(api.gitTokenHistory[serverID], tokenAttempt{
			Timestamp: now,
			Allowed:   false,
			Message:   fmt.Sprintf("rate limited, backoff %s", backoff),
		})
		if len(history) > 10 {
			history = history[len(history)-10:]
		}
		api.gitTokenHistory[serverID] = history
		return false
	}

	api.gitTokenBackoff[serverID] = gitTokenCooldown
	api.gitTokenCooldownUntil[serverID] = now.Add(gitTokenCooldown)
	history := append(api.gitTokenHistory[serverID], tokenAttempt{
		Timestamp: now,
		Allowed:   true,
		Message:   "token issued",
	})
	if len(history) > 10 {
		history = history[len(history)-10:]
	}
	api.gitTokenHistory[serverID] = history
	return true
}

// AgentUpdateStatusHandler handles status updates from agents using token auth
//
//	@Summary		Update agent status (Agent Token)
//	@Description	Receives status updates from agents including drift detection and errors. Authenticated via agent token.
//	@Tags			Agent
//	@Accept			json
//	@Produce		json
//	@Security		AgentAuth
//	@Param			request	body		AgentStatusPayload	true	"Status update payload"
//	@Success		200		{object}	map[string]string
//	@Failure		400		{object}	apierrors.Error
//	@Failure		401		{object}	apierrors.Error
//	@Failure		404		{object}	apierrors.Error
//	@Failure		500		{object}	apierrors.Error
//	@Router			/agent/status [post]
func (api *API) AgentUpdateStatusHandler(w http.ResponseWriter, r *http.Request) {

	agentToken, ok := GetAgentToken(r.Context())
	if !ok || agentToken == nil {
		writeAPIError(w, apierrors.Unauthorized("Agent token required"))
		return
	}

	serverID := agentToken.ServerID
	var payload AgentStatusPayload

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		api.log().Error("bad request for agent status update", "server_id", serverID, "error", err)
		writeAPIError(w, apierrors.BadRequestf("Invalid status payload: %s", err.Error()))
		return
	}
	defer r.Body.Close()

	logAttrs := []any{
		"server_id", serverID,
		"commit_hash", payload.CommitHash,
		"status", payload.Status,
		"is_drifted", payload.IsDrifted,
	}
	if payload.ErrorMessage != nil {
		logAttrs = append(logAttrs, "error_message", *payload.ErrorMessage)
	}
	api.log().Info("agent status update received", logAttrs...)

	status := models.AgentStatus{
		ServerID:     serverID,
		CommitHash:   payload.CommitHash,
		IsDrifted:    payload.IsDrifted,
		Status:       payload.Status,
		Timestamp:    time.Now().UTC(),
		ErrorMessage: payload.ErrorMessage,
		AgentVersion: payload.AgentVersion,
		DriftDetails: payload.DriftDetails,
	}

	if err := api.Repo.CreateAgentStatus(r.Context(), &status); err != nil {
		api.log().Error("failed to store agent status", "server_id", serverID, "error", err)
		if errors.Is(err, database.ErrNotFound) {
			writeAPIError(w, apierrors.NotFoundf("Server %s not registered", serverID))
		} else {
			writeAPIError(w, apierrors.Internal("Failed to store agent status"))
		}
		return
	} else if status.ID == 0 {
		api.log().Debug("agent status no changes detected, skipping history entry", "server_id", serverID)
	} else {
		api.log().Info("stored new agent status entry", "server_id", serverID, "status_id", status.ID)
	}

	if api.Notifications != nil && (payload.IsDrifted || payload.Status == "error") {
		api.sendStatusNotification(r.Context(), serverID, payload)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}
