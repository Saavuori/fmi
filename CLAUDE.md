# CLAUDE.md

## What this is

Tutka — one live weather map of Finland on the Finnish Meteorological Institute's
open data, with three modes: **Tutka** (radar composites, animated), **Salama**
(lightning strikes), **Havainnot** (automatic weather stations). Go backend +
React 19/Vite/MapLibre frontend, shipped as a single container with the frontend
embedded via `go:embed`. Deliberate sibling of the Fintraffic live traffic map —
same architecture, same CI-owned versioning, same deploy.

## Architecture

- One Go binary (`backend/cmd/fmi`). `internal/core/` is mode-agnostic: `cache`
  (generic TTL key/value over Redis with an in-memory fallback), `config`,
  `upstream` (FMI HTTP client), `fmiwfs` (the multipointcoverage parser, shared
  because lightning and observations use the same encoding), `server` (router,
  embedded SPA, global `/api/health` + `/api/version` + `/metrics`).
- Each mode is a package (`internal/tutka/`, `internal/salama/`,
  `internal/havainnot/`) implementing `server.Mode` (`Name`, `Register`, `Health`)
  and mounting routes under `/api/<mode>/`. The global health endpoint aggregates
  every mode's `Health()` under `modes.<name>`.
- Every mode is poll-style: **pollers write to the Store, handlers only read from
  the Store, and nothing a visitor does triggers an upstream request.** For radar
  this is not merely tidy — FMI's terms require it (see below).
- Frontend: `src/App.tsx` is the mode-switcher shell; `src/modes/<mode>/` owns
  each mode; `src/shared/` holds mode-agnostic hooks/components. No router, no
  state library — plain hooks. New modes get `src/modes/<mode>/` and an entry in
  `MODES` in App.tsx.

## FMI data contract — the things that are not guessable

These were established against the live API and are easy to get wrong. **Do not
"simplify" them without re-checking against the real service.**

- **Never point the app's browser code at FMI's WMS.** Their manual states plainly
  that direct use of the WMS for anything but evaluation is not allowed, and that
  radar data for a web app must be downloaded via the download service and served
  by the application itself. The whole poller/archive design follows from this.
- **Fetch rasters, not pictures.** `styles=raster&format=image/geotiff` returns the
  raw single-band values. FMI's own rendered PNG paints "no echo" as **opaque
  white** and out-of-coverage as light grey, which would blank out the basemap —
  and a colour cannot be turned back into a dBZ figure, so the point readout and
  the nowcast would be impossible.
- **Bit depth differs per product.** `dbz` is 8-bit; every rain product (`rr`,
  `rr1h`, `rr12h`, `rr24h`) is **16-bit**. This decides the no-coverage sentinel
  and roughly doubles the bytes per frame.
- **`nodata` is the maximum value, `undetect` is zero.** 255/65535 means "outside
  radar coverage"; 0 means "inside coverage, nothing detected". Both draw
  transparent, but they are opposite answers to "is it raining here" — see
  `SampleState`. Conflating them lets the app claim dry weather over an area it
  cannot see.
- **Read the linear transformation per response, never hardcode it.** `value =
  raw*gain + offset`, and gain/offset genuinely differ per product (dbz 0.5/−32,
  the rain products 0.01/0). They arrive as `linearTransformationGain` /
  `…Offset` `om:NamedValue` entries in the WFS response.
- **GeoServer honours `crs=EPSG:3857`**, so frames arrive already in the projection
  MapLibre draws in and drop into an image source with no warp error. An EPSG:4326
  raster would *not*: latitude is nonlinear in mercator and across 1200 km the
  error is visible.
- **Resampling phase is request-dependent.** Two GetMap requests for the same area
  at different output sizes can disagree by about a pixel, because GeoServer
  resamples from a different internal level. Frames are all fetched with identical
  parameters so the archive is self-consistent; do not compare a frame against a
  differently-sized request and conclude the georeferencing is wrong.
- **`openwms.fmi.fi` gzips regardless of what was asked for.** Never set
  `Accept-Encoding` by hand — Go's transport only decompresses transparently for
  the header it added itself. `core/upstream` also unwraps explicitly.
- **Weather-station sentinels.** Snow depth encodes "no snow" as **−1** and "could
  not determine" as **−3** instead of omitting them. See `havainnot.normalize`.
- **Attribution is a licence condition.** FMI open data is CC BY 4.0; every mode
  puts the credit in the map's attribution control.
- **Rate limits**: 20 000 download and 10 000 view requests/day, 600 per 5 minutes
  combined. Steady state here is ~4 requests per 5 minutes; the boot backfill
  paces itself at one fetch per 1.5 s.

## Tutka specifics

