import React from 'react';
import { X } from 'lucide-react';
import { stopPanelClick } from '../shared/hooks/useCollapsiblePanel';
import { Panel } from '../shared/components/Panel';
import {
  type Parameter,
  type StationDetail,
  observationTime,
  readingText,
} from '../layers/havainnot/observations';

interface StationPanelProps {
  station: StationDetail;
  parameters: Parameter[];
  /** The parameter the map is coloured by; its recent series is charted below. */
  param: Parameter | undefined;
  onClose: () => void;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  isMobile: boolean;
}

/**
 * The readout for a tapped weather station. Lifted out of the old havainnot mode
 * unchanged in substance — it is a detail panel like the radar point readout, and
 * on a phone the two take the same screen, so the map only ever opens one.
 */
export const StationPanel: React.FC<StationPanelProps> = ({
  station,
  parameters,
  param,
  onClose,
  isCollapsed,
  onToggleCollapse,
  isMobile,
}) => (
  <Panel
    variant="detail"
    isMobile={isMobile}
    open
    className="station-detail"
    ariaLabel="Avaa aseman tiedot"
    collapsed={isCollapsed}
    onToggleCollapse={onToggleCollapse}
  >
    <div className="detail-header" onClick={!isMobile && isCollapsed ? undefined : stopPanelClick}>
      <div>
        <div className="detail-title">{station.name}</div>
        <div className="detail-subtitle">
          {station.region ? `${station.region} · ` : ''}
          {observationTime(station.timestamp)}
        </div>
      </div>
      <button className="icon-btn" onClick={onClose} aria-label="Sulje">
        <X size={16} />
      </button>
    </div>

    {(isMobile || !isCollapsed) && (
      <div className="detail-content" onClick={stopPanelClick}>
        <div className="telemetry-grid">
          {parameters.map(p => (
            <div className="telemetry-item" key={p.code}>
              <span className="telemetry-label">{p.label}</span>
              <span className="telemetry-value">{readingText(station.values, p)}</span>
            </div>
          ))}
        </div>

        {param && (
          <>
            <div className="filter-section-title">{param.label} · viime tunnit</div>
            <div className="obs-series">
              {station.series
                .filter(r => r.values[param.code] !== undefined)
                .slice(-24)
                .map(r => (
                  <div className="fact-row" key={r.timestamp}>
                    <span>{observationTime(r.timestamp)}</span>
                    <span>{readingText(r.values, param)}</span>
                  </div>
                ))}
              {station.series.every(r => r.values[param.code] === undefined) && (
                <div className="panel-note">
                  Tämä asema ei mittaa {param.label.toLowerCase()}.
                </div>
              )}
            </div>
          </>
        )}
      </div>
    )}
  </Panel>
);
