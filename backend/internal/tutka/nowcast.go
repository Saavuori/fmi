package tutka

import "math"

// Nowcasting parameters.
const (
	// Downsample factor for the correlation search. At the default grid this
	// turns a 1.2-megapixel comparison into ~74k cells per candidate offset,
	// which keeps a full search inside a few milliseconds.
	nowcastCoarsen = 4

	// Search radius in coarse cells. 25 coarse cells is 25*4*2000 mercator metres
	// per 5-minute step — well above any real weather system, and bounded so
	// noise cannot produce an absurd velocity.
	nowcastSearch = 25

	// A cell counts as raining above this reflectivity. Below it the field is
	// mostly detection-floor speckle, which correlates with nothing and would
	// drag the estimate toward zero.
	nowcastMinDBZ = 8

	// Minimum fraction of coarse cells that must be raining for a motion estimate
	// to mean anything. On a nearly dry map there is no pattern to track.
	nowcastMinCoverage = 0.004

	// Extrapolation horizon.
	nowcastHorizonMinutes = 45
	nowcastStepMinutes    = 5
)

// Motion is a translation of the precipitation field, in grid pixels per second.
//
// Confidence is the normalised correlation of the best match, in 0..1. It is
// reported rather than hidden because a single global vector is a real
// simplification: it describes a front crossing at a steady bearing well, and a
// rotating low or two systems moving differently badly. The number is how the UI
// knows which case it is in.
type Motion struct {
	PxPerSec   float64 `json:"px_per_sec_x"`
	PyPerSec   float64 `json:"px_per_sec_y"`
	Confidence float64 `json:"confidence"`
	// Speed and bearing are the human-readable form: km/h and degrees clockwise
	// from north, i.e. the direction the rain is heading towards.
	SpeedKmh float64 `json:"speed_kmh"`
	Bearing  float64 `json:"bearing_deg"`
	Valid    bool    `json:"valid"`
}

// coarseField is a downsampled rain-intensity field used only for matching. Cells
// hold a rain "score" rather than a physical value: what matters for correlation
// is the shape of the pattern, not its units.
type coarseField struct {
	w, h    int
	cells   []float64
	raining int
}

// coarsen reduces a frame to a matching field, averaging the raining cells in
// each block. No-coverage and below-threshold cells contribute zero, so radar
// edges do not read as a moving pattern.
func coarsen(f *Frame, factor int) coarseField {
	w := f.Grid.Width / factor
	h := f.Grid.Height / factor
	out := coarseField{w: w, h: h, cells: make([]float64, w*h)}

	for cy := 0; cy < h; cy++ {
		for cx := 0; cx < w; cx++ {
			sum := 0.0
			for dy := 0; dy < factor; dy++ {
				row := (cy*factor + dy) * f.Grid.Width
				for dx := 0; dx < factor; dx++ {
					raw := f.Values[row+cx*factor+dx]
					if raw == 0 || raw == f.NoData {
						continue
					}
					v := float64(raw)*f.Gain + f.Offset
					if v < nowcastMinDBZ {
						continue
					}
					sum += v - nowcastMinDBZ
				}
			}
			if sum > 0 {
				out.cells[cy*w+cx] = sum
				out.raining++
			}
		}
	}
	return out
}

