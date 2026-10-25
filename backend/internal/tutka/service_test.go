package tutka

import (
	"context"
	"errors"
	"testing"
	"time"

	"fmi/internal/core/cache"
	"fmi/internal/core/config"
)

// The archive has to stay contiguous backwards from the present while it fills.
// discoverFrames hands back frames oldest-first, and fetching them in that order
// filled each 24-hour chunk forward from its far end — leaving a ten-hour hole
// between yesterday's backfilled frames and the live poller's recent window,
// exactly where a visitor would scrub. This is the regression test for that.
func TestBackfillOrderIsNewestFirst(t *testing.T) {
	base := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	metas := []frameMeta{
		{Time: base},
		{Time: base.Add(5 * time.Minute)},
		{Time: base.Add(10 * time.Minute)},
		{Time: base.Add(15 * time.Minute)},
	}

	ordered := backfillOrder(metas)

	if len(ordered) != len(metas) {
		t.Fatalf("got %d frames, want %d — the backfill must not drop any", len(ordered), len(metas))
	}
	for i := 1; i < len(ordered); i++ {
		if !ordered[i].Time.Before(ordered[i-1].Time) {
			t.Errorf("frame %d (%s) should be older than frame %d (%s)",
				i, ordered[i].Time.Format(time.RFC3339),
				i-1, ordered[i-1].Time.Format(time.RFC3339))
		}
	}
	if !ordered[0].Time.Equal(base.Add(15 * time.Minute)) {
		t.Errorf("first fetch is %s, want the newest frame", ordered[0].Time.Format(time.RFC3339))
	}

	// The input must not be reordered in place: the caller's slice is also what
	// the "already have it" check iterates over elsewhere.
	if !metas[0].Time.Equal(base) {
		t.Error("backfillOrder mutated its input")
	}
}

func TestBackfillOrderHandlesEmptyAndSingle(t *testing.T) {
	if got := backfillOrder(nil); len(got) != 0 {
		t.Errorf("nil input gave %d frames, want 0", len(got))
	}
	one := []frameMeta{{Time: time.Now()}}
	if got := backfillOrder(one); len(got) != 1 || !got[0].Time.Equal(one[0].Time) {
		t.Error("a single frame should pass through unchanged")
	}
}

// A discovery request that fails during the boot backfill has to be retried for
// the same slice of history. It used to fall through to the loop's post
// statement, which moved on to the previous day: one transient FMI error left a
// 24-hour hole in the archive until the next restart.
func TestBackfillRetriesAFailedChunk(t *testing.T) {
	cfg := &config.Config{
		FramesDir:      t.TempDir(),
		GridBBox:       [4]float64{0, 0, 4000, 4000},
		GridResolution: 2000,
	}
	s := NewService(cfg, cache.NewMemoryCache())
	s.retryWait = time.Millisecond

	type window struct{ from, to time.Time }
	var calls []window
	s.discover = func(_ context.Context, _ Product, from, to time.Time) ([]frameMeta, error) {
		calls = append(calls, window{from, to})
		if len(calls) == 1 {
			return nil, errors.New("fmi wfs exception: temporarily unavailable")
		}
		return nil, nil
	}

	p := s.products[0]
	p.RetainHours = 48
	s.backfillProduct(context.Background(), p)

	if len(calls) != 3 {
		t.Fatalf("got %d discovery calls, want 3 (failed day, its retry, the day before)", len(calls))
	}
	if !calls[1].from.Equal(calls[0].from) || !calls[1].to.Equal(calls[0].to) {
		t.Errorf("retry asked for %v..%v, want the failed window %v..%v",
			calls[1].from, calls[1].to, calls[0].from, calls[0].to)
	}
	if !calls[2].to.Equal(calls[0].from) {
		t.Errorf("next window ends at %v, want it to continue from %v", calls[2].to, calls[0].from)
	}
}

// A chunk that keeps failing is given up on rather than retried forever, so one
// bad day cannot stall the backfill of everything older.
func TestBackfillGivesUpOnAPersistentlyFailingChunk(t *testing.T) {
	cfg := &config.Config{
		FramesDir:      t.TempDir(),
		GridBBox:       [4]float64{0, 0, 4000, 4000},
		GridResolution: 2000,
	}
	s := NewService(cfg, cache.NewMemoryCache())
	s.retryWait = time.Millisecond

	calls := 0
	s.discover = func(context.Context, Product, time.Time, time.Time) ([]frameMeta, error) {
		calls++
		return nil, errors.New("down")
	}

	p := s.products[0]
	p.RetainHours = 48
	s.backfillProduct(context.Background(), p)

	if want := 2 * backfillAttempts; calls != want {
		t.Errorf("got %d discovery calls, want %d (%d attempts for each of two days)", calls, want, backfillAttempts)
	}
}
