package salama

import (
	"context"
	"net/url"
	"time"

	"fmi/internal/core/fmiwfs"
	"fmi/internal/core/upstream"
)

const (
	wfsBase     = "https://opendata.fmi.fi/wfs"
	storedQuery = "fmi::observations::lightning::multipointcoverage"
)

// Field names in the lightning coverage's rangeType, in FMI's spelling.
const (
	fieldMultiplicity   = "multiplicity"
	fieldPeakCurrent    = "peak_current"
	fieldCloudIndicator = "cloud_indicator"
)

// fetchStrikes downloads the strikes in [from, to].
//
// No bbox is sent: the stored query already covers the Finnish detection
// network's area, and FMI's lightning feed is small enough that filtering
// server-side buys nothing while risking dropping strikes just outside a
// hand-drawn box that still matter for a map of Finland.
func fetchStrikes(ctx context.Context, client *upstream.Client, from, to time.Time) ([]Strike, error) {
	q := url.Values{}
	q.Set("service", "WFS")
	q.Set("version", "2.0.0")
	q.Set("request", "getFeature")
	q.Set("storedquery_id", storedQuery)
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

	strikes := make([]Strike, 0, len(mp.Samples))
	for _, s := range mp.Samples {
		strike := Strike{
			Latitude:     s.Lat,
			Longitude:    s.Lon,
			Timestamp:    s.Time.Unix(),
			Multiplicity: 1,
		}
		if v, ok := mp.Field(s, fieldMultiplicity); ok && v >= 1 {
			strike.Multiplicity = int(v)
		}
		if v, ok := mp.Field(s, fieldPeakCurrent); ok {
			strike.PeakCurrent = v
		}
		// FMI encodes 1 for an intra-cloud flash and 0 for cloud-to-ground.
		if v, ok := mp.Field(s, fieldCloudIndicator); ok && v >= 0.5 {
			strike.CloudFlash = true
		}
		strikes = append(strikes, strike)
	}
	return strikes, nil
}
