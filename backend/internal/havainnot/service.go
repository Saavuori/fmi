package havainnot

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"fmi/internal/core/cache"
	"fmi/internal/core/server"
	"fmi/internal/core/upstream"
)

const (
	// Stations report every ten minutes, so polling every five keeps the map
	// within one reporting cycle of live without wasting requests.
	pollInterval  = 5 * time.Minute
	retryInterval = 60 * time.Second

	// How much history each poll asks for. This is also exactly the series the
	// detail panel draws, so there is one number rather than two that can drift.
	seriesWindow = 3 * time.Hour
)

// Service is the havainnot (observations) mode: it polls FMI's weather-station
// observations across Finland and serves the latest readings plus a few hours of
// history per station. It implements server.Mode.
type Service struct {
	client   *upstream.Client
	store    *Store
	handlers *Handlers

	lastPoll atomic.Int64
	stations atomic.Int64
}

func NewService(liveCache cache.Cache) *Service {
	store := NewStore(liveCache)
	return &Service{
		client:   upstream.NewClient(wfsBase),
		store:    store,
		handlers: NewHandlers(store),
	}
}

// Start launches the polling loop, which stops when ctx is cancelled.
func (s *Service) Start(ctx context.Context) error {
	log.Println("Havainnot: starting FMI weather observation polling...")
	go s.poll(ctx)
	return nil
}

// Stop is a no-op: ctx cancellation from main is the only shutdown channel a poll
// mode needs.
func (s *Service) Stop() {}

func (s *Service) Name() string { return "havainnot" }

// Register mounts the havainnot routes under /api/havainnot/.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/havainnot/stations", s.handlers.Stations)
	mux.HandleFunc("GET /api/havainnot/station/{fmisid}", s.handlers.Station)
}

// sleepCtx sleeps for d or until ctx is cancelled, reporting whether to keep
// running.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func (s *Service) poll(ctx context.Context) {
	logged := false
	for {
		now := time.Now()
		details, err := fetchObservations(ctx, s.client, now.Add(-seriesWindow), now)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Havainnot: error fetching observations: %v", err)
			if !sleepCtx(ctx, retryInterval) {
				return
			}
			continue
		}

		s.store.Set(ctx, details, now)
		s.lastPoll.Store(now.Unix())
		s.stations.Store(int64(len(details)))
		if !logged {
			log.Printf("Havainnot: cached %d stations", len(details))
			logged = true
		}

		if !sleepCtx(ctx, pollInterval) {
			return
		}
	}
}

// Health implements server.Mode.
func (s *Service) Health(ctx context.Context) server.ModeHealth {
	last := s.lastPoll.Load()
	age := int64(-1)
	if last > 0 {
		age = time.Now().Unix() - last
	}

	status := "healthy"
	// Two poll intervals of silence is a real outage rather than a slow cycle.
	if last == 0 || age > 2*int64(pollInterval.Seconds()) {
		status = "degraded"
	}

	return server.ModeHealth{
		Status: status,
		Details: map[string]any{
			"stations":     s.stations.Load(),
			"poll_age_sec": age, // -1 until the first successful poll
		},
	}
}
