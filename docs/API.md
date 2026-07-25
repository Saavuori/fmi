# Tutka HTTP API

All responses are JSON unless noted, sent with `Access-Control-Allow-Origin: *`.
There is no authentication.

**Cold start.** A mode that has not completed its first successful poll answers
`503` with `data not available yet`. This is a documented loading state, not an
error: the frontend shows a spinner for it. It matters most for the radar, whose
first frame lands within about a minute of boot.

**Attribution.** Every mode returns an `attribution` string. The data is the
Finnish Meteorological Institute's open data under CC BY 4.0, so displaying it is a
licence condition rather than a courtesy.

---

## Global

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/health` | Aggregate health, with each mode's own report under `modes.<name>` |
| GET | `/api/version` | Build `version`, `build_date`, `git_sha` (injected via ldflags) |
| GET | `/metrics` | Prometheus metrics |
| GET | `/` | The embedded SPA |

### GET /api/health

`status` is `degraded` if Redis is unreachable or any mode reports degraded.

```json
{
  "status": "healthy",
  "redis_connected": true,
  "uptime_seconds": 3600,
  "modes": {
    "tutka": {
      "status": "healthy",
      "details": {
        "archive_enabled": true,
        "frames_on_disk": 2540,
        "disk_bytes": 512000000,
        "backfill_running": false,
        "backfilled_frames": 1980,
        "primary_poll_age_sec": 12,
        "grid": { "width": 800, "height": 1475, "resolution_m": 2000 },
        "products": {
          "dbz": { "frames": 2016, "retain_hours": 168, "poll_age_sec": 12, "latest": "2026-07-25T14:45:00Z" }
        }
      }
    },
    "salama": { "status": "healthy", "details": { "strikes_retained": 68, "window_minutes": 120, "poll_age_sec": 30 } },
    "havainnot": { "status": "healthy", "details": { "stations": 194, "poll_age_sec": 120 } }
  }
}
```

`poll_age_sec` is `-1` until a mode's first successful poll. Watch `disk_bytes`:
the archive grows for its first week and settles around 650 MB at the default grid
and retention.

---

## Tutka (`/api/tutka`) — radar composites

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/tutka/products` | Products, units, cadence, and the grid the frames are drawn on |
| GET | `/api/tutka/frames` | The animation index for one product |
| GET | `/api/tutka/frame/{product}/{stamp}.png` | One coloured frame, transparent PNG |
| GET | `/api/tutka/legend/{product}` | The palette stops the pixels were coloured from |
| GET | `/api/tutka/point` | Values under a location, plus a short extrapolation |

Products: `dbz` (dBZ), `rr` (mm/h), `rr1h`, `rr12h`, `rr24h` (mm). Where a
`product` parameter is optional, it defaults to `dbz`.

### GET /api/tutka/products

`corners` is top-left, top-right, bottom-right, bottom-left as `[lon, lat]` —
exactly the order a MapLibre image source wants. Because the frames are fetched in
EPSG:3857, pinning them to these corners places them with no warp error.

```json
{
  "attribution": "Sadetutka: Ilmatieteen laitos, CC BY 4.0",
  "grid": {
    "width": 800,
    "height": 1475,
    "resolution_m": 2000,
    "bbox_epsg3857": [2000000, 8250000, 3600000, 11200000],
    "corners": [[17.966, 70.399], [32.339, 70.399], [32.339, 59.321], [17.966, 59.321]]
  },
  "products": [
    {
      "id": "dbz", "label": "Sadetutka", "unit": "dBZ",
      "step_minutes": 5, "palette": "dbz", "retain_hours": 168,
      "palettes": ["dbz", "dbz-cvd"], "frames": 2016,
      "latest": "2026-07-25T14:45:00Z"
    }
  ]
}
```

### GET /api/tutka/frames

| Parameter | Meaning |
|---|---|
| `product` | Product id; defaults to `dbz` |
| `from`, `to` | RFC3339 instant or a compact `20260725T1305Z` stamp; omit for unbounded |

An empty `frames` array means the product has frames but none in the requested
window — that is a valid answer. A `503` means the product has no frames at all yet.

```json
{
  "product": "dbz",
  "frames": [
    { "time": "2026-07-25T13:45:00Z", "stamp": "20260725T1345Z" },
    { "time": "2026-07-25T13:50:00Z", "stamp": "20260725T1350Z" }
  ]
}
```

### GET /api/tutka/frame/{product}/{stamp}.png

| Parameter | Meaning |
|---|---|
| `stamp` | Compact UTC stamp from the frames index, e.g. `20260725T1305Z` |
| `palette` | Optional palette id. A palette whose unit does not match the product is ignored in favour of the product's default, because serving a mm/h ramp for a dBZ raster would mislabel every pixel |

Returns `image/png` with alpha. Transparent where the radar detected nothing **and**
where it has no coverage — the two look identical here, and `/api/tutka/point` is
how you tell them apart.

Sent with `Cache-Control: public, max-age=31536000, immutable`: a past frame never
changes, so browsers and the reverse proxy absorb the repeat traffic of animating.

### GET /api/tutka/legend/{product}

