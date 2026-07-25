package salama

import (
	"encoding/json"
	"log"
	"net/http"
)

// Attribution is FMI's required credit; their open data is CC BY 4.0.
const Attribution = "Salamahavainnot: Ilmatieteen laitos, CC BY 4.0"

type Handlers struct {
	store *Store
}

func NewHandlers(store *Store) *Handlers {
	return &Handlers{store: store}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("Salama: failed to encode response: %v", err)
	}
}

// serviceUnavailable is the cold-start answer: no poll has succeeded yet. The
// frontend shows it as a loading state, not an error — and importantly not as
// "no lightning", which an empty array would imply.
func serviceUnavailable(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Error(w, "data not available yet", http.StatusServiceUnavailable)
}

type strikesResponse struct {
	StrikeSummary
	Attribution string `json:"attribution"`
}

// Strikes serves the retained lightning strikes.
func (h *Handlers) Strikes(w http.ResponseWriter, r *http.Request) {
	summary, ok := h.store.Get(r.Context(), retainWindow)
	if !ok {
		serviceUnavailable(w)
		return
	}
	if summary.Strikes == nil {
		// Encode an empty array rather than null, so the client can iterate
		// without a guard.
		summary.Strikes = []Strike{}
	}
	// Strikes turn over every minute, so allow a short shared cache window but no
	// more — a stale thunderstorm is worse than no cache.
	w.Header().Set("Cache-Control", "public, max-age=30")
	writeJSON(w, strikesResponse{StrikeSummary: summary, Attribution: Attribution})
}
