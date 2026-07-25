package tutka

import (
	"strings"
	"testing"
	"time"
)

// wfsSample is trimmed from a real fmi::radar::composite::dbz response. It keeps
// the namespace prefixes and the nesting exactly as FMI sends them, because the
// parser matches on local names and that behaviour is what is being tested.
const wfsSample = `<?xml version="1.0" encoding="UTF-8"?>
<wfs:FeatureCollection timeStamp="2026-07-25T13:32:55Z" numberMatched="2" numberReturned="2"
    xmlns:wfs="http://www.opengis.net/wfs/2.0"
    xmlns:om="http://www.opengis.net/om/2.0"
    xmlns:omso="http://inspire.ec.europa.eu/schemas/omso/3.0"
    xmlns:gml="http://www.opengis.net/gml/3.2"
    xmlns:xlink="http://www.w3.org/1999/xlink">
  <wfs:member>
    <omso:GridSeriesObservation gml:id="a">
      <om:phenomenonTime>
        <gml:TimeInstant gml:id="t0"><gml:timePosition>2026-07-25T12:35:00Z</gml:timePosition></gml:TimeInstant>
      </om:phenomenonTime>
      <om:resultTime>
        <gml:TimeInstant gml:id="r0"><gml:timePosition>2026-07-25T12:35:00Z</gml:timePosition></gml:TimeInstant>
      </om:resultTime>
      <om:procedure xlink:href="http://xml.fmi.fi/inspire/process/suomi_dbz_eureffin"/>
      <om:parameter>
        <om:NamedValue>
          <om:name xlink:href="https://inspire.ec.europa.eu/codeList/ProcessParameterValue/value/radar/linearTransformationGain"/>
          <om:value><gml:Measure uom="double">0.5</gml:Measure></om:value>
        </om:NamedValue>
      </om:parameter>
      <om:parameter>
        <om:NamedValue>
          <om:name xlink:href="https://inspire.ec.europa.eu/codeList/ProcessParameterValue/value/radar/linearTransformationOffset"/>
          <om:value><gml:Measure uom="double">-32</gml:Measure></om:value>
        </om:NamedValue>
      </om:parameter>
    </omso:GridSeriesObservation>
  </wfs:member>
  <wfs:member>
    <omso:GridSeriesObservation gml:id="b">
      <om:phenomenonTime>
        <gml:TimeInstant gml:id="t1"><gml:timePosition>2026-07-25T12:30:00Z</gml:timePosition></gml:TimeInstant>
      </om:phenomenonTime>
    </omso:GridSeriesObservation>
  </wfs:member>
</wfs:FeatureCollection>`

func TestParseWFSFramesReadsTransformationPerFrame(t *testing.T) {
	dbz, _ := ProductByID("dbz")
	frames, err := parseWFSFrames([]byte(wfsSample), dbz)
	if err != nil {
		t.Fatalf("parseWFSFrames: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}

	// Ascending order, so the 12:30 member that arrived second must come first.
	if want := time.Date(2026, 7, 25, 12, 30, 0, 0, time.UTC); !frames[0].Time.Equal(want) {
		t.Errorf("frames[0].Time = %v, want %v", frames[0].Time, want)
	}
	if want := time.Date(2026, 7, 25, 12, 35, 0, 0, time.UTC); !frames[1].Time.Equal(want) {
		t.Errorf("frames[1].Time = %v, want %v", frames[1].Time, want)
	}

	// The member carrying an explicit transformation must use it...
	if frames[1].Gain != 0.5 || frames[1].Offset != -32 {
		t.Errorf("frames[1] gain/offset = %v/%v, want 0.5/-32", frames[1].Gain, frames[1].Offset)
	}
	// ...and the member without one must fall back to the product default rather
	// than to a zero gain, which would flatten every value to the offset.
	if frames[0].Gain != dbz.Gain || frames[0].Offset != dbz.Offset {
		t.Errorf("frames[0] gain/offset = %v/%v, want the product default %v/%v",
			frames[0].Gain, frames[0].Offset, dbz.Gain, dbz.Offset)
	}
}

// The rain products use a different transformation from reflectivity, which is
// exactly why it is read per response instead of hardcoded.
func TestRainProductsHaveTheirOwnScaling(t *testing.T) {
	dbz, _ := ProductByID("dbz")
	rr, _ := ProductByID("rr")
	if dbz.Gain == rr.Gain && dbz.Offset == rr.Offset {
		t.Fatal("dbz and rr should not share a transformation")
	}
	if rr.Gain != 0.01 || rr.Offset != 0 {
		t.Errorf("rr gain/offset = %v/%v, want 0.01/0", rr.Gain, rr.Offset)
	}
	if rr.Bits != 16 || dbz.Bits != 8 {
		t.Errorf("depths are dbz=%d rr=%d, want 8 and 16", dbz.Bits, rr.Bits)
	}
}

// FMI returns some failures with HTTP 200, so a successful fetch is not a
// successful query and the parser has to say so.
func TestParseWFSFramesSurfacesExceptions(t *testing.T) {
	body := `<?xml version="1.0"?>
<ExceptionReport xmlns="http://www.opengis.net/ows/1.1" version="2.0.0">
  <Exception exceptionCode="OperationParsingFailed">
    <ExceptionText>Invalid time interval!</ExceptionText>
  </Exception>
</ExceptionReport>`
	dbz, _ := ProductByID("dbz")
	if _, err := parseWFSFrames([]byte(body), dbz); err == nil {
		t.Fatal("expected an error for an exception report")
	} else if !strings.Contains(err.Error(), "Invalid time interval") {
		t.Errorf("error %q should quote the upstream text", err)
	}
}

func TestGetMapURLRequestsRawValuesInWebMercator(t *testing.T) {
	dbz, _ := ProductByID("dbz")
	g := defaultGrid()
	u := getMapURL(dbz, g, time.Date(2026, 7, 25, 13, 5, 0, 0, time.UTC))

	// styles=raster is what makes this a data fetch rather than a picture fetch.
	for _, want := range []string{
		"styles=raster",
		"format=image%2Fgeotiff",
		"crs=EPSG%3A3857",
		"width=800",
		"height=1475",
		"bbox=2000000%2C8250000%2C3600000%2C11200000",
		"time=2026-07-25T13%3A05%3A00Z",
		"layers=suomi_dbz_eureffin",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("GetMap URL is missing %q\n  got: %s", want, u)
		}
	}
}
