package tutka

import (
	"testing"
	"time"

	"fmi/internal/core/cache"
)

func storeTestFrame(g Grid, ts time.Time) *Frame {
	return &Frame{
		Product: "dbz",
		Time:    ts,
		Grid:    g,
		Bits:    8,
		NoData:  255,
		Gain:    0.5,
		Offset:  -32,
		Values:  make([]uint16, g.Width*g.Height),
	}
}

// With FRAMES_DIR empty (or unwritable) the mode is documented to keep serving
// what it holds in memory. It used to index nothing at all: the radar never
// showed a frame, and because Has() stayed false every poll downloaded the whole
// lookback window from FMI again.
func TestMemoryOnlyStoreIndexesWhatItHolds(t *testing.T) {
	g := NewGrid([4]float64{0, 0, 4000, 4000}, 2000)
	s := NewStore("", g, cache.NewMemoryCache())
	base := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	total := decodedCacheSize + 6
	for i := 0; i < total; i++ {
		if err := s.Put(storeTestFrame(g, base.Add(time.Duration(i)*5*time.Minute))); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	// Every frame fetched once is known, so the poller does not fetch it again.
	for i := 0; i < total; i++ {
		if ts := base.Add(time.Duration(i) * 5 * time.Minute); !s.Has("dbz", ts) {
			t.Fatalf("Has(%s) = false after Put; the poller would re-download it", ts.Format(time.RFC3339))
		}
	}

	// Only what is still held is listed, and every listed frame can be served.
	times := s.Times("dbz", time.Time{}, time.Time{})
	if len(times) != decodedCacheSize {
		t.Fatalf("Times lists %d frames, want the %d still held", len(times), decodedCacheSize)
	}
	for _, ts := range times {
		if _, err := s.Get("dbz", ts); err != nil {
			t.Errorf("listed frame %s cannot be served: %v", ts.Format(time.RFC3339), err)
		}
	}
	if latest, ok := s.Latest("dbz"); !ok || !latest.Equal(base.Add(time.Duration(total-1)*5*time.Minute)) {
		t.Errorf("Latest = %v, %v; want the newest frame", latest, ok)
	}

	// Pruning forgets fetched frames past retention, so the record stays bounded.
	s.Prune("dbz", base.Add(time.Duration(total)*5*time.Minute))
	if s.Has("dbz", base) {
		t.Error("Prune left a frame past the cutoff marked as fetched")
	}
}

// The archive path must be unchanged: every frame written is listed.
func TestArchiveStoreListsEveryFrame(t *testing.T) {
	g := NewGrid([4]float64{0, 0, 4000, 4000}, 2000)
	s := NewStore(t.TempDir(), g, cache.NewMemoryCache())
	base := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)

	total := decodedCacheSize + 6
	for i := 0; i < total; i++ {
		if err := s.Put(storeTestFrame(g, base.Add(time.Duration(i)*5*time.Minute))); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	if n := len(s.Times("dbz", time.Time{}, time.Time{})); n != total {
		t.Fatalf("Times lists %d frames, want %d", n, total)
	}
	if _, err := s.Get("dbz", base); err != nil {
		t.Errorf("an evicted frame should be read back from disk: %v", err)
	}
}
