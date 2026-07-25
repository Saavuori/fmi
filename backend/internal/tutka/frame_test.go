package tutka

import (
	"math"
	"testing"
	"time"
)

// testFrame builds a 2x2 reflectivity frame exercising all four cell meanings.
func testFrame() *Frame {
	g := NewGrid([4]float64{2_000_000, 8_250_000, 2_004_000, 8_254_000}, 2000) // 2x2
	return &Frame{
		Product: "dbz",
		Time:    time.Date(2026, 7, 25, 13, 55, 0, 0, time.UTC),
		Grid:    g,
		Bits:    8,
		NoData:  255,
		Gain:    0.5,
		Offset:  -32,
		// row 0 (north): no-echo, a real 20.5 dBZ
		// row 1 (south): no-coverage, a real -31.5 dBZ
		Values: []uint16{0, 105, 255, 1},
	}
}

// The distinction this asserts is the whole reason SampleState exists: an empty
// pixel because the radar saw nothing is not the same claim as an empty pixel
// because the radar cannot see there, and only the first is evidence of dry
// weather.
func TestValueAtPixelDistinguishesNoEchoFromNoCoverage(t *testing.T) {
	f := testFrame()
	cases := []struct {
		px, py    int
		wantValue float64
		wantState SampleState
	}{
		{0, 0, 0, StateNoEcho},
		{1, 0, 20.5, StateMeasured}, // 105*0.5 - 32
		{0, 1, 0, StateNoCoverage},
		{1, 1, -31.5, StateMeasured}, // 1*0.5 - 32, the lowest real value
		{5, 5, 0, StateOutsideGrid},
	}
	for _, c := range cases {
		v, state := f.ValueAtPixel(c.px, c.py)
		if state != c.wantState {
			t.Errorf("pixel (%d,%d) state = %q, want %q", c.px, c.py, state, c.wantState)
		}
		if state == StateMeasured && v != c.wantValue {
			t.Errorf("pixel (%d,%d) value = %v, want %v", c.px, c.py, v, c.wantValue)
		}
	}
}

// A 16-bit rain product must not treat 255 as anything special — that is only the
// 8-bit sentinel, and confusing them would blank out 2.55 mm/h readings.
func TestNoDataIsDepthSpecific(t *testing.T) {
	dbz, _ := ProductByID("dbz")
	rr, _ := ProductByID("rr")
	if got := dbz.NoData(); got != 255 {
		t.Errorf("dbz NoData() = %d, want 255", got)
	}
	if got := rr.NoData(); got != 65535 {
		t.Errorf("rr NoData() = %d, want 65535", got)
	}

	rain := &Frame{
		Grid:   NewGrid([4]float64{0, 0, 2000, 2000}, 2000),
		Bits:   16,
		NoData: 65535,
		Gain:   0.01,
		Values: []uint16{255},
	}
	v, state := rain.ValueAtPixel(0, 0)
	if state != StateMeasured {
		t.Fatalf("raw 255 in a 16-bit frame gave state %q, want measured", state)
	}
	// Compared with a tolerance because 255*0.01 is not exact in binary floating
	// point; the API rounds for presentation, the maths deliberately does not.
	if math.Abs(v-2.55) > 1e-9 {
		t.Errorf("raw 255 at gain 0.01 = %v mm/h, want 2.55", v)
	}
}

func TestValueAtUsesGeography(t *testing.T) {
	f := testFrame()
	// The northern row is rows 0; ask for a point in the north-east cell.
	lon, lat := f.Grid.PixelCentre(1, 0)
	v, state := f.ValueAt(lon, lat)
	if state != StateMeasured || v != 20.5 {
		t.Errorf("ValueAt(north-east cell) = (%v, %q), want (20.5, measured)", v, state)
	}
}

// The archive round-trip must be lossless at both depths: it is the only copy of
// the raw values, so a lossy path would quietly corrupt the point readout.
func TestEncodeDecodeFrameRoundTrip(t *testing.T) {
	for _, bits := range []int{8, 16} {
		f := testFrame()
		f.Bits = bits
		f.NoData = uint16(1<<bits - 1)
		if bits == 16 {
			f.Gain, f.Offset = 0.01, 0
			f.Values = []uint16{0, 4003, 65535, 12345}
		}

		body, err := encodeFrame(f)
		if err != nil {
			t.Fatalf("%d-bit encodeFrame: %v", bits, err)
		}
		meta := productMeta{Grid: f.Grid, Bits: f.Bits, Gain: f.Gain, Offset: f.Offset}
		got, err := decodeFrame(body, f.Product, f.Time, meta)
		if err != nil {
			t.Fatalf("%d-bit decodeFrame: %v", bits, err)
		}
		for i, want := range f.Values {
			if got.Values[i] != want {
				t.Errorf("%d-bit round-trip: cell %d = %d, want %d", bits, i, got.Values[i], want)
			}
		}
		if got.NoData != f.NoData || got.Gain != f.Gain {
			t.Errorf("%d-bit round-trip lost scaling: NoData=%d gain=%v", bits, got.NoData, got.Gain)
		}
	}
}
