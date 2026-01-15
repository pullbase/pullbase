package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/pullbase/pullbase/server/pkg/apierrors"
	"github.com/pullbase/pullbase/server/pkg/auth"
	"github.com/pullbase/pullbase/server/pkg/csrf"
	"github.com/pullbase/pullbase/server/pkg/database"
	"github.com/pullbase/pullbase/server/pkg/logging"
	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/notifications"
)

// ContextKey is a type used for context keys to avoid collisions.
type ContextKey string

// UserClaimsKey is the key used to store user claims in the request context.
const UserClaimsKey ContextKey = "userClaims"

const (
	healthCheckTimeout     = 5 * time.Second
	dbLatencyDegradedThres = 1 * time.Second
)

// HealthStatus represents the status of a health check.
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// HealthCheckResponse is the JSON response for health check endpoints.
type HealthCheckResponse struct {
	Status  HealthStatus           `json:"status"`
	Service string                 `json:"service"`
	Checks  map[string]CheckResult `json:"checks,omitempty"`
}

// CheckResult represents the result of an individual health check.
type CheckResult struct {
	Status    HealthStatus `json:"status"`
	LatencyMs *int64       `json:"latency_ms,omitempty"`
	Version   *int         `json:"version,omitempty"`
	Error     string       `json:"error,omitempty"`
}

// InstallationTokenProvider issues Git provider installation tokens for agents.
type InstallationTokenProvider interface {
	GetInstallationToken(ctx context.Context, installationID int64) (string, time.Time, error)
}

// API represents the dependencies for the API handlers
type API struct {
	Repo                  *database.Repository
	Auth                  *auth.Service
	CSRF                  *csrf.Manager
	WebhookHandlers       *WebhookHandlers
	RollbackHandlers      *RollbackHandlers
	TokenProvider         InstallationTokenProvider
	Notifications         *notifications.Service
	Logger                *logging.Logger
	gitTokenMu            sync.Mutex
	gitTokenCooldownUntil map[string]time.Time
	gitTokenBackoff       map[string]time.Duration
	gitTokenHistory       map[string][]tokenAttempt

	bootstrapMu         sync.Mutex
	bootstrapSecretHash []byte
	bootstrapEnabled    bool
	bootstrapAttempts   map[string]time.Time
	bootstrapSecretPath string
}

func (a *API) log() *logging.Logger {
	if a.Logger != nil {
		return a.Logger
	}
	return logging.Default()
}

// ErrorResponse defines the structure for API error responses
type ErrorResponse struct {
	Error string `json:"error"`
}

// writeJSON sends a JSON response with a given status code
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// writeError sends a JSON error response with consistent formatting.
// Uses apierrors.WriteHTTPError for standardized error responses that include
// the error kind based on the HTTP status code.
func writeError(w http.ResponseWriter, status int, message string) {
	apierrors.WriteHTTPError(w, status, message)
}

// writeAPIError writes an apierrors.Error to the response.
// This is the preferred way to write errors when using the apierrors package.
func writeAPIError(w http.ResponseWriter, err error) {
	apierrors.WriteError(w, err)
}

// HealthCheckHandler provides a simple health check endpoint
func (a *API) HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	a.LivenessHandler(w, r)
}

// LivenessHandler godoc
//
//	@Summary		Liveness probe
//	@Description	Kubernetes-style liveness check. Returns healthy if service is running and database is reachable.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	HealthCheckResponse
//	@Failure		503	{object}	HealthCheckResponse
//	@Router			/healthz [get]
func (a *API) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	checks := make(map[string]CheckResult)
	overallStatus := HealthStatusHealthy

	dbCheck := a.Repo.CheckHealth(ctx)
	latencyMs := dbCheck.Latency.Milliseconds()

	if dbCheck.Healthy {
		dbStatus := HealthStatusHealthy
		if dbCheck.Latency > dbLatencyDegradedThres {
			dbStatus = HealthStatusDegraded
			if overallStatus == HealthStatusHealthy {
				overallStatus = HealthStatusDegraded
			}
		}
		checks["database"] = CheckResult{
			Status:    dbStatus,
			LatencyMs: &latencyMs,
		}
	} else {
		overallStatus = HealthStatusUnhealthy
		errMsg := "connection failed"
		if dbCheck.Error != nil {
			errMsg = dbCheck.Error.Error()
		}
		checks["database"] = CheckResult{
			Status: HealthStatusUnhealthy,
			Error:  errMsg,
		}
	}

	response := HealthCheckResponse{
		Status:  overallStatus,
		Service: "pullbase-server",
		Checks:  checks,
	}

	statusCode := http.StatusOK
	if overallStatus == HealthStatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSON(w, statusCode, response)
}

