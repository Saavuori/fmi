import React from 'react';
import { ChevronLeft, Layers, Moon, Sun, Thermometer, X, Zap } from 'lucide-react';
import { stopPanelClick } from '../shared/hooks/useCollapsiblePanel';
import { Panel } from '../shared/components/Panel';
import type { LegendResponse, ProductInfo } from '../radar/api';
import type { Theme } from '../map/theme';
import { WINDOWS } from '../radar/windows';
import { type SalamaState, AGE_BANDS } from '../layers/salama/strikes';
import { type HavainnotState, SCALES } from '../layers/havainnot/observations';

interface LayersPanelProps {
  /* Radar — the base layer, always on. */
  products: ProductInfo[];
  product: string;
  onProduct: (id: string) => void;
  accessiblePalette: boolean;
  onAccessiblePalette: (on: boolean) => void;
  opacity: number;
  onOpacity: (v: number) => void;
  windowHours: number;
  onWindowHours: (h: number) => void;
  legend: LegendResponse | null;
  frameCount: number;

  /* Lightning overlay. */
  salamaOn: boolean;
  onSalamaOn: (on: boolean) => void;
  salama: SalamaState;

  /* Weather-station overlay. */
  havainnotOn: boolean;
  onHavainnotOn: (on: boolean) => void;
  havainnot: HavainnotState;
  paramCode: string;
  onParamCode: (code: string) => void;

  theme: Theme;
  onToggleTheme: () => void;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  isMobile: boolean;
  /** False while a readout is up on mobile — see Panel's `open`. */
  open?: boolean;
}

/**
 * The one control panel: the radar product below, the two overlays as toggles
 * that reveal their own controls when switched on, and the view settings.
 *
 * This used to be three panels behind three tabs, one per mode. Weather does not
 * arrive in modes — rain, lightning and what the nearest station actually
 * measured are all answers about the same place at the same moment — so they are
 * layers over one map, and the bottom tab bar that used to switch between them
 * is gone. The room that bought is why the timeline can run the full width.
 */
