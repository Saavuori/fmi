package havainnot

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"fmi/internal/core/cache"
)

const (
	stationsKey = "fmi:havainnot:stations"

	// Generous relative to the poll cadence: the TTL exists to stop truly ancient
	// data being served forever if the poller dies, not to force refreshes.
	stationsTTL = 30 * time.Minute
)

// Store stands between visitors and FMI: the poller writes here, handlers read
// here, and nothing a visitor does triggers an upstream request.
//
// Only the station list goes to Redis. The per-station series stay in process:
// they are the bulk of the data, only the detail panel reads them, and losing them
// on restart costs one poll cycle.
type Store struct {
	cache cache.Cache

	mu       sync.RWMutex
	stations []Station
	details  map[string]StationDetail
	updated  int64
	haveData bool
}

func NewStore(c cache.Cache) *Store {
	return &Store{cache: c, details: make(map[string]StationDetail)}
}

// Set publishes a freshly polled set of stations.
func (s *Store) Set(ctx context.Context, details []StationDetail, now time.Time) {
	stations := make([]Station, 0, len(details))
	byID := make(map[string]StationDetail, len(details))
	for _, d := range details {
		stations = append(stations, d.Station)
		byID[d.FMISID] = d
	}

	s.mu.Lock()
	s.stations = stations
	s.details = byID
	s.updated = now.Unix()
	s.haveData = true
	s.mu.Unlock()

	body, err := json.Marshal(Snapshot{Stations: stations, Parameters: Parameters, Updated: now.Unix()})
	if err != nil {
		log.Printf("Havainnot: failed to marshal stations: %v", err)
		return
	}
	if err := s.cache.SetValue(ctx, stationsKey, body, stationsTTL); err != nil {
		log.Printf("Havainnot: cache write failed: %v (memory fallback holds)", err)
	}
}

// Snapshot returns the station list. The bool is load-bearing: false means no poll
// has succeeded yet, which handlers turn into a 503 so the frontend shows a
// loading state.
func (s *Store) Snapshot(ctx context.Context) (Snapshot, bool) {
	if body, err := s.cache.GetValue(ctx, stationsKey); err == nil && body != nil {
		var snap Snapshot
		if err := json.Unmarshal(body, &snap); err == nil {
			return snap, true
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.haveData {
		return Snapshot{}, false
	}
	return Snapshot{Stations: s.stations, Parameters: Parameters, Updated: s.updated}, true
}

// Detail returns one station with its recent series.
func (s *Store) Detail(fmisid string) (StationDetail, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.details[fmisid]
	return d, ok
}

// Count returns the station count, for the health report.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.stations)
}
