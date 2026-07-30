# Tutka Changelog

All notable changes to this project will be documented in this file. Tutka is a live weather map of Finland built on the Finnish Meteorological Institute's open data, and is a sibling of the Fintraffic live traffic map — same Go + React/MapLibre single-container architecture, same CI-owned versioning.

## [v0.4.3] - 2026-07-30

### Changed
- **The ⓘ credit says the data is Nordic, not just Finnish**: the badge named Ilmatieteen laitos and the licence and stopped there, which quietly implies a national service behind a national map. It now reads *Ilmatieteen laitos, CC BY 4.0 — säädataa kaikkiin Pohjoismaihin*: the institute supplies weather data across all the Nordic countries, and that is worth one clause in the one place the map already explains where its pixels come from.

## [v0.4.2] - 2026-07-29

### Fixed
- **Switching the theme no longer wipes the lightning strikes off the map**: going light or dark left nothing but the basemap. `setStyle()` discards every custom source, so the map waits for the new style before putting the layers back — but it treated `styledata` as that signal, and `styledata` also fires for ordinary edits to the *outgoing* style, including the overlays tearing themselves down and re-adding in the same commit as the swap. So the rebuild ran a moment too early, against the style that was about to be thrown away, and then swallowed the real `style.load` it had unregistered itself from. The radar quietly recovered whenever the next frame arrived, which is what made this look like a lightning problem: the strikes and the station markers had no such second chance and stayed gone for the rest of the visit. The map now waits for `style.load` alone.

## [v0.4.1] - 2026-07-25

### Changed
- **The Ilmatieteen laitos credit is a small ⓘ badge above the locate button**: the attribution pill sat on its own in the bottom-left corner, a second thing anchored to the bottom edge of a map that already has a timeline along it. It is now a compact badge stacked directly on top of the locate control in the bottom-right, so the map's furniture reads as one column and the corner is clear. The credit itself is unchanged and one tap away — it is a licence condition, not decoration.

## [v0.4.0] - 2026-07-25

### Changed
- **The animation plays once and stops on the newest frame**: it used to wrap back to the start and run forever, which meant a map left open spent most of its time showing the past while looking like a live view — and the wrap itself, from "now" straight back to an hour ago, is the moment that misreading is easiest. It now runs once from the oldest frame in the window, dwells on the newest and stops there, so an unattended map is showing the current weather. Play from a finished run starts it over from the beginning.
- **The frame's time is above the map, not down in the transport**: the timestamp is the label that stops an hour-old frame being read as the current weather, and it used to sit at the bottom edge competing with the controls around it. It is now centred at the top of the map, in the reading path of someone watching a front move, with the weekday under it — which matters once the look-back runs to a week.
- **The look-back is chosen from the timeline**: `Aikaväli` — 1 h through 7 vrk — now sits at the right-hand end of the transport bar, in the room the removed buttons freed, rather than inside the map menu. It says how far back the scrubber beside it reaches, which is a property of the timeline and not of the map, and it was two clicks away behind a panel that had nothing else to do with time. On a phone the buttons take a row of their own beneath the scrubber. The bar stays up with the window buttons alone while a freshly picked window loads, rather than disappearing out from under the control that was just pressed.

### Removed
- **The step, live and speed buttons**: the transport is play, the scrubber and the look-back. Stepping a frame at a time is what a drag on the scrubber already does; the live pill had nothing left to jump back to now that the loop ends on the newest frame; and playback is fixed at 2 fps, which at five minutes per frame is the pace that reads as movement — the other rates only made the animation harder to follow.

## [v0.3.0] - 2026-07-25

### Added
- **Search for a place**: a search button on the map takes a Finnish place name and flies there. The zoom follows what was found — a *maakunta*, a town and a street do not want the same frame — and each hit is subtitled with its town, because Finland has a Kaisaniemi in Helsinki, three around Jyväskylä and one near Ylivieska, and a list of five identical words is not an answer. Names come from OpenStreetMap through a new `/api/places`, which is the only endpoint in the app a visitor can cause an outbound request from: it is not weather data and cannot be polled in advance. It caches every answer for a day, paces itself to one upstream request a second, and searches when you submit rather than as you type, which is what their usage policy asks for.
- **Lightning over the rain that is making it**: with both drawn on one map, the question "which of these cells is actually electrified" is now something you can look at rather than infer across a tab switch. Likewise the station readings, which used to be a separate screen from the radar sweeping over them.

### Changed
- **The three modes are one map with layers**: `Salama` and `Havainnot` are no longer separate apps reached from a bottom tab bar — they are overlays over the radar, switched on from the one panel, and which ones you leave on is remembered. The panel shows each layer's own controls only while that layer is on, so an unused layer costs one row rather than a screenful of parameter buttons, and a layer that is off makes no requests at all.
- **The bottom of a phone screen is the map's again**: the tab bar reserved a 64px band across every screen for navigation between three things nobody wanted separately. The timeline now reaches the bottom edge itself, and the locate button and attribution sit that much lower.
- **One readout at a time**: tapping a weather station opens its panel and closes the radar point readout, and tapping the map does the reverse. Both used to be able to claim the same right-hand rail. A tap that lands on a station is the station's alone — it no longer also drops a radar pin behind the panel that just opened.

