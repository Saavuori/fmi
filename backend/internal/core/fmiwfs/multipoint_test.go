package fmiwfs

import (
	"math"
	"strings"
	"testing"
	"time"
)

// obsSample mirrors the shape of an fmi::observations::weather::multipointcoverage
// response: two stations, two timestamps each, three parameters, with one missing
// reading written as NaN the way FMI writes it.
const obsSample = `<?xml version="1.0" encoding="UTF-8"?>
<wfs:FeatureCollection xmlns:wfs="http://www.opengis.net/wfs/2.0"
    xmlns:om="http://www.opengis.net/om/2.0"
    xmlns:omso="http://inspire.ec.europa.eu/schemas/omso/3.0"
    xmlns:gml="http://www.opengis.net/gml/3.2"
    xmlns:gmlcov="http://www.opengis.net/gmlcov/1.0"
    xmlns:swe="http://www.opengis.net/swe/2.0"
    xmlns:sam="http://www.opengis.net/sampling/2.0"
    xmlns:sams="http://www.opengis.net/samplingSpatial/2.0"
    xmlns:target="http://xml.fmi.fi/namespace/om/atmosphericfeatures/1.1"
    xmlns:xlink="http://www.w3.org/1999/xlink">
  <wfs:member>
    <omso:GridSeriesObservation gml:id="obs">
      <om:featureOfInterest>
        <sams:SF_SpatialSamplingFeature gml:id="sf">
          <sam:sampledFeature>
            <target:LocationCollection gml:id="lc">
              <target:member>
                <target:Location gml:id="obsloc-1">
                  <gml:identifier codeSpace="http://xml.fmi.fi/namespace/stationcode/fmisid">100683</gml:identifier>
                  <gml:name codeSpace="http://xml.fmi.fi/namespace/locationcode/name">Porvoo Kilpilahti satama</gml:name>
                  <gml:name codeSpace="http://xml.fmi.fi/namespace/locationcode/geoid">-16777356</gml:name>
                  <gml:name codeSpace="http://xml.fmi.fi/namespace/locationcode/wmo">2994</gml:name>
                  <target:region codeSpace="http://xml.fmi.fi/namespace/location/region">Porvoo</target:region>
                </target:Location>
              </target:member>
              <target:member>
                <target:Location gml:id="obsloc-2">
                  <gml:identifier codeSpace="http://xml.fmi.fi/namespace/stationcode/fmisid">100907</gml:identifier>
                  <gml:name codeSpace="http://xml.fmi.fi/namespace/locationcode/name">Jomala Maarianhamina lentoasema</gml:name>
                  <gml:name codeSpace="http://xml.fmi.fi/namespace/locationcode/wmo">2970</gml:name>
                  <target:region codeSpace="http://xml.fmi.fi/namespace/location/region">Jomala</target:region>
                </target:Location>
              </target:member>
            </target:LocationCollection>
          </sam:sampledFeature>
          <sams:shape>
            <gml:MultiPoint gml:id="mp">
              <gml:pointMember>
                <gml:Point gml:id="point-100683"><gml:pos>60.30373 25.54916</gml:pos></gml:Point>
              </gml:pointMember>
              <gml:pointMember>
                <gml:Point gml:id="point-100907"><gml:pos>60.12000 19.90000</gml:pos></gml:Point>
              </gml:pointMember>
            </gml:MultiPoint>
          </sams:shape>
        </sams:SF_SpatialSamplingFeature>
      </om:featureOfInterest>
      <om:result>
        <gmlcov:MultiPointCoverage gml:id="cov">
          <gml:domainSet>
            <gmlcov:SimpleMultiPoint gml:id="smp">
              <gmlcov:positions>
                60.30373 25.54916 1784944200
                60.30373 25.54916 1784944800
                60.12000 19.90000 1784944200
                60.12000 19.90000 1784944800
              </gmlcov:positions>
            </gmlcov:SimpleMultiPoint>
          </gml:domainSet>
          <gml:rangeSet>
            <gml:DataBlock>
              <gml:doubleOrNilReasonTupleList>
                16.2 4.4 NaN
                16.1 4.2 0.1
                14.0 7.1 0.0
                13.8 6.9 NaN
              </gml:doubleOrNilReasonTupleList>
            </gml:DataBlock>
          </gml:rangeSet>
          <gmlcov:rangeType>
            <swe:DataRecord>
              <swe:field name="t2m" xlink:href="t2m"/>
              <swe:field name="ws_10min" xlink:href="ws_10min"/>
              <swe:field name="r_1h" xlink:href="r_1h"/>
            </swe:DataRecord>
          </gmlcov:rangeType>
        </gmlcov:MultiPointCoverage>
      </om:result>
    </omso:GridSeriesObservation>
  </wfs:member>
</wfs:FeatureCollection>`

