import { useEffect, useRef, useState } from 'react';
import type * as maplibregl from 'maplibre-gl';
import type { FeatureCollection, Point } from 'geojson';
import {
  STATIONS_LAYER,
  STATION_LABELS_LAYER,
  beforeLayer,
  removeLayers,
} from '../../map/layers';
import type { Theme } from '../../map/theme';
import {
  type HavainnotState,
  type Parameter,
  type Snapshot,
  type Station,
  SCALES,
  colorFor,
} from './observations';
import './havainnot.css';

const POLL_MS = 5 * 60_000;
const STATIONS_SOURCE = 'stations';

interface HavainnotLayerProps {
  /** Null until the map has a style that will accept sources. */
  map: maplibregl.Map | null;
  /** Bumped on every style load; re-running the install is what survives a theme swap. */
  epoch: number;
  enabled: boolean;
  theme: Theme;
  /** The parameter being drawn. Owned by the panel, because that is where it is picked. */
  paramCode: string;
  onState: (state: HavainnotState) => void;
  onSelectStation: (fmisid: string) => void;
}

function toGeoJSON(stations: Station[], param: Parameter): FeatureCollection<Point> {
  return {
    type: 'FeatureCollection',
    // A station that does not report the selected parameter is left off the map
    // rather than drawn in a "no data" grey: ~200 grey dots would bury the dozens
    // that do have a reading.
    features: stations
      .filter(s => s.values[param.code] !== undefined)
      .map(s => {
        const value = s.values[param.code];
        const decimals = SCALES[param.code]?.decimals ?? 1;
        return {
          type: 'Feature' as const,
          geometry: { type: 'Point' as const, coordinates: [s.longitude, s.latitude] },
          properties: {
            fmisid: s.fmisid,
            name: s.name,
            value,
            label: value.toFixed(decimals),
            color: colorFor(param.code, value),
          },
        };
      }),
  };
}

/**
 * Weather stations as an overlay on the one map. Renders no DOM: it owns the
 * station source and its two layers, and hands the selection and the feed's
 * status up to the panel. See SalamaLayer for the lifecycle this shares with it.
 */
export function HavainnotLayer({
  map,
  epoch,
  enabled,
  theme,
  paramCode,
  onState,
  onSelectStation,
}: HavainnotLayerProps) {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [coldStart, setColdStart] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // Written in an effect, not during render: a ref write during render is not safe
  // under concurrent rendering, where a render can be discarded after the write.
  const onStateRef = useRef(onState);
  const onSelectRef = useRef(onSelectStation);
  useEffect(() => {
    onStateRef.current = onState;
    onSelectRef.current = onSelectStation;
  });

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const load = async () => {
      try {
        const res = await fetch('/api/havainnot/stations');
        if (res.status === 503) {
          if (!cancelled) setColdStart(true);
          return;
        }
        if (!res.ok) throw new Error(String(res.status));
        const body = (await res.json()) as Snapshot;
        if (cancelled) return;
        setSnapshot(body);
        setColdStart(false);
        setError(null);
      } catch {
        if (!cancelled) setError('Säähavaintojen haku ei onnistunut.');
      }
    };
    load();
    const timer = window.setInterval(load, POLL_MS);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [enabled]);

  // Install and tear down. Keyed on the epoch as well as the toggle: a theme swap
  // discards every custom source, and re-running this is what puts them back.
  useEffect(() => {
    if (!map || !enabled) return;

    map.addSource(STATIONS_SOURCE, {
      type: 'geojson',
      data: { type: 'FeatureCollection', features: [] },
    });
    map.addLayer(
      {
        id: STATIONS_LAYER,
        type: 'circle',
        source: STATIONS_SOURCE,
        paint: {
          // Markers grow with zoom so the country view stays uncluttered while a
          // regional view is readable.
          'circle-radius': ['interpolate', ['linear'], ['zoom'], 4, 7, 8, 14],
          'circle-color': ['get', 'color'],
          'circle-stroke-width': 1,
          'circle-stroke-color': theme === 'dark' ? '#0b1220' : '#ffffff',
        },
      },
      beforeLayer(map, STATIONS_LAYER)
    );
    map.addLayer(
      {
        id: STATION_LABELS_LAYER,
        type: 'symbol',
        source: STATIONS_SOURCE,
        // The value is printed inside the marker from zoom 6, where there is room.
        minzoom: 5.5,
        layout: {
          'text-field': ['get', 'label'],
          'text-size': ['interpolate', ['linear'], ['zoom'], 5.5, 9, 9, 12],
          'text-font': ['Open Sans Bold', 'Arial Unicode MS Bold'],
          'text-allow-overlap': false,
        },
        paint: {
          'text-color': theme === 'dark' ? '#0b1220' : '#111827',
          'text-halo-color': theme === 'dark' ? '#e2e8f0' : '#ffffff',
          'text-halo-width': 1,
        },
      },
      beforeLayer(map, STATION_LABELS_LAYER)
    );

    const onClick = (e: maplibregl.MapLayerMouseEvent) => {
      const fmisid = e.features?.[0]?.properties?.fmisid;
      if (typeof fmisid === 'string') onSelectRef.current(fmisid);
    };
    const onEnter = () => {
      map.getCanvas().style.cursor = 'pointer';
    };
    const onLeave = () => {
      map.getCanvas().style.cursor = '';
    };
    map.on('click', STATIONS_LAYER, onClick);
    map.on('mouseenter', STATIONS_LAYER, onEnter);
    map.on('mouseleave', STATIONS_LAYER, onLeave);

    return () => {
      map.off('click', STATIONS_LAYER, onClick);
      map.off('mouseenter', STATIONS_LAYER, onEnter);
      map.off('mouseleave', STATIONS_LAYER, onLeave);
      removeLayers(map, STATIONS_SOURCE, STATION_LABELS_LAYER, STATIONS_LAYER);
    };
  }, [map, epoch, enabled, theme]);

  // Declared after the install effect, so on any run of both the source it writes
  // to already exists.
  useEffect(() => {
    if (!map || !enabled || !snapshot) return;
    const param = snapshot.parameters.find(p => p.code === paramCode);
    if (!param) return;
    const source = map.getSource(STATIONS_SOURCE) as maplibregl.GeoJSONSource | undefined;
    if (!source) return;
    source.setData(toGeoJSON(snapshot.stations, param));
  }, [map, epoch, enabled, theme, snapshot, paramCode]);

  useEffect(() => {
    onStateRef.current({ snapshot, coldStart, error });
  }, [snapshot, coldStart, error]);

  return null;
}
