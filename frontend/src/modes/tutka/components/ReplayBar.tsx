import React from 'react';
import { Pause, Play, Radio, SkipBack, SkipForward } from 'lucide-react';
import { clockDateTime, clockTime } from '../lib/api';
import type { RadarLoop } from '../hooks/useRadarLoop';

const SPEEDS = [2, 4, 8];

interface ReplayBarProps {
  loop: RadarLoop;
}

/**
 * The animation transport: play/pause, a scrubber over the loaded window, and the
 * clock of the frame on screen.
 *
 * The clock is the most important element here. A radar animation with no
 * timestamp invites the viewer to read an hour-old frame as current weather, so
 * the time is always visible and the newest frame is labelled as live.
 */
export const ReplayBar: React.FC<ReplayBarProps> = ({ loop }) => {
  const { frames, index, current, playing, speed, loadedCount, live } = loop;

  if (frames.length === 0) return null;

  const last = frames.length - 1;
  const loadPercent = frames.length > 0 ? Math.round((loadedCount / frames.length) * 100) : 0;

  return (
    <div className="replay-bar replay-bar--radar">
      <button
        className="replay-play"
        onClick={loop.togglePlaying}
        aria-label={playing ? 'Pysäytä' : 'Toista'}
        title={playing ? 'Pysäytä' : 'Toista'}
      >
        {playing ? <Pause size={18} /> : <Play size={18} />}
      </button>

      <button
        className="icon-btn radar-step"
        onClick={() => loop.step(-1)}
        disabled={index === 0}
        aria-label="Edellinen ruutu"
        title="Edellinen ruutu"
      >
        <SkipBack size={15} />
      </button>
      <button
        className="icon-btn radar-step"
        onClick={() => loop.step(1)}
        disabled={index >= last}
        aria-label="Seuraava ruutu"
        title="Seuraava ruutu"
      >
        <SkipForward size={15} />
      </button>

      <div className="radar-clock">
        <span className="replay-clock replay-clock--wide">
          {current ? clockTime(current.time) : '--:--'}
        </span>
        <span className="radar-clock__date">{current ? clockDateTime(current.time) : ''}</span>
      </div>

      <div className="radar-scrub">
        <input
          className="replay-scrubber"
          type="range"
          min={0}
          max={last}
          step={1}
          value={index}
          onChange={e => loop.setIndex(Number(e.target.value))}
          aria-label="Siirry ajassa"
        />
        {/* While the loop is still downloading, say so rather than letting a
            half-loaded animation look like stuttering. */}
        {loadedCount < frames.length && (
          <span className="radar-scrub__loading">ladataan {loadPercent} %</span>
        )}
      </div>

      <button
        className={`radar-live${live ? ' radar-live--on' : ''}`}
        onClick={loop.goLive}
        title="Siirry uusimpaan"
      >
        <Radio size={13} />
        <span>nyt</span>
      </button>

      <div className="radar-speeds" role="group" aria-label="Nopeus">
        {SPEEDS.map(s => (
          <button
            key={s}
            className={`radar-speed${s === speed ? ' radar-speed--on' : ''}`}
            onClick={() => loop.setSpeed(s)}
            title={`${s} ruutua sekunnissa`}
          >
            {s}×
          </button>
        ))}
      </div>
    </div>
  );
};