// EstimateMotion finds the translation that best carries `from` onto `to`, by
// maximising normalised cross-correlation over a bounded offset search.
//
// dtSeconds is the interval between the two frames. Both frames must share a grid.
func EstimateMotion(from, to *Frame, dtSeconds float64) Motion {
	if from == nil || to == nil || dtSeconds <= 0 {
		return Motion{}
	}
	if from.Grid.Width != to.Grid.Width || from.Grid.Height != to.Grid.Height {
		return Motion{}
	}

	a := coarsen(from, nowcastCoarsen)
	b := coarsen(to, nowcastCoarsen)

	total := float64(a.w * a.h)
	if total == 0 {
		return Motion{}
	}
	// Both frames need enough rain in them: matching a dry field against a dry
	// field yields a confident-looking zero that means nothing.
	if float64(a.raining)/total < nowcastMinCoverage || float64(b.raining)/total < nowcastMinCoverage {
		return Motion{}
	}

	bestScore := -1.0
	bestDX, bestDY := 0, 0
	for dy := -nowcastSearch; dy <= nowcastSearch; dy++ {
		for dx := -nowcastSearch; dx <= nowcastSearch; dx++ {
			if score := correlate(a, b, dx, dy); score > bestScore {
				bestScore, bestDX, bestDY = score, dx, dy
			}
		}
	}
	if bestScore <= 0 {
		return Motion{}
	}

	// Coarse cells back to full-resolution pixels per second.
	pxPerSec := float64(bestDX*nowcastCoarsen) / dtSeconds
	pyPerSec := float64(bestDY*nowcastCoarsen) / dtSeconds

	// Mercator metres are stretched by 1/cos(lat), so converting a pixel velocity
	// to a ground speed needs the latitude it was measured at. The middle of the
	// grid is the honest single choice for a whole-field estimate.
	_, midLat := to.Grid.PixelCentre(to.Grid.Width/2, to.Grid.Height/2)
	metresPerPx := to.Grid.Resolution * math.Cos(midLat*math.Pi/180)
	speedMs := math.Hypot(pxPerSec, pyPerSec) * metresPerPx

	// Screen y grows southward, so a positive dy is southward motion.
	bearing := math.Atan2(pxPerSec, -pyPerSec) * 180 / math.Pi
	if bearing < 0 {
		bearing += 360
	}

	return Motion{
		PxPerSec:   pxPerSec,
		PyPerSec:   pyPerSec,
		Confidence: bestScore,
		SpeedKmh:   speedMs * 3.6,
		Bearing:    bearing,
		Valid:      true,
	}
}

// correlate scores offset (dx, dy) as the cosine similarity of the overlapping
// region. Cosine similarity rather than raw sum keeps a large offset that happens
// to overlap two intense areas from beating the true match.
func correlate(a, b coarseField, dx, dy int) float64 {
	var dot, na, nb float64
	for y := 0; y < a.h; y++ {
		sy := y + dy
		if sy < 0 || sy >= a.h {
			continue
		}
		for x := 0; x < a.w; x++ {
			sx := x + dx
			if sx < 0 || sx >= a.w {
				continue
			}
			av := a.cells[y*a.w+x]
			bv := b.cells[sy*b.w+sx]
			dot += av * bv
			na += av * av
			nb += bv * bv
		}
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}

// Extrapolation is one step of the point forecast.
type Extrapolation struct {
	// MinutesAhead from the latest frame.
	MinutesAhead int         `json:"minutes_ahead"`
	Value        *float64    `json:"value"`
	State        SampleState `json:"state"`
}

// ExtrapolatePoint projects what will pass over a location, by walking backwards
// along the motion vector from the latest frame.
//
// This is advection and nothing more: it moves existing rain across the map at a
// measured speed. It does not know about growth, decay or new convection, so it is
// labelled an extrapolation rather than a forecast, and it stops at 45 minutes
// where that assumption is still defensible.
func ExtrapolatePoint(latest *Frame, motion Motion, lon, lat float64) []Extrapolation {
	if latest == nil || !motion.Valid {
		return nil
	}

	px, py := latest.Grid.LonLatToPixel(lon, lat)
	out := make([]Extrapolation, 0, nowcastHorizonMinutes/nowcastStepMinutes)

	for minutes := nowcastStepMinutes; minutes <= nowcastHorizonMinutes; minutes += nowcastStepMinutes {
		dt := float64(minutes) * 60
		// The rain that will be over the point in `dt` is the rain that is
		// currently upwind of it, so step backwards along the motion vector.
		sx := int(px - motion.PxPerSec*dt)
		sy := int(py - motion.PyPerSec*dt)

		value, state := latest.ValueAtPixel(sx, sy)
		step := Extrapolation{MinutesAhead: minutes, State: state}
		if state == StateMeasured {
			v := roundTo(value, 2)
			step.Value = &v
		}
		out = append(out, step)
	}
	return out
}

// ArrivalMinutes reports how long until rain reaches the point, or -1 when the
// extrapolation shows none arriving inside the horizon. Only measured cells count:
// a no-coverage cell upwind means we cannot see what is coming, not that nothing
// is.
func ArrivalMinutes(steps []Extrapolation, thresholdDBZ float64) int {
	for _, s := range steps {
		if s.State == StateMeasured && s.Value != nil && *s.Value >= thresholdDBZ {
			return s.MinutesAhead
		}
	}
	return -1
}
