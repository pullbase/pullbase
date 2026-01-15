package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	chi "github.com/go-chi/chi/v5"
	"github.com/pullbase/pullbase/server/pkg/apierrors"
	"github.com/pullbase/pullbase/server/pkg/auth"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/logging"
	"github.com/pullbase/pullbase/server/pkg/models"
)

const (
	bootstrapAttemptCooldown = 2 * time.Second
)

var bootstrapUsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,64}$`)

// LoginRequest defines the expected structure for login requests
type LoginRequest struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	EnvironmentID *int64 `json:"environment_id,omitempty"`
	ServerID      string `json:"server_id,omitempty"` // For backward compatibility
}

// LoginResponse defines the structure for successful login responses
type LoginResponse struct {
	AccessToken string      `json:"access_token"`
	User        UserSummary `json:"user"`
}

// BootstrapAdminRequest defines the shape of a bootstrap admin creation request.
type BootstrapAdminRequest struct {
	BootstrapSecret string `json:"bootstrap_secret"`
	Username        string `json:"username"`
	Password        string `json:"password"`
}

// BootstrapAdminResponse returns credentials resulting from a successful bootstrap.
type BootstrapAdminResponse struct {
	AccessToken string      `json:"access_token"`
	User        UserSummary `json:"user"`
}

type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type CreateUserResponse struct {
	User UserSummary `json:"user"`
}

type ListUsersResponse struct {
	Users  []UserSummary `json:"users"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
	Role   string        `json:"role,omitempty"`
}

type DeleteUserRequest struct {
	ConfirmUsername string `json:"confirm_username"`
}

// UserSummary provides basic user info without sensitive details
type UserSummary struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

var allowedUserRoles = []string{
	models.RoleAdmin,
	models.RoleUser,
	models.RoleViewer,
}

// EnableBootstrap stores the hashed bootstrap secret and makes the bootstrap endpoint available.
func (a *API) EnableBootstrap(secret, secretPath string) {
	if a == nil {
		return
	}

	sum := sha256.Sum256([]byte(secret))
	hash := make([]byte, len(sum))
	copy(hash, sum[:])

	a.bootstrapMu.Lock()
	defer a.bootstrapMu.Unlock()

	a.bootstrapSecretHash = hash
	a.bootstrapEnabled = true
	a.bootstrapSecretPath = secretPath
	a.bootstrapAttempts = make(map[string]time.Time)
}

// DisableBootstrap clears any bootstrap credentials and disables the endpoint.
func (a *API) DisableBootstrap() {
	if a == nil {
		return
	}

	a.bootstrapMu.Lock()
	defer a.bootstrapMu.Unlock()

	a.bootstrapEnabled = false
	a.bootstrapSecretHash = nil
	a.bootstrapAttempts = nil
	if a.bootstrapSecretPath != "" {
		if err := os.Remove(a.bootstrapSecretPath); err != nil {
			if !os.IsNotExist(err) {
				a.log().Warn("failed to remove bootstrap secret file", "path", a.bootstrapSecretPath, "error", err)
			}
		} else {
			a.log().Info("removed bootstrap secret file", "path", a.bootstrapSecretPath)
		}
	}
	a.bootstrapSecretPath = ""
}

