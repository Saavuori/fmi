import { useEffect, useRef, useState } from 'react';
import * as maplibregl from 'maplibre-gl';
import type { FeatureCollection, Point } from 'geojson';
import { STRIKES_GLOW_LAYER, STRIKES_LAYER, beforeLayer, removeLayers } from '../../map/layers';
import type { Theme } from '../../map/theme';
import {
  type SalamaState,
  type Strike,
  type StrikesResponse,
  AGE_BANDS,
  ageBandIndex,
  countByBand,
  strikeTime,
} from './strikes';
import './salama.css';

const POLL_MS = 60_000;
/** How often the strikes are re-banded, so they fade with age between polls. */
const FADE_MS = 15_000;
const STRIKES_SOURCE = 'strikes';

interface SalamaLayerProps {
  /** Null until the map has a style that will accept sources. */
  map: maplibregl.Map | null;
  /** Bumped on every style load; re-running the install is what survives a theme swap. */
  epoch: number;
  enabled: boolean;
  theme: Theme;
  onState: (state: SalamaState) => void;
}

/**
 * Resolves each strike's age band to concrete colour, radius and opacity in the
 * feature properties, so the paint expressions are plain `['get', …]` lookups.
 *
 * Doing it here rather than as a MapLibre `match` expression over a band index
 * keeps the banding logic in TypeScript where it is readable and testable, and
 * means the map reads the very same AGE_BANDS table the panel legend does.
 */
function toGeoJSON(strikes: Strike[], nowSeconds: number, theme: Theme): FeatureCollection<Point> {
  return {
    type: 'FeatureCollection',
    features: strikes.map(s => {
      const band = AGE_BANDS[ageBandIndex(s, nowSeconds)];
      return {
        type: 'Feature' as const,
        geometry: { type: 'Point' as const, coordinates: [s.longitude, s.latitude] },
        properties: {
          color: band.color[theme],
          radius: band.radius,
          opacity: band.opacity,
          glowRadius: band.radius * 2.6,
          glowOpacity: band.opacity * 0.18,
          cloudFlash: s.cloudFlash,
          time: strikeTime(s.timestamp),
          peak: Math.round(Math.abs(s.peakCurrent)),
          multiplicity: s.multiplicity,
        },
      };
    }),
  };
}

/**
 * Lightning as an overlay on the one map, rather than a mode with a map of its
 * own. It renders no DOM: it owns the strike source, its two circle layers and
 * the popup, and reports upwards for the panel to describe.
 *
 * Everything that used to be map lifecycle here — mounting, the theme swap, the
 * rebuild after setStyle() discards custom sources — now belongs to Map. What is
 * left is install, paint, tear down, keyed on `epoch` so a theme change puts the
 * layers back.
 *
 * It polls only while it is switched on. The backend is poll-style either way, so
 * this only decides whether the browser asks — but a layer nobody enabled asking
 * every minute would make "off" mean nothing.
 */
