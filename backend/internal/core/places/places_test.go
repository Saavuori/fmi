package places

import (
	"context"
	"testing"
	"time"

	"fmi/internal/core/cache"
)

// A trimmed but otherwise verbatim jsonv2 response: a city, a village whose name
// repeats elsewhere in the country, and a street. The three cover what the
// parser has to decide — which subtitle disambiguates, and how far to zoom.
const sampleResponse = `[
  {
    "lat": "60.1674881", "lon": "24.9427473",
    "name": "Helsinki",
    "display_name": "Helsinki, Uusimaa, Manner-Suomi, Suomi",
    "addresstype": "city", "type": "city",
    "address": {"city": "Helsinki", "state": "Uusimaa", "country": "Suomi", "country_code": "fi"}
  },
  {
    "lat": "62.8924", "lon": "27.6782",
    "name": "Vehmasmäki",
    "display_name": "Vehmasmäki, Kuopio, Pohjois-Savo, Itä-Suomi, Suomi",
    "addresstype": "village", "type": "village",
    "address": {"village": "Vehmasmäki", "municipality": "Kuopio", "state": "Pohjois-Savo", "country": "Suomi"}
  },
  {
    "lat": "60.1699", "lon": "24.9384",
    "name": "Mannerheimintie",
    "display_name": "Mannerheimintie, Kluuvi, Helsinki, Uusimaa, Suomi",
    "addresstype": "road", "type": "primary",
    "address": {"road": "Mannerheimintie", "city": "Helsinki", "state": "Uusimaa"}
  }
]`

func TestParseResults(t *testing.T) {
	got, err := parseResults([]byte(sampleResponse))
	if err != nil {
		t.Fatalf("parseResults: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d places, want 3", len(got))
	}

	want := []Place{
		{Name: "Helsinki", Detail: "Uusimaa", Latitude: 60.1674881, Longitude: 24.9427473, Zoom: 10},
		{Name: "Vehmasmäki", Detail: "Kuopio", Latitude: 62.8924, Longitude: 27.6782, Zoom: 12},
		{Name: "Mannerheimintie", Detail: "Helsinki", Latitude: 60.1699, Longitude: 24.9384, Zoom: 14},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("place %d = %+v, want %+v", i, got[i], w)
		}
	}
}

// The trap this endpoint is most likely to be "simplified" back into: OSM's
// Finnish addresses put the seutukunta in `municipality`, so preferring that
// field over `city` labels a Helsinki quarter "Helsingin seutukunta" — which
// disambiguates nothing, since every other hit in the capital region says the
// same. Verbatim address from a live `q=kaisaniemi` response.
func TestDetailPrefersCityOverTheSeutukunta(t *testing.T) {
	r := nominatimPlace{
		Name:        "Kaisaniemi",
		AddressType: "quarter",
		DisplayName: "Kaisaniemi, Kluuvi, Helsinki, Uusimaa, Suomi",
		Address: map[string]string{
			"quarter":      "Kaisaniemi",
			"suburb":       "Kluuvi",
			"city":         "Helsinki",
			"municipality": "Helsingin seutukunta",
			"state":        "Uusimaa",
			"region":       "Manner-Suomi",
			"country":      "Suomi",
		},
	}
	if got := detailFor(r, "Kaisaniemi"); got != "Helsinki" {
		t.Errorf("detailFor = %q, want %q", got, "Helsinki")
	}
}

// The subtitle must never repeat the name: "Helsinki · Helsinki" tells a viewer
// choosing between two hits nothing at all.
func TestDetailNeverRepeatsTheName(t *testing.T) {
	r := nominatimPlace{
		Name:        "Kuopio",
		DisplayName: "Kuopio, Pohjois-Savo, Itä-Suomi, Suomi",
		Address:     map[string]string{"city": "Kuopio", "state": "Pohjois-Savo"},
	}
	if got := detailFor(r, "Kuopio"); got != "Pohjois-Savo" {
		t.Errorf("detailFor = %q, want %q", got, "Pohjois-Savo")
	}
}