// GetBootstrapStatus godoc
//
//	@Summary		Get bootstrap status
//	@Description	Check if bootstrap mode is enabled and get the secret file path.
//	@Tags			Auth
//	@Produce		json
//	@Success		200	{object}	map[string]interface{}
//	@Router			/bootstrap/status [get]
func (a *API) GetBootstrapStatus(w http.ResponseWriter, r *http.Request) {
	a.bootstrapMu.Lock()
	enabled := a.bootstrapEnabled
	secretPath := a.bootstrapSecretPath
	a.bootstrapMu.Unlock()

	resp := map[string]interface{}{
		"bootstrap_enabled": enabled,
	}

	if enabled && secretPath != "" {
		resp["secret_path"] = secretPath
	}

	if a != nil && a.Repo != nil {
		if adminCount, err := a.Repo.CountActiveAdmins(r.Context()); err != nil {
			a.log().Warn("failed to count admin users", "error", err)
		} else {
			resp["admin_count"] = adminCount
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// LoginHandler godoc
//
//	@Summary		User login
//	@Description	Authenticate with username and password to receive a JWT token.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LoginRequest	true	"Login credentials"
//	@Success		200		{object}	LoginResponse
//	@Failure		401		{object}	ErrorResponse
//	@Router			/auth/login [post]
func (a *API) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, apierrors.BadRequest("Invalid request body"))
		return
	}

	if req.Username == "" || req.Password == "" {
		writeAPIError(w, apierrors.Validation("Username and password are required"))
		return
	}

	user, err := a.Repo.GetUser(r.Context(), req.Username)
	if err != nil || user == nil {
		writeAPIError(w, apierrors.Unauthorized("Invalid username or password"))
		return
	}

	if !database.CheckPassword(req.Password, user.PasswordHash) {
		writeAPIError(w, apierrors.Unauthorized("Invalid username or password"))
		return
	}

	var tokenString string
	var environmentID *int64

	// Determine environment ID from either direct specification or server lookup
	if req.EnvironmentID != nil {
		environmentID = req.EnvironmentID
	} else if req.ServerID != "" {
		// Backward compatibility: look up environment ID from server ID
		serverEnvID, err := a.Repo.GetServerEnvironmentID(r.Context(), req.ServerID)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				writeAPIError(w, apierrors.NotFound("Server", req.ServerID))
				return
			}
			a.log().Error("failed to get environment for server", "server_id", req.ServerID, "error", err)
			writeAPIError(w, apierrors.Internal("Failed to validate server"))
			return
		}

		// Require servers to have an environment for agent authentication
		if serverEnvID == nil {
			a.log().Warn("server not assigned to any environment", "server_id", req.ServerID)
			writeAPIError(w, apierrors.BadRequestf("Server %s must be assigned to an environment for agent authentication", req.ServerID))
			return
		}

		environmentID = serverEnvID
	}

	// Generate appropriate token based on environment
	if environmentID != nil {
		// Validate environment exists
		_, err := a.Repo.GetEnvironment(r.Context(), *environmentID)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				writeAPIError(w, apierrors.NotFoundf("Environment %d not found", *environmentID))
				return
			}
			a.log().Error("failed to validate environment", "environment_id", *environmentID, "username", req.Username, "error", err)
			writeAPIError(w, apierrors.Internal("Failed to validate environment"))
			return
		}

		tokenString, err = a.Auth.GenerateTokenForEnvironment(user, environmentID)
		if err != nil {
			a.log().Error("failed to generate environment-specific token", "username", req.Username, "environment_id", *environmentID, "error", err)
			writeAPIError(w, apierrors.Internal("Failed to generate authentication token"))
			return
		}

		if req.ServerID != "" {
			a.log().Info("server-to-environment login successful", "username", req.Username, "server_id", req.ServerID, "environment_id", *environmentID)
		} else {
			a.log().Info("environment-specific login successful", "username", req.Username, "environment_id", *environmentID)
		}
	} else {
		tokenString, err = a.Auth.GenerateToken(user)
		if err != nil {
			a.log().Error("failed to generate token", "username", req.Username, "error", err)
			writeAPIError(w, apierrors.Internal("Failed to generate authentication token"))
			return
		}

		a.log().Info("login successful", "username", req.Username)
	}

	a.RecordAuditLog(r, "login", "user", strconv.Itoa(user.ID), map[string]interface{}{
		"environment_id": environmentID,
		"server_id":      req.ServerID,
		"username":       req.Username,
	})

	resp := LoginResponse{
		AccessToken: tokenString,
		User: UserSummary{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// BootstrapAdminHandler godoc
//
//	@Summary		Create bootstrap admin
//	@Description	Create the first admin user using the bootstrap secret. Only available when bootstrap is enabled.
//	@Tags			Auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		BootstrapAdminRequest	true	"Bootstrap credentials"
//	@Success		201		{object}	BootstrapAdminResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		429		{object}	ErrorResponse
//	@Router			/bootstrap/admin [post]
func (a *API) BootstrapAdminHandler(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.Repo == nil || a.Auth == nil {
		writeAPIError(w, apierrors.Internal("Bootstrap not available"))
		return
	}

	defer r.Body.Close()

	clientIP := extractClientIP(r)
	if clientIP == "" {
		clientIP = strings.TrimSpace(r.RemoteAddr)
	}

	now := time.Now()

	a.bootstrapMu.Lock()
	if !a.bootstrapEnabled || len(a.bootstrapSecretHash) == 0 {
		a.bootstrapMu.Unlock()
		writeError(w, http.StatusGone, "Admin bootstrap is not available")
		return
	}

	if a.bootstrapAttempts == nil {
		a.bootstrapAttempts = make(map[string]time.Time)
	}
	if last, ok := a.bootstrapAttempts[clientIP]; ok && now.Sub(last) < bootstrapAttemptCooldown {
		a.bootstrapMu.Unlock()
		writeAPIError(w, apierrors.TooManyRequests("Bootstrap attempts are rate limited"))
		return
	}
	a.bootstrapAttempts[clientIP] = now

	secretHash := make([]byte, len(a.bootstrapSecretHash))
	copy(secretHash, a.bootstrapSecretHash)
	a.bootstrapMu.Unlock()

	var req BootstrapAdminRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, apierrors.BadRequest("Invalid request body"))
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.BootstrapSecret == "" {
		writeAPIError(w, apierrors.Validation("Bootstrap secret is required"))
		return
	}
	if req.Username == "" {
		writeAPIError(w, apierrors.Validation("Username is required"))
		return
	}
	if !bootstrapUsernamePattern.MatchString(req.Username) {
		writeAPIError(w, apierrors.Validation("Username must be 3-64 characters and contain only letters, numbers, '.', '_' or '-'"))
		return
	}
	if utf8.RuneCountInString(req.Password) < auth.BootstrapPasswordMinLength {
		writeAPIError(w, apierrors.Validationf("Password must be at least %d characters long", auth.BootstrapPasswordMinLength))
		return
	}

	providedHash := sha256.Sum256([]byte(req.BootstrapSecret))
	if subtle.ConstantTimeCompare(providedHash[:], secretHash) != 1 {
		writeAPIError(w, apierrors.Unauthorized("Invalid bootstrap secret"))
		return
	}

	hasAdmin, err := a.Repo.HasActiveAdmin(r.Context())
	if err != nil {
		a.log().Error("failed to verify existing admin users", "error", err)
		writeAPIError(w, apierrors.Internal("Failed to verify existing admin users"))
		return
	}
	if hasAdmin {
		a.DisableBootstrap()
		writeError(w, http.StatusGone, "Admin bootstrap has already been completed")
		return
	}

	if err := a.Repo.CreateUser(r.Context(), req.Username, req.Password, models.RoleAdmin); err != nil {
		if errors.Is(err, database.ErrConflict) {
			writeAPIError(w, apierrors.Conflict("Username already exists"))
			return
		}
		a.log().Error("failed to create admin user", "error", err)
		writeAPIError(w, apierrors.Internal("Failed to create admin user"))
		return
	}

	user, err := a.Repo.GetUser(r.Context(), req.Username)
	if err != nil {
		a.log().Error("failed to reload newly created admin user", "error", err)
		writeAPIError(w, apierrors.Internal("Failed to verify admin user"))
		return
	}

	tokenString, err := a.Auth.GenerateToken(user)
	if err != nil {
		a.log().Error("failed to generate token for new admin", "error", err)
		writeAPIError(w, apierrors.Internal("Failed to generate admin token"))
		return
	}

	a.DisableBootstrap()

	a.log().Info("admin bootstrap completed", "username", req.Username, "source_ip", clientIP)
	a.RecordAuditLog(r, "bootstrap_admin", "user", strconv.Itoa(user.ID), map[string]interface{}{
		"username": req.Username,
		"sourceIP": clientIP,
	})

	writeJSON(w, http.StatusCreated, BootstrapAdminResponse{
		AccessToken: tokenString,
		User: UserSummary{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
	})
}

// GetCurrentUserHandler godoc
//
//	@Summary		Get current user
//	@Description	Get the authenticated user's information.
//	@Tags			Auth
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	UserSummary
//	@Failure		401	{object}	ErrorResponse
//	@Router			/auth/me [get]
func (a *API) GetCurrentUserHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := GetUserClaims(r.Context())
	if !ok || claims == nil {
		writeAPIError(w, apierrors.Unauthorized("Authorization header or session cookie required"))
		return
	}

	// Return the relevant user info from claims
	resp := UserSummary{
		ID:       claims.UserID,
		Username: claims.Username,
		Role:     claims.Role,
	}

	writeJSON(w, http.StatusOK, resp)
}

// CreateUserHandler godoc
//
//	@Summary		Create user
//	@Description	Create a new user. Admin only.
//	@Tags			Users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			request	body		CreateUserRequest	true	"User details"
//	@Success		201		{object}	CreateUserResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		409		{object}	ErrorResponse
//	@Router			/users [post]
func (api *API) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin)
	if !authorized {
		writeAPIError(w, apierrors.Forbidden("Permission denied to create user"))
		return
	}

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, apierrors.BadRequest("Invalid request body"))
		return
	}
	defer r.Body.Close()

	req.Username = strings.TrimSpace(req.Username)
	req.Role = strings.TrimSpace(req.Role)

	if req.Username == "" || !bootstrapUsernamePattern.MatchString(req.Username) {
		writeAPIError(w, apierrors.Validation("Username must be 3-64 characters and contain only letters, numbers, '.', '_' or '-'"))
		return
	}

	if utf8.RuneCountInString(req.Password) < auth.BootstrapPasswordMinLength {
		writeAPIError(w, apierrors.Validationf("Password must be at least %d characters long", auth.BootstrapPasswordMinLength))
		return
	}

	if req.Role == "" {
		req.Role = models.RoleUser
	}
	if !slices.Contains(allowedUserRoles, req.Role) {
		writeAPIError(w, apierrors.Validation("Role must be one of: admin, user, viewer"))
		return
	}

	if err := api.Repo.CreateUser(r.Context(), req.Username, req.Password, req.Role); err != nil {
		if errors.Is(err, database.ErrConflict) || strings.Contains(err.Error(), "already exists") {
			writeAPIError(w, apierrors.Conflict("Username already exists"))
			return
		}
		api.log().Error("failed to create user", "created_by", claims.Username, "created_by_id", claims.UserID, "error", err)
		writeAPIError(w, apierrors.Internal("Failed to create user"))
		return
	}

	createdUser, err := api.Repo.GetUser(r.Context(), req.Username)
	if err != nil {
		api.log().Error("failed to reload created user", "username", req.Username, "error", err)
		writeAPIError(w, apierrors.Internal("Failed to load created user"))
		return
	}

	api.RecordAuditLog(r, "create", "user", strconv.Itoa(createdUser.ID), map[string]any{
		"username":       createdUser.Username,
		"role":           createdUser.Role,
		"created_by":     claims.Username,
		"created_by_id":  claims.UserID,
		"source_ip":      r.RemoteAddr,
		"impersonation":  false,
		"creation_scope": "manual",
	})

	resp := CreateUserResponse{
		User: UserSummary{
			ID:       createdUser.ID,
			Username: createdUser.Username,
			Role:     createdUser.Role,
		},
	}
	writeJSON(w, http.StatusCreated, resp)
}

