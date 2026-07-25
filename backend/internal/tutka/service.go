package tutka

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"fmi/internal/core/cache"
	"fmi/internal/core/config"
	"fmi/internal/core/server"
	"fmi/internal/core/upstream"
)

const (
	// FMI publishes a new composite within about five minutes. Asking every
	// minute catches it promptly and costs one small WFS query — the expensive
	// GetMap fetch only happens for a timestamp we do not already hold.
	liveInterval   = 60 * time.Second
	hourlyInterval = 10 * time.Minute
	retryInterval  = 30 * time.Second
	pruneInterval  = 6 * time.Hour

	// How far back each live poll looks for frames it might have missed. Wide
	// enough to heal a short outage without a backfill pass.
	liveLookback   = 30 * time.Minute
	hourlyLookback = 6 * time.Hour

	// Pace of the boot-time backfill. One fetch per 1.5 s is about 40 requests a
	// minute against a documented ceiling of 600 per five minutes, so a full
	// seven-day catch-up stays comfortably polite while it runs.
	backfillPace = 1500 * time.Millisecond

	// Backfill asks in day-long slices so a single WFS response stays small.
	backfillChunk = 24 * time.Hour
)

// Service is the tutka (radar) mode: it downloads FMI's radar composites, keeps
// them as an archive of raw-value rasters, and serves coloured frames from that
// archive. It implements server.Mode.
//
// The shape follows the poll-style modes of the Fintraffic sibling: pollers write
// to the Store, handlers only read from the Store, and nothing a visitor does
// triggers an upstream request. Here that invariant is not merely tidy — FMI's
// open-data terms require it. Their manual is explicit that pointing an
// application at their WMS is not allowed, and that radar data must be downloaded
// and then served by the application itself.
type Service struct {
	client   *upstream.Client
	store    *Store
	handlers *Handlers
	grid     Grid
	products []Product

	// Health bookkeeping. The maps are built once in NewService and never
	// written again, so the atomics inside them need no further locking.
	lastPoll    map[string]*atomic.Int64
	lastErr     map[string]*atomic.Pointer[string]
	backfilling atomic.Bool
	backfilled  atomic.Int64
}

// NewService wires the mode. Retention comes from config, falling back to the
// per-product defaults, so an operator can shrink the archive on a tight disk
// without a rebuild.
func NewService(cfg *config.Config, liveCache cache.Cache) *Service {
	grid := NewGrid(cfg.GridBBox, cfg.GridResolution)

	products := make([]Product, 0, len(Products))
	for _, p := range Products {
		p.RetainHours = defaultRetainHours[p.ID]
		if h, ok := cfg.RetentionHours[p.ID]; ok {
			p.RetainHours = h
		}
		products = append(products, p)
	}

	store := NewStore(cfg.FramesDir, grid, liveCache)

	s := &Service{
		client:   upstream.NewClient(wmsBase),
		store:    store,
		handlers: NewHandlers(store, grid, products),
		grid:     grid,
		products: products,
		lastPoll: make(map[string]*atomic.Int64, len(products)),
		lastErr:  make(map[string]*atomic.Pointer[string], len(products)),
	}
	for _, p := range products {
		s.lastPoll[p.ID] = new(atomic.Int64)
		s.lastErr[p.ID] = new(atomic.Pointer[string])
	}
	return s
}

// Start scans the archive, then launches one poller per product plus the prune
// and backfill loops. They all stop when ctx is cancelled.
func (s *Service) Start(ctx context.Context) error {
	s.store.Open()
	s.logBudget()

	log.Printf("Tutka: grid %dx%d at %.0f m/px, archive=%v",
		s.grid.Width, s.grid.Height, s.grid.Resolution, s.store.ArchiveEnabled())

	for _, p := range s.products {
		go s.pollProduct(ctx, p)
	}
	go s.pruneLoop(ctx)
	// One backfill goroutine for every product, so the catch-up rate is bounded
	// globally rather than per product. It starts late to let the live pollers
	// populate the recent window first — a visitor arriving in the first minute
	// should get current weather, not history.
	go func() {
		if !sleepCtx(ctx, 2*time.Minute) {
			return
		}
		s.backfillAll(ctx)
	}()
	return nil
}

// Stop is a no-op: ctx cancellation from main is the only shutdown channel the
// poll loops need, and the archive is plain files with no handle to close.
func (s *Service) Stop() {}

func (s *Service) Name() string { return "tutka" }

// Register mounts the tutka routes under /api/tutka/.
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tutka/products", s.handlers.ProductList)
	mux.HandleFunc("GET /api/tutka/frames", s.handlers.Frames)
	mux.HandleFunc("GET /api/tutka/frame/{product}/{stamp}", s.handlers.FrameImage)
	mux.HandleFunc("GET /api/tutka/legend/{product}", s.handlers.Legend)
	mux.HandleFunc("GET /api/tutka/point", s.handlers.Point)
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

