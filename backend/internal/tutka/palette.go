package tutka

import (
	"image/color"
	"strconv"
)

// Stop is one anchor of a colour ramp: a physical value and the colour it maps
// to. Alpha is part of the stop so a ramp can fade in at its low end instead of
// switching on abruptly.
type Stop struct {
	Value      float64 `json:"value"`
	R, G, B, A uint8   `json:"-"`
	Hex        string  `json:"color"` // filled by legendStops for the API
}

// Palette is a named colour ramp over one unit. Colours are interpolated
// linearly between stops rather than banded: at country zoom a smooth ramp reads
// as weather, whereas hard bands read as contour lines.
//
// We own these ramps rather than using FMI's rendered PNGs, which is what buys
// us real transparency (theirs paints "no echo" opaque white), a legend that
// matches the pixels exactly, and theme and accessibility variants.
type Palette struct {
	ID    string
	Label string
	Unit  string
	Stops []Stop
}

// The reflectivity ramp. Below the first stop nothing is drawn: drizzle at the
// radar's detection floor is mostly clutter, and painting it hides the map.
var paletteDBZ = Palette{
	ID: "dbz", Label: "Heijastavuus", Unit: "dBZ",
	Stops: []Stop{
		{Value: 4, R: 90, G: 190, B: 255, A: 0},
		{Value: 8, R: 90, G: 190, B: 255, A: 150},
		{Value: 15, R: 46, G: 216, B: 182, A: 205},
		{Value: 22, R: 92, G: 226, B: 100, A: 228},
		{Value: 30, R: 240, G: 219, B: 78, A: 242},
		{Value: 38, R: 249, G: 152, B: 54, A: 250},
		{Value: 46, R: 238, G: 74, B: 62, A: 255},
		{Value: 54, R: 199, G: 44, B: 141, A: 255},
		{Value: 64, R: 233, G: 205, B: 255, A: 255},
	},
}

// Rain rate, spaced roughly logarithmically because that is how rainfall is
// actually distributed — most of the map is under 1 mm/h whenever it rains.
var paletteRate = Palette{
	ID: "rate", Label: "Sateen voimakkuus", Unit: "mm/h",
	Stops: []Stop{
		{Value: 0.1, R: 90, G: 190, B: 255, A: 0},
		{Value: 0.2, R: 90, G: 190, B: 255, A: 145},
		{Value: 0.5, R: 46, G: 216, B: 182, A: 205},
		{Value: 1, R: 92, G: 226, B: 100, A: 228},
		{Value: 2.5, R: 240, G: 219, B: 78, A: 242},
		{Value: 6, R: 249, G: 152, B: 54, A: 250},
		{Value: 15, R: 238, G: 74, B: 62, A: 255},
		{Value: 30, R: 199, G: 44, B: 141, A: 255},
		{Value: 50, R: 233, G: 205, B: 255, A: 255},
	},
}

// Accumulations, in millimetres over the product's window.
var paletteAccum = Palette{
	ID: "accum", Label: "Sadesumma", Unit: "mm",
	Stops: []Stop{
		{Value: 0.2, R: 90, G: 190, B: 255, A: 0},
		{Value: 0.5, R: 90, G: 190, B: 255, A: 150},
		{Value: 2, R: 46, G: 216, B: 182, A: 208},
		{Value: 5, R: 92, G: 226, B: 100, A: 230},
		{Value: 10, R: 240, G: 219, B: 78, A: 243},
		{Value: 20, R: 249, G: 152, B: 54, A: 250},
		{Value: 40, R: 238, G: 74, B: 62, A: 255},
		{Value: 70, R: 199, G: 44, B: 141, A: 255},
		{Value: 100, R: 233, G: 205, B: 255, A: 255},
	},
}

// Colour-vision-deficiency variants. The default ramps run green -> yellow ->
// red, which is the pairing red-green deficiencies cannot separate — and on a
// radar map that is exactly the difference between "a shower" and "take cover".
// These run blue -> cyan -> sand -> orange -> dark red instead, so intensity is
// carried by lightness as well as hue.
var paletteDBZCVD = Palette{
	ID: "dbz-cvd", Label: "Heijastavuus (saavutettava)", Unit: "dBZ",
	Stops: []Stop{
		{Value: 4, R: 70, G: 110, B: 220, A: 0},
		{Value: 8, R: 70, G: 110, B: 220, A: 155},
		{Value: 15, R: 66, G: 176, B: 226, A: 208},
		{Value: 22, R: castUint8(140), G: 214, B: 226, A: 228},
		{Value: 30, R: 233, G: 226, B: 170, A: 242},
		{Value: 38, R: 240, G: 178, B: 90, A: 250},
		{Value: 46, R: 224, G: 118, B: 46, A: 255},
		{Value: 54, R: 168, G: 54, B: 32, A: 255},
		{Value: 64, R: 92, G: 20, B: 24, A: 255},
	},
}

var paletteRateCVD = Palette{
	ID: "rate-cvd", Label: "Sateen voimakkuus (saavutettava)", Unit: "mm/h",
	Stops: []Stop{
		{Value: 0.1, R: 70, G: 110, B: 220, A: 0},
		{Value: 0.2, R: 70, G: 110, B: 220, A: 150},
		{Value: 0.5, R: 66, G: 176, B: 226, A: 208},
		{Value: 1, R: 140, G: 214, B: 226, A: 228},
		{Value: 2.5, R: 233, G: 226, B: 170, A: 242},
		{Value: 6, R: 240, G: 178, B: 90, A: 250},
		{Value: 15, R: 224, G: 118, B: 46, A: 255},
		{Value: 30, R: 168, G: 54, B: 32, A: 255},
		{Value: 50, R: 92, G: 20, B: 24, A: 255},
	},
}

