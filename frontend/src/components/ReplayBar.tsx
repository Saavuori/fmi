import React from 'react';
import { Pause, Play } from 'lucide-react';
import { usePublishedHeight } from '../shared/hooks/usePublishedHeight';
import { WINDOWS } from '../radar/windows';
import type { RadarLoop } from '../radar/useRadarLoop';

interface ReplayBarProps {
  loop: RadarLoop;
  /** The look-back the viewer asked for, in hours. */
  windowHours: number;
  onWindowHours: (h: number) => void;
}

/** Below this fraction of the requested window, say so. The archive fills
 * backwards from the present after a deploy, so a timeline that only reaches a few
 * hours back is correct-but-still-growing — and indistinguishable from missing
 * data unless it is spelled out. */
const COVERAGE_NOTE_THRESHOLD = 0.8;

/**
 * The animation transport: play/pause, a scrubber over the loaded window, and the
 * look-back that window covers.
 *
 * Three controls, deliberately. The step buttons, the live pill and the speed
 * picker all came out: stepping a frame at a time is what the scrubber does with
 * a drag, the loop now ends on the newest frame so there is nothing to jump back
 * to, and playback is fixed at 2 fps because the other rates only made the
 * animation harder to read. What that room bought is the look-back — 1 h through
 * 7 vrk, previously two clicks away inside the map menu — sitting next to the
 * scrubber it governs, which is the one thing here that is genuinely a choice.
 *
 * The clock is not here either: it is above the map (`FrameClock`), where a
 * viewer watching the rain move does not have to look down to find out which
 * moment they are watching.
 *
 * On a phone the bar is docked full-bleed along the bottom edge and the window
 * buttons wrap onto a row of their own (radar.css). Its measured height is
 * published to `--timeline-height` for the map controls that sit above it.
 */
export const ReplayBar: React.FC<ReplayBarProps> = ({ loop, windowHours, onWindowHours }) => {
  const { frames, index, playing, loadedCount } = loop;
  const barRef = usePublishedHeight('--timeline-height');

  const hasFrames = frames.length > 0;
  const last = Math.max(frames.length - 1, 0);
  const loadPercent = hasFrames ? Math.round((loadedCount / frames.length) * 100) : 0;

  const spanHours = hasFrames
    ? (new Date(frames[last].time).getTime() - new Date(frames[0].time).getTime()) / 3_600_000
    : 0;
  const stillFilling = frames.length > 1 && spanHours < windowHours * COVERAGE_NOTE_THRESHOLD;

  const windowButtons = (
    <div className="radar-windows-bar" role="group" aria-label="Aikaväli">
      {WINDOWS.map(w => (
        <button
          key={w.hours}
          className={`radar-window${w.hours === windowHours ? ' radar-window--on' : ''}`}
          onClick={() => onWindowHours(w.hours)}
          aria-pressed={w.hours === windowHours}
          title={`Näytä viimeiset ${w.label}`}
        >
          {w.label}
        </button>
      ))}
    </div>
  );

  // Switching the window empties the frame list until the new one loads. The bar
  // stays up through that with the window buttons alone, rather than vanishing
  // out from under the control that was just pressed and stranding the viewer on
  // a week of frames with no way back.
  if (!hasFrames) {
    return (
      <div className="replay-bar replay-bar--radar replay-bar--empty" ref={barRef}>
        {windowButtons}
      </div>
    );
  }

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

      {windowButtons}
    </div>
  );
};
