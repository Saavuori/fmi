import { useCallback, useEffect, useMemo, useState } from 'react';
import type * as maplibregl from 'maplibre-gl';
import Map from './map/Map';
import {
  STATIONS_LAYER,
  STATION_LABELS_LAYER,
  STRIKES_GLOW_LAYER,
  STRIKES_LAYER,
} from './map/layers';
import { type Theme, DEFAULT_OPACITY } from './map/theme';
import { LayersPanel } from './components/LayersPanel';
import { PointPanel } from './components/PointPanel';
import { StationPanel } from './components/StationPanel';
import { ReplayBar } from './components/ReplayBar';
import { FrameClock } from './components/FrameClock';
import { PlaceSearch, type Place } from './shared/components/PlaceSearch';
import { useIsMobile, MOBILE_QUERY } from './shared/hooks/useMediaQuery';
import { useRadarLoop } from './radar/useRadarLoop';
import {
  type LegendResponse,
  type PointResponse,
  type ProductsResponse,
  NotReadyError,
  fetchLegend,
  fetchPoint,
  fetchProducts,
  frameUrl,
} from './radar/api';
import { SalamaLayer } from './layers/salama/SalamaLayer';
import { type SalamaState, EMPTY_SALAMA_STATE } from './layers/salama/strikes';
import { HavainnotLayer } from './layers/havainnot/HavainnotLayer';
import {
  type HavainnotState,
  type StationDetail,
  DEFAULT_PARAMETER,
  EMPTY_HAVAINNOT_STATE,
} from './layers/havainnot/observations';
import './radar/radar.css';

interface WeatherMapProps {
  theme: Theme;
  onToggleTheme: () => void;
}

/**
 * Every feed FMI publishes here is the same licensor under the same licence, so
 * one credit covers the radar, the strikes and the stations. MapLibre reads it
 * when the attribution control is constructed, which is why it is a constant
 * rather than something waited for.
 */
const ATTRIBUTION = 'Ilmatieteen laitos, CC BY 4.0';

const LAYERS_KEY = 'tutka-layers';

function loadEnabledLayers(): { salama: boolean; havainnot: boolean } {
  try {
    const saved = JSON.parse(localStorage.getItem(LAYERS_KEY) ?? '{}');
    return { salama: saved.salama === true, havainnot: saved.havainnot === true };
  } catch {
    return { salama: false, havainnot: false };
  }
}

/**
 * The whole app: one map, radar underneath, the other two feeds as layers over
 * it.
 *
 * This was three sibling apps behind a tab bar, each with a MapLibre instance
 * and a copy of the mount / theme-swap / style-reload dance. Only the shape
 * changed — the feeds, the palettes and the readouts are the same ones — but
 * lightning over the rain that is producing it is a thing you can now actually
 * look at, and the tab bar's 64px band at the bottom of every phone screen went
 * back to the map.
 */
