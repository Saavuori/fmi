import React, { useEffect, useRef } from 'react';
import * as maplibregl from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { type GridInfo } from '../radar/api';
import { type Theme, BASEMAP_STYLES, PROBE_COLORS } from './theme';
import { PROBE_LAYER, RADAR_LAYER, beforeLayer } from './layers';
import { LocateControl } from '../shared/components/LocateControl';

const RADAR_SOURCE = 'radar';
const PROBE_SOURCE = 'probe';

interface MapProps {
  grid: GridInfo | null;
  /** URL of the radar frame to display, or null while there is nothing to show. */
  frameUrl: string | null;
  opacity: number;
  theme: Theme;
  attribution: string;
  /** The point being read out, drawn as a pin. */
  probe: { lat: number; lon: number } | null;
  onPickPoint: (lngLat: { lat: number; lon: number }) => void;
  /**
   * Called with the map every time it has a style that will accept sources —
   * once on load, and again after each theme swap with a bumped epoch. Overlay
   * layers key their install effect on the epoch, which is what rebuilds them
   * after setStyle() throws every custom source away.
   */
  onStyleReady: (map: maplibregl.Map, epoch: number) => void;
  /**
   * Layer ids the overlays own. A click that landed on one of these belongs to
   * that overlay — the station it hit, say — so it must not also drop a radar
   * probe pin behind the panel that is about to open.
   */
  overlayLayerIds: string[];
}

/** MapLibre wants the image corners as a 4-tuple of [lon, lat]. The backend
 * already emits them in the required order (TL, TR, BR, BL) from the same grid
 * definition the raster was fetched on, so there is nothing to reorder here. */
type Corners = [[number, number], [number, number], [number, number], [number, number]];