export const LayersPanel: React.FC<LayersPanelProps> = ({
  products,
  product,
  onProduct,
  accessiblePalette,
  onAccessiblePalette,
  opacity,
  onOpacity,
  windowHours,
  onWindowHours,
  legend,
  frameCount,
  salamaOn,
  onSalamaOn,
  salama,
  havainnotOn,
  onHavainnotOn,
  havainnot,
  paramCode,
  onParamCode,
  theme,
  onToggleTheme,
  isCollapsed,
  onToggleCollapse,
  isMobile,
  open = true,
}) => {
  const bodyCollapsed = !isMobile && isCollapsed;
  const selected = products.find(p => p.id === product);
  const parameters = havainnot.snapshot?.parameters ?? [];
  const param = parameters.find(p => p.code === paramCode);
  const scale = SCALES[paramCode];
  const strikeTotal = salama.data?.strikes.length ?? 0;

  return (
    <Panel
      variant="filter"
      isMobile={isMobile}
      open={open}
      ariaLabel="Avaa karttavalikko"
      collapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
    >
      <div className="panel-header" onClick={bodyCollapsed ? undefined : stopPanelClick}>
        <div className="panel-title">
          <Layers size={16} />
          <span>Kartta</span>
        </div>
        {/* The same button closes the desktop rail into its sliver and dismisses
            the mobile overlay — on a phone it is the only way back to the map,
            so it says so with an X rather than a fold-away chevron. */}
        {!bodyCollapsed && (
          <button
            className="icon-btn"
            onClick={e => {
              e.stopPropagation();
              onToggleCollapse();
            }}
            aria-label={isMobile ? 'Sulje karttavalikko' : 'Pienennä karttavalikko'}
          >
            {isMobile ? <X size={18} /> : <ChevronLeft size={16} />}
          </button>
        )}
      </div>

      {!bodyCollapsed && (
        <div className="filter-content" onClick={stopPanelClick}>
          <div className="panel-stats">
            <span className="conn-dot" title="Ilmatieteen laitos" />
            <span>
              {selected ? selected.label : '—'} · {frameCount} ruutua
            </span>
          </div>

          <div className="filter-scroll-area">
            <div className="filter-section-title">Tuote</div>
            <div className="layer-toggles radar-products">
              {products.map(p => (
                <button
                  key={p.id}
                  className={`layer-toggle ${p.id === product ? 'on' : ''}`}
                  onClick={() => onProduct(p.id)}
                  aria-pressed={p.id === product}
                >
                  <span>{p.label}</span>
                  <span className="radar-unit">{p.unit}</span>
                </button>
              ))}
            </div>

            <div className="filter-section-title" style={{ marginTop: 14 }}>
              Aikaväli
            </div>
            <div className="layer-toggles radar-windows">
              {WINDOWS.map(w => (
                <button
                  key={w.hours}
                  className={`layer-toggle ${w.hours === windowHours ? 'on' : ''}`}
                  onClick={() => onWindowHours(w.hours)}
                  aria-pressed={w.hours === windowHours}
                >
                  <span>{w.label}</span>
                </button>
              ))}
            </div>

            {legend && (
              <>
                <div className="filter-section-title" style={{ marginTop: 14 }}>
                  {legend.label} ({legend.unit})
                </div>
                {/* Built from the same stops the pixels were coloured from, so the
                    scale cannot drift from what is drawn on the map. */}
                <div className="radar-legend">
                  <div className="radar-legend__bar">
                    {legend.stops.map(stop => (
                      <span
                        key={stop.value}
                        className="radar-legend__swatch"
                        style={{ background: stop.color }}
                        title={`${stop.label} ${legend.unit}`}
                      />
                    ))}
                  </div>
                  <div className="radar-legend__labels">
                    {legend.stops.map(stop => (
                      <span key={stop.value} className="radar-legend__label">
                        {stop.label}
                      </span>
                    ))}
                  </div>
                </div>
              </>
            )}

            {/* ---- The overlays. Each one's controls only exist while it is on:
                    an off layer has nothing to configure, and a panel that shows
                    ten parameter buttons for a layer that isn't drawn is noise. */}
            <div className="filter-section-title" style={{ marginTop: 16 }}>
              Tasot
            </div>

            <div className="layer-block layer-block--salama">
              <button
                className={`layer-toggle layer-toggle--wide ${salamaOn ? 'on' : ''}`}
                onClick={() => onSalamaOn(!salamaOn)}
                aria-pressed={salamaOn}
              >
                <Zap size={14} />
                <span>Salamat</span>
                <span className="layer-toggle__count">
                  {salamaOn ? `${strikeTotal} kpl` : ''}
                </span>
              </button>

              {salamaOn && (
                <div className="layer-block__body">
                  <div className="category-list">
                    {AGE_BANDS.map((band, i) => (
                      <div className="category-row" key={band.label}>
                        <span
                          className="category-swatch"
                          style={{ background: band.color[theme], opacity: band.opacity }}
                        />
                        <span className="category-label">{band.label}</span>
                        <span className="category-count">{salama.counts[i]}</span>
                      </div>
                    ))}
                  </div>

                  <div className="category-list" style={{ marginTop: 6 }}>
                    <div className="category-row">
                      <span className="category-swatch salama-swatch--ground" />
                      <span className="category-label">Maasalama</span>
                    </div>
                    <div className="category-row">
                      <span className="category-swatch salama-swatch--cloud" />
                      <span className="category-label">Pilvisalama</span>
                    </div>
                  </div>

                  {/* Zero strikes is the normal state in Finland, and saying so stops
                      an empty layer reading as a broken feed. */}
                  {strikeTotal === 0 && !salama.coldStart && (
                    <div className="panel-note">
                      Ei salamoita viimeisen {salama.data?.windowMinutes ?? 120} minuutin
                      aikana. Tyhjä kartta on Suomessa tavallisin tilanne.
                    </div>
                  )}
                  {salama.error && <div className="panel-note">{salama.error}</div>}
                </div>
              )}
            </div>

            <div className="layer-block layer-block--havainnot">
              <button
                className={`layer-toggle layer-toggle--wide ${havainnotOn ? 'on' : ''}`}
                onClick={() => onHavainnotOn(!havainnotOn)}
                aria-pressed={havainnotOn}
              >
                <Thermometer size={14} />
                <span>Havaintoasemat</span>
                <span className="layer-toggle__count">
                  {havainnotOn ? `${havainnot.snapshot?.stations.length ?? 0} kpl` : ''}
                </span>
              </button>

              {havainnotOn && (
                <div className="layer-block__body">
                  <div className="layer-toggles obs-params">
                    {parameters.map(p => (
                      <button
                        key={p.code}
                        className={`layer-toggle ${p.code === paramCode ? 'on' : ''}`}
                        onClick={() => onParamCode(p.code)}
                        aria-pressed={p.code === paramCode}
                      >
                        <span>{p.label}</span>
                        <span className="obs-unit">{p.unit}</span>
                      </button>
                    ))}
                  </div>

                  {scale && param && paramCode !== 'wd_10min' && (
                    <>
                      <div className="filter-section-title" style={{ marginTop: 10 }}>
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
                                      (scale.stops[scale.stops.length - 1][0] -
                                        scale.stops[0][0])) *
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

                  <div className="panel-note">
                    Vain suureen mittaavat asemat näkyvät kartalla. Napauta asemaa
                    nähdäksesi sen viime tuntien havainnot.
                  </div>
                  {havainnot.error && <div className="panel-note">{havainnot.error}</div>}
                </div>
              )}
            </div>

            <div className="filter-section-title" style={{ marginTop: 16 }}>
              Näkymä
            </div>
            <label className="radar-slider">
              <span>Peittävyys</span>
              <input
                type="range"
                min={0.2}
                max={1}
                step={0.05}
                value={opacity}
                onChange={e => onOpacity(Number(e.target.value))}
                aria-label="Sadekuvan peittävyys"
              />
              <span className="radar-slider__value">{Math.round(opacity * 100)} %</span>
            </label>

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

            {/* The default ramps run green -> yellow -> red, which is the pairing a
                red-green deficiency cannot separate — and on a radar map that is
                the difference between a shower and a storm. */}
            <div className="layer-toggles" style={{ marginTop: 8 }}>
              <button
                className={`layer-toggle ${accessiblePalette ? 'on' : ''}`}
                onClick={() => onAccessiblePalette(!accessiblePalette)}
                aria-pressed={accessiblePalette}
              >
                <span>Saavutettavat värit</span>
              </button>
            </div>

            <div className="panel-note">
              Napauta karttaa nähdäksesi paikan lukemat. Sade piirtyy läpinäkyvänä
              siellä missä tutka ei näe.
            </div>
          </div>
        </div>
      )}
    </Panel>
  );
};