// ListUsersHandler godoc
//
//	@Summary		List users
//	@Description	List all users. Admin only.
//	@Tags			Users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			limit	query		int		false	"Limit"		default(100)
//	@Param			offset	query		int		false	"Offset"	default(0)
//	@Param			role	query		string	false	"Filter by role"
//	@Success		200		{object}	ListUsersResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Router			/users [get]
func (api *API) ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	_, authorized := requireRole(r, models.RoleAdmin)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to list users")
		return
	}

	query := r.URL.Query()

	limit := 100
	if rawLimit := strings.TrimSpace(query.Get("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			if parsed > 500 {
				limit = 500
			} else {
				limit = parsed
			}
		} else {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
	}

	offset := 0
	if rawOffset := strings.TrimSpace(query.Get("offset")); rawOffset != "" {
		if parsed, err := strconv.Atoi(rawOffset); err == nil && parsed >= 0 {
			offset = parsed
		} else {
			writeError(w, http.StatusBadRequest, "offset must be zero or a positive integer")
			return
		}
	}

	var roleFilter string
	if rawRole := strings.TrimSpace(query.Get("role")); rawRole != "" {
		if !slices.Contains(allowedUserRoles, rawRole) {
			writeError(w, http.StatusBadRequest, "role must be one of: admin, user, viewer")
			return
		}
		roleFilter = rawRole
	}

	users, total, err := api.Repo.ListUsers(r.Context(), limit, offset, roleFilter)
	if err != nil {
		api.log().Error("failed to list users", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}

	response := ListUsersResponse{
		Users:  make([]UserSummary, 0, len(users)),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	if roleFilter != "" {
		response.Role = roleFilter
	}

	for _, user := range users {
		if user == nil {
			continue
		}
		response.Users = append(response.Users, UserSummary{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// DeleteUserHandler godoc
//
//	@Summary		Delete user
//	@Description	Delete a user. Admin only. Requires username confirmation.
//	@Tags			Users
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			userID	path		int					true	"User ID"
//	@Param			request	body		DeleteUserRequest	true	"Confirmation"
//	@Success		204		"User deleted"
//	@Failure		400		{object}	ErrorResponse
//	@Failure		401		{object}	ErrorResponse
//	@Failure		403		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Router			/users/{userID} [delete]
func (api *API) DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	claims, authorized := requireRole(r, models.RoleAdmin)
	if !authorized {
		writeError(w, http.StatusForbidden, "Permission denied to delete user")
		return
	}

	userIDParam := chi.URLParam(r, "userID")
	userID, err := strconv.Atoi(strings.TrimSpace(userIDParam))
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if claims.UserID == userID {
		writeError(w, http.StatusBadRequest, "You cannot delete your own account")
		return
	}

	var req DeleteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	confirmUsername := strings.TrimSpace(req.ConfirmUsername)
	if confirmUsername == "" {
		writeError(w, http.StatusBadRequest, "confirm_username is required")
		return
	}

	targetUser, err := api.Repo.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "User not found")
			return
		}
		api.log().Error("failed to fetch user for deletion", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to load user")
		return
	}

	if targetUser.Username != confirmUsername {
		writeError(w, http.StatusBadRequest, "Confirmation does not match username")
		return
	}

	if targetUser.Role == models.RoleAdmin {
		_, totalAdmins, err := api.Repo.ListUsers(r.Context(), 2, 0, models.RoleAdmin)
		if err != nil {
			api.log().Error("failed to verify admin count before deletion", "error", err)
			writeError(w, http.StatusInternalServerError, "Failed to verify admin count")
			return
		}
		if totalAdmins <= 1 {
			writeError(w, http.StatusBadRequest, "Cannot delete the only active admin user")
			return
		}
	}

	if err := api.Repo.DeleteUser(r.Context(), userID); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			writeError(w, http.StatusNotFound, "User not found")
			return
		}
		api.log().Error("failed to delete user", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to delete user")
		return
	}

	api.RecordAuditLog(r, "delete", "user", strconv.Itoa(userID), map[string]any{
		"deleted_username": targetUser.Username,
		"deleted_role":     targetUser.Role,
		"deleted_by":       claims.Username,
		"deleted_by_id":    claims.UserID,
	})

	w.WriteHeader(http.StatusNoContent)
}

// AuthMiddleware creates a middleware handler that validates JWT tokens
func AuthMiddleware(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenString string
			var claims *auth.Claims
			var err error

			cookie, errCookie := r.Cookie("session_token")
			if errCookie == nil && cookie.Value != "" {
				tokenString = cookie.Value
				claims, err = authService.ValidateToken(tokenString)
				if err != nil {
					logging.Warn("invalid session_token cookie", "error", err)

					http.SetCookie(w, &http.Cookie{
						Name:     "session_token",
						Value:    "",
						Path:     "/",
						Expires:  time.Unix(0, 0),
						MaxAge:   -1,
						HttpOnly: true,
						Secure:   isRequestSecure(r),
						SameSite: http.SameSiteLaxMode,
					})
				}
			}

			if claims == nil || err != nil {
				authHeader := r.Header.Get("Authorization")
				if authHeader == "" {
					if errCookie == nil && cookie.Value != "" {
						writeError(w, http.StatusUnauthorized, "Invalid session token")
					} else {
						writeError(w, http.StatusUnauthorized, "Authorization header or session cookie required")
					}
					return
				}

				parts := strings.Split(authHeader, " ")
				if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
					writeError(w, http.StatusUnauthorized, "Invalid Authorization header format (must be Bearer token)")
					return
				}
				tokenString = parts[1]

				claims, err = authService.ValidateToken(tokenString)
				if err != nil {
					writeError(w, http.StatusUnauthorized, "Invalid or expired token")
					return
				}
			}

			ctx := context.WithValue(r.Context(), UserClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isRequestSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// GetUserClaims retrieves user claims from the request context.
// Helper function for handlers to access user info.
func GetUserClaims(ctx context.Context) (*auth.Claims, bool) {
	claims, ok := ctx.Value(UserClaimsKey).(*auth.Claims)
	return claims, ok
}
