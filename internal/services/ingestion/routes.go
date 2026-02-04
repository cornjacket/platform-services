package ingestion

import "net/http"

// RegisterRoutes registers the ingestion service routes on the provided mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/events", h.HandleIngest)
	mux.HandleFunc("/health", h.HandleHealth)
}