// ReadinessHandler godoc
//
//	@Summary		Readiness probe
//	@Description	Kubernetes-style readiness check. Returns healthy if service is ready to accept traffic.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	HealthCheckResponse
//	@Failure		503	{object}	HealthCheckResponse
//	@Router			/readyz [get]
func (a *API) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
	defer cancel()

	checks := make(map[string]CheckResult)
	overallStatus := HealthStatusHealthy

	// Database check
	dbCheck := a.Repo.CheckHealth(ctx)
	latencyMs := dbCheck.Latency.Milliseconds()

	if dbCheck.Healthy {
		dbStatus := HealthStatusHealthy
		if dbCheck.Latency > dbLatencyDegradedThres {
			dbStatus = HealthStatusDegraded
			if overallStatus == HealthStatusHealthy {
				overallStatus = HealthStatusDegraded
			}
		}
		checks["database"] = CheckResult{
			Status:    dbStatus,
			LatencyMs: &latencyMs,
		}

		migrationStatus := a.Repo.GetMigrationStatus(ctx)
		if migrationStatus.Error != nil {
			overallStatus = HealthStatusUnhealthy
			checks["migrations"] = CheckResult{
				Status: HealthStatusUnhealthy,
				Error:  migrationStatus.Error.Error(),
			}
		} else if migrationStatus.Dirty {
			overallStatus = HealthStatusUnhealthy
			version := migrationStatus.Version
			checks["migrations"] = CheckResult{
				Status:  HealthStatusUnhealthy,
				Version: &version,
				Error:   "migration is dirty (failed or incomplete)",
			}
		} else {
			version := migrationStatus.Version
			checks["migrations"] = CheckResult{
				Status:  HealthStatusHealthy,
				Version: &version,
			}
		}
	} else {
		overallStatus = HealthStatusUnhealthy
		errMsg := "connection failed"
		if dbCheck.Error != nil {
			errMsg = dbCheck.Error.Error()
		}
		checks["database"] = CheckResult{
			Status: HealthStatusUnhealthy,
			Error:  errMsg,
		}
		checks["migrations"] = CheckResult{
			Status: HealthStatusUnknown,
		}
	}

	response := HealthCheckResponse{
		Status:  overallStatus,
		Service: "pullbase-server",
		Checks:  checks,
	}

	statusCode := http.StatusOK
	if overallStatus == HealthStatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	writeJSON(w, statusCode, response)
}

// requireRole checks if the authenticated user in the context has one of the allowed roles.
// Returns the claims and true if authorized, otherwise returns claims and false.
func requireRole(r *http.Request, allowedRoles ...string) (*auth.Claims, bool) {
	claims, ok := GetUserClaims(r.Context())
	if !ok {
		logging.Error("claims not found in context for role check")
		return nil, false
	}

	if slices.Contains(allowedRoles, claims.Role) {
		return claims, true
	}

	logging.Warn("user attempted action without required role",
		"username", claims.Username, "user_id", claims.UserID, "role", claims.Role, "required_roles", allowedRoles)
	return claims, false
}

// RecordAuditLog persists an audit log entry capturing user actions.
func (api *API) RecordAuditLog(r *http.Request, action, resourceType, resourceID string, details interface{}) {
	if api == nil || api.Repo == nil || r == nil {
		return
	}

	var userID *int
	if claims, ok := GetUserClaims(r.Context()); ok && claims != nil {
		userID = &claims.UserID
	}

	var rawDetails json.RawMessage
	if details != nil {
		if encoded, err := json.Marshal(details); err == nil {
			rawDetails = encoded
		} else {
			api.log().Warn("failed to encode audit details", "resource_type", resourceType, "action", action, "error", err)
		}
	}

	entry := &models.AuditLog{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      rawDetails,
		IPAddress:    extractClientIP(r),
		Timestamp:    time.Now(),
	}

	if err := api.Repo.CreateAuditLog(r.Context(), entry); err != nil {
		api.log().Error("failed to persist audit log", "action", action, "resource_type", resourceType, "error", err)
	}
}

func extractClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}

	return host
}