func TestParseZipsPositionsAgainstValues(t *testing.T) {
	mp, err := Parse([]byte(obsSample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if want := []string{"t2m", "ws_10min", "r_1h"}; len(mp.Fields) != 3 ||
		mp.Fields[0] != want[0] || mp.Fields[2] != want[2] {
		t.Fatalf("Fields = %v, want %v", mp.Fields, want)
	}
	if len(mp.Samples) != 4 {
		t.Fatalf("got %d samples, want 4", len(mp.Samples))
	}

	first := mp.Samples[0]
	if first.Lat != 60.30373 || first.Lon != 25.54916 {
		t.Errorf("first sample at (%v, %v), want (60.30373, 25.54916)", first.Lat, first.Lon)
	}
	if want := time.Unix(1784944200, 0).UTC(); !first.Time.Equal(want) {
		t.Errorf("first sample time = %v, want %v", first.Time, want)
	}

	// The third row belongs to the second station: if positions and values were
	// zipped in the wrong order, this would carry Porvoo's temperature.
	if v, ok := mp.Field(mp.Samples[2], "t2m"); !ok || v != 14.0 {
		t.Errorf("third sample t2m = %v (ok=%v), want 14.0", v, ok)
	}
	if v, ok := mp.Field(mp.Samples[2], "ws_10min"); !ok || v != 7.1 {
		t.Errorf("third sample ws_10min = %v, want 7.1", v)
	}
}

// A missing reading must be reported as absent, never as zero: 0.0 mm of rain and
// "this station does not measure rain" are different claims.
func TestFieldRejectsNaN(t *testing.T) {
	mp, err := Parse([]byte(obsSample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if _, ok := mp.Field(mp.Samples[0], "r_1h"); ok {
		t.Error("a NaN reading should report ok=false")
	}
	if math.IsNaN(mp.Samples[0].Values[2]) == false {
		t.Error("the raw value should still be NaN so callers can see it was present-but-missing")
	}
	// A genuine zero must survive.
	if v, ok := mp.Field(mp.Samples[2], "r_1h"); !ok || v != 0 {
		t.Errorf("a real 0.0 reading = %v (ok=%v), want 0 and ok", v, ok)
	}
	if _, ok := mp.Field(mp.Samples[0], "not_requested"); ok {
		t.Error("an unknown field should report ok=false")
	}
}

// A response whose halves disagree must be rejected: zipping a short tuple list
// against a long position list would attach readings to the wrong places.
func TestParseRejectsMismatchedHalves(t *testing.T) {
	broken := strings.Replace(obsSample, "13.8 6.9 NaN", "", 1)
	if _, err := Parse([]byte(broken)); err == nil {
		t.Fatal("expected an error when the tuple list is short")
	}
}

func TestParseLocationsJoinsByPointID(t *testing.T) {
	locs, err := ParseLocations([]byte(obsSample))
	if err != nil {
		t.Fatalf("ParseLocations: %v", err)
	}
	if len(locs) != 2 {
		t.Fatalf("got %d locations, want 2", len(locs))
	}

	byID := map[string]Location{}
	for _, l := range locs {
		byID[l.FMISID] = l
	}

	porvoo, ok := byID["100683"]
	if !ok {
		t.Fatal("station 100683 is missing")
	}
	// The human-readable name lives in the "/name" codeSpace; picking a gml:name by
	// position would yield the geoid or WMO number instead.
	if porvoo.Name != "Porvoo Kilpilahti satama" {
		t.Errorf("name = %q, want the /name codeSpace value", porvoo.Name)
	}
	if porvoo.Region != "Porvoo" {
		t.Errorf("region = %q, want Porvoo", porvoo.Region)
	}
	// Coordinates come from the shape branch, matched on point-<fmisid>.
	if porvoo.Lat != 60.30373 || porvoo.Lon != 25.54916 {
		t.Errorf("coords = (%v, %v), want (60.30373, 25.54916)", porvoo.Lat, porvoo.Lon)
	}

	// The second station has no geoid name, so the parser must not depend on a
	// fixed number of gml:name entries.
	if byID["100907"].Name != "Jomala Maarianhamina lentoasema" {
		t.Errorf("second station name = %q", byID["100907"].Name)
	}
}

func TestParseSurfacesExceptions(t *testing.T) {
	body := `<ExceptionReport xmlns="http://www.opengis.net/ows/1.1">
  <Exception exceptionCode="InvalidParameterValue">
    <ExceptionText>No data available for the requested parameters</ExceptionText>
  </Exception>
</ExceptionReport>`
	if _, err := Parse([]byte(body)); err == nil {
		t.Fatal("expected an error for an exception report")
	} else if !strings.Contains(err.Error(), "No data available") {
		t.Errorf("error %q should quote the upstream text", err)
	}
	if _, err := ParseLocations([]byte(body)); err == nil {
		t.Fatal("ParseLocations should also surface the exception")
	}
}

// Lightning responses carry positions but no station branch, and must parse
// without one.
func TestParseWithoutLocations(t *testing.T) {
	lightning := `<wfs:FeatureCollection xmlns:wfs="http://www.opengis.net/wfs/2.0"
    xmlns:om="http://www.opengis.net/om/2.0"
    xmlns:omso="http://inspire.ec.europa.eu/schemas/omso/3.0"
    xmlns:gml="http://www.opengis.net/gml/3.2"
    xmlns:gmlcov="http://www.opengis.net/gmlcov/1.0"
    xmlns:swe="http://www.opengis.net/swe/2.0">
  <wfs:member><omso:GridSeriesObservation gml:id="o"><om:result>
    <gmlcov:MultiPointCoverage gml:id="c">
      <gml:domainSet><gmlcov:SimpleMultiPoint gml:id="s">
        <gmlcov:positions>61.5 24.1 1784986409</gmlcov:positions>
      </gmlcov:SimpleMultiPoint></gml:domainSet>
      <gml:rangeSet><gml:DataBlock>
        <gml:doubleOrNilReasonTupleList>2 -14.5 0</gml:doubleOrNilReasonTupleList>
      </gml:DataBlock></gml:rangeSet>
      <gmlcov:rangeType><swe:DataRecord>
        <swe:field name="multiplicity"/><swe:field name="peak_current"/><swe:field name="cloud_indicator"/>
      </swe:DataRecord></gmlcov:rangeType>
    </gmlcov:MultiPointCoverage>
  </om:result></omso:GridSeriesObservation></wfs:member>
</wfs:FeatureCollection>`

	mp, err := Parse([]byte(lightning))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(mp.Samples) != 1 {
		t.Fatalf("got %d samples, want 1", len(mp.Samples))
	}
	if v, ok := mp.Field(mp.Samples[0], "peak_current"); !ok || v != -14.5 {
		t.Errorf("peak_current = %v, want -14.5 (negative polarity must survive)", v)
	}
	if v, ok := mp.Field(mp.Samples[0], "cloud_indicator"); !ok || v != 0 {
		t.Errorf("cloud_indicator = %v, want 0 (cloud-to-ground)", v)
	}

	locs, err := ParseLocations([]byte(lightning))
	if err != nil {
		t.Fatalf("ParseLocations on a lightning response: %v", err)
	}
	if len(locs) != 0 {
		t.Errorf("got %d locations, want none", len(locs))
	}
}
