import React from 'react';
import { MapPin, X } from 'lucide-react';
import { stopPanelClick } from '../shared/hooks/useCollapsiblePanel';
import { Panel } from '../shared/components/Panel';
import { type Nowcast, type PointResponse, bearingText, clockTime, sampleText } from '../radar/api';

interface PointPanelProps {
  point: PointResponse | null;
  loading: boolean;
  error: string | null;
  onClose: () => void;
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  isMobile: boolean;
}

/**
 * The point readout: what the radar sees at one location now and over the last
 * hour.
 *
 * This is only possible because the backend keeps the raw values rather than
 * pictures — there is no way to recover a dBZ figure from a coloured pixel.
 */
export const PointPanel: React.FC<PointPanelProps> = ({
  point,
  loading,
  error,
  onClose,
  isCollapsed,
  onToggleCollapse,
  isMobile,
}) => {
  const bodyCollapsed = !isMobile && isCollapsed;

  return (
    <Panel
      variant="detail"
      isMobile={isMobile}
      open
      ariaLabel="Avaa paikkatiedot"
      collapsed={isCollapsed}
      onToggleCollapse={onToggleCollapse}
    >
      <div className="detail-header" onClick={bodyCollapsed ? undefined : stopPanelClick}>
        <div>
          <div className="detail-title">
            <MapPin size={15} />
            <span>
              {point ? `${point.lat.toFixed(3)}, ${point.lon.toFixed(3)}` : 'Paikka'}
            </span>
          </div>
          <div className="detail-subtitle">
            {point ? sampleText(point.current, point.unit) : loading ? 'haetaan…' : '—'}
          </div>
        </div>
        <button className="icon-btn" onClick={onClose} aria-label="Sulje">
          <X size={16} />
        </button>
      </div>

      {!bodyCollapsed && (
        <div className="detail-content" onClick={stopPanelClick}>
          {error && <div className="status-callout">{error}</div>}
          {!error && loading && !point && <div className="panel-note">Haetaan lukemia…</div>}

          {point && (
            <>
              <div className="detail-facts">
                <div className="fact-row">
                  <span>Nyt</span>
                  <span>{sampleText(point.current, point.unit)}</span>
                </div>
                <div className="fact-row">
                  <span>Mittaushetki</span>
                  <span>{clockTime(point.current.time)}</span>
                </div>
              </div>

              {point.nowcast && <NowcastBlock nowcast={point.nowcast} unit={point.unit} />}

              {/* A sparkline of the last hour. Bars are drawn from the measured
                  values only; gaps stay gaps rather than being interpolated into a
                  smooth line that implies data we do not have. */}
              <div className="filter-section-title">Viimeinen tunti</div>
              <Sparkline point={point} />

              <div className="panel-note">
                {point.current.state === 'no_coverage'
                  ? 'Tutka ei näe tähän paikkaan, joten poutakin on tässä vain arvaus.'
                  : 'Lukema on tutkan havainto sadealueen korkeudelta, ei maanpinnan sademäärä.'}
              </div>
            </>
          )}
        </div>
      )}
    </Panel>
  );
};

/** Below this correlation the single-vector assumption is not describing the field
 * well enough to put a minute count in front of someone. */
const MIN_TRUSTED_CONFIDENCE = 0.55;

/**
 * The extrapolation block.
 *
 * Everything here is phrased to keep the viewer's expectations calibrated. It says
 * "arvio" (estimate) rather than forecast, names the mechanism, and when the
 * motion estimate is weak it withholds the minute count instead of dressing up a
 * bad number — a confident "rain in 15 minutes" that is wrong is worse than
 * saying the field is not moving as one body.
 */
const NowcastBlock: React.FC<{ nowcast: Nowcast; unit: string }> = ({ nowcast, unit }) => {
  const { motion, arrival_minutes: arrival, steps } = nowcast;
  const trusted = motion.confidence >= MIN_TRUSTED_CONFIDENCE;
  const peak = steps.reduce<number | null>(
    (max, s) => (s.value !== null && (max === null || s.value > max) ? s.value : max),
    null
  );

  return (
    <>
      <div className="filter-section-title">Arvio seuraavalle 45 min</div>

      <div className="detail-facts">
        <div className="fact-row">
          <span>Sade liikkuu</span>
          <span>
            {Math.round(motion.speed_kmh)} km/h {bearingText(motion.bearing_deg)}
          </span>
        </div>
        {trusted && (
          <div className="fact-row">
            <span>Sade saapuu</span>
            <span>{arrival > 0 ? `noin ${arrival} min` : 'ei 45 min sisällä'}</span>
          </div>
        )}
        {trusted && peak !== null && (
          <div className="fact-row">
            <span>Voimakkuus enintään</span>
            <span>
              {peak} {unit}
            </span>
          </div>
        )}
      </div>

      <div className={`radar-nowcast-note${trusted ? '' : ' radar-nowcast-note--weak'}`}>
        {trusted
          ? 'Arvio siirtää nykyistä sadetta mitatulla nopeudella. Se ei tunne sateen syntyä tai hiipumista, joten pitkälle se ei kanna.'
          : 'Sadealue ei liiku yhtenä kokonaisuutena, joten saapumisaikaa ei kannata arvioida. Tämä on tavallista puuskittaisissa kuurosateissa.'}
      </div>
    </>
  );
};

const Sparkline: React.FC<{ point: PointResponse }> = ({ point }) => {
  const values = point.series.map(s => (s.state === 'measured' ? (s.value ?? 0) : null));
  const measured = values.filter((v): v is number => v !== null);

  if (measured.length === 0) {
    // An empty chart has three quite different causes, and saying "no
    // observations" for all of them would be wrong twice over: a full hour of
    // "the radar looked and saw nothing" is a real, useful answer, and so is "the
    // radar cannot see here".
    if (point.series.length === 0) {
      return <div className="panel-note">Ei tutkakuvia viimeisen tunnin ajalta.</div>;
    }
    if (point.series.every(s => s.state === 'no_echo')) {
      return <div className="panel-note">Ei sadetta viimeisen tunnin aikana.</div>;
    }
    if (point.series.every(s => s.state === 'no_coverage')) {
      return <div className="panel-note">Tutka ei näe tähän paikkaan.</div>;
    }
    return <div className="panel-note">Ei mitattuja lukemia viimeisen tunnin aikana.</div>;
  }

  const max = Math.max(...measured);
  // dBZ can be negative, so the floor is the series minimum rather than zero.
  const min = Math.min(...measured, 0);
  const span = max - min || 1;

  return (
    <div className="radar-spark" role="img" aria-label={`Lukemat: ${measured.join(', ')} ${point.unit}`}>
      {point.series.map((sample, i) => {
        const v = values[i];
        const height = v === null ? 0 : Math.max(2, ((v - min) / span) * 100);
        return (
          <span
            key={sample.time}
            className={`radar-spark__bar${v === null ? ' radar-spark__bar--gap' : ''}`}
            style={{ height: `${height}%` }}
            title={`${clockTime(sample.time)} · ${sampleText(sample, point.unit)}`}
          />
        );
      })}
    </div>
  );
};
