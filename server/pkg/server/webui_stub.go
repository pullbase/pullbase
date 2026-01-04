//go:build test
// +build test

package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/pullbase/pullbase/server/pkg/auth"
)

// SetupWebUIRoutes is a no-op stub for tests to avoid embed.FS requirements
func SetupWebUIRoutes(r chi.Router, authService *auth.Service, api *API) {
	// No-op during tests - UI not required
}
