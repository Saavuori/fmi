import { useCallback, useEffect, useRef, useState } from 'react';
import * as maplibregl from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import type { FeatureCollection, Point } from 'geojson';
import { ChevronLeft, Moon, Sun, Thermometer, X } from 'lucide-react';
import { stopPanelClick } from '../../shared/hooks/useCollapsiblePanel';
import { Panel } from '../../shared/components/Panel';
import { LocateControl } from '../../shared/components/LocateControl';
import { useIsMobile, MOBILE_QUERY } from '../../shared/hooks/useMediaQuery';
import {
  type Parameter,
  type Snapshot,
  type Station,
  type StationDetail,
  type Theme,
  BASEMAP_STYLES,
  DEFAULT_PARAMETER,
  SCALES,
  colorFor,
  observationTime,
  readingText,
} from './lib/observations';
import './havainnot.css';

const POLL_MS = 5 * 60_000;
const STATIONS_SOURCE = 'stations';
const STATIONS_LAYER = 'stations-circles';
const LABELS_LAYER = 'stations-labels';

interface HavainnotAppProps {
  theme: Theme;
  onToggleTheme: () => void;
}

function toGeoJSON(
  stations: Station[],
  param: Parameter
): FeatureCollection<Point> {
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

function HavainnotApp({ theme, onToggleTheme }: HavainnotAppProps) {
  const isMobile = useIsMobile();
  const mapContainer = useRef<HTMLDivElement | null>(null);
  const map = useRef<maplibregl.Map | null>(null);

  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [paramCode, setParamCode] = useState(DEFAULT_PARAMETER);
  const [selected, setSelected] = useState<StationDetail | null>(null);
  const [coldStart, setColdStart] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isFilterCollapsed, setIsFilterCollapsed] = useState<boolean>(
    typeof window !== 'undefined' ? window.matchMedia(MOBILE_QUERY).matches : false
  );
  const [isDetailCollapsed, setIsDetailCollapsed] = useState(false);

  const parameters = snapshot?.parameters ?? [];
  const param = parameters.find(p => p.code === paramCode);

  // Written in an effect, not during render: a ref write during render is not safe
  // under concurrent rendering, where a render can be discarded after the write.
  const snapshotRef = useRef(snapshot);
  const paramRef = useRef(param);
  const themeRef = useRef(theme);
  const selectRef = useRef<(fmisid: string) => void>(() => {});
  const styleThemeRef = useRef(theme);
  const installRef = useRef<(() => void) | null>(null);
  const styleReadyRef = useRef(false);

  const paint = useCallback(() => {
    const m = map.current;
    const snap = snapshotRef.current;
    const p = paramRef.current;
    if (!m || !styleReadyRef.current || !snap || !p) return;
    const source = m.getSource(STATIONS_SOURCE) as maplibregl.GeoJSONSource | undefined;
    if (!source) return;
    source.setData(toGeoJSON(snap.stations, p));
  }, []);

  useEffect(() => {
    if (map.current || !mapContainer.current) return;

    const m = new maplibregl.Map({
      container: mapContainer.current,
      style: BASEMAP_STYLES[themeRef.current],
      center: [25.5, 64.5],
      zoom: 4.5,
      attributionControl: false,
    });
    map.current = m;
    m.addControl(
      new maplibregl.AttributionControl({
        compact: true,
        customAttribution: 'Säähavainnot: Ilmatieteen laitos, CC BY 4.0',
      }),
      'bottom-left'
    );

    const install = () => {
      if (m.getSource(STATIONS_SOURCE)) return;
      m.addSource(STATIONS_SOURCE, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      });
      m.addLayer({
        id: STATIONS_LAYER,
        type: 'circle',
        source: STATIONS_SOURCE,
        paint: {
          // Markers grow with zoom so the country view stays uncluttered while a
          // regional view is readable.
          'circle-radius': ['interpolate', ['linear'], ['zoom'], 4, 7, 8, 14],
          'circle-color': ['get', 'color'],
          'circle-stroke-width': 1,
          'circle-stroke-color': themeRef.current === 'dark' ? '#0b1220' : '#ffffff',
        },
      });
      m.addLayer({
        id: LABELS_LAYER,
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
          'text-color': themeRef.current === 'dark' ? '#0b1220' : '#111827',
          'text-halo-color': themeRef.current === 'dark' ? '#e2e8f0' : '#ffffff',
          'text-halo-width': 1,
        },
      });
      paint();
    };
    installRef.current = install;

    m.on('load', () => {
      styleReadyRef.current = true;
      install();
    });

    m.on('click', STATIONS_LAYER, e => {
      const fmisid = e.features?.[0]?.properties?.fmisid;
      if (typeof fmisid === 'string') selectRef.current(fmisid);
    });
    m.on('mouseenter', STATIONS_LAYER, () => {
      m.getCanvas().style.cursor = 'pointer';
    });
    m.on('mouseleave', STATIONS_LAYER, () => {
      m.getCanvas().style.cursor = '';
    });

    return () => {
      // Load-bearing under StrictMode.
      m.remove();
      map.current = null;
      styleReadyRef.current = false;
      installRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    themeRef.current = theme;
    const m = map.current;
    if (!m || styleThemeRef.current === theme) return;
    styleThemeRef.current = theme;
    styleReadyRef.current = false;
    m.setStyle(BASEMAP_STYLES[theme]);

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

  useEffect(() => {
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
  }, []);

  useEffect(() => {
    paint();
  }, [snapshot, paramCode, paint]);

  const selectStation = useCallback(async (fmisid: string) => {
    setIsDetailCollapsed(false);
    if (window.matchMedia(MOBILE_QUERY).matches) setIsFilterCollapsed(true);
    try {
      const res = await fetch(`/api/havainnot/station/${fmisid}`);
      if (!res.ok) return;
      setSelected((await res.json()) as StationDetail);
    } catch {
      // A failed detail fetch leaves the previous selection alone rather than
      // clearing the panel out from under the viewer.
    }
  }, []);

  useEffect(() => {
    snapshotRef.current = snapshot;
    paramRef.current = param;
    themeRef.current = theme;
    selectRef.current = selectStation;
  });

  const bodyCollapsed = !isMobile && isFilterCollapsed;
  const scale = SCALES[paramCode];

  return (
    <div className="dashboard-container mode-havainnot">
      <div className="map-container" ref={mapContainer}>
        <LocateControl getMap={() => map.current} />
      </div>

      <Panel
        variant="filter"
        isMobile={isMobile}
        open={!isMobile || !selected}
        ariaLabel="Avaa havaintovalikko"
        collapsed={isFilterCollapsed}
        onToggleCollapse={() => setIsFilterCollapsed(v => !v)}
      >
        <div className="panel-header" onClick={bodyCollapsed ? undefined : stopPanelClick}>
          <div className="panel-title">
            <Thermometer size={16} />
            <span>Havainnot</span>
          </div>
          {/* Folds the desktop rail away; on a phone it dismisses the overlay,
              which is the only way back to the map — hence the X. */}
          {!bodyCollapsed && (
            <button
              className="icon-btn"
              onClick={e => {
                e.stopPropagation();
                setIsFilterCollapsed(v => !v);
              }}
              aria-label={isMobile ? 'Sulje havaintovalikko' : 'Pienennä havaintovalikko'}
            >
              {isMobile ? <X size={18} /> : <ChevronLeft size={16} />}
            </button>
          )}
        </div>

        {!bodyCollapsed && (
          <div className="filter-content" onClick={stopPanelClick}>
            <div className="panel-stats">
              <span className="conn-dot" title="Ilmatieteen laitos" />
              <span>{snapshot?.stations.length ?? 0} asemaa</span>
            </div>

            <div className="filter-scroll-area">
              <div className="filter-section-title">Suure</div>
              <div className="layer-toggles obs-params">
                {parameters.map(p => (
                  <button
                    key={p.code}
                    className={`layer-toggle ${p.code === paramCode ? 'on' : ''}`}
                    onClick={() => setParamCode(p.code)}
                    aria-pressed={p.code === paramCode}
                  >
                    <span>{p.label}</span>
                    <span className="obs-unit">{p.unit}</span>
                  </button>
                ))}
              </div>

              {scale && param && paramCode !== 'wd_10min' && (
                <>
                  <div className="filter-section-title" style={{ marginTop: 14 }}>
                    {param.label} ({param.unit})
                  </div>
                  {/* Drawn from the same stops the markers are coloured with. */}
                  <div className="radar-legend">
                    <div
                      className="radar-legend__bar"
                      style={{
                        background: `linear-gradient(to right, ${scale.stops
                          .map(
                            ([v, c]) =>
                              `${c} ${
                                ((v - scale.stops[0][0]) /
                                  (scale.stops[scale.stops.length - 1][0] - scale.stops[0][0])) *
                                100
                              }%`
                          )
                          .join(', ')})`,
                      }}
                    />
                    <div className="radar-legend__labels">
                      {scale.stops.map(([v]) => (
                        <span key={v} className="radar-legend__label">
                          {v}
                        </span>
                      ))}
                    </div>
                  </div>
                </>
              )}

              <div className="filter-section-title" style={{ marginTop: 14 }}>
                Näkymä
              </div>
              <div className="layer-toggles" style={{ gridTemplateColumns: '1fr 1fr' }}>
                <button
                  className={`layer-toggle ${theme === 'dark' ? 'on' : ''}`}
                  onClick={() => {
                    if (theme !== 'dark') onToggleTheme();
                  }}
                  aria-pressed={theme === 'dark'}
                >
                  <Moon size={14} />
                  <span>Tumma</span>
                </button>
                <button
                  className={`layer-toggle ${theme === 'light' ? 'on' : ''}`}
                  onClick={() => {
                    if (theme !== 'light') onToggleTheme();
                  }}
                  aria-pressed={theme === 'light'}
                >
                  <Sun size={14} />
                  <span>Vaalea</span>
                </button>
              </div>

              <div className="panel-note">
                Vain suureen mittaavat asemat näkyvät kartalla. Napauta asemaa
                nähdäksesi sen viime tuntien havainnot.
              </div>
            </div>
          </div>
        )}
      </Panel>

      {selected && param && (
        <Panel
          variant="detail"
          isMobile={isMobile}
          open
          ariaLabel="Avaa aseman tiedot"
          collapsed={isDetailCollapsed}
          onToggleCollapse={() => setIsDetailCollapsed(v => !v)}
        >
          <div
            className="detail-header"
            onClick={!isMobile && isDetailCollapsed ? undefined : stopPanelClick}
          >
            <div>
              <div className="detail-title">{selected.name}</div>
              <div className="detail-subtitle">
                {selected.region ? `${selected.region} · ` : ''}
                {observationTime(selected.timestamp)}
              </div>
            </div>
            <button className="icon-btn" onClick={() => setSelected(null)} aria-label="Sulje">
              <X size={16} />
            </button>
          </div>

          {(isMobile || !isDetailCollapsed) && (
            <div className="detail-content" onClick={stopPanelClick}>
              <div className="telemetry-grid">
                {parameters.map(p => (
                  <div className="telemetry-item" key={p.code}>
                    <span className="telemetry-label">{p.label}</span>
                    <span className="telemetry-value">{readingText(selected.values, p)}</span>
                  </div>
                ))}
              </div>

              <div className="filter-section-title">{param.label} · viime tunnit</div>
              <div className="obs-series">
                {selected.series
                  .filter(r => r.values[param.code] !== undefined)
                  .slice(-24)
                  .map(r => (
                    <div className="fact-row" key={r.timestamp}>
                      <span>{observationTime(r.timestamp)}</span>
                      <span>{readingText(r.values, param)}</span>
                    </div>
                  ))}
                {selected.series.every(r => r.values[param.code] === undefined) && (
                  <div className="panel-note">Tämä asema ei mittaa {param.label.toLowerCase()}.</div>
                )}
              </div>
            </div>
          )}
        </Panel>
      )}

      {coldStart && <div className="radar-status">Ladataan säähavaintoja…</div>}
      {error && <div className="radar-status radar-status--error">{error}</div>}
    </div>
  );
}

export default HavainnotApp;
