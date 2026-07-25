package tutka

import "testing"

func TestColourAtInterpolatesAndClamps(t *testing.T) {
	pal := paletteDBZ

	// Exactly on a stop.
	if got := pal.ColourAt(15); got.R != 46 || got.G != 216 || got.B != 182 {
		t.Errorf("ColourAt(15) = %v, want the 15 dBZ stop (46,216,182)", got)
	}

	// Halfway between the 8 and 15 stops.
	mid := pal.ColourAt(11.5)
	loA, hiA := pal.Stops[1].A, pal.Stops[2].A
	if mid.A < loA || mid.A > hiA {
		t.Errorf("ColourAt(11.5) alpha = %d, want between %d and %d", mid.A, loA, hiA)
	}

	// Below the ramp clamps to the transparent anchor, so weak clutter does not
	// paint over the basemap.
	if got := pal.ColourAt(-30); got.A != 0 {
		t.Errorf("ColourAt(-30) alpha = %d, want 0", got.A)
	}
	// Above the ramp clamps to the top stop rather than wrapping to transparent,
	// which would erase the most intense cells on the map.
	top := pal.Stops[len(pal.Stops)-1]
	if got := pal.ColourAt(120); got.A != top.A || got.R != top.R {
		t.Errorf("ColourAt(120) = %v, want the top stop %v", got, top)
	}
}

// Both special values must be fully transparent, and the sentinel must be indexed
// at the frame's own depth.
func TestLUTLeavesSpecialValuesTransparent(t *testing.T) {
	for _, bits := range []int{8, 16} {
		f := &Frame{Bits: bits, NoData: uint16(1<<bits - 1), Gain: 0.5, Offset: -32}
		lut := paletteDBZ.LUT(f)

		if want := int(f.NoData) + 1; len(lut) != want {
			t.Fatalf("%d-bit LUT length = %d, want %d", bits, len(lut), want)
		}
		if lut[0].A != 0 {
			t.Errorf("%d-bit LUT: no-echo (0) alpha = %d, want 0", bits, lut[0].A)
		}
		if lut[f.NoData].A != 0 {
			t.Errorf("%d-bit LUT: no-coverage (%d) alpha = %d, want 0", bits, f.NoData, lut[f.NoData].A)
		}
		// A mid-scale cell must actually be painted, or the whole ramp is dead.
		if lut[120].A == 0 {
			t.Errorf("%d-bit LUT: raw 120 (%.1f dBZ) is transparent", bits, 120*f.Gain+f.Offset)
		}
	}
}

// Serving a mm/h ramp for a dBZ raster would mislabel every pixel and the legend
// with it, so a unit mismatch has to fall back rather than oblige.
func TestResolvePaletteRejectsUnitMismatch(t *testing.T) {
	dbz, _ := ProductByID("dbz")
	rr, _ := ProductByID("rr")

	if got := ResolvePalette(dbz, "rate"); got.ID != "dbz" {
		t.Errorf("ResolvePalette(dbz, \"rate\") = %q, want the dbz default", got.ID)
	}
	if got := ResolvePalette(dbz, "dbz-cvd"); got.ID != "dbz-cvd" {
		t.Errorf("ResolvePalette(dbz, \"dbz-cvd\") = %q, want dbz-cvd", got.ID)
	}
	if got := ResolvePalette(dbz, "nonsense"); got.ID != "dbz" {
		t.Errorf("ResolvePalette(dbz, \"nonsense\") = %q, want the dbz default", got.ID)
	}
	if got := ResolvePalette(rr, ""); got.Unit != "mm/h" {
		t.Errorf("ResolvePalette(rr, \"\") has unit %q, want mm/h", got.Unit)
	}
}

// Every product needs a default palette that exists and matches its unit,
// otherwise ResolvePalette hands back a zero-value ramp that renders nothing.
func TestEveryProductHasAMatchingDefaultPalette(t *testing.T) {
	for _, p := range Products {
		pal, ok := Palettes[p.Palette]
		if !ok {
			t.Errorf("product %s names palette %q, which is not registered", p.ID, p.Palette)
			continue
		}
		if pal.Unit != p.Unit {
			t.Errorf("product %s is in %s but its palette %q is in %s", p.ID, p.Unit, pal.ID, pal.Unit)
		}
	}
}

// The legend has to describe the pixels, so it must skip the invisible fade-in
// anchor and otherwise track the ramp.
func TestLegendSkipsTheTransparentAnchor(t *testing.T) {
	legend := paletteDBZ.Legend()
	if len(legend) != len(paletteDBZ.Stops)-1 {
		t.Fatalf("legend has %d stops, want %d", len(legend), len(paletteDBZ.Stops)-1)
	}
	if legend[0].Value != 8 {
		t.Errorf("first legend stop = %v, want 8 (4 is the transparent anchor)", legend[0].Value)
	}
	if legend[0].Color != "#5abeff" {
		t.Errorf("first legend colour = %q, want #5abeff", legend[0].Color)
	}
	// Values must ascend, or the scale reads backwards.
	for i := 1; i < len(legend); i++ {
		if legend[i].Value <= legend[i-1].Value {
			t.Errorf("legend stop %d (%v) does not exceed the previous (%v)", i, legend[i].Value, legend[i-1].Value)
		}
	}
}

func TestFormatValueDropsTrailingZeros(t *testing.T) {
	cases := map[float64]string{30: "30", 0.5: "0.5", 2.5: "2.5", 100: "100"}
	for in, want := range cases {
		if got := formatValue(in); got != want {
			t.Errorf("formatValue(%v) = %q, want %q", in, got, want)
		}
	}
}