var paletteAccumCVD = Palette{
	ID: "accum-cvd", Label: "Sadesumma (saavutettava)", Unit: "mm",
	Stops: []Stop{
		{Value: 0.2, R: 70, G: 110, B: 220, A: 0},
		{Value: 0.5, R: 70, G: 110, B: 220, A: 155},
		{Value: 2, R: 66, G: 176, B: 226, A: 208},
		{Value: 5, R: 140, G: 214, B: 226, A: 230},
		{Value: 10, R: 233, G: 226, B: 170, A: 243},
		{Value: 20, R: 240, G: 178, B: 90, A: 250},
		{Value: 40, R: 224, G: 118, B: 46, A: 255},
		{Value: 70, R: 168, G: 54, B: 32, A: 255},
		{Value: 100, R: 92, G: 20, B: 24, A: 255},
	},
}

// Palettes is the registry the API and renderer look palettes up in.
var Palettes = map[string]Palette{
	paletteDBZ.ID:      paletteDBZ,
	paletteRate.ID:     paletteRate,
	paletteAccum.ID:    paletteAccum,
	paletteDBZCVD.ID:   paletteDBZCVD,
	paletteRateCVD.ID:  paletteRateCVD,
	paletteAccumCVD.ID: paletteAccumCVD,
}

// castUint8 exists only so a stop table entry can be written without a literal
// type conversion cluttering the line.
func castUint8(v int) uint8 { return uint8(v) }

// ResolvePalette picks the palette for a request: the caller's choice when it is
// both known and dimensionally compatible with the product, otherwise the
// product's own default. Serving a mm/h ramp for a dBZ raster would silently
// mislabel every pixel, so a unit mismatch falls back rather than obliging.
func ResolvePalette(p Product, requested string) Palette {
	if requested != "" {
		if pal, ok := Palettes[requested]; ok && pal.Unit == p.Unit {
			return pal
		}
	}
	return Palettes[p.Palette]
}

// ColourAt interpolates the ramp at a physical value. Below the first stop and
// above the last it clamps, so the ramp's own endpoints define the extremes.
func (pal Palette) ColourAt(v float64) color.NRGBA {
	if len(pal.Stops) == 0 {
		return color.NRGBA{}
	}
	first, last := pal.Stops[0], pal.Stops[len(pal.Stops)-1]
	if v <= first.Value {
		return color.NRGBA{R: first.R, G: first.G, B: first.B, A: first.A}
	}
	if v >= last.Value {
		return color.NRGBA{R: last.R, G: last.G, B: last.B, A: last.A}
	}
	for i := 1; i < len(pal.Stops); i++ {
		hi := pal.Stops[i]
		if v > hi.Value {
			continue
		}
		lo := pal.Stops[i-1]
		span := hi.Value - lo.Value
		t := 0.0
		if span > 0 {
			t = (v - lo.Value) / span
		}
		return color.NRGBA{
			R: lerp8(lo.R, hi.R, t),
			G: lerp8(lo.G, hi.G, t),
			B: lerp8(lo.B, hi.B, t),
			A: lerp8(lo.A, hi.A, t),
		}
	}
	return color.NRGBA{R: last.R, G: last.G, B: last.B, A: last.A}
}

func lerp8(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5)
}

// LUT builds a raw-value to colour table for one frame, so rendering is a plain
// array index per pixel rather than a ramp walk. The table is sized to the
// frame's depth: 256 entries for the 8-bit dbz product, 65536 (256 KB) for the
// 16-bit rain products, which is cheap next to the 1.2-megapixel raster it saves
// work on.
//
// Both special values resolve to fully transparent: 0 because the radar looked
// and found nothing, NoData because it cannot see there at all.
func (pal Palette) LUT(f *Frame) []color.NRGBA {
	n := int(f.NoData) + 1
	lut := make([]color.NRGBA, n)
	for raw := 1; raw < n-1; raw++ {
		lut[raw] = pal.ColourAt(float64(raw)*f.Gain + f.Offset)
	}
	// lut[0] and lut[NoData] stay the zero value, i.e. transparent.
	return lut
}

// legendStop is one row of the legend the frontend draws.
type legendStop struct {
	Value float64 `json:"value"`
	Label string  `json:"label"`
	Color string  `json:"color"`
}

// Legend renders the palette as stops the frontend can draw a scale from. The
// first stop is skipped: it is the fully transparent fade-in anchor and would
// show as an invisible swatch.
func (pal Palette) Legend() []legendStop {
	out := make([]legendStop, 0, len(pal.Stops))
	for i, s := range pal.Stops {
		if i == 0 && s.A == 0 {
			continue
		}
		out = append(out, legendStop{
			Value: s.Value,
			Label: formatValue(s.Value),
			Color: hex(s.R, s.G, s.B),
		})
	}
	return out
}

func hex(r, g, b uint8) string {
	const digits = "0123456789abcdef"
	buf := []byte{'#', digits[r>>4], digits[r&15], digits[g>>4], digits[g&15], digits[b>>4], digits[b&15]}
	return string(buf)
}

// formatValue prints a scale value without trailing noise: 0.5 stays 0.5, 30
// stays 30 rather than becoming 30.0.
func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
