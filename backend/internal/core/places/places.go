// Package places is the app's place search: a name in, a map location out.
//
// It is the one thing here that is not FMI data, and the one endpoint a visitor
// can cause an upstream request from. Both facts shape it.
//
// The geocoder is OpenStreetMap's Nominatim, which is free and needs no key but
// asks for something in return: a descriptive User-Agent, at most one request a
// second, no per-keystroke autocomplete, and results cached rather than re-asked.
// So the browser searches on submit, this package serialises every outbound call
// through a one-per-second gate, and answers come back out of the shared cache
// for a day. Steady state for a small map is a handful of misses an hour.
package places

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"fmi/internal/core/cache"
	"fmi/internal/core/upstream"
)

const (
	nominatimBase = "https://nominatim.openstreetmap.org"

	// Nominatim's absolute ceiling is one request per second. Everything queues
	// behind this; a query that would have to wait longer than maxQueue is
	// refused with a 429 rather than held until the viewer has given up.
	minInterval = 1100 * time.Millisecond
	maxQueue    = 3 * time.Second

	// Places do not move. A day is short enough that a genuinely new suburb
	// shows up eventually and long enough that a search anyone repeats costs
	// nothing.
	cacheTTL = 24 * time.Hour

	// Enough to disambiguate without turning the popover into a list to read.
	maxResults = 6

	minQueryLen = 2
	maxQueryLen = 80

	// User-facing: a search must not sit on a connection the way a full-grid
	// raster fetch may.
	requestTimeout = 10 * time.Second

	attribution = "© OpenStreetMap contributors (ODbL), Nominatim"
)

var errBusy = errors.New("places: rate gate is backed up")

// Place is one search hit. Mirrors the Place interface in
// frontend/src/shared/components/PlaceSearch.tsx — change one, change both.
type Place struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	// Named in full rather than lat/lon, to match the station and strike models.
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	// What to zoom to on arrival. A municipality and a farm track are both
	// "places", and flying to the same zoom for each would overshoot one of them.
	Zoom float64 `json:"zoom"`
}

type searchResponse struct {
	Results     []Place `json:"results"`
	Attribution string  `json:"attribution"`
}

// Service holds the cache, the upstream client and the rate gate. It is not a
// server.Mode: there is no poller and no snapshot, so it has nothing to report
// to /api/health — it either answers a search or it does not.
type Service struct {
	cache  cache.Cache
	client *upstream.Client

	mu sync.Mutex
	// Earliest instant the next outbound request may leave.
	next time.Time
}

func NewService(c cache.Cache) *Service {
	return &Service{cache: c, client: upstream.NewClient(nominatimBase)}
}

func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/places", s.handleSearch)
}