// Falls back to the display name when there is no structured address, dropping
// the name itself and the country — neither says anything on a map of Finland.
func TestDetailFallsBackToDisplayName(t *testing.T) {
	r := nominatimPlace{
		Name:        "Nuorgam",
		DisplayName: "Nuorgam, Utsjoki, Lappi, Suomi",
	}
	if got := detailFor(r, "Nuorgam"); got != "Utsjoki, Lappi" {
		t.Errorf("detailFor = %q, want %q", got, "Utsjoki, Lappi")
	}
}

func TestParseResultsSkipsUnusableRecords(t *testing.T) {
	// No coordinates to fly to, and no name to show: both are unusable rather
	// than partially usable, and one bad record must not fail the search.
	body := `[
      {"lat": "not-a-number", "lon": "24.9", "name": "Rikki"},
      {"lat": "60.1", "lon": "24.9", "name": "", "display_name": ""},
      {"lat": "61.4991", "lon": "23.7871", "name": "Tampere", "addresstype": "city"}
    ]`
	got, err := parseResults([]byte(body))
	if err != nil {
		t.Fatalf("parseResults: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Tampere" {
		t.Fatalf("got %+v, want only Tampere", got)
	}
}

// A record with no short name borrows the first component of the display name,
// which is what Nominatim returns for a plain address.
func TestParseResultsBorrowsNameFromDisplayName(t *testing.T) {
	body := `[{"lat": "60.45", "lon": "22.27", "display_name": "Aurakatu 1, Turku, Suomi", "addresstype": "building"}]`
	got, err := parseResults([]byte(body))
	if err != nil {
		t.Fatalf("parseResults: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Aurakatu 1" {
		t.Fatalf("got %+v, want name %q", got, "Aurakatu 1")
	}
	if got[0].Zoom != 15 {
		t.Errorf("zoom = %v, want 15", got[0].Zoom)
	}
}

// The gate is what keeps this inside Nominatim's one-request-per-second rule, so
// it is worth asserting rather than trusting: the first caller goes straight
// through, the second is made to wait.
func TestAcquireSpacesRequests(t *testing.T) {
	s := NewService(cache.NewMemoryCache())
	ctx := context.Background()

	start := time.Now()
	if err := s.acquire(ctx); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("first acquire waited %v, want ~0", elapsed)
	}

	start = time.Now()
	if err := s.acquire(ctx); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if elapsed := time.Since(start); elapsed < minInterval/2 {
		t.Errorf("second acquire waited %v, want at least %v", elapsed, minInterval/2)
	}
}

// A queue longer than maxQueue is refused rather than held: the viewer has a
// search box open, and an answer that arrives after they have given up is worse
// than being told to try again.
func TestAcquireRefusesALongQueue(t *testing.T) {
	s := NewService(cache.NewMemoryCache())
	s.next = time.Now().Add(maxQueue + time.Second)

	if err := s.acquire(context.Background()); err != errBusy {
		t.Errorf("acquire = %v, want errBusy", err)
	}
}

// Cached searches must not touch the gate at all — that is the whole point of
// caching them.
func TestSearchServesFromCacheWithoutUpstream(t *testing.T) {
	c := cache.NewMemoryCache()
	s := NewService(c)
	ctx := context.Background()

	payload := []byte(`[{"name":"Oulu","detail":"Pohjois-Pohjanmaa","latitude":65.0,"longitude":25.5,"zoom":10}]`)
	if err := c.SetValue(ctx, cacheKey("  OULU  "), payload, time.Minute); err != nil {
		t.Fatalf("seeding cache: %v", err)
	}

	// Nothing stubs the network here: if this reached Nominatim the test would
	// either hang on the gate or fail on the request.
	got, err := s.Search(ctx, "oulu")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Oulu" {
		t.Fatalf("got %+v, want the cached Oulu", got)
	}
}

// Whitespace and case are not part of the question being asked.
func TestCacheKeyNormalises(t *testing.T) {
	if a, b := cacheKey("  Helsinki   Kaisaniemi "), cacheKey("helsinki kaisaniemi"); a != b {
		t.Errorf("cacheKey mismatch: %q vs %q", a, b)
	}
}
