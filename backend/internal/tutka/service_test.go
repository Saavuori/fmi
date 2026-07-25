package tutka

import (
	"testing"
	"time"
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
