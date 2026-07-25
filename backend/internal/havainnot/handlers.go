package havainnot

import (
	"encoding/json"
	"log"
	"net/http"
)

// Attribution is FMI's required credit; their open data is CC BY 4.0.
const Attribution = "Säähavainnot: Ilmatieteen laitos, CC BY 4.0"

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
		log.Printf("Havainnot: failed to encode response: %v", err)
	}
}

// serviceUnavailable is the cold-start answer: no poll has succeeded yet. The
// frontend shows it as a loading state, not an error.
func serviceUnavailable(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Error(w, "data not available yet", http.StatusServiceUnavailable)
}

type stationsResponse struct {
	Snapshot
	Attribution string `json:"attribution"`
}

// Stations serves every station's latest readings.
func (h *Handlers) Stations(w http.ResponseWriter, r *http.Request) {
	snap, ok := h.store.Snapshot(r.Context())
	if !ok {
		serviceUnavailable(w)
		return
	}
	if snap.Stations == nil {
		snap.Stations = []Station{}
	}
	// Stations report every ten minutes, so a couple of minutes of shared caching
	// costs nothing in freshness.
	w.Header().Set("Cache-Control", "public, max-age=120")
	writeJSON(w, stationsResponse{Snapshot: snap, Attribution: Attribution})
}

type stationResponse struct {
	StationDetail
	Parameters  []Parameter `json:"parameters"`
	Attribution string      `json:"attribution"`
}

// Station serves one station with its recent series, computed on demand from the
// cached snapshot — a visitor clicking a station never triggers an upstream
// request.
func (h *Handlers) Station(w http.ResponseWriter, r *http.Request) {
	fmisid := r.PathValue("fmisid")
	if fmisid == "" {
		http.NotFound(w, r)
		return
	}

	// A miss during cold start is a 503 (come back shortly); a miss once data is
	// loaded is a genuine 404 (no such station).
	detail, ok := h.store.Detail(fmisid)
	if !ok {
		if h.store.Count() == 0 {
			serviceUnavailable(w)
			return
		}
		http.NotFound(w, r)
		return
	}
	if detail.Series == nil {
		detail.Series = []Reading{}
	}
	w.Header().Set("Cache-Control", "public, max-age=120")
	writeJSON(w, stationResponse{StationDetail: detail, Parameters: Parameters, Attribution: Attribution})
}
