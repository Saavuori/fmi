package tutka

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/image/tiff"

	"fmi/internal/core/upstream"
)

const (
	wfsBase = "https://opendata.fmi.fi/wfs"
	wmsBase = "https://openwms.fmi.fi/geoserver/Radar/wms"
)

// FMI keeps roughly seven days of radar history and publishes a new composite
// within about five minutes. Both numbers are theirs, not ours.
const upstreamHistory = 7 * 24 * time.Hour

// wfsFeatureCollection is the slice of FMI's WFS GridSeriesObservation response
// we actually need: when each frame was measured, and the linear transformation
// that turns its raster cells into physical values.
//
// Fields match on local name only, so the om:/gml:/omso: prefixes do not have to
// be spelled out. The transformation is read per response rather than hardcoded
// because it genuinely differs per product.
type wfsFeatureCollection struct {
	XMLName       xml.Name `xml:"FeatureCollection"`
	NumberMatched int      `xml:"numberMatched,attr"`
	Members       []struct {
		Observation struct {
			PhenomenonTime struct {
				TimeInstant struct {
					TimePosition string `xml:"timePosition"`
				} `xml:"TimeInstant"`
			} `xml:"phenomenonTime"`
			Parameters []struct {
				NamedValue struct {
					Name struct {
						Href string `xml:"href,attr"`
					} `xml:"name"`
					Value struct {
						Measure string `xml:"Measure"`
					} `xml:"value"`
				} `xml:"NamedValue"`
			} `xml:"parameter"`
		} `xml:"GridSeriesObservation"`
	} `xml:"member"`
}

// wfsException is FMI's error envelope. It comes back with HTTP 200 in some
// cases, so a successful fetch is not the same as a successful query.
//
// There is deliberately no field for the exceptionCode attribute: `a>b,attr` is
// not a legal encoding/xml tag, and including one makes Unmarshal fail for the
// whole struct — which previously swallowed the upstream message and reported a
// confusing "expected <FeatureCollection>" error instead.
type wfsException struct {
	XMLName xml.Name `xml:"ExceptionReport"`
	Text    string   `xml:"Exception>ExceptionText"`
}

// frameMeta is what discovery yields for one available frame.
type frameMeta struct {
	Time   time.Time
	Gain   float64
	Offset float64
}

// parseWFSFrames extracts the available frames and their linear transformations
// from a WFS GridSeriesObservation response. Results are in ascending time order
// and de-duplicated, because FMI occasionally repeats a timestamp across members.
func parseWFSFrames(body []byte, def Product) ([]frameMeta, error) {
	// An exception body is never a frame list, so report it as an error whether or
	// not the text can be extracted — falling through to the generic parse would
	// only turn a clear upstream message into a confusing schema complaint.
	if bytes.Contains(body, []byte("ExceptionReport")) {
		var exc wfsException
		text := ""
		if err := xml.Unmarshal(body, &exc); err == nil {
			text = strings.TrimSpace(exc.Text)
		}
		if text == "" {
			text = "unspecified WFS exception"
		}
		return nil, fmt.Errorf("fmi wfs exception: %s", text)
	}

	var fc wfsFeatureCollection
	if err := xml.Unmarshal(body, &fc); err != nil {
		return nil, fmt.Errorf("parse wfs response: %w", err)
	}

	seen := make(map[int64]struct{}, len(fc.Members))
	out := make([]frameMeta, 0, len(fc.Members))
	for _, m := range fc.Members {
		raw := strings.TrimSpace(m.Observation.PhenomenonTime.TimeInstant.TimePosition)
		if raw == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			continue
		}
		ts = ts.UTC()
		if _, dup := seen[ts.Unix()]; dup {
			continue
		}
		seen[ts.Unix()] = struct{}{}

		// Fall back to the product's documented transformation if a member omits
		// it — a frame with a plausible default beats no frame at all.
		meta := frameMeta{Time: ts, Gain: def.Gain, Offset: def.Offset}
		for _, p := range m.Observation.Parameters {
			href := p.NamedValue.Name.Href
			v, err := strconv.ParseFloat(strings.TrimSpace(p.NamedValue.Value.Measure), 64)
			if err != nil {
				continue
			}
			switch {
			case strings.HasSuffix(href, "linearTransformationGain"):
				meta.Gain = v
			case strings.HasSuffix(href, "linearTransformationOffset"):
				meta.Offset = v
			}
		}
		out = append(out, meta)
	}

	sortFrameMetas(out)
	return out, nil
}

