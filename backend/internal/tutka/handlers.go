package tutka

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Attribution is FMI's required credit. Their open data is CC BY 4.0, so this is
// a licence condition rather than a courtesy — the frontend puts it in the map's
// attribution control, and it is served here so there is one canonical wording.
const Attribution = "Sadetutka: Ilmatieteen laitos, CC BY 4.0"

// stampLayout is the compact UTC form used in frame URLs: 20260725T1305Z. It
// sorts lexicographically, survives a URL path without escaping, and reads
// clearly in a log line.
const stampLayout = "20060102T1504Z"

// pointSeriesWindow is how much history the point readout returns.
const pointSeriesWindow = time.Hour

type Handlers struct {
	store    *Store
	grid     Grid
	products []Product
}

func NewHandlers(store *Store, grid Grid, products []Product) *Handlers {
	return &Handlers{store: store, grid: grid, products: products}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("Tutka: failed to encode response: %v", err)
	}
}

// serviceUnavailable is the cold-start answer: no frame has been archived yet.
// The frontend shows it as a loading state, not an error.
func serviceUnavailable(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Error(w, "data not available yet", http.StatusServiceUnavailable)
}

func badRequest(w http.ResponseWriter, msg string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	http.Error(w, msg, http.StatusBadRequest)
}

type gridInfo struct {
	Width      int           `json:"width"`
	Height     int           `json:"height"`
	Resolution float64       `json:"resolution_m"`
	BBox3857   [4]float64    `json:"bbox_epsg3857"`
	Corners    [4][2]float64 `json:"corners"` // TL, TR, BR, BL as [lon, lat]
}

type productInfo struct {
	Product
	Palettes []string `json:"palettes"`
	Frames   int      `json:"frames"`
	Latest   string   `json:"latest,omitempty"`
}

type productsResponse struct {
	Attribution string        `json:"attribution"`
	Grid        gridInfo      `json:"grid"`
	Products    []productInfo `json:"products"`
}

// ProductList describes what the frontend can ask for: the products, their
// units and cadence, the palettes that fit each one, and the grid corners a
// MapLibre image source needs.
func (h *Handlers) ProductList(w http.ResponseWriter, r *http.Request) {
	res := productsResponse{
		Attribution: Attribution,
		Grid: gridInfo{
			Width:      h.grid.Width,
			Height:     h.grid.Height,
			Resolution: h.grid.Resolution,
			BBox3857:   [4]float64{h.grid.MinX, h.grid.MinY, h.grid.MaxX, h.grid.MaxY},
			Corners:    h.grid.Corners(),
		},
	}
	for _, p := range h.products {
		info := productInfo{Product: p, Frames: h.store.Count(p.ID)}
		for id, pal := range Palettes {
			if pal.Unit == p.Unit {
				info.Palettes = append(info.Palettes, id)
			}
		}
		sortStrings(info.Palettes)
		if latest, ok := h.store.Latest(p.ID); ok {
			info.Latest = latest.Format(time.RFC3339)
		}
		res.Products = append(res.Products, info)
	}
	writeJSON(w, res)
}

type frameRef struct {
	Time  string `json:"time"`
	Stamp string `json:"stamp"`
}

type framesResponse struct {
	Product string     `json:"product"`
	Frames  []frameRef `json:"frames"`
}

// Frames is the animation index: which frames exist, in ascending time order.
// The client walks these and requests each one's PNG.
func (h *Handlers) Frames(w http.ResponseWriter, r *http.Request) {
	p, ok := h.product(r.URL.Query().Get("product"))
	if !ok {
		badRequest(w, "unknown product")
		return
	}

	from, err := parseTimeParam(r.URL.Query().Get("from"))
	if err != nil {
		badRequest(w, "invalid from")
		return
	}
	to, err := parseTimeParam(r.URL.Query().Get("to"))
	if err != nil {
		badRequest(w, "invalid to")
		return
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		badRequest(w, "to must not precede from")
		return
	}

	times := h.store.Times(p.ID, from, to)
	if len(times) == 0 {
		if h.store.Count(p.ID) == 0 {
			serviceUnavailable(w)
			return
		}
		// The product has frames, just none in this window — an empty list is the
		// correct answer, not an error.
	}

	res := framesResponse{Product: p.ID, Frames: make([]frameRef, 0, len(times))}
	for _, t := range times {
		res.Frames = append(res.Frames, frameRef{Time: t.Format(time.RFC3339), Stamp: t.Format(stampLayout)})
	}
	writeJSON(w, res)
}

