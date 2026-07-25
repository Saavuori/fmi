import React from 'react';
import { Pause, Play, Radio, SkipBack, SkipForward } from 'lucide-react';
import { usePublishedHeight } from '../shared/hooks/usePublishedHeight';
import { clockDateTime, clockTime } from '../radar/api';
import type { RadarLoop } from '../radar/useRadarLoop';

const SPEEDS = [2, 4, 8];

interface ReplayBarProps {
  loop: RadarLoop;
  /** The look-back the viewer asked for, in hours. */
  windowHours: number;
}

/** Below this fraction of the requested window, say so. The archive fills
 * backwards from the present after a deploy, so a timeline that only reaches a few
 * hours back is correct-but-still-growing — and indistinguishable from missing
 * data unless it is spelled out. */
const COVERAGE_NOTE_THRESHOLD = 0.8;

/**
 * The animation transport: play/pause, a scrubber over the loaded window, and the
 * clock of the frame on screen.
 *
 * The clock is the most important element here. A radar animation with no
 * timestamp invites the viewer to read an hour-old frame as current weather, so
 * the time is always visible and the newest frame is labelled as live.
 *
 * On a phone the bar is docked full-bleed above the tab bar and the scrubber
 * wraps onto a row of its own (tutka.css), so dragging through time gets the
 * whole width of the screen instead of the gap left between the clock and the
 * speed buttons. Its measured height is published to `--timeline-height` for the
 * map controls that have to sit above it.
 */
export const ReplayBar: React.FC<ReplayBarProps> = ({ loop, windowHours }) => {
  const { frames, index, current, playing, speed, loadedCount, live } = loop;
  const barRef = usePublishedHeight('--timeline-height');

  // Below the hook, not above it: an empty archive still has to run every hook
  // this component declares, and the callback ref publishes 0 when the bar is
  // absent anyway.
  if (frames.length === 0) return null;

  const last = frames.length - 1;
  const loadPercent = frames.length > 0 ? Math.round((loadedCount / frames.length) * 100) : 0;

  const spanHours =
    (new Date(frames[last].time).getTime() - new Date(frames[0].time).getTime()) / 3_600_000;
  const stillFilling = frames.length > 1 && spanHours < windowHours * COVERAGE_NOTE_THRESHOLD;

  return (
    <div className="replay-bar replay-bar--radar" ref={barRef}>
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
        {/* Two different "not everything is here yet" states share this slot.
            Image preloading takes priority because it is the one that makes
            playback stutter. */}
        {loadedCount < frames.length ? (
          <span className="radar-scrub__loading">ladataan {loadPercent} %</span>
        ) : (
          stillFilling && (
            <span className="radar-scrub__loading">
              historia ulottuu {spanHours < 1 ? '<1' : Math.round(spanHours)} h taakse
            </span>
          )
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