### Removed
- **Three MapLibre instances, and three copies of the code that drove them**: each mode mounted its own map and carried its own version of the mount guard, the theme swap and the rebuild that a `setStyle()` demands. There is one map now; a layer says what it draws and when it is on, and the map republishes itself after a style change so the layers can put themselves back. The draw order is a single list rather than an accident of which tab you opened first.

## [v0.2.0] - 2026-07-25

### Changed
- **The timeline gets the whole width of the phone screen**: the transport bar is now docked edge to edge above the tab bar, with the buttons on one row and the scrubber alone on the row beneath it, and a thumb sized for a finger rather than a mouse. It used to be a centred pill sharing one line with the clock, the live button and the speed group, which left the scrubber about a third of the screen — an hour of five-minute frames in roughly two pixels each, on the one control a phone visitor actually drags. Nothing was hidden to buy the room: the wrap gives it back, and the speed buttons that used to be dropped on small screens are back with it.
- **The mobile panels are full-screen overlays instead of a bottom sheet**: the product picker, the legend and the point readout open over the map and close from their own header, rather than living in a sheet that was permanently parked along the bottom edge and dragged between peek, half and full. The filter panel is opened from a launcher button in the top-right corner; a map selection still opens the readout directly. The desktop rails are unchanged.

### Removed
- **The draggable bottom sheet, and the layout plumbing it needed**: snap-point physics, fling detection, the per-sheet height each one published to CSS and the rule that no two sheets could share the bottom edge — which is why picking a point on the radar had to fold the filter away first. What is left is one measured value, the docked timeline's height, which the locate button and the map attribution rest on.

## [v0.1.1] - 2026-07-25

### Fixed
- **The radar timeline no longer opens a hole in the middle while the archive fills**: the boot-time backfill asked FMI for history in day-long slices newest-slice-first, but then fetched the frames *within* each slice oldest-first. So a fresh deploy held yesterday afternoon plus the live poller’s last half hour, with a ten-hour gap between them — precisely the stretch a visitor scrubs first, and indistinguishable from the feed being broken. Frames are now fetched newest-first throughout, so the archive stays contiguous backwards from the present and only ever reaches further back.
- **A short timeline says it is short**: the transport bar now reports how far back the history actually goes when it falls well short of the selected window. Correct-but-still-filling looked exactly like missing data, which is what made the gap above so hard to read.

## [v0.1.0] - 2026-07-25

### Added
- **Radar, lightning and station observations on one map**: the app ships three modes over FMI's open data. `Tutka` animates the nationwide radar composites, `Salama` plots lightning strikes from the last two hours, and `Havainnot` shows the ~194 automatic weather stations. The mode switcher, theme handling and glass design system are shared, so a fourth mode is a directory and one array entry.
- **The radar archive holds raw values, not pictures**: FMI's own terms rule out pointing an application at their WMS — radar data has to be downloaded and then served by the application itself. So the backend fetches the single-band rasters behind each composite, keeps them as an archive of actual dBZ and mm/h values, and colours them on the way out. That is what makes real transparency possible (FMI's rendered PNG paints "no echo" opaque white, which would blank out the basemap), and it is what lets the app answer "what is falling on this exact spot" rather than only "what colour is this pixel".
- **Frames arrive already in Web Mercator**: GeoServer will reproject on request, so each frame is fetched on the same EPSG:3857 grid MapLibre draws in and drops into an image source with no warp error. The same grid definition serves the poller, the renderer, the point sampler and the frontend corners, so there is one place for that geometry to be right.
- **Seven days of scrubbing**: reflectivity is retained for the full week FMI keeps, rain rate for two days, and the hourly accumulations for a week. A boot-time backfill walks the history at one fetch per 1.5 s — about 40 requests a minute against a documented ceiling of 600 per five minutes — so catching up stays polite while it runs. Retention, grid extent and resolution are all environment variables.
- **"No coverage" and "no rain" are kept apart**: an empty pixel because the radar looked and found nothing is a different claim from an empty pixel outside radar range, and only the first is evidence of dry weather. Both render transparent, but the point readout reports which one it is rather than blurring them into a reassuring zero.
- **Palettes we own, including an accessible one**: reflectivity, rain rate and accumulation ramps, each with a colour-vision-deficiency variant that carries intensity by lightness instead of the green-to-red pairing the default ramps use. The legend is generated from the same stops the pixels were coloured from, so the scale cannot drift from the map.
- **Disk use is stated, not discovered**: the archive logs its current and projected size at startup, measured from the frames already on disk rather than from a guessed compression ratio, and `/api/health` exposes `disk_bytes`. The host's root volume runs close to full, so this is worth putting where an operator will see it.
- **The release build can be triggered by hand**: `CI/CD Build and Release` gains a `workflow_dispatch` trigger. The push filter cannot cover every case — the branch-creating push does not evaluate `paths-ignore` the way a later push does — and a release sometimes needs re-cutting after a docs-only fix.
- **Tap the map for what is falling on one spot**: the point readout gives the reflectivity and rain rate under a location, the last hour as a sparkline, and a 45-minute extrapolation of when rain reaches it. The extrapolation advects the current frame along a single motion vector measured by cross-correlating the last two frames, and it reports that correlation as a confidence — the app withholds the arrival time below 0.55 rather than dressing up a bad number, because one vector describes a front crossing as one body well and scattered convection badly. It is labelled an estimate, never a forecast.