// FrameImage serves one coloured frame as a transparent PNG.
//
// A past frame never changes, so it is cached hard and marked immutable: the
// browser and the reverse proxy absorb repeat traffic, which matters because
// animating an hour of radar is a dozen image requests per visitor.
func (h *Handlers) FrameImage(w http.ResponseWriter, r *http.Request) {
	p, ok := h.product(r.PathValue("product"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	stamp := strings.TrimSuffix(r.PathValue("stamp"), ".png")
	ts, err := time.ParseInLocation(stampLayout, stamp, time.UTC)
	if err != nil {
		badRequest(w, "stamp must look like 20260725T1305Z")
		return
	}

	pal := ResolvePalette(p, r.URL.Query().Get("palette"))
	body, err := h.store.RenderedFrame(p.ID, ts, pal)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		log.Printf("Tutka: failed to render %s %s: %v", p.ID, stamp, err)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		http.Error(w, "failed to render frame", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Write(body)
}

type legendResponse struct {
	Product     string       `json:"product"`
	Palette     string       `json:"palette"`
	Label       string       `json:"label"`
	Unit        string       `json:"unit"`
	Stops       []legendStop `json:"stops"`
	Attribution string       `json:"attribution"`
}

// Legend gives the frontend the exact stops the pixels were coloured from, so
// the scale it draws cannot drift from what is on the map.
func (h *Handlers) Legend(w http.ResponseWriter, r *http.Request) {
	p, ok := h.product(r.PathValue("product"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	pal := ResolvePalette(p, r.URL.Query().Get("palette"))
	writeJSON(w, legendResponse{
		Product:     p.ID,
		Palette:     pal.ID,
		Label:       pal.Label,
		Unit:        pal.Unit,
		Stops:       pal.Legend(),
		Attribution: Attribution,
	})
}

type pointSample struct {
	Time  string      `json:"time"`
	Value *float64    `json:"value"` // null unless State is "measured"
	State SampleState `json:"state"`
}

type nowcastResponse struct {
	Motion Motion          `json:"motion"`
	Steps  []Extrapolation `json:"steps"`
	// ArrivalMinutes is how long until rain reaches the point, or -1 when none
	// arrives inside the horizon.
	ArrivalMinutes int `json:"arrival_minutes"`
	// Method names what this actually is, so a client cannot present it as a
	// forecast by accident.
	Method string `json:"method"`
}

type pointResponse struct {
	Lat     float64       `json:"lat"`
	Lon     float64       `json:"lon"`
	Product string        `json:"product"`
	Unit    string        `json:"unit"`
	Current pointSample   `json:"current"`
	Series  []pointSample `json:"series"`
	// Nowcast is present only when two recent frames yielded a usable motion
	// estimate — on a dry or stationary map there is nothing to extrapolate, and
	// inventing a vector would be worse than admitting that.
	Nowcast *nowcastResponse `json:"nowcast,omitempty"`
}

// arrivalThresholdDBZ is the reflectivity that counts as "rain reaching you". 15
// dBZ is light but unambiguous rain rather than drizzle at the detection floor.
const arrivalThresholdDBZ = 15

// Point reads the raw values under a location: what the radar sees there now and
// over the last hour.
//
// The value is null unless the cell was actually measured, and State says why.
// "No radar coverage" and "measured, nothing there" both look like empty map, but
// only the second is evidence of dry weather, and the API refuses to blur them.
func (h *Handlers) Point(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	lat, err := strconv.ParseFloat(q.Get("lat"), 64)
	if err != nil || lat < -90 || lat > 90 {
		badRequest(w, "lat must be a number between -90 and 90")
		return
	}
	lon, err := strconv.ParseFloat(q.Get("lon"), 64)
	if err != nil || lon < -180 || lon > 180 {
		badRequest(w, "lon must be a number between -180 and 180")
		return
	}

	p, ok := h.product(q.Get("product"))
	if !ok {
		badRequest(w, "unknown product")
		return
	}

	times := h.store.Times(p.ID, time.Now().Add(-pointSeriesWindow), time.Time{})
	if len(times) == 0 {
		serviceUnavailable(w)
		return
	}

	res := pointResponse{Lat: lat, Lon: lon, Product: p.ID, Unit: p.Unit}

	// Keep the last two frames aside for the motion estimate rather than
	// re-reading them: they are the most expensive thing on this path.
	var previous, latest *Frame
	var latestTime time.Time
	for _, t := range times {
		frame, err := h.store.Get(p.ID, t)
		if err != nil {
			continue
		}
		previous, latest, latestTime = latest, frame, t

		v, state := frame.ValueAt(lon, lat)
		sample := pointSample{Time: t.Format(time.RFC3339), State: state}
		if state == StateMeasured {
			value := roundTo(v, 2)
			sample.Value = &value
		}
		res.Series = append(res.Series, sample)
	}

	if len(res.Series) == 0 {
		serviceUnavailable(w)
		return
	}
	res.Current = res.Series[len(res.Series)-1]
	res.Nowcast = h.nowcast(p, previous, latest, latestTime, times, lon, lat)

	writeJSON(w, res)
}

// nowcast extrapolates the point forward from the two most recent frames, or
// returns nil when that cannot be done honestly.
//
// It is offered for reflectivity only. The accumulation products are already
// integrals over past time, so advecting them would produce a number that means
// nothing; and rain rate is noisier than reflectivity for pattern matching while
// answering the same question.
func (h *Handlers) nowcast(
	p Product,
	previous, latest *Frame,
	latestTime time.Time,
	times []time.Time,
	lon, lat float64,
) *nowcastResponse {
	if p.ID != "dbz" || previous == nil || latest == nil || len(times) < 2 {
		return nil
	}

	dt := latestTime.Sub(times[len(times)-2]).Seconds()
	// A gap far from the nominal cadence means a missing frame sat between these
	// two, and treating it as one step would halve or third the speed.
	nominal := float64(p.StepMinutes) * 60
	if dt <= 0 || dt > nominal*1.5 {
		return nil
	}

	motion := EstimateMotion(previous, latest, dt)
	if !motion.Valid {
		return nil
	}

	steps := ExtrapolatePoint(latest, motion, lon, lat)
	return &nowcastResponse{
		Motion:         motion,
		Steps:          steps,
		ArrivalMinutes: ArrivalMinutes(steps, arrivalThresholdDBZ),
		Method:         "advection of the latest radar frame along a single measured motion vector",
	}
}

// product resolves a product id, defaulting to the first (reflectivity) when the
// parameter is absent.
func (h *Handlers) product(id string) (Product, bool) {
	if id == "" {
		return h.products[0], true
	}
	for _, p := range h.products {
		if p.ID == id {
			return p, true
		}
	}
	return Product{}, false
}

// parseTimeParam accepts an RFC3339 instant, the compact stamp form, or an empty
// string meaning unbounded.
func parseTimeParam(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC(), nil
	}
	t, err := time.ParseInLocation(stampLayout, v, time.UTC)
	if err != nil {
		return time.Time{}, err
	}
	return t, nil
}

// roundTo trims a value for presentation. The scaled integers FMI sends do not
// land on exact binary fractions — raw 255 at gain 0.01 is 2.5500000000000003 —
// and shipping that in JSON reads as false precision. The maths upstream of here
// deliberately stays unrounded.
func roundTo(v float64, places int) float64 {
	scale := math.Pow(10, float64(places))
	return math.Round(v*scale) / scale
}

// sortStrings is an insertion sort for the short palette-id lists, keeping the
// import list to the standard few.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
