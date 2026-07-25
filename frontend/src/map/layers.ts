import type * as maplibregl from 'maplibre-gl';

// Every layer id drawn on the one map, and the order they stack in.
//
// There is a single map now, so the layers are added and removed at whatever
// moment the viewer flicks a toggle — which means insertion order says nothing
// about draw order. `beforeLayer` resolves that: a layer is always inserted
// under the nearest successor that happens to be mounted, so the stack below
// holds however the toggles were reached.

export const RADAR_LAYER = 'radar-raster';
export const STATIONS_LAYER = 'stations-circles';
export const STATION_LABELS_LAYER = 'stations-labels';
export const STRIKES_GLOW_LAYER = 'strikes-glow';
export const STRIKES_LAYER = 'strikes-circles';
export const PROBE_LAYER = 'probe-point';

/**
 * Bottom to top. Radar is a full-bleed raster so everything reads on top of it;
 * strikes go above stations because a flash is a moment and a station reading is
 * a background fact; the probe pin stays on top because it marks what the viewer
 * just asked about.
 */
export const DRAW_ORDER: readonly string[] = [
  RADAR_LAYER,
  STATIONS_LAYER,
  STATION_LABELS_LAYER,
  STRIKES_GLOW_LAYER,
  STRIKES_LAYER,
  PROBE_LAYER,
];

/**
 * The `beforeId` to insert `id` at: the first layer above it in DRAW_ORDER that
 * is currently on the map, or undefined (append on top) when none is.
 */
export function beforeLayer(map: maplibregl.Map, id: string): string | undefined {
  const rank = DRAW_ORDER.indexOf(id);
  if (rank < 0) return undefined;
  return DRAW_ORDER.slice(rank + 1).find(candidate => map.getLayer(candidate));
}

/** Removes layers and their source, tolerating any of them already being gone —
 * a style swap discards the lot before the cleanup that owns them runs. */
export function removeLayers(map: maplibregl.Map, source: string, ...layers: string[]) {
  for (const layer of layers) {
    if (map.getLayer(layer)) map.removeLayer(layer);
  }
  if (map.getSource(source)) map.removeSource(source);
}
