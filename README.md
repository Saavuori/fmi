# 🌧️ Tutka — Live Finnish Weather (Tutka · Salama · Havainnot)

[![Live](https://img.shields.io/badge/live-tutka.duckdns.org-2563eb)](https://tutka.duckdns.org/)
[![Changelog](https://img.shields.io/badge/changelog-GitHub%20Pages-a78bfa)](https://saavuori.github.io/fmi/)

A live weather map of Finland built on the Finnish Meteorological Institute's open
data: animated radar composites, with lightning strikes and the automatic weather
station network as layers you switch on over them — one map, one container, no
build step at runtime.

Sibling of [Fintraffic](https://github.com/Saavuori/Fintraffic): same Go +
React/MapLibre architecture, same CI-owned versioning, same deploy.

## Architecture

```
backend/
  cmd/fmi/main.go                 # wiring: cache, modes, graceful shutdown
  internal/core/                  # mode-agnostic
    cache/                        # generic TTL key/value: Redis + in-memory fallback
    config/                       # env + .env + flags
    upstream/                     # HTTP client (User-Agent, gzip unwrap)
    fmiwfs/                       # multipointcoverage parser (shared by 2 modes)
    places/                       # place search: Nominatim proxy, cached + paced
    server/                       # router, embedded SPA, health/version/metrics
  internal/tutka/                 # radar: grid, palettes, archive, nowcast
  internal/salama/                # lightning
  internal/havainnot/             # weather stations
frontend/
  src/App.tsx                     # shell: theme owner, nothing else
  src/WeatherMap.tsx              # the app: radar + layer toggles + panels
  src/map/                        # the one MapLibre instance, basemap, layer order
  src/radar/                      # radar API client, loop, styles
  src/layers/{salama,havainnot}/  # overlay layers: own their sources, render no DOM
  src/components/                 # panels, transport bar
  src/shared/                     # feature-agnostic hooks + components
scripts/                          # changelog renderer + Renovate entry generator
.github/workflows/                # PR checks, release, Pages, Renovate
deploy/                           # compose + installer + cron updater
Dockerfile                        # vite build -> go build w/ embed -> alpine
```

Each mode implements one interface and mounts itself under `/api/<mode>/`:

```go
type Mode interface {
	Name() string
	Register(mux *http.ServeMux)
	Health(ctx context.Context) ModeHealth
}
```

Every mode is poll-style: **pollers write to a Store, handlers only read from the
Store, and nothing a visitor does triggers an upstream FMI request.**

The backend still has three modes; the *frontend* no longer does. Radar is the
base layer and the other two are overlays on the same map, toggled from one panel
— so a `/api/<mode>/` namespace is an answer the map can draw, not a screen you
navigate to.

Place search is the one exception to the poll rule, and the only endpoint a
visitor can cause an outbound request from. It is not FMI data and cannot be
polled ahead of time, so it caches for a day and paces itself to one upstream
request per second (see `internal/core/places`).

## ✨ Tutka (weather radar) — the base layer

- **Animated nationwide radar**, 5-minute cadence, scrubbable over the full week
  FMI retains. Play/pause, step, speed, and a live pill that goes dark when you
  have scrubbed into the past.
- **Five composite products**: reflectivity (dBZ), rain rate (mm/h), and 1 h / 12 h
  / 24 h accumulations (mm).
- **The backend holds raw values, not pictures.** It downloads the single-band
  rasters and colours them itself, which is what makes real transparency possible
  and what allows a point readout in physical units.
- **Tap anywhere** for the dBZ and rain rate at that spot, the last hour as a
  sparkline, and a 45-minute extrapolation of when rain reaches you.
- **Palettes we own**, including a colour-vision-deficiency variant per product
  that carries intensity by lightness rather than the green-to-red pairing.
- **"No rain" and "no radar coverage" are never merged.** Both draw transparent,
  but only the first is evidence of dry weather, and the readout says which.

## ✨ Salama (lightning) — an overlay

Strikes from the last two hours, coloured by age so the eye lands on where the
storm *is* and the older strikes read as its track. Cloud-to-ground strikes are
filled dots and intra-cloud flashes are rings — only the former is a ground-level
hazard. Peak current and multiplicity on tap. An empty map is the normal state in
Finland, and the panel says so rather than looking broken. Over the radar it is a
different question answered: which of the cells on screen is actually electrified.

## ✨ Havainnot (weather stations) — an overlay

Roughly 194 automatic stations, with markers coloured and labelled by whichever of
ten parameters you pick — temperature, wind, gusts, direction, humidity, hourly
rain, dew point, pressure, visibility, snow depth. Stations that do not measure the
selected parameter are left off the map rather than drawn as grey noise. Tap for
every reading plus the last three hours.

## ✨ Place search

A search button on the map takes a Finnish place name and flies there, at a zoom
chosen for what was found — a *maakunta* and a street are both "places" and do not
want the same frame. Results are subtitled with their town so the several
Kaisaniemis can be told apart.

Both layers start switched off, and which ones are on is remembered.

## Data sources

All weather data is the Finnish Meteorological Institute's open data, licensed
**CC BY 4.0**, credited in the map's attribution control. No API key is needed for
anything here.

| Layer | Service | Used for |
|---|---|---|
| Tutka | `opendata.fmi.fi/wfs`, stored queries `fmi::radar::composite::{dbz,rr,rr1h,rr12h,rr24h}` | Which frames exist, and their value scaling |
| Tutka | `openwms.fmi.fi/geoserver/Radar/wms` GetMap, `styles=raster&format=image/geotiff` | The raster values themselves |
| Salama | `fmi::observations::lightning::multipointcoverage` | Strike position, time, current, polarity |
| Havainnot | `fmi::observations::weather::multipointcoverage` | Station readings and metadata |
| Place search | `nominatim.openstreetmap.org/search` | Name → coordinates. OpenStreetMap data, **ODbL**, credited in the results list |

### Why the app re-serves radar images instead of proxying FMI

FMI's manual is explicit that direct use of their WMS for anything beyond
evaluation is not allowed, and that an application wanting radar data must download
it through the download service and serve the images itself. This app does exactly
that: a poller fetches each new frame once, stores it, and every visitor is served
from that local archive. Steady-state upstream traffic is about four requests per
five minutes, against a documented ceiling of 600 — and the boot-time backfill
paces itself at one fetch per 1.5 s.

Fetching the raw rasters rather than FMI's rendered images is also what makes the
app work at all: their default rendering paints "no echo" as opaque white, which
would hide the basemap, and a colour cannot be converted back into a dBZ figure.

### Why place search goes through the backend

Nominatim is free and needs no key, and asks in return for a descriptive
User-Agent, no more than one request a second, no per-keystroke autocomplete, and
results cached rather than re-asked. Proxying it is what makes all four possible:
the browser searches on submit, the backend serialises every outbound call through
a one-per-second gate, and answers — including "no matches" — are served from the
shared cache for a day.

## HTTP API

See [docs/API.md](docs/API.md).

## Technical stack

Go (stdlib `http.ServeMux`, `golang.org/x/image/tiff`, `go-redis`, Prometheus
client), Redis 8 as a pure cache, React 19 + Vite + MapLibre GL 6, CARTO basemaps.
Frontend embedded into the binary with `go:embed`; multi-arch image built and
released by GitHub Actions.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `REDIS_URL` | `redis://fmi-cache:6379` | Redis connection |
| `NO_REDIS` | unset | `true` (or `--no-redis`) uses the in-memory cache |
| `PORT` | `8080` | HTTP listen port |
| `FRAMES_DIR` | `/data/frames` | Radar frame archive. Empty disables persistence |
| `TUTKA_RETENTION_HOURS` | `dbz=168,rr=48,rr1h=168,rr12h=168,rr24h=168` | Per-product archive depth |
| `GRID_BBOX` | `2000000,8250000,3600000,11200000` | Frame extent, EPSG:3857 |
| `GRID_RESOLUTION` | `2000` | Mercator metres per pixel |

A `.env` is auto-loaded from the working directory, its parent, or `backend/`.

**On disk use.** At the defaults the archive settles around **650 MB** once every
retention window is full, and the mode logs its measured projection at startup and
exposes `disk_bytes` on `/api/health`. Changing `GRID_BBOX` or `GRID_RESOLUTION`
makes existing frames geometrically wrong for the new setting, so the store detects
the mismatch and discards the archive rather than serving misplaced weather.

## Local development

### 1. Run the backend (no Redis needed)

```bash
cd backend
FRAMES_DIR=./frames go run ./cmd/fmi --no-redis
```

The first radar frame lands within about a minute; the archive fills in from there.

### 2. Run the frontend dev server

```bash
cd frontend
npm install && npm run dev
```

Serves on `:5173` and proxies `/api` to `:8080`.

## Deployment

### Local (Docker)

```bash
docker build -t tutka .
docker run -p 8080:8080 -e NO_REDIS=true -v tutka-frames:/data tutka
```

### Production (RHEL & rootless Podman)

```bash
curl -fsSL https://raw.githubusercontent.com/Saavuori/fmi/main/deploy/install.sh | bash
```

`install.sh` is idempotent: it writes the compose file and `update.sh`, ensures the
external `web-proxy` network and the frame volume exist, registers the 5-minute
auto-update cron, pulls the image, recreates the stack, and then verifies that the
archive is actually recording. Override with `DOMAIN`, `APP_DIR` or `IMAGE`.

TLS is **not** part of this stack: a shared Caddy on the `web-proxy` network
terminates it. Add the vhost and reload Caddy yourself — it only picks up a
Caddyfile edit on `caddy reload`:

```
tutka.duckdns.org {
    reverse_proxy fmi-backend:8080
    encode gzip zstd
}
```

`install.sh` handles first-time setup and config changes; `update.sh` only pulls a
new image and redeploys when the digest changes.

## Dependency updates

A self-hosted Renovate runs weekly (Mondays 04:00 UTC) and opens **one grouped PR**
across all five pinned surfaces — `backend/go.mod`, `frontend/package.json`, GitHub
Actions, the `Dockerfile`, and the `deploy/` compose images. Majors are split out so
each gets a real review. It writes its own changelog entry via a
`postUpgradeTasks` command.

### Required setup

Two repository secrets: **`RENOVATE_APP_ID`** and **`RENOVATE_APP_PRIVATE_KEY`**,
from a GitHub App installed on this repo with `contents: write`,
`pull-requests: write` and `workflows: write`.

It must **not** be `GITHUB_TOKEN`: PRs opened by the built-in token do not fire
`pull_request` workflows, so the required checks would never report and the PR could
never merge.

Renovate is self-hosted rather than using the Mend app because `postUpgradeTasks`
runs arbitrary commands, which the hosted app does not allow —
`RENOVATE_ALLOWED_COMMANDS` in the workflow is the actual security boundary, and it
is an anchored regex matching exactly one script.

Run it by hand from the Actions tab (`workflow_dispatch`), optionally with
`dryRun` to see what it would do.
