import { useCallback, useEffect, useRef, useState } from 'react';
import * as maplibregl from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import type { FeatureCollection, Point } from 'geojson';
import { ChevronLeft, Moon, Sun, Zap } from 'lucide-react';
import { stopPanelClick } from '../../shared/hooks/useCollapsiblePanel';
import { BottomSheet } from '../../shared/components/BottomSheet';
import { LocateControl } from '../../shared/components/LocateControl';
import { useIsMobile, MOBILE_QUERY } from '../../shared/hooks/useMediaQuery';
import {
  type Strike,
  type StrikesResponse,
  type Theme,
  AGE_BANDS,
  BASEMAP_STYLES,
  ageBandIndex,
  countByBand,
  strikeTime,
} from './lib/strikes';
import './salama.css';

const POLL_MS = 60_000;
const STRIKES_SOURCE = 'strikes';
const STRIKES_LAYER = 'strikes-circles';
const STRIKES_GLOW_LAYER = 'strikes-glow';

interface SalamaAppProps {
  theme: Theme;
  onToggleTheme: () => void;
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

function SalamaApp({ theme, onToggleTheme }: SalamaAppProps) {
  const isMobile = useIsMobile();
  const mapContainer = useRef<HTMLDivElement | null>(null);
  const map = useRef<maplibregl.Map | null>(null);

  const [data, setData] = useState<StrikesResponse | null>(null);
  const [coldStart, setColdStart] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isCollapsed, setIsCollapsed] = useState<boolean>(
    typeof window !== 'undefined' ? window.matchMedia(MOBILE_QUERY).matches : false
  );

  // A clock that ticks with the repaint, so the age bands advance without a
  // Date.now() call during render.
  const [now, setNow] = useState(() => Date.now() / 1000);

  // Written in an effect, not during render: a ref write during render is not safe
  // under concurrent rendering.
  const dataRef = useRef(data);
  const themeRef = useRef(theme);
  useEffect(() => {
    dataRef.current = data;
    themeRef.current = theme;
  });
  const styleThemeRef = useRef(theme);
  const installRef = useRef<(() => void) | null>(null);
  const styleReadyRef = useRef(false);

  const paint = useCallback(() => {
    const m = map.current;
    const current = dataRef.current;
    if (!m || !styleReadyRef.current) return;
    const source = m.getSource(STRIKES_SOURCE) as maplibregl.GeoJSONSource | undefined;
    if (!source) return;
    source.setData(toGeoJSON(current?.strikes ?? [], Date.now() / 1000, themeRef.current));
  }, []);