export function SalamaLayer({ map, epoch, enabled, theme, onState }: SalamaLayerProps) {
  const [data, setData] = useState<StrikesResponse | null>(null);
  const [coldStart, setColdStart] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // A clock that ticks with the fade, so the age bands advance between polls
  // without a Date.now() call during render.
  const [now, setNow] = useState(() => Date.now() / 1000);

  // Written in an effect, not during render: a ref write during render is not safe
  // under concurrent rendering, where a render can be discarded after the write.
  const onStateRef = useRef(onState);
  useEffect(() => {
    onStateRef.current = onState;
  });

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    const load = async () => {
      try {
        const res = await fetch('/api/salama/strikes');
        if (res.status === 503) {
          if (!cancelled) setColdStart(true);
          return;
        }
        if (!res.ok) throw new Error(String(res.status));
        const body = (await res.json()) as StrikesResponse;
        if (cancelled) return;
        setData(body);
        setColdStart(false);
        setError(null);
      } catch {
        if (!cancelled) setError('Salamahavaintojen haku ei onnistunut.');
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

    map.addSource(STRIKES_SOURCE, {
      type: 'geojson',
      data: { type: 'FeatureCollection', features: [] },
    });
    // A soft halo under each strike so a single flash still reads at country
    // zoom, where a 7 px dot on its own would vanish.
    map.addLayer(
      {
        id: STRIKES_GLOW_LAYER,
        type: 'circle',
        source: STRIKES_SOURCE,
        paint: {
          'circle-radius': ['get', 'glowRadius'],
          'circle-color': ['get', 'color'],
          'circle-opacity': ['get', 'glowOpacity'],
          'circle-blur': 1,
        },
      },
      beforeLayer(map, STRIKES_GLOW_LAYER)
    );
    map.addLayer(
      {
        id: STRIKES_LAYER,
        type: 'circle',
        source: STRIKES_SOURCE,
        paint: {
          'circle-radius': ['get', 'radius'],
          'circle-color': ['get', 'color'],
          'circle-opacity': ['get', 'opacity'],
          // Intra-cloud flashes get a hollow look: they matter meteorologically
          // but they are not a hazard at ground level.
          'circle-stroke-width': ['case', ['get', 'cloudFlash'], 1.5, 0],
          'circle-stroke-color': theme === 'dark' ? '#e2e8f0' : '#1f2937',
          'circle-stroke-opacity': 0.7,
        },
      },
      beforeLayer(map, STRIKES_LAYER)
    );

    const popup = new maplibregl.Popup({ closeButton: false, offset: 10 });
    const onClick = (e: maplibregl.MapLayerMouseEvent) => {
      const f = e.features?.[0];
      if (!f) return;
      const p = f.properties as Record<string, unknown>;
      popup
        .setLngLat((f.geometry as Point).coordinates as [number, number])
        .setHTML(
          `<div class="strike-popup">
             <strong>${p.cloudFlash ? 'Pilvisalama' : 'Maasalama'}</strong>
             <span>${p.time}</span>
             <span>${p.peak} kA${Number(p.multiplicity) > 1 ? ` · ${p.multiplicity} iskua` : ''}</span>
           </div>`
        )
        .addTo(map);
    };
    const onEnter = () => {
      map.getCanvas().style.cursor = 'pointer';
    };
    const onLeave = () => {
      map.getCanvas().style.cursor = '';
    };
    map.on('click', STRIKES_LAYER, onClick);
    map.on('mouseenter', STRIKES_LAYER, onEnter);
    map.on('mouseleave', STRIKES_LAYER, onLeave);

    return () => {
      map.off('click', STRIKES_LAYER, onClick);
      map.off('mouseenter', STRIKES_LAYER, onEnter);
      map.off('mouseleave', STRIKES_LAYER, onLeave);
      popup.remove();
      removeLayers(map, STRIKES_SOURCE, STRIKES_LAYER, STRIKES_GLOW_LAYER);
    };
  }, [map, epoch, enabled, theme]);

  // Paint, and keep repainting on a slow tick so strikes fade through the age
  // bands without waiting for the next poll. Declared after the install effect,
  // so on any run of both the source it writes to already exists.
  useEffect(() => {
    if (!map || !enabled) return;
    const repaint = () => {
      const source = map.getSource(STRIKES_SOURCE) as maplibregl.GeoJSONSource | undefined;
      if (!source) return;
      source.setData(toGeoJSON(data?.strikes ?? [], Date.now() / 1000, theme));
    };
    repaint();
    const timer = window.setInterval(() => {
      setNow(Date.now() / 1000);
      repaint();
    }, FADE_MS);
    return () => window.clearInterval(timer);
  }, [map, epoch, enabled, theme, data]);

  // Hand the panel what it needs to describe the layer it is toggling.
  useEffect(() => {
    onStateRef.current({
      data,
      coldStart,
      error,
      counts: countByBand(data?.strikes ?? [], now),
    });
  }, [data, coldStart, error, now]);

  return null;
}