func (s *Service) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < minQueryLen || len([]rune(query)) > maxQueryLen {
		http.Error(w, fmt.Sprintf("q must be %d-%d characters", minQueryLen, maxQueryLen), http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
	defer cancel()

	results, err := s.Search(ctx, query)
	if err != nil {
		if errors.Is(err, errBusy) {
			w.Header().Set("Retry-After", "2")
			http.Error(w, "place search is busy", http.StatusTooManyRequests)
			return
		}
		log.Printf("places: search %q: %v\n", query, err)
		http.Error(w, "place search failed", http.StatusBadGateway)
		return
	}

	// No Access-Control-Allow-Origin, unlike every mode endpoint. Those serve
	// FMI open data that anyone may take; this one spends a shared rate budget
	// against a volunteer-run service under our User-Agent, and opening it up
	// would make this app the front door to someone else's geocoding.
	//
	// Cached in the browser too: the same search from the same visitor should
	// not come back here at all.
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	json.NewEncoder(w).Encode(searchResponse{Results: results, Attribution: attribution})
}

// Search returns the places matching query, from cache when possible.
func (s *Service) Search(ctx context.Context, query string) ([]Place, error) {
	key := cacheKey(query)

	if payload, err := s.cache.GetValue(ctx, key); err == nil && payload != nil {
		var cached []Place
		if err := json.Unmarshal(payload, &cached); err == nil {
			return cached, nil
		}
		// A stale or corrupt entry is not worth failing a search over; fall
		// through and re-fetch.
	}

	if err := s.acquire(ctx); err != nil {
		return nil, err
	}

	body, err := s.client.GetURL(ctx, s.searchURL(query))
	if err != nil {
		return nil, err
	}

	results, err := parseResults(body)
	if err != nil {
		return nil, err
	}

	// A no-match answer is cached too. "Ei osumia" is an expensive thing to
	// re-derive, and a typo gets retyped more than once.
	if payload, err := json.Marshal(results); err == nil {
		if err := s.cache.SetValue(ctx, key, payload, cacheTTL); err != nil {
			log.Printf("places: caching %q: %v\n", query, err)
		}
	}
	return results, nil
}

func cacheKey(query string) string {
	return "fmi:places:" + strings.ToLower(strings.Join(strings.Fields(query), " "))
}

// countrycodes pins the search to Finland: this is a map of Finland, and an
// unqualified "Kuopio" should not have to compete with the rest of the planet.
func (s *Service) searchURL(query string) string {
	q := url.Values{}
	q.Set("q", query)
	q.Set("format", "jsonv2")
	q.Set("countrycodes", "fi")
	q.Set("limit", strconv.Itoa(maxResults))
	q.Set("addressdetails", "1")
	q.Set("accept-language", "fi")
	return nominatimBase + "/search?" + q.Encode()
}

// acquire blocks until this caller owns the next outbound slot, or gives up if
// the queue in front of it is already longer than maxQueue.
//
// The gate is a single instant rather than a token bucket because the limit it
// enforces has no burst allowance: one request per second means one request per
// second, not sixty banked for the top of the minute.
func (s *Service) acquire(ctx context.Context) error {
	s.mu.Lock()
	now := time.Now()
	slot := s.next
	if slot.Before(now) {
		slot = now
	}
	if slot.Sub(now) > maxQueue {
		s.mu.Unlock()
		return errBusy
	}
	s.next = slot.Add(minInterval)
	s.mu.Unlock()

	wait := time.Until(slot)
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// nominatimPlace is the subset of Nominatim's jsonv2 record this uses.
type nominatimPlace struct {
	Lat         string            `json:"lat"`
	Lon         string            `json:"lon"`
	Name        string            `json:"name"`
	DisplayName string            `json:"display_name"`
	AddressType string            `json:"addresstype"`
	Type        string            `json:"type"`
	Address     map[string]string `json:"address"`
}

func parseResults(body []byte) ([]Place, error) {
	var raw []nominatimPlace
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("places: decoding search response: %w", err)
	}

	places := make([]Place, 0, len(raw))
	for _, r := range raw {
		lat, errLat := strconv.ParseFloat(r.Lat, 64)
		lon, errLon := strconv.ParseFloat(r.Lon, 64)
		if errLat != nil || errLon != nil {
			continue
		}
		name := r.Name
		if name == "" {
			// Some records (a plain address, say) carry no short name. The first
			// component of the display name is the closest thing to one.
			name, _, _ = strings.Cut(r.DisplayName, ",")
			name = strings.TrimSpace(name)
		}
		if name == "" {
			continue
		}
		places = append(places, Place{
			Name:      name,
			Detail:    detailFor(r, name),
			Latitude:  lat,
			Longitude: lon,
			Zoom:      zoomFor(r),
		})
	}
	return places, nil
}

// addressKeys is the order the subtitle is looked up in, most useful first.
//
// `municipality` deliberately ranks below `city`: in Finland OSM fills it with
// the *seutukunta*, the sub-region — Kaisaniemi in Helsinki comes back with
// municipality "Helsingin seutukunta" and city "Helsinki", and only the second
// is what anyone would use to tell two places apart. `region` is left out
// entirely, being "Manner-Suomi" for everywhere but Åland.
var addressKeys = []string{"city", "town", "village", "municipality", "county", "state"}

// detailFor answers "which one is this" — the town or region under the name,
// because plenty of Finnish place names repeat across the country.
//
// The structured address is preferred, since Nominatim's display_name varies in
// length and ordering by what kind of thing was matched. The display name is the
// fallback: its first component is the name itself, which is dropped, as is the
// country, which on a map of Finland says nothing.
func detailFor(r nominatimPlace, name string) string {
	for _, key := range addressKeys {
		if v := r.Address[key]; v != "" && v != name {
			return v
		}
	}

	parts := strings.Split(r.DisplayName, ",")
	out := make([]string, 0, 2)
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		if p == "" || p == name || p == "Suomi" || p == "Finland" {
			continue
		}
		out = append(out, p)
		if len(out) == 2 {
			break
		}
	}
	return strings.Join(out, ", ")
}

// zoomFor picks the zoom that frames the kind of thing that was found. Nominatim
// reports it as `addresstype` (falling back to `type`), whose vocabulary is
// OpenStreetMap's place=* values plus the address components.
var zoomByType = map[string]float64{
	"country":       5,
	"state":         7,
	"region":        7,
	"county":        8,
	"municipality":  9,
	"city":          10,
	"town":          11,
	"island":        10,
	"suburb":        12,
	"village":       12,
	"hamlet":        13,
	"neighbourhood": 13,
	"quarter":       13,
	"locality":      13,
	"road":          14,
	"building":      15,
	"house":         15,
}

func zoomFor(r nominatimPlace) float64 {
	if z, ok := zoomByType[r.AddressType]; ok {
		return z
	}
	if z, ok := zoomByType[r.Type]; ok {
		return z
	}
	// Between a town and a suburb: close enough to read the streets around a
	// place, far enough not to have overshot a small one.
	return 11
}
