package salama

// Strike is one lightning discharge.
//
// The frontend mirrors this in src/modes/salama/lib/strikes.ts — change one,
// change both.
type Strike struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	// Timestamp is unix seconds. Strikes are drawn faded by age, so the client
	// needs the instant rather than a formatted string.
	Timestamp int64 `json:"timestamp"`
	// Multiplicity is how many discharges the detection network merged into this
	// record; 1 for a single stroke.
	Multiplicity int `json:"multiplicity"`
	// PeakCurrent is in kiloamperes, negative for the common
	// negative-cloud-to-ground polarity. Its magnitude is what indicates severity.
	PeakCurrent float64 `json:"peakCurrent"`
	// CloudFlash is true for an intra-cloud discharge, false for cloud-to-ground.
	// Only the latter is a hazard at ground level, so they are drawn differently.
	CloudFlash bool `json:"cloudFlash"`
}

// StrikeSummary is the response shape: the strikes plus enough context for the
// frontend to describe the window without recomputing it.
type StrikeSummary struct {
	Strikes []Strike `json:"strikes"`
	// WindowMinutes is how far back the retained set reaches.
	WindowMinutes int `json:"windowMinutes"`
	// Updated is unix seconds of the last successful poll.
	Updated int64 `json:"updated"`
}
