//go:build !test
// +build !test

package server

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pullbase/pullbase/server/pkg/auth"
	"github.com/pullbase/pullbase/server/pkg/database"
)

//go:embed ui/*
var uiFiles embed.FS

// SetupWebUIRoutes configures the embedded web UI routes
func SetupWebUIRoutes(r chi.Router, authService *auth.Service, api *API) {
	uiFS, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		return
	}

	// Serve Vite static assets (JS, CSS, fonts, etc.)
	r.Get("/ui/assets/*", func(w http.ResponseWriter, r *http.Request) {
		requestPath := strings.TrimPrefix(r.URL.Path, "/ui")

		content, err := fs.ReadFile(uiFS, requestPath[1:])
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// Set appropriate MIME type
		ext := filepath.Ext(requestPath)
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			// Fallback MIME types for common assets
			switch ext {
			case ".js":
				mimeType = "application/javascript"
			case ".css":
				mimeType = "text/css"
			case ".woff2":
				mimeType = "font/woff2"
			case ".woff":
				mimeType = "font/woff"
			case ".ttf":
				mimeType = "font/ttf"
			case ".svg":
				mimeType = "image/svg+xml"
			default:
				mimeType = "application/octet-stream"
			}
		}

		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Write(content)
	})

	// Serve favicon and other static files
	r.Get("/ui/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		content, err := fs.ReadFile(uiFS, "favicon.ico")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(content)
	})

	r.Get("/ui/vite.svg", func(w http.ResponseWriter, r *http.Request) {
		content, err := fs.ReadFile(uiFS, "vite.svg")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(content)
	})

	r.Get("/ui/logo-nav-dark.svg", func(w http.ResponseWriter, r *http.Request) {
		content, err := fs.ReadFile(uiFS, "logo-nav-dark.svg")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(content)
	})

	r.Get("/ui/login", func(w http.ResponseWriter, r *http.Request) {
		content, err := fs.ReadFile(uiFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(content)
	})

	r.Get("/ui/login/", func(w http.ResponseWriter, r *http.Request) {
		content, err := fs.ReadFile(uiFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(content)
	})

	// Handle web form login submission
	r.Post("/ui/login", func(w http.ResponseWriter, r *http.Request) {
		username := r.FormValue("username")
		password := r.FormValue("password")

		if username == "" || password == "" {
			http.Redirect(w, r, "/ui/login?error=missing_credentials", http.StatusSeeOther)
			return
		}

		user, err := api.Repo.GetUser(r.Context(), username)
		if err != nil || user == nil {
			http.Redirect(w, r, "/ui/login?error=invalid_credentials", http.StatusSeeOther)
			return
		}

		// Validate password
		if !database.CheckPassword(password, user.PasswordHash) {
			http.Redirect(w, r, "/ui/login?error=invalid_credentials", http.StatusSeeOther)
			return
		}

		// Use the auth service directly (replicating login logic)
		tokenString, err := api.Auth.GenerateToken(user)
		if err != nil {
			http.Redirect(w, r, "/ui/login?error=server_error", http.StatusSeeOther)
			return
		}

		// Set the session cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    tokenString,
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
			SameSite: http.SameSiteLaxMode,
		})

		http.Redirect(w, r, "/ui/servers", http.StatusSeeOther)
	})

	// Protected UI routes
	r.Group(func(r chi.Router) {
		r.Use(authService.AuthCookieMiddleware)

		r.Get("/ui/*", func(w http.ResponseWriter, r *http.Request) {
			requestedPath := strings.TrimPrefix(r.URL.Path, "/ui")
			if requestedPath == "" || requestedPath == "/" {
				requestedPath = "/index.html"
			}

			// Serve static assets (CSS, JS, images, etc.)
			if strings.HasPrefix(requestedPath, "/assets/") ||
				strings.HasSuffix(requestedPath, ".css") ||
				strings.HasSuffix(requestedPath, ".js") ||
				strings.HasSuffix(requestedPath, ".svg") ||
				strings.HasSuffix(requestedPath, ".png") ||
				strings.HasSuffix(requestedPath, ".ico") {
				content, err := fs.ReadFile(uiFS, requestedPath[1:])
				if err == nil {
					ext := filepath.Ext(requestedPath)
					mimeType := mime.TypeByExtension(ext)
					if mimeType == "" {
						mimeType = "application/octet-stream"
					}
					w.Header().Set("Content-Type", mimeType)
					w.Write(content)
					return
				}
			}

			// For all other routes, serve the main index.html (SPA routing)
			indexContent, err := fs.ReadFile(uiFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(indexContent)
		})
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})
}

// ValidateLocalAccess ensures the request is coming from localhost
func ValidateLocalAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			clientIP = strings.Split(forwarded, ",")[0]
		}
		if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
			clientIP = realIP
		}

		if strings.Contains(clientIP, ":") {
			clientIP = strings.Split(clientIP, ":")[0]
		}

		allowedIPs := []string{"127.0.0.1", "::1", "localhost"}
		isLocal := false
		for _, allowedIP := range allowedIPs {
			if clientIP == allowedIP || strings.HasPrefix(clientIP, "127.") {
				isLocal = true
				break
			}
		}

		if !isLocal {
			http.Error(w, "Access denied: Web UI only accessible from localhost", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