  // Mount once. The ref guard matters under StrictMode, which runs effects twice.
  useEffect(() => {
    if (map.current || !mapContainer.current) return;

    const m = new maplibregl.Map({
      container: mapContainer.current,
      style: BASEMAP_STYLES[themeRef.current],
      center: [25.5, 63.5],
      zoom: 4.6,
      attributionControl: false,
    });
    map.current = m;
    m.addControl(
      new maplibregl.AttributionControl({
        compact: true,
        customAttribution: 'Salamahavainnot: Ilmatieteen laitos, CC BY 4.0',
      }),
      'bottom-left'
    );

    const install = () => {
      if (m.getSource(STRIKES_SOURCE)) return;
      m.addSource(STRIKES_SOURCE, {
        type: 'geojson',
        data: { type: 'FeatureCollection', features: [] },
      });
      // A soft halo under each strike so a single flash still reads at country
      // zoom, where a 7 px dot on its own would vanish.
      m.addLayer({
        id: STRIKES_GLOW_LAYER,
        type: 'circle',
        source: STRIKES_SOURCE,
        paint: {
          'circle-radius': ['get', 'glowRadius'],
          'circle-color': ['get', 'color'],
          'circle-opacity': ['get', 'glowOpacity'],
          'circle-blur': 1,
        },
      });
      m.addLayer({
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
          'circle-stroke-color': themeRef.current === 'dark' ? '#e2e8f0' : '#1f2937',
          'circle-stroke-opacity': 0.7,
        },
      });
      paint();
    };
    installRef.current = install;

    m.on('load', () => {
      styleReadyRef.current = true;
      install();
    });

    const popup = new maplibregl.Popup({ closeButton: false, offset: 10 });
    m.on('click', STRIKES_LAYER, e => {
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
        .addTo(m);
    });
    m.on('mouseenter', STRIKES_LAYER, () => {
      m.getCanvas().style.cursor = 'pointer';
    });
    m.on('mouseleave', STRIKES_LAYER, () => {
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

  // setStyle() discards every custom source, so the layers are rebuilt once the
  // new style will accept them again.
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
  }, []);

  // Repaint when new data arrives, and also on a slow tick so strikes fade
  // through the age bands without waiting for the next poll.
  useEffect(() => {
    paint();
    const timer = window.setInterval(() => {
      setNow(Date.now() / 1000);
      paint();
    }, 15_000);
    return () => window.clearInterval(timer);
  }, [data, paint]);

  const counts = countByBand(data?.strikes ?? [], now);
  const total = data?.strikes.length ?? 0;
  const bodyCollapsed = !isMobile && isCollapsed;

  return (
    <div className="dashboard-container mode-salama">
      <div className="map-container" ref={mapContainer}>
        <LocateControl getMap={() => map.current} />
      </div>

      <BottomSheet
        variant="filter"
        isMobile={isMobile}
        open
        ariaLabel="Avaa salamavalikko"
        collapsed={isCollapsed}
        onToggleCollapse={() => setIsCollapsed(v => !v)}
      >
        <div className="panel-header" onClick={bodyCollapsed ? undefined : stopPanelClick}>
          <div className="panel-title">
            <Zap size={16} />
            <span>Salama</span>
          </div>
          {!bodyCollapsed && (
            <button
              className="icon-btn"
              onClick={e => {
                e.stopPropagation();
                setIsCollapsed(v => !v);
              }}
              aria-label="Pienennä salamavalikko"
            >
              <ChevronLeft size={16} />
            </button>
          )}
        </div>

        {!bodyCollapsed && (
          <div className="filter-content" onClick={stopPanelClick}>
            <div className="panel-stats">
              <span className="conn-dot" title="Ilmatieteen laitos" />
              <span>
                {total} salamaa · {data ? `${data.windowMinutes} min` : '—'}
              </span>
            </div>

            <div className="filter-scroll-area">
              <div className="filter-section-title">Ikä</div>
              <div className="category-list">
                {AGE_BANDS.map((band, i) => (
                  <div className="category-row" key={band.label}>
                    <span
                      className="category-swatch"
                      style={{ background: band.color[theme], opacity: band.opacity }}
                    />
                    <span className="category-label">{band.label}</span>
                    <span className="category-count">{counts[i]}</span>
                  </div>
                ))}
              </div>

              <div className="filter-section-title" style={{ marginTop: 14 }}>
                Tyyppi
              </div>
              <div className="category-list">
                <div className="category-row">
                  <span className="category-swatch salama-swatch--ground" />
                  <span className="category-label">Maasalama</span>
                </div>
                <div className="category-row">
                  <span className="category-swatch salama-swatch--cloud" />
                  <span className="category-label">Pilvisalama</span>
                </div>
              </div>

              {/* Zero strikes is the normal state in Finland, and saying so stops an
                  empty map reading as a broken feed. */}
              {total === 0 && !coldStart && (
                <div className="panel-note">
                  Ei salamoita viimeisen {data?.windowMinutes ?? 120} minuutin aikana.
                  Tyhjä kartta on Suomessa tavallisin tilanne.
                </div>
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
                Vain maasalamat ovat vaaraksi maanpinnalla; pilvisalamat piirtyvät
                renkaana.
              </div>
            </div>
          </div>
        )}
      </BottomSheet>

      {coldStart && <div className="radar-status">Ladataan salamahavaintoja…</div>}
      {error && <div className="radar-status radar-status--error">{error}</div>}
    </div>
  );
}

export default SalamaApp;