- **One canonical grid** (`grid.go`) serves the poller, the store, the renderer,
  the point sampler and the frontend corners. Default is EPSG:3857
  `2000000,8250000,3600000,11200000` at 2000 m/px → 800×1475, about 1.0 km on the
  ground at 60°N. Changing `GRID_BBOX`/`GRID_RESOLUTION` makes every archived
  frame geometrically wrong for the new setting, so `Store.Open()` compares the
  stored `meta.json` grid and **discards the archive** on a mismatch.
- **Frames are files, not database rows** — `<dir>/<product>/<YYYYMMDD>/<HHMM>.png`
  as single-channel PNG at native depth. This is the one deliberate departure from
  the Fintraffic sibling's SQLite trail store: pruning a few thousand 100–600 KB
  blobs must not need a `VACUUM`, which wants scratch space equal to the database,
  on a host whose root volume runs near full. As files, pruning is unlink and `du`
  tells the truth.
- **Disk is the binding constraint.** At the default grid and retention the archive
  is ~650 MB when full (measured, not estimated — see `logBudget`). Retentions are
  env-tunable per product; `/api/health` exposes `disk_bytes`.
- **The nowcast is deliberately modest.** One global motion vector by
  cross-correlation, then advection. It reports its own correlation as a
  confidence and the UI withholds the arrival time below 0.55, because a single
  vector describes a front well and scattered convection badly. It is labelled an
  extrapolation, never a forecast.

## Adding a mode

Backend package with a `Service` implementing `server.Mode` — pollers writing typed
snapshots through a `Store` over the core cache, handlers reading only from the
store; frontend module under `src/modes/<mode>/` mounted from the shell with theme
passed down as props, plus a scoped `<mode>.css` (`.mode-<name>` on the app root)
for what the shared design system doesn't cover. Keep types duplicated
backend/frontend in sync ("change one, change both").

## Conventions & gotchas

- **Versioning is CI-owned**: GitHub Actions auto-tags semver from conventional
  commits (`fix:` patch, `feat:` minor, `feat!:` major) on push to main and injects
  version metadata via ldflags (`-X fmi/internal/core/server.Version=...`). Never
  hand-tag.
- **Changelog**: real versioned headings (`## [vX.Y.Z] - date`) matching the CI tag
  — never `[Unreleased]`. `scripts/build-changelog.js` renders it to a GitHub Pages
  site. **Every PR must add a changelog entry** (CI-enforced on PRs to main), even
  for chores/CI-only changes — a PR with no user- or ops-facing change at all is
  the only exception. Since the CI tag isn't known until after merge, predict it
  yourself: read the current top heading in `CHANGELOG.md`, bump it using the same
  conventional-commit rule CI uses (worst bump wins if the PR mixes types), and add
  your new heading *above* the current top one with today's date. If the prediction
  turns out wrong after merge, fix the heading in a follow-up rather than leaving
  it mismatched.
- **Dependencies**: self-hosted Renovate (`renovate.json5` +
  `.github/workflows/renovate.yml`) watches all five pinned surfaces —
  `backend/go.mod`, `frontend/package.json`, GitHub Actions, the Dockerfile, and
  `deploy/` compose images — as one grouped weekly PR, majors split out. It writes
  its own changelog entry via a `postUpgradeTasks` command
  (`scripts/changelog-entry.js`); that text is factual only, so expand it by hand
  when a bump actually matters. Adding a new kind of pinned version means adding a
  pattern to `SOURCES` in that script. Needs `RENOVATE_APP_ID` and
  `RENOVATE_APP_PRIVATE_KEY` secrets — *not* `GITHUB_TOKEN`, whose PRs don't fire
  `pull_request` workflows, so required checks would never report. See README,
  "Dependency updates".
- **MapLibre**: the map mounts once behind a ref guard; cleanup on unmount is
  load-bearing under StrictMode. `setStyle()` discards added sources — re-add them
  after style/theme switches. Refs holding latest props are written in an effect,
  not during render (the lint rules enforce this, and a render can be discarded
  after a write).
- **Embedded frontend**: `//go:embed all:dist` lives in `internal/core/server`; the
  Dockerfile copies the Vite build into `internal/core/server/dist/`. A placeholder
  `dist/index.html` must stay tracked so dev builds compile.
- **Redis is a cache, not a store**: the app must keep working when Redis is down
  (in-memory fallback). The only persistent data is the radar frame archive on a
  named volume.
- **Deploys**: rootless Podman on an Oracle Cloud host, shared Caddy (a container
  in the ratikka stack) on the external `web-proxy` network terminates TLS;
  `deploy/update.sh` cron-pulls new images every 5 minutes (no Watchtower).

## Commands

```bash
# backend (from backend/)
go run ./cmd/fmi --no-redis        # writes frames to /data/frames unless FRAMES_DIR is set
go test ./...

# frontend (from frontend/)
npm run dev        # proxies /api to :8080
npm run build
npx vitest run
npm run lint
```
