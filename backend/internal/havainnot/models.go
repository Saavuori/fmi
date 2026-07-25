package havainnot

// Parameter is one observed quantity. Keeping the FMI code, a Finnish label and a
// unit together means the frontend can render a parameter picker and a legend
// without duplicating FMI's vocabulary.
//
// The frontend mirrors this in src/modes/havainnot/lib/observations.ts — change
// one, change both.
type Parameter struct {
	Code  string `json:"code"` // FMI's parameter name, e.g. "t2m"
	Label string `json:"label"`
	Unit  string `json:"unit"`
}

// Parameters is the set polled for every station. It is deliberately short: each
// additional parameter widens every row of every station's time series, and the
// untrimmed response is already megabytes.
var Parameters = []Parameter{
	{Code: "t2m", Label: "Lämpötila", Unit: "°C"},
	{Code: "ws_10min", Label: "Tuuli", Unit: "m/s"},
	{Code: "wg_10min", Label: "Puuska", Unit: "m/s"},
	{Code: "wd_10min", Label: "Tuulen suunta", Unit: "°"},
	{Code: "rh", Label: "Kosteus", Unit: "%"},
	{Code: "r_1h", Label: "Sade 1 h", Unit: "mm"},
	{Code: "td", Label: "Kastepiste", Unit: "°C"},
	{Code: "p_sea", Label: "Paine", Unit: "hPa"},
	{Code: "vis", Label: "Näkyvyys", Unit: "m"},
	{Code: "snow_aws", Label: "Lumensyvyys", Unit: "cm"},
}

// ParameterCodes returns the codes in table order, for the WFS query.
func ParameterCodes() []string {
	out := make([]string, 0, len(Parameters))
	for _, p := range Parameters {
		out = append(out, p.Code)
	}
	return out
}

// Station is one weather station's latest reading.
//
// Values is keyed by FMI parameter code and only contains parameters the station
// actually reported: most stations measure a subset, and a missing key is
// meaningfully different from a zero. Omitting them rather than sending nulls
// keeps the payload for ~200 stations small.
type Station struct {
	FMISID    string             `json:"fmisid"`
	Name      string             `json:"name"`
	Region    string             `json:"region,omitempty"`
	Latitude  float64            `json:"latitude"`
	Longitude float64            `json:"longitude"`
	Timestamp int64              `json:"timestamp"` // unix seconds of the reading
	Values    map[string]float64 `json:"values"`
}

// StationDetail adds the recent history of one station, for the detail panel.
type StationDetail struct {
	Station
	// Series is oldest-first. Each entry carries only the parameters reported at
	// that instant, for the same reason as Station.Values.
	Series []Reading `json:"series"`
}

// Reading is one station observation at one instant.
type Reading struct {
	Timestamp int64              `json:"timestamp"`
	Values    map[string]float64 `json:"values"`
}

// Snapshot is the response shape for the station list.
type Snapshot struct {
	Stations   []Station   `json:"stations"`
	Parameters []Parameter `json:"parameters"`
	Updated    int64       `json:"updated"`
}
