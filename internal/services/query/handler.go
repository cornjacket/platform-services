package query

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Handler handles HTTP requests for the query service.
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler creates a new query HTTP handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger.With("handler", "query"),
	}
}

// HandleGetProjection handles GET /api/v1/projections/{projection_type}/{aggregate_id}
func (h *Handler) HandleGetProjection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse path parameters
	// Expected path: /api/v1/projections/{projection_type}/{aggregate_id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/projections/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		h.writeError(w, http.StatusBadRequest, "invalid path: expected /api/v1/projections/{type}/{id}")
		return
	}

	projectionType := parts[0]
	aggregateID := parts[1]

	// Validate projection type
	if !IsValidProjectionType(projectionType) {
		h.writeError(w, http.StatusBadRequest, "invalid projection type: "+projectionType)
		return
	}

	projection, err := h.service.GetProjection(r.Context(), projectionType, aggregateID)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			h.writeError(w, http.StatusNotFound, "projection not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.writeJSON(w, http.StatusOK, projection)
}

// HandleListProjections handles GET /api/v1/projections/{projection_type}
func (h *Handler) HandleListProjections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Parse path parameter
	// Expected path: /api/v1/projections/{projection_type}
	projectionType := strings.TrimPrefix(r.URL.Path, "/api/v1/projections/")
	if projectionType == "" || strings.Contains(projectionType, "/") {
		h.writeError(w, http.StatusBadRequest, "invalid path: expected /api/v1/projections/{type}")
		return
	}

	// Validate projection type
	if !IsValidProjectionType(projectionType) {
		h.writeError(w, http.StatusBadRequest, "invalid projection type: "+projectionType)
		return
	}

	// Parse query parameters
	limit := 20
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil {
			offset = o
		}
	}

	list, err := h.service.ListProjections(r.Context(), projectionType, limit, offset)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.writeJSON(w, http.StatusOK, list)
}

// HandleHealth handles GET /health
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "healthy"})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}
