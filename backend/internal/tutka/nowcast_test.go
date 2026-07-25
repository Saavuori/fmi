package tutka

import (
	"math"
	"testing"
)

// motionTestGrid is big enough that the coarsening and search have room to work
// while keeping the test fast.
func motionTestGrid() Grid {
	return NewGrid([4]float64{2_000_000, 8_250_000, 2_000_000 + 400*2000, 8_250_000 + 400*2000}, 2000)
}

// blobFrame paints a solid rain blob centred on (cx, cy) into an otherwise
// no-echo frame.
func blobFrame(g Grid, cx, cy, radius int) *Frame {
	f := &Frame{
		Product: "dbz",
		Grid:    g,
		Bits:    8,
		NoData:  255,
		Gain:    0.5,
		Offset:  -32,
		Values:  make([]uint16, g.Width*g.Height),
	}
	// raw 120 -> 28 dBZ, comfortably above the 8 dBZ matching threshold.
	for y := cy - radius; y <= cy+radius; y++ {
		if y < 0 || y >= g.Height {
			continue
		}
		for x := cx - radius; x <= cx+radius; x++ {
			if x < 0 || x >= g.Width {
				continue
			}
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= radius*radius {
				f.Values[y*g.Width+x] = 120
			}
		}
	}
	return f
}

// The whole point of the estimator is recovering a known translation, so the test
// plants one and checks it comes back.
func TestEstimateMotionRecoversAKnownShift(t *testing.T) {
	g := motionTestGrid()
	const dt = 300.0 // one five-minute step

	cases := []struct {
		name           string
		shiftX, shiftY int
	}{
		{"eastward", 24, 0},
		{"southward", 0, 20},
		{"north-east", 16, -16},
		{"stationary", 0, 0},
	}

	for _, c := range cases {
		from := blobFrame(g, 200, 200, 22)
		to := blobFrame(g, 200+c.shiftX, 200+c.shiftY, 22)

		motion := EstimateMotion(from, to, dt)
		if !motion.Valid {
			t.Errorf("%s: motion should be valid", c.name)
			continue
		}

		// The search runs on a 4x-coarsened grid, so the recovered offset is
		// quantised to 4 pixels.
		wantPx := float64(c.shiftX) / dt
		wantPy := float64(c.shiftY) / dt
		tolerance := float64(nowcastCoarsen) / dt
		if math.Abs(motion.PxPerSec-wantPx) > tolerance {
			t.Errorf("%s: px/s = %v, want ~%v", c.name, motion.PxPerSec, wantPx)
		}
		if math.Abs(motion.PyPerSec-wantPy) > tolerance {
			t.Errorf("%s: py/s = %v, want ~%v", c.name, motion.PyPerSec, wantPy)
		}
		// A clean synthetic translation should match almost perfectly.
		if motion.Confidence < 0.9 {
			t.Errorf("%s: confidence %v is low for an exact translation", c.name, motion.Confidence)
		}
	}
}

func TestEstimateMotionBearing(t *testing.T) {
	g := motionTestGrid()
	cases := []struct {
		name           string
		shiftX, shiftY int
		wantBearing    float64
	}{
		// Screen y grows southward, so a negative dy is northward motion.
		{"due north", 0, -20, 0},
		{"due east", 20, 0, 90},
		{"due south", 0, 20, 180},
		{"due west", -20, 0, 270},
	}
	for _, c := range cases {
		motion := EstimateMotion(blobFrame(g, 200, 200, 22), blobFrame(g, 200+c.shiftX, 200+c.shiftY, 22), 300)
		if !motion.Valid {
			t.Errorf("%s: motion should be valid", c.name)
			continue
		}
		diff := math.Abs(motion.Bearing - c.wantBearing)
		if diff > 180 {
			diff = 360 - diff
		}
		if diff > 15 {
			t.Errorf("%s: bearing = %v, want ~%v", c.name, motion.Bearing, c.wantBearing)
		}
	}
}

// A dry map has no pattern to track. Reporting a confident zero there would be
// worse than reporting nothing, because the UI would draw a forecast from it.
func TestEstimateMotionRefusesADryField(t *testing.T) {
	g := motionTestGrid()
	empty := &Frame{Grid: g, Bits: 8, NoData: 255, Gain: 0.5, Offset: -32, Values: make([]uint16, g.Width*g.Height)}
	blob := blobFrame(g, 200, 200, 22)

	if m := EstimateMotion(empty, empty, 300); m.Valid {
		t.Error("two empty frames should not yield a motion estimate")
	}
	if m := EstimateMotion(empty, blob, 300); m.Valid {
		t.Error("appearing-from-nothing should not yield a motion estimate")
	}
	// A speck far below the coverage threshold must not count as a pattern.
	speck := blobFrame(g, 200, 200, 1)
	if m := EstimateMotion(speck, speck, 300); m.Valid {
		t.Error("a single-cell speck should not meet the coverage floor")
	}
}

