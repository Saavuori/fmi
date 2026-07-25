import React from 'react';
import { ChevronLeft, CloudRain, Moon, Sun } from 'lucide-react';
import { stopPanelClick } from '../../../shared/hooks/useCollapsiblePanel';
import { BottomSheet } from '../../../shared/components/BottomSheet';
import type { LegendResponse, ProductInfo } from '../lib/api';
import type { Theme } from '../lib/theme';
import { WINDOWS } from '../lib/windows';

interface FilterPanelProps {
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
  theme: Theme;
  onToggleTheme: () => void;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  isMobile: boolean;
  /** False while a detail sheet is up on mobile — see BottomSheet's `open`. */
  open?: boolean;
}

export const FilterPanel: React.FC<FilterPanelProps> = ({
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
  theme,
  onToggleTheme,
  isCollapsed,
  onToggleCollapse,
  isMobile,
  open = true,
}) => {
  const bodyCollapsed = !isMobile && isCollapsed;
  const selected = products.find(p => p.id === product);

  return (
    <BottomSheet
      variant="filter"
      isMobile={isMobile}
      open={open}
      ariaLabel="Avaa tutkavalikko"
      collapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
    >
      <div className="panel-header" onClick={bodyCollapsed ? undefined : stopPanelClick}>
        <div className="panel-title">
          <CloudRain size={16} />
          <span>Tutka</span>
        </div>
        {!bodyCollapsed && (
          <button
            className="icon-btn"
            onClick={e => {
              e.stopPropagation();
              onToggleCollapse();
            }}
            aria-label="Pienennä tutkavalikko"
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

            <div className="filter-section-title" style={{ marginTop: 14 }}>
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
    </BottomSheet>
  );
};
