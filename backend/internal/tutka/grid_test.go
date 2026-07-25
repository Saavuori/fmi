package tutka

import (
	"math"
	"testing"
)

// defaultGrid is the shipped configuration, so the assertions below double as a
// guard on the numbers the frontend is handed.
func defaultGrid() Grid {
	return NewGrid([4]float64{2_000_000, 8_250_000, 3_600_000, 11_200_000}, 2000)
}

func TestNewGridShapeAndSnapping(t *testing.T) {
	g := defaultGrid()
	if g.Width != 800 || g.Height != 1475 {
		t.Fatalf("grid is %dx%d, want 800x1475", g.Width, g.Height)
	}
	// The north and east edges must be a whole number of pixels from the origin,
	// or the corners handed to MapLibre disagree with the raster.
	if got := g.MinX + float64(g.Width)*g.Resolution; got != g.MaxX {
		t.Errorf("MaxX = %v, want %v (a whole number of pixels)", g.MaxX, got)
	}
	if got := g.MinY + float64(g.Height)*g.Resolution; got != g.MaxY {
		t.Errorf("MaxY = %v, want %v (a whole number of pixels)", g.MaxY, got)
	}

	// A bbox that is not an exact multiple of the resolution gets snapped
	// outward-consistently rather than silently producing rectangular pixels.
	odd := NewGrid([4]float64{0, 0, 5000, 3000}, 2000)
	if odd.Width != 3 || odd.Height != 2 {
		t.Errorf("odd grid is %dx%d, want 3x2", odd.Width, odd.Height)
	}
	if odd.MaxX != 6000 || odd.MaxY != 4000 {
		t.Errorf("odd grid snaps to (%v,%v), want (6000,4000)", odd.MaxX, odd.MaxY)
	}
}

func TestMercatorRoundTrip(t *testing.T) {
	places := []struct {
		name     string
		lat, lon float64
	}{
		{"Helsinki", 60.17, 24.94},
		{"Nuorgam", 70.08, 27.89},
		{"Märket", 60.30, 19.13},
		{"Virmajärvi", 63.10, 31.58},
	}
	for _, p := range places {
		x, y := LonLatToMercator(p.lon, p.lat)
		lon, lat := MercatorToLonLat(x, y)
		if math.Abs(lon-p.lon) > 1e-9 || math.Abs(lat-p.lat) > 1e-9 {
			t.Errorf("%s round-trips to (%v, %v), want (%v, %v)", p.name, lon, lat, p.lon, p.lat)
		}
	}
}

// The corner coordinates are what MapLibre pins the image to, so a regression
// here silently moves the weather. These values are cross-checked against an
// independent computation of the Web Mercator inverse.
func TestCornersMatchIndependentComputation(t *testing.T) {
	g := defaultGrid()
	corners := g.Corners()

	wantWestLon := 2_000_000 * 180 / (math.Pi * earthRadius)
	wantNorthLat := (2*math.Atan(math.Exp(11_200_000/earthRadius)) - math.Pi/2) * 180 / math.Pi
	wantEastLon := 3_600_000 * 180 / (math.Pi * earthRadius)
	wantSouthLat := (2*math.Atan(math.Exp(8_250_000/earthRadius)) - math.Pi/2) * 180 / math.Pi

	checks := []struct {
		label     string
		got, want float64
	}{
		{"top-left lon", corners[0][0], wantWestLon},
		{"top-left lat", corners[0][1], wantNorthLat},
		{"bottom-right lon", corners[2][0], wantEastLon},
		{"bottom-right lat", corners[2][1], wantSouthLat},
	}
	for _, c := range checks {
		if math.Abs(c.got-c.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", c.label, c.got, c.want)
		}
	}

	// Corner order is top-left, top-right, bottom-right, bottom-left — the order
	// MapLibre's image source expects. Getting it wrong flips the raster.
	if corners[0][1] <= corners[2][1] {
		t.Error("first corner should be the northern edge")
	}
	if corners[0][0] >= corners[1][0] {
		t.Error("second corner should be the eastern edge")
	}
}

// Finland must fall inside the grid, and the pixel a place maps to must map back
// to within half a pixel of itself.
func TestPixelIndexCoversFinland(t *testing.T) {
	g := defaultGrid()
	places := []struct {
		name     string
		lat, lon float64
	}{
		{"Helsinki", 60.17, 24.94},
		{"Nuorgam (northernmost)", 70.08, 27.89},
		{"Märket (westernmost)", 60.30, 19.13},
		{"Virmajärvi (easternmost)", 63.10, 31.58},
		{"Bogskär (southernmost)", 59.50, 20.35},
	}
	for _, p := range places {
		px, py, ok := g.PixelIndex(p.lon, p.lat)
		if !ok {
			t.Errorf("%s (%v, %v) falls outside the grid", p.name, p.lat, p.lon)
			continue
		}
		lon, lat := g.PixelCentre(px, py)
		// Half a pixel is 1000 mercator metres; in degrees that is well under 0.02.
		if math.Abs(lon-p.lon) > 0.02 || math.Abs(lat-p.lat) > 0.02 {
			t.Errorf("%s maps to pixel (%d,%d) whose centre is (%v,%v)", p.name, px, py, lat, lon)
		}
	}

	// Somewhere far outside must be rejected rather than clamped, or a stray
	// query would read an edge pixel and report it as local weather.
	if _, _, ok := g.PixelIndex(2.35, 48.85); ok { // Paris
		t.Error("Paris should fall outside the Finland grid")
	}
}

// Row 0 is the northern edge. If this inverts, the whole country renders
// upside down, which is exactly the kind of bug that looks plausible at a glance.
func TestPixelRowZeroIsNorth(t *testing.T) {
	g := defaultGrid()
	_, topLat := g.PixelCentre(g.Width/2, 0)
	_, bottomLat := g.PixelCentre(g.Width/2, g.Height-1)
	if topLat <= bottomLat {
		t.Errorf("row 0 latitude %v should be north of the last row's %v", topLat, bottomLat)
	}
}

func TestBBoxStringHasNoExponent(t *testing.T) {
	g := defaultGrid()
	// A coordinate rendered as "2e+06" is rejected by GeoServer, so the formatting
	// matters as much as the value.
	if got, want := g.BBoxString(), "2000000,8250000,3600000,11200000"; got != want {
		t.Errorf("BBoxString() = %q, want %q", got, want)
	}
}
