package cache

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Place search writes one key per distinct query a visitor types, so with Redis
// down the in-memory fallback's key set is visitor-driven, not the small fixed
// set the modes use. Expired entries have to be reclaimed, or every query ever
// searched stays in memory for the life of the process.
func TestMemoryCacheReclaimsExpiredEntries(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryCache()

	for i := 0; i < 100; i++ {
		if err := m.SetValue(ctx, fmt.Sprintf("fmi:places:q%d", i), []byte("[]"), time.Nanosecond); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(time.Millisecond)

	// Pretend the last sweep was long enough ago that the next write runs one.
	m.lastSweep = time.Now().Add(-2 * sweepInterval)
	if err := m.SetValue(ctx, "fmi:salama:strikes", []byte("{}"), time.Hour); err != nil {
		t.Fatal(err)
	}

	if n := len(m.values); n != 1 {
		t.Errorf("%d entries held after a sweep, want only the live one", n)
	}
	if v, _ := m.GetValue(ctx, "fmi:salama:strikes"); v == nil {
		t.Error("the sweep dropped a live entry")
	}
}
