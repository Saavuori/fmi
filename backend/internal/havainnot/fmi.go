package havainnot

import (
	"context"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"fmi/internal/core/fmiwfs"
	"fmi/internal/core/upstream"
)

const (
	wfsBase     = "https://opendata.fmi.fi/wfs"
	storedQuery = "fmi::observations::weather::multipointcoverage"

	// Finland plus a margin of sea, as lon/lat. This matches the radar grid's
	// footprint so the two modes cover the same country.
	bboxParam = "19,59,32,71,EPSG::4326"
)

// fetchObservations downloads station readings for [from, to] and folds them into
// per-station series.
//
// The time window is always bounded. FMI's default window returns every station's
// full recent history for every requested parameter, which measured 2.4 MB for
// three parameters — with ten it would be far worse, and almost all of it
// discarded.
func fetchObservations(ctx context.Context, client *upstream.Client, from, to time.Time) ([]StationDetail, error) {
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", "2.0.0")
	q.Set("request", "getFeature")
	q.Set("storedquery_id", storedQuery)
	q.Set("bbox", bboxParam)
	q.Set("parameters", strings.Join(ParameterCodes(), ","))
	q.Set("starttime", from.UTC().Format(time.RFC3339))
	q.Set("endtime", to.UTC().Format(time.RFC3339))

	body, err := client.GetURL(ctx, wfsBase+"?"+q.Encode())
	if err != nil {
		return nil, err
	}

	mp, err := fmiwfs.Parse(body)
	if err != nil {
		return nil, err
	}
	locations, err := fmiwfs.ParseLocations(body)
	if err != nil {
		return nil, err
	}

	return assemble(mp, locations), nil
}

// assemble joins the flat sample list to the station list and collapses it into
// one series per station.
//
// Samples carry coordinates but no station id, and the station branch carries ids
// and coordinates but no readings, so position is the only join key available.
// Coordinates are quantised to five decimals (about a metre) because the two
// branches format the same number with different precision.
func assemble(mp *fmiwfs.MultiPoint, locations []fmiwfs.Location) []StationDetail {
	byPos := make(map[[2]int64]*StationDetail, len(locations))
	order := make([]*StationDetail, 0, len(locations))

	for _, loc := range locations {
		d := &StationDetail{Station: Station{
			FMISID:    loc.FMISID,
			Name:      loc.Name,
			Region:    loc.Region,
			Latitude:  loc.Lat,
			Longitude: loc.Lon,
			Values:    map[string]float64{},
		}}
		byPos[posKey(loc.Lat, loc.Lon)] = d
		order = append(order, d)
	}

	for _, s := range mp.Samples {
		station, ok := byPos[posKey(s.Lat, s.Lon)]
		if !ok {
			// A reading whose position matches no station in the same response is
			// unattributable; dropping it beats guessing which station it belongs to.
			continue
		}

		values := make(map[string]float64, len(mp.Fields))
		for _, p := range Parameters {
			if v, ok := mp.Field(s, p.Code); ok && !math.IsNaN(v) {
				values[p.Code] = v
			}
		}
		if len(values) == 0 {
			// An all-missing row is a placeholder FMI emits for a timestamp the
			// station did not report; keeping it would draw a gap as a data point.
			continue
		}

		ts := s.Time.Unix()
		station.Series = append(station.Series, Reading{Timestamp: ts, Values: values})
	}

	out := make([]StationDetail, 0, len(order))
	for _, d := range order {
		if len(d.Series) == 0 {
			// A station that reported nothing in the window would render as a
			// marker with no data behind it.
			continue
		}
		sort.Slice(d.Series, func(i, j int) bool { return d.Series[i].Timestamp < d.Series[j].Timestamp })

		// The station's headline values are the most recent reading of each
		// parameter, not the last row: stations report parameters at different
		// cadences, so the newest row alone would blank out the slower ones.
		for _, reading := range d.Series {
			for code, v := range reading.Values {
				d.Values[code] = v
			}
			if reading.Timestamp > d.Timestamp {
				d.Timestamp = reading.Timestamp
			}
		}
		out = append(out, *d)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func posKey(lat, lon float64) [2]int64 {
	return [2]int64{int64(math.Round(lat * 1e5)), int64(math.Round(lon * 1e5))}
}