function WeatherMap({ theme, onToggleTheme }: WeatherMapProps) {
  const isMobile = useIsMobile();

  // The map, once it has a style that will take sources. `epoch` steps on every
  // style load, which is the signal the overlays rebuild on.
  const [mapState, setMapState] = useState<{ map: maplibregl.Map; epoch: number } | null>(null);
  const onStyleReady = useCallback((map: maplibregl.Map, epoch: number) => {
    setMapState({ map, epoch });
  }, []);

  const [meta, setMeta] = useState<ProductsResponse | null>(null);
  const [metaError, setMetaError] = useState<string | null>(null);
  const [product, setProduct] = useState('dbz');
  const [accessiblePalette, setAccessiblePalette] = useState(false);
  const [windowHours, setWindowHours] = useState(1);
  const [legend, setLegend] = useState<LegendResponse | null>(null);

  // Which overlays are on. Remembered, because it is a statement about what the
  // viewer cares about rather than a transient view state.
  const [enabled, setEnabled] = useState(loadEnabledLayers);
  useEffect(() => {
    localStorage.setItem(LAYERS_KEY, JSON.stringify(enabled));
  }, [enabled]);

  const [salama, setSalama] = useState<SalamaState>(EMPTY_SALAMA_STATE);
  const [havainnot, setHavainnot] = useState<HavainnotState>(EMPTY_HAVAINNOT_STATE);
  const [paramCode, setParamCode] = useState(DEFAULT_PARAMETER);

  // Opacity is derived, not stored: the theme's default applies until the viewer
  // picks their own. Storing it would need an effect to re-apply the default on a
  // theme switch, which is a cascading render for something a fallback expresses
  // directly.
  const [opacityOverride, setOpacityOverride] = useState<number | null>(null);
  const opacity = opacityOverride ?? DEFAULT_OPACITY[theme];

  const [probe, setProbe] = useState<{ lat: number; lon: number } | null>(null);
  const [point, setPoint] = useState<PointResponse | null>(null);
  const [pointError, setPointError] = useState<string | null>(null);
  const [station, setStation] = useState<StationDetail | null>(null);

  const [isFilterCollapsed, setIsFilterCollapsed] = useState<boolean>(
    typeof window !== 'undefined' ? window.matchMedia(MOBILE_QUERY).matches : false
  );
  const [isDetailCollapsed, setIsDetailCollapsed] = useState(false);

  // The palette id follows the product's unit and the accessibility preference.
  // The backend rejects a palette whose unit does not match the product, so this
  // has to derive from the selected product rather than being free-form.
  const palette = useMemo(() => {
    const base = meta?.products.find(p => p.id === product)?.palette ?? 'dbz';
    return accessiblePalette ? `${base}-cvd` : base;
  }, [meta, product, accessiblePalette]);

  const loop = useRadarLoop(product, palette, windowHours);

  // Only the layers actually on can swallow a click. Recomputed rather than
  // constant so a tap on empty map still drops a radar probe when the overlays
  // are off.
  const overlayLayerIds = useMemo(() => {
    const ids: string[] = [];
    if (enabled.havainnot) ids.push(STATIONS_LAYER, STATION_LABELS_LAYER);
    if (enabled.salama) ids.push(STRIKES_LAYER, STRIKES_GLOW_LAYER);
    return ids;
  }, [enabled]);

  useEffect(() => {
    let cancelled = false;
    fetchProducts()
      .then(res => {
        if (cancelled) return;
        setMeta(res);
        setMetaError(null);
      })
      .catch(err => {
        if (cancelled) return;
        setMetaError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    fetchLegend(product, palette)
      .then(res => {
        if (!cancelled) setLegend(res);
      })
      .catch(() => {
        if (!cancelled) setLegend(null);
      });
    return () => {
      cancelled = true;
    };
  }, [product, palette]);

  // Re-read the point whenever the location or product changes. It is not
  // re-read per animation frame: the response already carries the whole recent
  // series, so stepping through the loop needs no extra requests.
  useEffect(() => {
    if (!probe) return;
    let cancelled = false;
    fetchPoint(probe.lat, probe.lon, product)
      .then(res => {
        if (cancelled) return;
        setPoint(res);
        setPointError(null);
      })
      .catch(err => {
        if (cancelled) return;
        setPoint(null);
        setPointError(
          err instanceof NotReadyError
            ? 'Tutkakuvia ei ole vielä ladattu.'
            : 'Lukemien haku ei onnistunut.'
        );
      });
    return () => {
      cancelled = true;
    };
  }, [probe, product]);

  // Derived rather than a separate loading flag, which would have to be set
  // synchronously in the effect above.
  const pointLoading = probe !== null && point === null && pointError === null;

  // Clearing the previous readout belongs in the handlers rather than in the fetch
  // effect: it keeps the derived `pointLoading` honest (a new location shows as
  // loading rather than briefly showing the old location's numbers) and avoids a
  // synchronous state update inside an effect.
  const pickPoint = useCallback((next: { lat: number; lon: number }) => {
    setStation(null);
    setProbe(next);
    setPoint(null);
    setPointError(null);
    setIsDetailCollapsed(false);
    // The readout takes the screen on a phone, so the filter goes back to being
    // its launcher rather than reopening behind it when the readout is closed.
    if (window.matchMedia(MOBILE_QUERY).matches) setIsFilterCollapsed(true);
  }, []);

  const closePoint = useCallback(() => {
    setProbe(null);
    setPoint(null);
    setPointError(null);
  }, []);

  // One detail panel at a time: on desktop both would land on the same right
  // rail, and on a phone both are the whole screen.
  const selectStation = useCallback(async (fmisid: string) => {
    closePoint();
    setIsDetailCollapsed(false);
    if (window.matchMedia(MOBILE_QUERY).matches) setIsFilterCollapsed(true);
    try {
      const res = await fetch(`/api/havainnot/station/${fmisid}`);
      if (!res.ok) return;
      setStation((await res.json()) as StationDetail);
    } catch {
      // A failed detail fetch leaves the previous selection alone rather than
      // clearing the panel out from under the viewer.
    }
  }, [closePoint]);

  const flyToPlace = useCallback(
    (place: Place) => {
      mapState?.map.flyTo({
        center: [place.longitude, place.latitude],
        zoom: place.zoom,
        speed: 1.4,
      });
    },
    [mapState]
  );

  const currentFrameUrl = loop.current ? frameUrl(product, loop.current.stamp, palette) : null;
  const parameters = havainnot.snapshot?.parameters ?? [];
  const detailOpen = probe !== null || station !== null;

  return (
    <div className="dashboard-container weather-map">
      <Map
        grid={meta?.grid ?? null}
        frameUrl={currentFrameUrl}
        opacity={opacity}
        theme={theme}
        attribution={ATTRIBUTION}
        probe={probe}
        onPickPoint={pickPoint}
        onStyleReady={onStyleReady}
        overlayLayerIds={overlayLayerIds}
      />

      {/* The overlays render nothing; they own map layers and report upwards. */}
      <SalamaLayer
        map={mapState?.map ?? null}
        epoch={mapState?.epoch ?? 0}
        enabled={enabled.salama}
        theme={theme}
        onState={setSalama}
      />
      <HavainnotLayer
        map={mapState?.map ?? null}
        epoch={mapState?.epoch ?? 0}
        enabled={enabled.havainnot}
        theme={theme}
        paramCode={paramCode}
        onState={setHavainnot}
        onSelectStation={selectStation}
      />

      <PlaceSearch onPick={flyToPlace} />

      <LayersPanel
        /* The panel stands down while a readout is up on mobile — see Panel's
           `open`. Desktop shows both rails. */
        open={!isMobile || !detailOpen}
        products={meta?.products ?? []}
        product={product}
        onProduct={setProduct}
        accessiblePalette={accessiblePalette}
        onAccessiblePalette={setAccessiblePalette}
        opacity={opacity}
        onOpacity={setOpacityOverride}
        legend={legend}
        frameCount={loop.frames.length}
        salamaOn={enabled.salama}
        onSalamaOn={on => setEnabled(e => ({ ...e, salama: on }))}
        salama={salama}
        havainnotOn={enabled.havainnot}
        onHavainnotOn={on => setEnabled(e => ({ ...e, havainnot: on }))}
        havainnot={havainnot}
        paramCode={paramCode}
        onParamCode={setParamCode}
        theme={theme}
        onToggleTheme={onToggleTheme}
        isCollapsed={isFilterCollapsed}
        onToggleCollapse={() => setIsFilterCollapsed(v => !v)}
        isMobile={isMobile}
      />

      {probe && (
        <PointPanel
          point={point}
          loading={pointLoading}
          error={pointError}
          onClose={closePoint}
          isCollapsed={isDetailCollapsed}
          onToggleCollapse={() => setIsDetailCollapsed(v => !v)}
          isMobile={isMobile}
        />
      )}

      {station && (
        <StationPanel
          station={station}
          parameters={parameters}
          param={parameters.find(p => p.code === paramCode)}
          onClose={() => setStation(null)}
          isCollapsed={isDetailCollapsed}
          onToggleCollapse={() => setIsDetailCollapsed(v => !v)}
          isMobile={isMobile}
        />
      )}

      {/* The clock owns the top centre — but the status banner lands there too,
          and it only appears when there is nothing worth dating anyway. */}
      {!loop.coldStart && !metaError && !loop.error && <FrameClock current={loop.current} />}

      <ReplayBar loop={loop} windowHours={windowHours} onWindowHours={setWindowHours} />

      {/* Cold start is a loading state, not an error: the backend is still filling
          its archive from FMI and will have frames within a minute or two. */}
      {loop.coldStart && (
        <div className="radar-status">Ladataan tutkakuvia Ilmatieteen laitokselta…</div>
      )}
      {(metaError || loop.error) && (
        <div className="radar-status radar-status--error">
          Tutkatietojen haku ei onnistunut. {metaError ?? loop.error}
        </div>
      )}
    </div>
  );
}

export default WeatherMap;
