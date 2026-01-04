package server

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	chi "github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/pullbase/pullbase/server/docs"
	"github.com/pullbase/pullbase/server/pkg/auth"
)

// CORSConfig holds CORS-related configuration for the server.
type CORSConfig struct {
	AllowedOrigins []string
	IsDevelopment  bool
}

func buildAllowOriginFunc(cfg CORSConfig) func(r *http.Request, origin string) bool {
	allowedSet := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowedSet[strings.TrimSpace(o)] = struct{}{}
	}

	return func(r *http.Request, origin string) bool {
		if origin == "" {
			return false
		}

		parsed, err := url.Parse(origin)
		if err != nil {
			return false
		}

		if _, ok := allowedSet[origin]; ok {
			return true
		}

		if cfg.IsDevelopment {
			host := parsed.Hostname()
			if host == "localhost" || host == "127.0.0.1" {
				return true
			}
		}

		if parsed.Host == r.Host {
			return true
		}

		return false
	}
}

// SetupRoutes configures the API routes
func SetupRoutes(api *API, authService *auth.Service, corsConfig CORSConfig) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   corsConfig.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		AllowOriginFunc:  buildAllowOriginFunc(corsConfig),
		MaxAge:           300,
	}))
	r.Use(middleware.Timeout(60 * time.Second))

	// Health checks
	r.Get("/api/v1/healthz", api.LivenessHandler)
	r.Get("/api/v1/readyz", api.ReadinessHandler)

	// Swagger documentation
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
	))

	// Bootstrap and auth
	r.Get("/api/v1/bootstrap/status", api.GetBootstrapStatus)
	r.Post("/api/v1/bootstrap/admin", api.BootstrapAdminHandler)
	r.Post("/api/v1/auth/login", api.LoginHandler)
	r.Get("/api/v1/serverinfo/{serverID}", api.GetServerInfoHandler)

	// Webhook endpoints (public, but signature validated)
	r.Route("/webhooks", func(r chi.Router) {
		r.Post("/{provider}", api.WebhookHandlers.HandleWebhook)
	})

	// Agent API routes (require agent token authentication)
	r.Group(func(r chi.Router) {
		r.Use(AgentAuthMiddleware(api.Repo))

		// Agent-specific endpoints
		r.Route("/api/v1/agent", func(r chi.Router) {
			r.Get("/serverinfo", api.AgentGetServerInfoHandler)
			r.Put("/status", api.AgentUpdateStatusHandler)
			r.Get("/git-token", api.AgentRequestGitTokenHandler)
		})
	})

	// Protected routes (require JWT)
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(authService))

		// User info
		r.Get("/api/v1/auth/me", api.GetCurrentUserHandler)

		// User management
		r.Get("/api/v1/users", api.ListUsersHandler)
		r.Post("/api/v1/users", api.CreateUserHandler)
		r.Delete("/api/v1/users/{userID}", api.DeleteUserHandler)

		// Config validation
		r.Post("/api/v1/validate-config", api.ValidateConfigHandler)

		// Token expiration warnings
		r.Get("/api/v1/tokens/expiring", api.GetExpiringTokensHandler)

		// Server CRUD
		r.Route("/api/v1/servers", func(r chi.Router) {
			r.Post("/", api.CreateServerHandler)
			r.Get("/", api.ListServersHandler)
			r.Get("/{serverID}", api.GetServerHandler)
			r.Put("/{serverID}", api.UpdateServerHandler)
			r.Delete("/{serverID}", api.DeleteServerHandler)
			r.Post("/{serverID}/toggle-auto-reconcile", api.ToggleAutoReconcileHandler)

			// Agent status reporting
			r.Put("/{serverID}/status", api.UpdateAgentStatusHandler)
			r.Get("/{serverID}/status/history", api.GetServerStatusHistoryHandler)
			r.Get("/{serverID}/drift", api.GetServerDriftHandler)

			// Agent token management
			r.Get("/{serverID}/tokens", api.ListServerTokensHandler)
			r.Post("/{serverID}/tokens", api.CreateServerTokenHandler)
			r.Delete("/{serverID}/tokens/{tokenID}", api.DeactivateServerTokenHandler)
			r.Get("/{serverID}/install", api.GetServerInstallInstructionsHandler)
			r.Get("/{serverID}/install-script", api.GetServerInstallScriptHandler)
		})

		// Environment CRUD and Rollback endpoints
		r.Route("/api/v1/environments", func(r chi.Router) {
			r.Post("/", api.WebhookHandlers.CreateEnvironment)
			r.Get("/", api.WebhookHandlers.ListEnvironments)
			r.Get("/{environmentID}", api.WebhookHandlers.GetEnvironment)
			r.Put("/{environmentID}", api.WebhookHandlers.UpdateEnvironment)
			r.Delete("/{environmentID}", api.WebhookHandlers.DeleteEnvironment)
			r.Post("/{environmentID}/toggle-auto-reconcile", api.WebhookHandlers.ToggleEnvironmentAutoReconcile)
			r.Post("/{environmentID}/test-webhook", api.TestWebhookHandler)

			// Rollback endpoints
			r.Post("/{id}/rollback", api.RollbackHandlers.InitiateRollback)
			r.Get("/{id}/rollbacks", api.RollbackHandlers.ListRollbacks)
			r.Get("/{id}/commits", api.RollbackHandlers.GetAvailableCommits)
		})

		// Webhook status & environment health
		r.Get("/api/v1/webhook-statuses", api.WebhookHandlers.GetWebhookStatuses)
		r.Get("/api/v1/environments/health", api.GetEnvironmentHealth)

		// Rollback status endpoint
		r.Route("/api/v1/rollbacks", func(r chi.Router) {
			r.Get("/{id}", api.RollbackHandlers.GetRollbackStatus)
		})

		// Metrics endpoints
		r.Route("/api/v1/metrics", func(r chi.Router) {
			r.Get("/drift", api.GetDriftMetricsHandler)
			r.Get("/reconciliation", api.GetReconciliationMetricsHandler)
			r.Get("/connectivity", api.GetAgentConnectivityHandler)
		})
	})

	// --- Embedded Web UI Routes ---
	// Web UI routes (accessible from any IP when server binds to 0.0.0.0)
	SetupWebUIRoutes(r, authService, api)

	// Apply localhost-only access control to UI routes
	// r.Group(func(r chi.Router) {
	// 	r.Use(ValidateLocalAccess)
	// 	SetupWebUIRoutes(r)
	// })

	return r
}
