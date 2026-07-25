package tutka

import (
	"strconv"
	"time"
)

// Product is one FMI radar composite. Everything that differs between the
// composites lives here, so FMI's announced layer churn ("new radar layers will
// be introduced ... old radar layers will be phased out") is a table edit rather
// than a code change.
//
// Bits is the raster depth FMI serves for this product and is not cosmetic: dbz
// comes back as 8-bit, every rain product as 16-bit. Gain and Offset are the
// documented defaults; the poller overwrites them per frame from the WFS
// response, which is the authoritative source and differs per product
// (dbz is 0.5/-32, the rain products 0.01/0).
type Product struct {
	ID          string  `json:"id"`
	Label       string  `json:"label"`        // Finnish UI label
	StoredQuery string  `json:"-"`            // opendata.fmi.fi WFS stored query id
	Layer       string  `json:"-"`            // openwms.fmi.fi WMS layer name
	Unit        string  `json:"unit"`         // "dBZ", "mm/h", "mm"
	Bits        int     `json:"-"`            // 8 or 16
	Gain        float64 `json:"-"`            // default; per-frame value wins
	Offset      float64 `json:"-"`            // default; per-frame value wins
	StepMinutes int     `json:"step_minutes"` // nominal cadence
	Palette     string  `json:"palette"`      // default palette id for this unit
	RetainHours int     `json:"retain_hours"` // filled from config at startup
}

// NoData is the raster value meaning "outside radar coverage" and is always the
// maximum for the depth: 255 for the 8-bit dbz product, 65535 for the 16-bit
// rain products. Zero means the opposite — inside coverage, nothing detected.
//
// Both render transparent, but they are not the same answer to "is it raining
// here", and the point readout reports them differently.
func (p Product) NoData() uint16 {
	return uint16(1<<p.Bits - 1)
}

// Products is the full composite set, most useful first. Single-radar products
// (per-station PPI, radial velocity, echo top) are deliberately absent: they are
// 12 radars times 4 quantities, which is a different scale of polling and UI.
var Products = []Product{
	{
		ID: "dbz", Label: "Sadetutka", StoredQuery: "fmi::radar::composite::dbz",
		Layer: "suomi_dbz_eureffin", Unit: "dBZ", Bits: 8,
		Gain: 0.5, Offset: -32, StepMinutes: 5, Palette: "dbz",
	},
	{
		ID: "rr", Label: "Sateen voimakkuus", StoredQuery: "fmi::radar::composite::rr",
		Layer: "suomi_rr_eureffin", Unit: "mm/h", Bits: 16,
		Gain: 0.01, Offset: 0, StepMinutes: 5, Palette: "rate",
	},
	{
		ID: "rr1h", Label: "Sadesumma 1 h", StoredQuery: "fmi::radar::composite::rr1h",
		Layer: "suomi_rr1h_eureffin", Unit: "mm", Bits: 16,
		Gain: 0.01, Offset: 0, StepMinutes: 60, Palette: "accum",
	},
	{
		ID: "rr12h", Label: "Sadesumma 12 h", StoredQuery: "fmi::radar::composite::rr12h",
		Layer: "suomi_rr12h_eureffin", Unit: "mm", Bits: 16,
		Gain: 0.01, Offset: 0, StepMinutes: 60, Palette: "accum",
	},
	{
		ID: "rr24h", Label: "Sadesumma 24 h", StoredQuery: "fmi::radar::composite::rr24h",
		Layer: "suomi_rr24h_eureffin", Unit: "mm", Bits: 16,
		Gain: 0.01, Offset: 0, StepMinutes: 60, Palette: "accum",
	},
}

// defaultRetainHours is the per-product archive depth before config overrides.
// dbz matches the seven days FMI itself keeps. rr is held for two days because
// it is 16-bit — twice the bytes of dbz per frame at the same cadence — and the
// hourly accumulations cover a longer look-back far more cheaply.
var defaultRetainHours = map[string]int{
	"dbz":   7 * 24,
	"rr":    48,
	"rr1h":  7 * 24,
	"rr12h": 7 * 24,
	"rr24h": 7 * 24,
}

// ProductByID looks a product up in the table.
func ProductByID(id string) (Product, bool) {
	for _, p := range Products {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}

// SampleState says what a raster cell actually means. Keeping "no radar
// coverage" distinct from "radar looked and found nothing" is the whole reason
// this type exists: they render identically (transparent) but they are opposite
// answers to "is it raining here", and conflating them would let the app claim
// dry weather over an area it cannot see.
type SampleState string

const (
	StateOutsideGrid SampleState = "outside_grid" // the point is off the raster entirely
	StateNoCoverage  SampleState = "no_coverage"  // inside the raster, outside radar range
	StateNoEcho      SampleState = "no_echo"      // measured, nothing detected
	StateMeasured    SampleState = "measured"     // a real value
)

// Frame is one decoded raster: a product at an instant, on the canonical grid.
//
// Values is always uint16 regardless of the product's native depth, so callers
// need one code path; Bits and NoData carry the depth-specific detail. At the
// default grid a frame is ~2.4 MB in memory, which is why the archive lives on
// disk and only a bounded window is kept decoded.
type Frame struct {
	Product string
	Time    time.Time
	Grid    Grid
	Bits    int
	NoData  uint16
	Gain    float64
	Offset  float64
	Values  []uint16 // len == Grid.Width*Grid.Height, row-major, row 0 = north
}

// Raw returns the stored cell value at a pixel.
func (f *Frame) Raw(px, py int) (uint16, bool) {
	if px < 0 || py < 0 || px >= f.Grid.Width || py >= f.Grid.Height {
		return 0, false
	}
	return f.Values[py*f.Grid.Width+px], true
}

// ValueAtPixel converts a cell to its physical value, reporting what the cell
// means. The returned float is only meaningful for StateMeasured.
func (f *Frame) ValueAtPixel(px, py int) (float64, SampleState) {
	raw, ok := f.Raw(px, py)
	if !ok {
		return 0, StateOutsideGrid
	}
	switch raw {
	case f.NoData:
		return 0, StateNoCoverage
	case 0:
		return 0, StateNoEcho
	}
	return float64(raw)*f.Gain + f.Offset, StateMeasured
}

// ValueAt samples the frame at a geographic point.
func (f *Frame) ValueAt(lon, lat float64) (float64, SampleState) {
	px, py, ok := f.Grid.PixelIndex(lon, lat)
	if !ok {
		return 0, StateOutsideGrid
	}
	return f.ValueAtPixel(px, py)
}

// formatFloat renders a coordinate for a WMS query string without an exponent or
// trailing zeros.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