func TestEstimateMotionRejectsBadInput(t *testing.T) {
	g := motionTestGrid()
	blob := blobFrame(g, 200, 200, 22)
	if m := EstimateMotion(nil, blob, 300); m.Valid {
		t.Error("a nil frame should not yield a motion estimate")
	}
	if m := EstimateMotion(blob, blob, 0); m.Valid {
		t.Error("a zero interval should not yield a motion estimate")
	}
	// Mismatched grids would index out of range if not rejected.
	other := NewGrid([4]float64{0, 0, 100 * 2000, 100 * 2000}, 2000)
	if m := EstimateMotion(blob, blobFrame(other, 50, 50, 10), 300); m.Valid {
		t.Error("mismatched grids should not yield a motion estimate")
	}
}

// The extrapolation has to look upwind, not downwind. Getting the sign wrong
// would confidently promise rain that is already past.
func TestExtrapolatePointLooksUpwind(t *testing.T) {
	g := motionTestGrid()
	// Rain sits west of the probe and moves east at 4 px per 5 minutes... using a
	// larger shift so it crosses the probe within the horizon.
	const dt = 300.0
	shift := 30
	from := blobFrame(g, 150, 200, 20)
	to := blobFrame(g, 150+shift, 200, 20)

	motion := EstimateMotion(from, to, dt)
	if !motion.Valid {
		t.Fatal("motion should be valid")
	}
	if motion.PxPerSec <= 0 {
		t.Fatalf("rain moves east, so px/s should be positive, got %v", motion.PxPerSec)
	}

	// Probe well to the east of the blob's current position: the rain is upwind
	// (to the west) and heading towards us, so it must arrive.
	probeLon, probeLat := g.PixelCentre(150+shift+60, 200)
	steps := ExtrapolatePoint(to, motion, probeLon, probeLat)
	if len(steps) == 0 {
		t.Fatal("expected extrapolation steps")
	}
	arrival := ArrivalMinutes(steps, arrivalThresholdDBZ)
	if arrival <= 0 {
		t.Errorf("rain approaching from upwind should arrive within the horizon, got %d", arrival)
	}

	// A probe on the far side — already passed by — must not be told rain is coming.
	behindLon, behindLat := g.PixelCentre(150-80, 200)
	behind := ExtrapolatePoint(to, motion, behindLon, behindLat)
	if got := ArrivalMinutes(behind, arrivalThresholdDBZ); got != -1 {
		t.Errorf("rain moving away should not be reported as arriving, got %d", got)
	}
}

func TestExtrapolatePointHorizonAndSteps(t *testing.T) {
	g := motionTestGrid()
	to := blobFrame(g, 200, 200, 20)
	motion := EstimateMotion(blobFrame(g, 180, 200, 20), to, 300)
	steps := ExtrapolatePoint(to, motion, 25.0, 63.0)

	want := nowcastHorizonMinutes / nowcastStepMinutes
	if len(steps) != want {
		t.Fatalf("got %d steps, want %d", len(steps), want)
	}
	for i, s := range steps {
		if s.MinutesAhead != (i+1)*nowcastStepMinutes {
			t.Errorf("step %d is %d minutes ahead, want %d", i, s.MinutesAhead, (i+1)*nowcastStepMinutes)
		}
	}

	// An invalid motion must produce nothing rather than a flat line of the
	// current value dressed up as a forecast.
	if got := ExtrapolatePoint(to, Motion{}, 25.0, 63.0); got != nil {
		t.Error("an invalid motion should produce no extrapolation")
	}
}

func TestArrivalMinutesIgnoresUnmeasuredCells(t *testing.T) {
	steps := []Extrapolation{
		{MinutesAhead: 5, State: StateNoCoverage},
		{MinutesAhead: 10, State: StateNoEcho},
		{MinutesAhead: 15, State: StateMeasured, Value: ptr(9.0)},  // below threshold
		{MinutesAhead: 20, State: StateMeasured, Value: ptr(22.0)}, // real rain
	}
	if got := ArrivalMinutes(steps, arrivalThresholdDBZ); got != 20 {
		t.Errorf("ArrivalMinutes = %d, want 20", got)
	}
	if got := ArrivalMinutes(steps[:3], arrivalThresholdDBZ); got != -1 {
		t.Errorf("ArrivalMinutes with nothing above threshold = %d, want -1", got)
	}
}

func ptr(v float64) *float64 { return &v }
