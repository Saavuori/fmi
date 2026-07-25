package tutka

import "math"

// earthRadius is the Web Mercator sphere radius; halfCircumference is the
// mercator x of 180°E, i.e. the value everything scales against.
const (
	earthRadius       = 6378137.0
	halfCircumference = math.Pi * earthRadius // 20037508.342789244
)

// Grid is the canonical raster geometry: an EPSG:3857 bounding box divided into
// square pixels. One definition serves the whole pipeline — the poller asks FMI
// for exactly this box at exactly this size, the store writes rasters of this
// shape, the renderer colours them, the point sampler indexes into them, and the
// frontend hands the same four corners to MapLibre.
//
// Web Mercator is deliberate rather than incidental. A MapLibre `image` source
// projects its four lon/lat corners into mercator and draws an axis-aligned
// quad, so a raster that is *itself* mercator lands with no warp error at all.
// The same raster in EPSG:4326 would not: latitude is nonlinear in mercator, and
// across 1200 km of Finland that error is visible.
type Grid struct {
	MinX, MinY, MaxX, MaxY float64 // EPSG:3857 metres
	Width, Height          int     // pixels
	Resolution             float64 // mercator metres per pixel
}

// NewGrid builds a grid from a bbox and a resolution, snapping the north and
// east edges so the pixels are exactly square and MaxX/MaxY are exactly
// MinX/MinY plus a whole number of pixels. Without that snap the corner
// coordinates handed to MapLibre would disagree with the raster by a fraction of
// a pixel, which shows up as the radar creeping as you zoom.
//
// Note the ground resolution varies with latitude: mercator metres are stretched
// by 1/cos(lat), so a 2000 m/px grid is ~1.0 km on the ground at 60°N and
// ~0.68 km at 70°N. Sizing against the southern edge means the north is
// oversampled rather than blurred.
func NewGrid(bbox [4]float64, resolution float64) Grid {
	minX, minY := bbox[0], bbox[1]
	w := int(math.Round((bbox[2] - minX) / resolution))
	h := int(math.Round((bbox[3] - minY) / resolution))
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return Grid{
		MinX:       minX,
		MinY:       minY,
		MaxX:       minX + float64(w)*resolution,
		MaxY:       minY + float64(h)*resolution,
		Width:      w,
		Height:     h,
		Resolution: resolution,
	}
}

// LonLatToMercator projects WGS84 degrees to EPSG:3857 metres. Latitudes are
// clamped just inside the poles, where the projection diverges.
func LonLatToMercator(lon, lat float64) (x, y float64) {
	if lat > 85.051129 {
		lat = 85.051129
	}
	if lat < -85.051129 {
		lat = -85.051129
	}
	x = lon * halfCircumference / 180
	y = earthRadius * math.Log(math.Tan(math.Pi/4+lat*math.Pi/360))
	return x, y
}

// MercatorToLonLat is the inverse of LonLatToMercator.
func MercatorToLonLat(x, y float64) (lon, lat float64) {
	lon = x * 180 / halfCircumference
	lat = (2*math.Atan(math.Exp(y/earthRadius)) - math.Pi/2) * 180 / math.Pi
	return lon, lat
}

// PixelCentre returns the lon/lat of the centre of pixel (px, py). Row 0 is the
// northern edge, matching raster convention.
func (g Grid) PixelCentre(px, py int) (lon, lat float64) {
	x := g.MinX + (float64(px)+0.5)*g.Resolution
	y := g.MaxY - (float64(py)+0.5)*g.Resolution
	return MercatorToLonLat(x, y)
}

// LonLatToPixel maps WGS84 degrees to fractional pixel coordinates. The result
// may fall outside the grid; callers decide whether that matters.
func (g Grid) LonLatToPixel(lon, lat float64) (px, py float64) {
	x, y := LonLatToMercator(lon, lat)
	return (x - g.MinX) / g.Resolution, (g.MaxY - y) / g.Resolution
}

// PixelIndex maps WGS84 degrees to an integer pixel, reporting whether it lands
// inside the grid.
func (g Grid) PixelIndex(lon, lat float64) (px, py int, ok bool) {
	fx, fy := g.LonLatToPixel(lon, lat)
	if fx < 0 || fy < 0 {
		return 0, 0, false
	}
	px, py = int(fx), int(fy)
	if px >= g.Width || py >= g.Height {
		return 0, 0, false
	}
	return px, py, true
}

// Corners returns the grid's four corners as MapLibre wants them for an image
// source: top-left, top-right, bottom-right, bottom-left, each [lon, lat].
func (g Grid) Corners() [4][2]float64 {
	wLon, nLat := MercatorToLonLat(g.MinX, g.MaxY)
	eLon, sLat := MercatorToLonLat(g.MaxX, g.MinY)
	return [4][2]float64{
		{wLon, nLat},
		{eLon, nLat},
		{eLon, sLat},
		{wLon, sLat},
	}
}

// BBoxString renders the bbox as the comma-separated minx,miny,maxx,maxy that a
// WMS GetMap request wants.
func (g Grid) BBoxString() string {
	return formatFloat(g.MinX) + "," + formatFloat(g.MinY) + "," +
		formatFloat(g.MaxX) + "," + formatFloat(g.MaxY)
}
