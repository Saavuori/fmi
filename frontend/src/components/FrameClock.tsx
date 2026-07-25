import React from 'react';
import { clockDate, clockTime } from '../radar/api';
import type { FrameRef } from '../radar/api';

interface FrameClockProps {
  current: FrameRef | undefined;
}

/**
 * The timestamp of the frame on screen, centred above the map.
 *
 * This is the single most important label in the app: a radar animation without
 * one invites a viewer to read an hour-old frame as the current weather. It used
 * to sit inside the transport bar at the bottom, where it competed with the
 * controls for the eye and sat as far as possible from the rain it was dating.
 * Above the map it is in the reading path of someone watching a front move.
 *
 * The date line carries the day as well as the time, which is what keeps the
 * label honest once the look-back runs to a week.
 */
export const FrameClock: React.FC<FrameClockProps> = ({ current }) => {
  if (!current) return null;

  return (
    <div className="frame-clock">
      <span className="frame-clock__time">{clockTime(current.time)}</span>
      <span className="frame-clock__date">{clockDate(current.time)}</span>
    </div>
  );
};