// sortFrameMetas orders frames oldest-first with an insertion sort: the slices
// are a handful to a few hundred entries and usually already sorted.
func sortFrameMetas(fs []frameMeta) {
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && fs[j].Time.Before(fs[j-1].Time); j-- {
			fs[j], fs[j-1] = fs[j-1], fs[j]
		}
	}
}

// discoverFrames asks FMI which frames of a product exist in [from, to].
func discoverFrames(ctx context.Context, client *upstream.Client, p Product, from, to time.Time) ([]frameMeta, error) {
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", "2.0.0")
	q.Set("request", "getFeature")
	q.Set("storedquery_id", p.StoredQuery)
	q.Set("starttime", from.UTC().Format(time.RFC3339))
	q.Set("endtime", to.UTC().Format(time.RFC3339))

	body, err := client.GetURL(ctx, wfsBase+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return parseWFSFrames(body, p)
}

// getMapURL builds the GetMap request for one frame on the canonical grid.
//
// `styles=raster` is what makes this a data fetch rather than a picture fetch: it
// returns the raw single-band raster instead of FMI's own colouring, whose
// "no echo" area is opaque white and would blank out the basemap. `crs=EPSG:3857`
// asks GeoServer to reproject for us, so the raster arrives already in the
// projection MapLibre draws in.
func getMapURL(p Product, g Grid, ts time.Time) string {
	q := url.Values{}
	q.Set("service", "WMS")
	q.Set("version", "1.3.0")
	q.Set("request", "GetMap")
	q.Set("layers", p.Layer)
	q.Set("styles", "raster")
	q.Set("format", "image/geotiff")
	q.Set("crs", "EPSG:3857")
	q.Set("bbox", g.BBoxString())
	q.Set("width", strconv.Itoa(g.Width))
	q.Set("height", strconv.Itoa(g.Height))
	q.Set("time", ts.UTC().Format(time.RFC3339))
	return wmsBase + "?" + q.Encode()
}

// fetchFrame downloads and decodes one frame onto the canonical grid.
func fetchFrame(ctx context.Context, client *upstream.Client, p Product, g Grid, meta frameMeta) (*Frame, error) {
	body, err := client.GetURL(ctx, getMapURL(p, g, meta.Time))
	if err != nil {
		return nil, err
	}
	// A WMS error is XML, not a raster, and arrives with a 200.
	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("<")) {
		return nil, fmt.Errorf("fmi wms returned XML instead of a raster for %s at %s", p.ID, meta.Time.Format(time.RFC3339))
	}

	values, bits, err := decodeRaster(body, g)
	if err != nil {
		return nil, fmt.Errorf("%s at %s: %w", p.ID, meta.Time.Format(time.RFC3339), err)
	}

	return &Frame{
		Product: p.ID,
		Time:    meta.Time,
		Grid:    g,
		Bits:    bits,
		NoData:  uint16(1<<bits - 1),
		Gain:    meta.Gain,
		Offset:  meta.Offset,
		Values:  values,
	}, nil
}

// decodeRaster turns an FMI GeoTIFF into the flat uint16 grid the rest of the
// package works with, and reports the native bit depth.
//
// FMI serves single-band rasters at two depths and it is not a detail we get to
// ignore: the dbz composite is 8-bit, every rain composite is 16-bit. The depth
// determines the no-coverage sentinel (255 vs 65535) and how many bytes a frame
// costs on disk.
func decodeRaster(body []byte, g Grid) ([]uint16, int, error) {
	img, err := tiff.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("decode geotiff: %w", err)
	}

	b := img.Bounds()
	if b.Dx() != g.Width || b.Dy() != g.Height {
		return nil, 0, fmt.Errorf("raster is %dx%d, expected %dx%d", b.Dx(), b.Dy(), g.Width, g.Height)
	}

	values := make([]uint16, g.Width*g.Height)
	switch src := img.(type) {
	case *image.Gray:
		for y := 0; y < g.Height; y++ {
			row := src.Pix[y*src.Stride : y*src.Stride+g.Width]
			out := values[y*g.Width : (y+1)*g.Width]
			for x, v := range row {
				out[x] = uint16(v)
			}
		}
		return values, 8, nil

	case *image.Gray16:
		// Gray16.Pix is big-endian pairs.
		for y := 0; y < g.Height; y++ {
			row := src.Pix[y*src.Stride : y*src.Stride+g.Width*2]
			out := values[y*g.Width : (y+1)*g.Width]
			for x := 0; x < g.Width; x++ {
				out[x] = uint16(row[x*2])<<8 | uint16(row[x*2+1])
			}
		}
		return values, 16, nil

	default:
		return nil, 0, fmt.Errorf("unexpected raster type %T (expected single-band grey)", img)
	}
}
