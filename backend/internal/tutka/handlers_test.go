package tutka

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"fmi/internal/core/cache"
)

// The nowcast's speed is the measured shift divided by the time between the two
// frames it compared. When a frame in the middle of the hour cannot be read, the
// two frames compared are ten minutes apart, not five; dividing by the nominal
// step doubled the speed and brought the arrival time forward. The handler must
// use the time of the frame it actually kept, and here that gap is too long to
// extrapolate from at all.
func TestPointNowcastUsesTheFramesItCompared(t *testing.T) {
	g, s, h, times := pointTestStore(t)

	// The middle frame is on the list but unreadable.
	if err := os.WriteFile(s.framePath("dbz", times[1]), []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.decoded.remove(frameKey("dbz", times[1]))

	res := readPoint(t, h, g)
	if len(res.Series) != 2 {
		t.Fatalf("series has %d samples, want the 2 readable frames", len(res.Series))
	}
	if res.Nowcast != nil {
		t.Errorf("nowcast built across a 10-minute gap as if it were 5: %.0f km/h", res.Nowcast.Motion.SpeedKmh)
	}
}

// With every frame readable the same data does produce a nowcast, at the speed
// of one 24-pixel step per five minutes.
func TestPointNowcastFromConsecutiveFrames(t *testing.T) {
	g, _, h, _ := pointTestStore(t)
	res := readPoint(t, h, g)
	if res.Nowcast == nil {
		t.Fatal("no nowcast from three consecutive frames")
	}
	if got := res.Nowcast.Motion.PxPerSec * 300; got < 20 || got > 28 {
		t.Errorf("motion = %.1f px per step, want ~24", got)
	}
}

// pointTestStore archives three dbz frames five minutes apart, ending now, with
// a rain blob moving 24 px east per frame.
func pointTestStore(t *testing.T) (Grid, *Store, *Handlers, []time.Time) {
	t.Helper()
	g := motionTestGrid()
	s := NewStore(t.TempDir(), g, cache.NewMemoryCache())
	h := NewHandlers(s, g, Products)

	latest := time.Now().UTC().Truncate(5 * time.Minute)
	times := []time.Time{latest.Add(-10 * time.Minute), latest.Add(-5 * time.Minute), latest}
	for i, ts := range times {
		f := blobFrame(g, 150+24*i, 200, 22)
		f.Time = ts
		if err := s.Put(f); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}
	return g, s, h, times
}

func readPoint(t *testing.T, h *Handlers, g Grid) pointResponse {
	t.Helper()
	lon, lat := g.PixelCentre(250, 200)
	rec := httptest.NewRecorder()
	h.Point(rec, httptest.NewRequest("GET", fmt.Sprintf("/api/tutka/point?lat=%f&lon=%f&product=dbz", lat, lon), nil))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var res pointResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}
