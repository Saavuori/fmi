import { useCallback, useEffect, useMemo, useState } from 'react';
import Map from './components/Map';
import { FilterPanel } from './components/FilterPanel';
import { PointPanel } from './components/PointPanel';
import { ReplayBar } from './components/ReplayBar';
import { useRadarLoop } from './hooks/useRadarLoop';
import { useIsMobile, MOBILE_QUERY } from '../../shared/hooks/useMediaQuery';
import {
  type LegendResponse,
  type PointResponse,
  type ProductsResponse,
  NotReadyError,
  fetchLegend,
  fetchPoint,
  fetchProducts,
  frameUrl,
} from './lib/api';
import { type Theme, DEFAULT_OPACITY } from './lib/theme';
import './tutka.css';

interface TutkaAppProps {
  theme: Theme;
  onToggleTheme: () => void;
}

function TutkaApp({ theme, onToggleTheme }: TutkaAppProps) {
  const isMobile = useIsMobile();

  const [meta, setMeta] = useState<ProductsResponse | null>(null);
  const [metaError, setMetaError] = useState<string | null>(null);
  const [product, setProduct] = useState('dbz');
  const [accessiblePalette, setAccessiblePalette] = useState(false);
  const [windowHours, setWindowHours] = useState(1);
  const [legend, setLegend] = useState<LegendResponse | null>(null);

  // Opacity is derived, not stored: the theme's default applies until the viewer
  // picks their own. Storing it would need an effect to re-apply the default on a
  // theme switch, which is a cascading render for something a fallback expresses
  // directly.
  const [opacityOverride, setOpacityOverride] = useState<number | null>(null);
  const opacity = opacityOverride ?? DEFAULT_OPACITY[theme];

  const [probe, setProbe] = useState<{ lat: number; lon: number } | null>(null);
  const [point, setPoint] = useState<PointResponse | null>(null);
  const [pointError, setPointError] = useState<string | null>(null);

  const [isFilterCollapsed, setIsFilterCollapsed] = useState<boolean>(
    typeof window !== 'undefined' ? window.matchMedia(MOBILE_QUERY).matches : false
  );
  const [isPointCollapsed, setIsPointCollapsed] = useState(false);

  // The palette id follows the product's unit and the accessibility preference.
  // The backend rejects a palette whose unit does not match the product, so this
  // has to derive from the selected product rather than being free-form.
  const palette = useMemo(() => {
    const base = meta?.products.find(p => p.id === product)?.palette ?? 'dbz';
    return accessiblePalette ? `${base}-cvd` : base;
  }, [meta, product, accessiblePalette]);

  const loop = useRadarLoop(product, palette, windowHours);

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
    setProbe(next);
    setPoint(null);
    setPointError(null);
    setIsPointCollapsed(false);
    // Two sheets cannot share one bottom edge on a phone, so folding the filter
    // away keeps the readout reachable.
    if (window.matchMedia(MOBILE_QUERY).matches) setIsFilterCollapsed(true);
  }, []);

  const closePoint = useCallback(() => {
    setProbe(null);
    setPoint(null);
    setPointError(null);
  }, []);

  const currentFrameUrl = loop.current ? frameUrl(product, loop.current.stamp, palette) : null;

  return (
    <div className="dashboard-container mode-tutka">
      <Map
        grid={meta?.grid ?? null}
        frameUrl={currentFrameUrl}
        opacity={opacity}
        theme={theme}
        attribution={meta?.attribution ?? 'Ilmatieteen laitos, CC BY 4.0'}
        probe={probe}
        onPickPoint={pickPoint}
      />

      <FilterPanel
        /* The filter sheet stands down while a readout is up on mobile — see
           BottomSheet's `open`. Desktop shows both rails. */
        open={!isMobile || !probe}
        products={meta?.products ?? []}
        product={product}
        onProduct={setProduct}
        accessiblePalette={accessiblePalette}
        onAccessiblePalette={setAccessiblePalette}
        opacity={opacity}
        onOpacity={setOpacityOverride}
        windowHours={windowHours}
        onWindowHours={setWindowHours}
        legend={legend}
        frameCount={loop.frames.length}
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
          isCollapsed={isPointCollapsed}
          onToggleCollapse={() => setIsPointCollapsed(v => !v)}
          isMobile={isMobile}
        />
      )}

      <ReplayBar loop={loop} windowHours={windowHours} />

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

export default TutkaApp;
