package salama

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
	// Strikes are worth having promptly — a thunderstorm's arrival is the whole
	// point of the layer — and the feed is tiny.
	pollInterval  = 60 * time.Second
	retryInterval = 30 * time.Second

	// Each poll asks for a little more than the interval so a slow cycle cannot
	// open a gap, and the Store de-duplicates the overlap.
	pollLookback = 5 * time.Minute

	// How long a strike stays on the map. Two hours shows a storm's whole track
	// across southern Finland without the display becoming a solid smear.
	retainWindow = 2 * time.Hour

	// Prime the map on boot so the first visitor sees the recent past rather than
	// an empty country.
	initialLookback = retainWindow
)

// Service is the salama (lightning) mode: it polls FMI's lightning observations
// and serves the strikes of the last couple of hours. It implements server.Mode.
type Service struct {
	client   *upstream.Client
	store    *Store
	handlers *Handlers

	lastPoll atomic.Int64
	strikes  atomic.Int64
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
	log.Println("Salama: starting FMI lightning polling...")
	go s.poll(ctx)
	return nil
}

// Stop is a no-op: ctx cancellation from main is the only shutdown channel a poll
// mode needs.
func (s *Service) Stop() {}

func (s *Service) Name() string { return "salama" }

// Register mounts the salama routes under /api/salama/.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/salama/strikes", s.handlers.Strikes)
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
	// The first pass reaches back over the whole retain window; later passes only
	// need the sliver since the previous one.
	lookback := initialLookback
	logged := false

	for {
		now := time.Now()
		strikes, err := fetchStrikes(ctx, s.client, now.Add(-lookback), now)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Salama: error fetching strikes: %v", err)
			if !sleepCtx(ctx, retryInterval) {
				return
			}
			continue
		}

		count := s.store.Merge(ctx, strikes, retainWindow, now)
		s.lastPoll.Store(now.Unix())
		s.strikes.Store(int64(count))
		if !logged {
			log.Printf("Salama: retaining %d strikes from the last %d min", count, int(retainWindow/time.Minute))
			logged = true
		}

		lookback = pollLookback
		if !sleepCtx(ctx, pollInterval) {
			return
		}
	}
}

// Health implements server.Mode. Note that zero strikes is a perfectly healthy
// answer — Finland is not usually being struck by lightning — so health tracks
// whether the poll is succeeding, never whether it found anything.
func (s *Service) Health(ctx context.Context) server.ModeHealth {
	last := s.lastPoll.Load()
	age := int64(-1)
	if last > 0 {
		age = time.Now().Unix() - last
	}

	status := "healthy"
	if last == 0 || age > 5*int64(pollInterval.Seconds()) {
		status = "degraded"
	}

	return server.ModeHealth{
		Status: status,
		Details: map[string]any{
			"strikes_retained": s.strikes.Load(),
			"window_minutes":   int(retainWindow / time.Minute),
			"poll_age_sec":     age, // -1 until the first successful poll
		},
	}
}