const Map: React.FC<MapProps> = ({
  grid,
  frameUrl,
  opacity,
  theme,
  attribution,
  probe,
  onPickPoint,
  onStyleReady,
  overlayLayerIds,
}) => {
  const mapContainer = useRef<HTMLDivElement | null>(null);
  const map = useRef<maplibregl.Map | null>(null);

  // Latest props, readable from the map's own callbacks and from the style-reload
  // path so a layer re-added after a theme swap starts with current values.
  //
  // Written in an effect rather than during render: a ref write during render is
  // not safe under concurrent rendering, where a render can be thrown away after
  // the write has already happened.
  const gridRef = useRef(grid);
  const frameUrlRef = useRef(frameUrl);
  const opacityRef = useRef(opacity);
  const themeRef = useRef(theme);
  const probeRef = useRef(probe);
  const onPickRef = useRef(onPickPoint);
  const onStyleReadyRef = useRef(onStyleReady);
  const overlayLayersRef = useRef(overlayLayerIds);
  useEffect(() => {
    gridRef.current = grid;
    frameUrlRef.current = frameUrl;
    opacityRef.current = opacity;
    themeRef.current = theme;
    probeRef.current = probe;
    onPickRef.current = onPickPoint;
    onStyleReadyRef.current = onStyleReady;
    overlayLayersRef.current = overlayLayerIds;
  });

  // Theme whose basemap style is currently loaded, so the theme effect can tell a
  // real switch from its own first run (the map mounts with the right style).
  const styleThemeRef = useRef(theme);
  // False while a style swap is in flight: an update that lands mid-swap must not
  // touch a style that is about to be thrown away.
  const styleReadyRef = useRef(false);
  // Bumped on every style load and handed to the overlays, which rebuild on it.
  const epochRef = useRef(0);
  // Set by the mount effect so the theme effect can rebuild what setStyle
  // discarded.
  const installRef = useRef<(() => void) | null>(null);

  // Mount once. The ref guard matters under StrictMode, which runs effects twice.
  useEffect(() => {
    if (map.current || !mapContainer.current) return;

    const m = new maplibregl.Map({
      container: mapContainer.current,
      style: BASEMAP_STYLES[themeRef.current],
      center: [25.5, 64.5], // roughly the middle of the radar composite
      zoom: 4.4,
      // Own compact attribution bottom-left rather than the default expanded one.
      attributionControl: false,
    });
    map.current = m;
    m.addControl(
      new maplibregl.AttributionControl({ compact: true, customAttribution: attribution }),
      'bottom-left'
    );
    if (import.meta.env.DEV) (window as unknown as { __map?: maplibregl.Map }).__map = m;

    // Adds the radar and probe layers. Called on first style load and again after
    // every theme swap, because setStyle() discards every custom source.
    const install = () => {
      const g = gridRef.current;
      const url = frameUrlRef.current;

      if (g && url && !m.getSource(RADAR_SOURCE)) {
        m.addSource(RADAR_SOURCE, {
          type: 'image',
          url,
          coordinates: g.corners as Corners,
        });
        m.addLayer(
          {
            id: RADAR_LAYER,
            type: 'raster',
            source: RADAR_SOURCE,
            paint: {
              'raster-opacity': opacityRef.current,
              // The frames are pre-rendered at a fixed resolution, so let the GPU
              // smooth them when zoomed in rather than showing 2 km squares.
              'raster-resampling': 'linear',
              // Cross-fading between zoom levels has nothing to fade between for a
              // single image and only adds a flash on update.
              'raster-fade-duration': 0,
            },
          },
          beforeLayer(m, RADAR_LAYER)
        );
      }

      if (!m.getSource(PROBE_SOURCE)) {
        m.addSource(PROBE_SOURCE, {
          type: 'geojson',
          data: { type: 'FeatureCollection', features: [] },
        });
        m.addLayer({
          id: PROBE_LAYER,
          type: 'circle',
          source: PROBE_SOURCE,
          paint: {
            'circle-radius': 6,
            'circle-color': 'transparent',
            'circle-stroke-width': 2.5,
            'circle-stroke-color': PROBE_COLORS[themeRef.current],
          },
        });
        syncProbe(m, probeRef.current);
      }

      // The overlays rebuild off this, and beforeLayer() places each of them
      // relative to what is already mounted — so it has to come last.
      onStyleReadyRef.current(m, ++epochRef.current);
    };
    installRef.current = install;

    m.on('load', () => {
      styleReadyRef.current = true;
      install();
    });

    // Tapping the map picks the point to read out. The radar layer is not
    // queryable, so this is a plain map click — but an overlay's features are, and
    // a tap that hit one of those is that overlay's to answer.
    m.on('click', e => {
      const overlays = overlayLayersRef.current.filter(id => m.getLayer(id));
      if (overlays.length && m.queryRenderedFeatures(e.point, { layers: overlays }).length) return;
      onPickRef.current({ lat: e.lngLat.lat, lon: e.lngLat.lng });
    });

    return () => {
      // Load-bearing under StrictMode: without it the first map instance keeps
      // its WebGL context and event listeners alive alongside the second.
      m.remove();
      map.current = null;
      styleReadyRef.current = false;
      installRef.current = null;
    };
    // Mount-only: every prop is read through a ref so remounting is never needed.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Swap the basemap when the theme changes. setStyle() throws away every custom
  // source and layer, so the install has to run again once the new style is ready.
  useEffect(() => {
    themeRef.current = theme;
    const m = map.current;
    if (!m || styleThemeRef.current === theme) return;
    styleThemeRef.current = theme;
    styleReadyRef.current = false;
    m.setStyle(BASEMAP_STYLES[theme]);

    // Whichever of these lands first means the new style will accept sources
    // again. isStyleLoaded() is deliberately not consulted: it still reports
    // false at this point even though addSource/addLayer are already accepted.
    let installed = false;
    const onReady = () => {
      if (installed) return;
      installed = true;
      m.off('style.load', onReady);
      m.off('styledata', onReady);
      styleReadyRef.current = true;
      installRef.current?.();
    };
    m.on('style.load', onReady);
    m.on('styledata', onReady);
    return () => {
      m.off('style.load', onReady);
      m.off('styledata', onReady);
    };
  }, [theme]);

  // Show the current frame. updateImage() swaps the texture in place, which is
  // what keeps playback smooth — adding and removing a source per frame would
  // flash on every step.
  useEffect(() => {
    const m = map.current;
    const g = grid;
    if (!m || !styleReadyRef.current || !g || !frameUrl) return;

    const source = m.getSource(RADAR_SOURCE) as maplibregl.ImageSource | undefined;
    if (!source) {
      installRef.current?.();
      return;
    }
    source.updateImage({ url: frameUrl, coordinates: g.corners as Corners });
  }, [frameUrl, grid]);

  useEffect(() => {
    const m = map.current;
    if (!m || !m.getLayer(RADAR_LAYER)) return;
    m.setPaintProperty(RADAR_LAYER, 'raster-opacity', opacity);
  }, [opacity]);

  useEffect(() => {
    const m = map.current;
    if (!m || !m.getLayer(PROBE_LAYER)) return;
    m.setPaintProperty(PROBE_LAYER, 'circle-stroke-color', PROBE_COLORS[theme]);
  }, [theme]);

  useEffect(() => {
    const m = map.current;
    if (!m) return;
    syncProbe(m, probe);
  }, [probe]);

  return (
    <div className="map-container" ref={mapContainer}>
      <LocateControl getMap={() => map.current} />
    </div>
  );
};

function syncProbe(m: maplibregl.Map, probe: { lat: number; lon: number } | null) {
  const source = m.getSource(PROBE_SOURCE) as maplibregl.GeoJSONSource | undefined;
  if (!source) return;
  source.setData({
    type: 'FeatureCollection',
    features: probe
      ? [{ type: 'Feature', properties: {}, geometry: { type: 'Point', coordinates: [probe.lon, probe.lat] } }]
      : [],
  });
}

export default Map;