Returns the palette stops used to colour the pixels, so a client-drawn scale
cannot drift from the map. The fully transparent fade-in anchor is omitted.

```json
{
  "product": "dbz", "palette": "dbz", "label": "Heijastavuus", "unit": "dBZ",
  "stops": [
    { "value": 8, "label": "8", "color": "#5abeff" },
    { "value": 15, "label": "15", "color": "#2ed8b6" }
  ],
  "attribution": "Sadetutka: Ilmatieteen laitos, CC BY 4.0"
}
```

Palettes: `dbz`, `rate`, `accum`, plus a `-cvd` variant of each that carries
intensity by lightness instead of the green-to-red pairing a red-green deficiency
cannot separate.

### GET /api/tutka/point

| Parameter | Meaning |
|---|---|
| `lat`, `lon` | WGS84 degrees (required) |
| `product` | Product id; defaults to `dbz` |

`state` is the important field:

| State | Meaning |
|---|---|
| `measured` | A real reading; `value` is set |
| `no_echo` | The radar looked and detected nothing — evidence of dry weather |
| `no_coverage` | Outside radar range — *not* evidence of anything |
| `outside_grid` | The point is off the raster entirely |

`value` is `null` for every state but `measured`. A client must not print a zero
for the others: "no rain here" and "we cannot see here" are different claims.

`nowcast` is present only for `dbz`, and only when the two most recent frames gave
a usable motion estimate. On a dry or non-translating field it is absent rather
than fabricated.

```json
{
  "lat": 60.17, "lon": 24.94, "product": "dbz", "unit": "dBZ",
  "current": { "time": "2026-07-25T14:45:00Z", "value": null, "state": "no_echo" },
  "series": [ { "time": "2026-07-25T13:50:00Z", "value": 23.5, "state": "measured" } ],
  "nowcast": {
    "motion": {
      "px_per_sec_x": 0.0133, "px_per_sec_y": -0.0133,
      "speed_kmh": 56.4, "bearing_deg": 45,
      "confidence": 0.91, "valid": true
    },
    "steps": [ { "minutes_ahead": 30, "value": 17.5, "state": "measured" } ],
    "arrival_minutes": 30,
    "method": "advection of the latest radar frame along a single measured motion vector"
  }
}
```

`series` covers the last hour. `bearing_deg` is the direction the rain is heading
towards, clockwise from north. `arrival_minutes` is when reflectivity reaches
15 dBZ over the point, or `-1` for none within 45 minutes.

**On trusting the nowcast.** It advects the current frame along one measured
vector: it does not model growth, decay or new convection. `confidence` is the
correlation of the best match — high for a front crossing as one body, low for
scattered showers. The app withholds the arrival time below 0.55, and a client
should do something similar rather than presenting it as a forecast.

---

## Salama (`/api/salama`) — lightning

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/salama/strikes` | Strikes retained over the last two hours |

```json
{
  "strikes": [
    { "latitude": 61.5, "longitude": 24.1, "timestamp": 1784986409,
      "multiplicity": 2, "peakCurrent": -14.5, "cloudFlash": false }
  ],
  "windowMinutes": 120,
  "updated": 1784986500,
  "attribution": "Salamahavainnot: Ilmatieteen laitos, CC BY 4.0"
}
```

`peakCurrent` is in kiloamperes, negative for the common negative
cloud-to-ground polarity; the magnitude is what indicates severity. `cloudFlash`
true is an intra-cloud discharge, which is not a hazard at ground level.
`multiplicity` is how many strokes the detection network merged into the record.

An empty `strikes` array is the normal state in Finland and does not indicate a
problem — a `503` is how "no data yet" is signalled.

---

## Havainnot (`/api/havainnot`) — weather stations

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/havainnot/stations` | Every station's latest readings |
| GET | `/api/havainnot/station/{fmisid}` | One station plus its recent series |

Parameters served: `t2m`, `ws_10min`, `wg_10min`, `wd_10min`, `rh`, `r_1h`, `td`,
`p_sea`, `vis`, `snow_aws`.

`values` contains **only the parameters a station actually reported**. A missing
key means "not measured here", which is different from a zero — most stations
measure a subset, and roughly 194 stations report at any time.

```json
{
  "stations": [
    {
      "fmisid": "100683", "name": "Porvoo Kilpilahti satama", "region": "Porvoo",
      "latitude": 60.30373, "longitude": 25.54916,
      "timestamp": 1784944800,
      "values": { "t2m": 16.1, "ws_10min": 4.2, "rh": 90 }
    }
  ],
  "parameters": [ { "code": "t2m", "label": "Lämpötila", "unit": "°C" } ],
  "updated": 1784944900,
  "attribution": "Säähavainnot: Ilmatieteen laitos, CC BY 4.0"
}
```

`GET /api/havainnot/station/{fmisid}` adds `series`, oldest-first, covering the
last three hours. Each entry carries only the parameters reported at that instant,
since stations report different quantities at different cadences. An unknown
`fmisid` is a `404` once data is loaded, and a `503` before then.

Snow depth is normalised on the way through: FMI encodes "no snow" as `-1` (served
as `0`) and "could not determine" as `-3` (omitted).
