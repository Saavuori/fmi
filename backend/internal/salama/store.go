package salama

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"sync"
	"time"

	"fmi/internal/core/cache"
)

const (
	strikesKey = "fmi:salama:strikes"

	// The TTL is generous relative to the poll cadence. It exists to stop truly
	// ancient data being served forever if the poller dies, not to force
	// refreshes — the poller owns the cadence.
	strikesTTL = 30 * time.Minute
)

// Store stands between visitors and FMI: the poller writes here, handlers read
// here, and nothing a visitor does triggers an upstream request. Values live in
// the shared cache and are mirrored in this process, so a Redis outage degrades to
// "serving slightly stale data" rather than an error page.
type Store struct {
	cache cache.Cache

	mu       sync.RWMutex
	fallback []Strike
	updated  int64
	haveData bool
}

func NewStore(c cache.Cache) *Store {
	return &Store{cache: c}
}

// Merge folds newly fetched strikes into the retained set, drops anything past
// the window, and publishes the result.
//
// Merging rather than replacing matters because each poll asks only for the last
// few minutes: replacing would shrink the map to that sliver every cycle. Strikes
// are keyed by time and position so a strike seen in two overlapping polls is
// stored once.
func (s *Store) Merge(ctx context.Context, fresh []Strike, window time.Duration, now time.Time) int {
	cutoff := now.Add(-window).Unix()

	s.mu.Lock()
	seen := make(map[strikeKey]bool, len(s.fallback)+len(fresh))
	merged := make([]Strike, 0, len(s.fallback)+len(fresh))
	for _, set := range [][]Strike{s.fallback, fresh} {
		for _, st := range set {
			if st.Timestamp < cutoff {
				continue
			}
			k := keyOf(st)
			if seen[k] {
				continue
			}
			seen[k] = true
			merged = append(merged, st)
		}
	}
	sort.Slice(merged, func(i, j int) bool { return merged[i].Timestamp < merged[j].Timestamp })

	s.fallback = merged
	s.updated = now.Unix()
	s.haveData = true
	count := len(merged)
	s.mu.Unlock()

	summary := StrikeSummary{
		Strikes:       merged,
		WindowMinutes: int(window / time.Minute),
		Updated:       now.Unix(),
	}
	body, err := json.Marshal(summary)
	if err != nil {
		log.Printf("Salama: failed to marshal strikes: %v", err)
		return count
	}
	if err := s.cache.SetValue(ctx, strikesKey, body, strikesTTL); err != nil {
		log.Printf("Salama: cache write failed: %v (memory fallback holds)", err)
	}
	return count
}

// Get returns the retained strikes. The bool is load-bearing: false means no poll
// has succeeded yet, which handlers turn into a 503 so the frontend shows a
// loading state rather than an empty map that looks like calm weather.
func (s *Store) Get(ctx context.Context, window time.Duration) (StrikeSummary, bool) {
	if body, err := s.cache.GetValue(ctx, strikesKey); err == nil && body != nil {
		var summary StrikeSummary
		if err := json.Unmarshal(body, &summary); err == nil {
			return summary, true
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.haveData {
		return StrikeSummary{}, false
	}
	return StrikeSummary{
		Strikes:       s.fallback,
		WindowMinutes: int(window / time.Minute),
		Updated:       s.updated,
	}, true
}

// Count returns the retained strike count, for the health report.
func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.fallback)
}

// strikeKey identifies a strike for de-duplication. Coordinates are quantised to
// whole microdegrees (~0.1 m) so a float that round-trips through JSON with a
// different last bit still matches itself.
type strikeKey struct {
	ts       int64
	lat, lon int64
}

func keyOf(s Strike) strikeKey {
	return strikeKey{
		ts:  s.Timestamp,
		lat: int64(s.Latitude * 1e6),
		lon: int64(s.Longitude * 1e6),
	}
}
