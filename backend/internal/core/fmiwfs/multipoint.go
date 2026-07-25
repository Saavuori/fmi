// Package fmiwfs parses the multipointcoverage encoding FMI's WFS uses for
// point observations.
//
// It lives in core rather than in a mode because two modes consume the exact
// same encoding: lightning strikes and weather-station observations differ only
// in which parameters they ask for. The format is fiddly enough — a flat list of
// positions zipped against a flat list of value tuples, with station identity
// held in a separate branch of the document — that parsing it twice would be a
// reliable way to get one of the copies subtly wrong.
package fmiwfs

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Sample is one row of a multipointcoverage: where and when, plus the parameter
// values in the same order as MultiPoint.Fields. Missing readings are NaN, which
// is how FMI writes them, and callers must check rather than assume.
type Sample struct {
	Lat, Lon float64
	Time     time.Time
	Values   []float64
}

// MultiPoint is a decoded coverage.
type MultiPoint struct {
	Fields  []string
	Samples []Sample
}

// Field returns the value of a named parameter for a sample, reporting false when
// the field is absent or the reading is missing.
func (m *MultiPoint) Field(s Sample, name string) (float64, bool) {
	for i, f := range m.Fields {
		if f != name {
			continue
		}
		if i >= len(s.Values) {
			return 0, false
		}
		v := s.Values[i]
		if math.IsNaN(v) {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

// Location is a weather station's identity and position, taken from the
// featureOfInterest branch. Lightning responses carry no locations.
type Location struct {
	FMISID string
	Name   string
	Region string
	Lat    float64
	Lon    float64
}

// coverageDoc is the subset of the WFS response we read. Fields match on local
// name only, so the om:/gml:/gmlcov:/swe:/target: prefixes need not be spelled
// out — FMI is not consistent about which namespace prefix carries which element
// across stored queries.
type coverageDoc struct {
	XMLName xml.Name `xml:"FeatureCollection"`
	Members []struct {
		Observation struct {
			Result struct {
				Coverage struct {
					DomainSet struct {
						MultiPoint struct {
							Positions string `xml:"positions"`
						} `xml:"SimpleMultiPoint"`
					} `xml:"domainSet"`
					RangeSet struct {
						DataBlock struct {
							Tuples string `xml:"doubleOrNilReasonTupleList"`
						} `xml:"DataBlock"`
					} `xml:"rangeSet"`
					RangeType struct {
						DataRecord struct {
							Fields []struct {
								Name string `xml:"name,attr"`
							} `xml:"field"`
						} `xml:"DataRecord"`
					} `xml:"rangeType"`
				} `xml:"MultiPointCoverage"`
			} `xml:"result"`
			FeatureOfInterest struct {
				SamplingFeature struct {
					SampledFeature struct {
						LocationCollection struct {
							Members []struct {
								Location struct {
									Identifier string `xml:"identifier"`
									Names      []struct {
										CodeSpace string `xml:"codeSpace,attr"`
										Value     string `xml:",chardata"`
									} `xml:"name"`
									Region string `xml:"region"`
								} `xml:"Location"`
							} `xml:"member"`
						} `xml:"LocationCollection"`
					} `xml:"sampledFeature"`
					Shape struct {
						MultiPoint struct {
							PointMembers []struct {
								Point struct {
									ID  string `xml:"id,attr"`
									Pos string `xml:"pos"`
								} `xml:"Point"`
							} `xml:"pointMember"`
						} `xml:"MultiPoint"`
					} `xml:"shape"`
				} `xml:"SF_SpatialSamplingFeature"`
			} `xml:"featureOfInterest"`
		} `xml:"GridSeriesObservation"`
	} `xml:"member"`
}

type exceptionDoc struct {
	XMLName xml.Name `xml:"ExceptionReport"`
	Text    string   `xml:"Exception>ExceptionText"`
}

// checkException turns FMI's error envelope into a Go error. It is worth checking
// explicitly because some failures arrive with HTTP 200, so a successful fetch is
// not the same as a successful query.
func checkException(body []byte) error {
	if !bytes.Contains(body, []byte("ExceptionReport")) {
		return nil
	}
	var exc exceptionDoc
	if err := xml.Unmarshal(body, &exc); err != nil {
		return fmt.Errorf("fmi wfs returned an unparseable exception report")
	}
	text := strings.TrimSpace(exc.Text)
	if text == "" {
		text = "unspecified WFS exception"
	}
	return fmt.Errorf("fmi wfs exception: %s", text)
}

// Parse decodes a multipointcoverage response.
//
// The two halves have to line up: positions is "lat lon epoch" triples and the
// tuple list is one row of values per triple. A response whose halves disagree is
// rejected rather than truncated, because silently zipping a short tuple list
// against a long position list would attach readings to the wrong places.
func Parse(body []byte) (*MultiPoint, error) {
	if err := checkException(body); err != nil {
		return nil, err
	}

	var doc coverageDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse multipointcoverage: %w", err)
	}

	out := &MultiPoint{}
	for _, m := range doc.Members {
		cov := m.Observation.Result.Coverage

		fields := make([]string, 0, len(cov.RangeType.DataRecord.Fields))
		for _, f := range cov.RangeType.DataRecord.Fields {
			fields = append(fields, f.Name)
		}
		if len(out.Fields) == 0 {
			out.Fields = fields
		}

		positions := strings.Fields(cov.DomainSet.MultiPoint.Positions)
		if len(positions)%3 != 0 {
			return nil, fmt.Errorf("positions list has %d entries, not a multiple of 3", len(positions))
		}
		count := len(positions) / 3

		values := strings.Fields(cov.RangeSet.DataBlock.Tuples)
		width := len(fields)
		if width == 0 {
			// No rangeType means no parameters to read; positions alone are still
			// meaningful for lightning-style "it happened here" data.
			width = 0
		}
		if width > 0 {
			if len(values) != count*width {
				return nil, fmt.Errorf("tuple list has %d values, expected %d positions x %d fields", len(values), count, width)
			}
		}

		for i := 0; i < count; i++ {
			lat, err := strconv.ParseFloat(positions[i*3], 64)
			if err != nil {
				continue
			}
			lon, err := strconv.ParseFloat(positions[i*3+1], 64)
			if err != nil {
				continue
			}
			epoch, err := strconv.ParseInt(positions[i*3+2], 10, 64)
			if err != nil {
				continue
			}

			s := Sample{Lat: lat, Lon: lon, Time: time.Unix(epoch, 0).UTC()}
			if width > 0 {
				s.Values = make([]float64, width)
				for j := 0; j < width; j++ {
					// ParseFloat accepts "NaN", which is exactly how FMI marks a
					// missing reading, so the NaN flows through as-is.
					v, err := strconv.ParseFloat(values[i*width+j], 64)
					if err != nil {
						v = math.NaN()
					}
					s.Values[j] = v
				}
			}
			out.Samples = append(out.Samples, s)
		}
	}

	return out, nil
}

// ParseLocations decodes the weather-station metadata.
//
// Station identity and station coordinates live in different branches of the
// document and are joined by a `point-<fmisid>` id, so this does that join rather
// than assuming the two lists happen to be in the same order.
func ParseLocations(body []byte) ([]Location, error) {
	if err := checkException(body); err != nil {
		return nil, err
	}

	var doc coverageDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse locations: %w", err)
	}

	var out []Location
	seen := make(map[string]bool)

	for _, m := range doc.Members {
		foi := m.Observation.FeatureOfInterest.SamplingFeature

		coords := make(map[string][2]float64)
		for _, pm := range foi.Shape.MultiPoint.PointMembers {
			id := strings.TrimPrefix(pm.Point.ID, "point-")
			parts := strings.Fields(pm.Point.Pos)
			if len(parts) < 2 {
				continue
			}
			lat, err1 := strconv.ParseFloat(parts[0], 64)
			lon, err2 := strconv.ParseFloat(parts[1], 64)
			if err1 != nil || err2 != nil {
				continue
			}
			coords[id] = [2]float64{lat, lon}
		}

		for _, member := range foi.SampledFeature.LocationCollection.Members {
			loc := member.Location
			fmisid := strings.TrimSpace(loc.Identifier)
			if fmisid == "" || seen[fmisid] {
				continue
			}

			// A Location carries several gml:name entries distinguished only by
			// codeSpace (name, geoid, wmo); the human-readable one is the
			// "/name" space, and picking by position would give a numeric id.
			var name string
			for _, n := range loc.Names {
				if strings.HasSuffix(n.CodeSpace, "/name") {
					name = strings.TrimSpace(n.Value)
					break
				}
			}
			if name == "" {
				continue
			}

			pos, ok := coords[fmisid]
			if !ok {
				continue
			}

			seen[fmisid] = true
			out = append(out, Location{
				FMISID: fmisid,
				Name:   name,
				Region: strings.TrimSpace(loc.Region),
				Lat:    pos[0],
				Lon:    pos[1],
			})
		}
	}

	return out, nil
}
