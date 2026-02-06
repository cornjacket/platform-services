package query

import (
	"net/http"
	"strings"
)

// RegisterRoutes registers query service routes on the provided mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Health check
	mux.HandleFunc("/health", h.HandleHealth)

	// Projection endpoints
	// We need to handle both:
	//   GET /api/v1/projections/{type} -> list
	//   GET /api/v1/projections/{type}/{id} -> get single
	mux.HandleFunc("/api/v1/projections/", h.routeProjections)
}

// routeProjections routes to either list or get based on path depth.
func (h *Handler) routeProjections(w http.ResponseWriter, r *http.Request) {
	// Strip the prefix and count path segments
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/projections/")
	path = strings.TrimSuffix(path, "/")

	parts := strings.Split(path, "/")

	switch len(parts) {
	case 1:
		// /api/v1/projections/{type}
		h.HandleListProjections(w, r)
	case 2:
		// /api/v1/projections/{type}/{id}
		h.HandleGetProjection(w, r)
	default:
		h.writeError(w, http.StatusNotFound, "not found")
	}
}