// pollProduct keeps one product's recent window current.
func (s *Service) pollProduct(ctx context.Context, p Product) {
	interval, lookback := liveInterval, liveLookback
	if p.StepMinutes >= 60 {
		interval, lookback = hourlyInterval, hourlyLookback
	}

	logged := false
	for {
		added, err := s.sync(ctx, p, time.Now().Add(-lookback), time.Now())
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Tutka: error syncing %s: %v", p.ID, err)
			msg := err.Error()
			s.lastErr[p.ID].Store(&msg)
			if !sleepCtx(ctx, retryInterval) {
				return
			}
			continue
		}
		s.lastPoll[p.ID].Store(time.Now().Unix())
		s.lastErr[p.ID].Store(nil)
		if added > 0 || !logged {
			log.Printf("Tutka: %s has %d frames archived (+%d)", p.ID, s.store.Count(p.ID), added)
			logged = true
		}
		s.store.publishIndex(ctx, p.ID)
		if !sleepCtx(ctx, interval) {
			return
		}
	}
}

// sync discovers the frames of a product in a window and fetches the ones not
// already archived. It returns how many were added.
func (s *Service) sync(ctx context.Context, p Product, from, to time.Time) (int, error) {
	metas, err := discoverFrames(ctx, s.client, p, from, to)
	if err != nil {
		return 0, err
	}

	added := 0
	for _, meta := range metas {
		if ctx.Err() != nil {
			return added, ctx.Err()
		}
		if s.store.Has(p.ID, meta.Time) {
			continue
		}
		frame, err := fetchFrame(ctx, s.client, p, s.grid, meta)
		if err != nil {
			// One bad frame should not abandon the rest of the window: FMI
			// occasionally lists a timestamp whose raster is not servable yet.
			log.Printf("Tutka: skipping %s %s: %v", p.ID, meta.Time.Format(time.RFC3339), err)
			continue
		}
		if err := s.store.Put(frame); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// backfillAll walks every product back to its retention horizon, filling gaps at
// a deliberately slow pace. Products are done in table order — reflectivity
// first, since that is what the app opens on.
func (s *Service) backfillAll(ctx context.Context) {
	if !s.store.ArchiveEnabled() {
		return
	}
	s.backfilling.Store(true)
	defer s.backfilling.Store(false)

	for _, p := range s.products {
		if ctx.Err() != nil {
			return
		}
		s.backfillProduct(ctx, p)
	}
	log.Printf("Tutka: backfill complete, %d frames recovered", s.backfilled.Load())
}

// backfillOrder is the sequence a chunk's frames should be fetched in:
// newest first.
//
// This matters more than it looks. discoverFrames returns frames oldest-first,
// and fetching them in that order fills a 24-hour chunk *forward* from its far
// end — so for the first hour of a deploy the archive holds yesterday afternoon
// plus the live poller's last half hour, with a ten-hour hole between them. The
// timeline looks broken precisely in the region a visitor is most likely to
// scrub. Fetching newest-first keeps the archive contiguous backwards from now,
// so it only ever reaches further back.
func backfillOrder(metas []frameMeta) []frameMeta {
	out := make([]frameMeta, len(metas))
	for i, meta := range metas {
		out[len(metas)-1-i] = meta
	}
	return out
}

// backfillProduct fills one product's history, newest first so the archive stays
// contiguous backwards from the present rather than growing a hole in the middle.
func (s *Service) backfillProduct(ctx context.Context, p Product) {
	horizon := time.Duration(p.RetainHours) * time.Hour
	if horizon > upstreamHistory {
		// Asking for more than FMI keeps just wastes requests on empty windows.
		horizon = upstreamHistory
	}
	deadline := time.Now().Add(-horizon)

	for to := time.Now(); to.After(deadline); to = to.Add(-backfillChunk) {
		if ctx.Err() != nil {
			return
		}
		from := to.Add(-backfillChunk)
		if from.Before(deadline) {
			from = deadline
		}

		metas, err := discoverFrames(ctx, s.client, p, from, to)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Tutka: backfill discovery for %s failed: %v", p.ID, err)
			if !sleepCtx(ctx, retryInterval) {
				return
			}
			continue
		}

		for _, meta := range backfillOrder(metas) {
			if s.store.Has(p.ID, meta.Time) {
				continue
			}
			if !sleepCtx(ctx, backfillPace) {
				return
			}
			frame, err := fetchFrame(ctx, s.client, p, s.grid, meta)
			if err != nil {
				log.Printf("Tutka: backfill skipping %s %s: %v", p.ID, meta.Time.Format(time.RFC3339), err)
				continue
			}
			if err := s.store.Put(frame); err != nil {
				log.Printf("Tutka: backfill write failed for %s: %v", p.ID, err)
				continue
			}
			if n := s.backfilled.Add(1); n%100 == 0 {
				log.Printf("Tutka: backfill progress %d frames, %s at %s, archive %.0f MB",
					n, p.ID, meta.Time.Format("2006-01-02 15:04"), float64(s.store.DiskBytes())/(1<<20))
			}
		}
	}
	s.store.publishIndex(ctx, p.ID)
}

// pruneLoop drops frames past each product's retention and reports what the
// archive now costs.
func (s *Service) pruneLoop(ctx context.Context) {
	for {
		if !sleepCtx(ctx, pruneInterval) {
			return
		}
		total := 0
		for _, p := range s.products {
			total += s.store.Prune(p.ID, time.Now().Add(-time.Duration(p.RetainHours)*time.Hour))
		}
		if total > 0 {
			log.Printf("Tutka: pruned %d frames, archive now %.0f MB", total, float64(s.store.DiskBytes())/(1<<20))
		}
	}
}

// logBudget states the archive's projected size at startup. The host this runs on
// has a root volume that sits near full, so the projection is worth printing where
// an operator will see it rather than leaving them to discover it.
//
// Where frames already exist the projection is measured from them, because a
// guess at PNG's compression ratio is not trustworthy: it swings with the weather
// (a rainy frame has far more detail to encode than a dry one) and by product
// (the accumulations are several times heavier than reflectivity). Only a product
// with nothing on disk yet falls back to an estimate.
func (s *Service) logBudget() {
	if !s.store.ArchiveEnabled() {
		return
	}

	var projected float64
	measured := 0
	for _, p := range s.products {
		frames := float64(p.RetainHours*60) / float64(p.StepMinutes)

		perFrame, ok := s.store.MeanFrameBytes(p.ID)
		if ok {
			measured++
		} else {
			// Raw is one byte per pixel at 8-bit, two at 16-bit; observed PNG
			// output lands around a fifth of that on a typical field.
			perFrame = float64(s.grid.Width*s.grid.Height) * float64(p.Bits) / 8 * 0.2
		}
		projected += perFrame * frames
	}

	note := "estimated"
	if measured == len(s.products) {
		note = "measured"
	} else if measured > 0 {
		note = "part measured"
	}
	log.Printf("Tutka: archive at %s, currently %.0f MB, projected ~%.0f MB when full (%s)",
		s.store.dir, float64(s.store.DiskBytes())/(1<<20), projected/(1<<20), note)
}

// Health implements server.Mode. The mode is healthy once the primary product has
// synced recently; before the first success it reports degraded, which the
// frontend shows as a loading state rather than an error.
func (s *Service) Health(ctx context.Context) server.ModeHealth {
	primary := s.products[0]
	last := s.lastPoll[primary.ID].Load()
	age := int64(-1)
	if last > 0 {
		age = time.Now().Unix() - last
	}

	status := "healthy"
	// Five poll intervals of silence is a real outage rather than a slow cycle.
	if last == 0 || age > 5*int64(liveInterval.Seconds()) {
		status = "degraded"
	}

	perProduct := make(map[string]any, len(s.products))
	for _, p := range s.products {
		entry := map[string]any{
			"frames":       s.store.Count(p.ID),
			"retain_hours": p.RetainHours,
		}
		if lp := s.lastPoll[p.ID].Load(); lp > 0 {
			entry["poll_age_sec"] = time.Now().Unix() - lp
		} else {
			entry["poll_age_sec"] = -1
		}
		if msg := s.lastErr[p.ID].Load(); msg != nil {
			entry["last_error"] = *msg
		}
		if latest, ok := s.store.Latest(p.ID); ok {
			entry["latest"] = latest.Format(time.RFC3339)
		}
		perProduct[p.ID] = entry
	}

	return server.ModeHealth{
		Status: status,
		Details: map[string]any{
			"archive_enabled":      s.store.ArchiveEnabled(),
			"frames_on_disk":       s.totalFrames(),
			"disk_bytes":           s.store.DiskBytes(),
			"backfill_running":     s.backfilling.Load(),
			"backfilled_frames":    s.backfilled.Load(),
			"primary_poll_age_sec": age, // -1 until the first successful sync
			"grid":                 map[string]any{"width": s.grid.Width, "height": s.grid.Height, "resolution_m": s.grid.Resolution},
			"products":             perProduct,
		},
	}
}

func (s *Service) totalFrames() int {
	n := 0
	for _, p := range s.products {
		n += s.store.Count(p.ID)
	}
	return n
}
