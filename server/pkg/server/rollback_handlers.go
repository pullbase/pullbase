package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/pullbase/pullbase/server/pkg/models"
	"github.com/pullbase/pullbase/server/pkg/rollback"
)

// RollbackHandlers contains HTTP handlers for rollback operations
type RollbackHandlers struct {
	rollbackService *rollback.Service
}

// NewRollbackHandlers creates new rollback handlers
func NewRollbackHandlers(rollbackService *rollback.Service) *RollbackHandlers {
	return &RollbackHandlers{
		rollbackService: rollbackService,
	}
}

func (h *RollbackHandlers) InitiateRollback(w http.ResponseWriter, r *http.Request) {
	environmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid environment ID"})
		return
	}

	var req rollback.RollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid request body"})
		return
	}

	// Set environment ID from URL
	req.EnvironmentID = environmentID

	// Extract user from context (assuming auth middleware sets this)
	if userCtx := r.Context().Value("user"); userCtx != nil {
		if user, ok := userCtx.(string); ok {
			req.InitiatedBy = user
		}
	}

	// Default to "api" if no user context
	if req.InitiatedBy == "" {
		req.InitiatedBy = "api"
	}

	// Validate request
	if err := h.rollbackService.ValidateRollbackRequest(r.Context(), &req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// Initiate rollback
	response, err := h.rollbackService.InitiateRollback(r.Context(), &req)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}

// GetRollbackStatus handles GET /api/v1/rollbacks/{id}
func (h *RollbackHandlers) GetRollbackStatus(w http.ResponseWriter, r *http.Request) {
	rollbackID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid rollback ID"})
		return
	}

	rollbackEvent, err := h.rollbackService.GetRollbackStatus(r.Context(), rollbackID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rollbackEvent)
}

// ListRollbacks handles GET /api/v1/environments/{id}/rollbacks
func (h *RollbackHandlers) ListRollbacks(w http.ResponseWriter, r *http.Request) {
	environmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid environment ID"})
		return
	}

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 20 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := 0 // default
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	rollbacks, err := h.rollbackService.ListRollbacks(r.Context(), environmentID, limit, offset)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if rollbacks == nil {
		rollbacks = make([]*models.RollbackEvent, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"rollbacks": rollbacks,
		"limit":     limit,
		"offset":    offset,
	})
}

// GetAvailableCommits handles GET /api/v1/environments/{id}/commits
func (h *RollbackHandlers) GetAvailableCommits(w http.ResponseWriter, r *http.Request) {
	environmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid environment ID"})
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20 // default
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	commits, err := h.rollbackService.GetAvailableCommits(r.Context(), environmentID, limit)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if commits == nil {
		commits = make([]*models.CommitInfo, 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"commits": commits,
		"limit":   limit,
	})
}
