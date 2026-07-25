import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { type FrameRef, NotReadyError, fetchFrames, frameUrl } from './api';

/** How often to re-check the index for newly published frames. The backend polls
 * FMI every minute, so anything faster only adds requests. */
const INDEX_REFRESH_MS = 60_000;

/** Extra dwell on the newest frame before playback stops. Without it the run ends
 * the instant the last frame appears, which reads as a stall mid-animation rather
 * than as "this is now". */
const END_DWELL_FRAMES = 4;

/** Concurrent image preloads. Enough to fill the loop quickly, few enough that
 * the current frame is not queued behind a dozen others. */
const PRELOAD_CONCURRENCY = 4;

/** Playback rate, in frames per second. Not adjustable: at five minutes per frame
 * this is the pace at which a front's movement reads as movement, and a speed
 * picker only offered viewers a way to make the animation harder to follow. */
const PLAYBACK_FPS = 2;

export interface RadarLoop {
  frames: FrameRef[];
  index: number;
  current: FrameRef | undefined;
  playing: boolean;
  loadedCount: number;
  /** True once there is at least one frame to draw. */
  ready: boolean;
  /** True while the backend is still filling its archive (HTTP 503). */
  coldStart: boolean;
  error: string | null;
  /** True when the view is pinned to the newest frame and follows new arrivals. */
  live: boolean;
  setIndex: (i: number) => void;
  setPlaying: (p: boolean) => void;
  togglePlaying: () => void;
}

/**
 * Drives the radar animation: it keeps the frame index current, preloads the
 * images so playback does not stutter, and advances through them once.
 *
 * Once, not forever. The run ends on the newest frame and stays there, so a map
 * nobody is watching is showing the present rather than replaying the past — and
 * the arrangement that follows from it is that the newest frame is where every
 * idle state lands: after a run, after a refresh brings a new frame in, after a
 * scrub to the right-hand end.
 *
 * Preloading is done with plain Image objects rather than by holding decoded
 * bitmaps. Frames are served immutable with a one-year max-age, so warming the
 * HTTP cache is all that is needed — and it leaves the browser to manage the
 * memory of a week-long archive instead of us.
 */
export function useRadarLoop(product: string, palette: string, windowHours: number): RadarLoop {
  const [frames, setFrames] = useState<FrameRef[]>([]);
  const [index, setIndexState] = useState(0);
  const [playing, setPlaying] = useState(true);
  const [loadedCount, setLoadedCount] = useState(0);
  const [coldStart, setColdStart] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [live, setLive] = useState(true);

  // Reset playback when the product or window changes: the old position means
  // nothing against a different frame list. This is React's documented
  // adjust-state-during-render pattern rather than an effect — an effect would
  // render one frame with the stale index before correcting it.
  const selectionKey = `${product}|${windowHours}`;
  const [prevSelectionKey, setPrevSelectionKey] = useState(selectionKey);
  if (selectionKey !== prevSelectionKey) {
    setPrevSelectionKey(selectionKey);
    setFrames([]);
    setIndexState(0);
    setLive(true);
    setLoadedCount(0);
  }

  // The dwell counter is animation bookkeeping the UI never renders, so it lives
  // in a ref to avoid a re-render per tick.
  const dwellRef = useRef(0);
  // Whether the view is following new arrivals, readable from the refresh
  // closure without making it depend on (and restart with) `live`.
  const liveRef = useRef(live);
  const indexRef = useRef(index);
  const playingRef = useRef(playing);
  useEffect(() => {
    liveRef.current = live;
    indexRef.current = index;
    playingRef.current = playing;
  }, [live, index, playing]);

  // Load (and periodically refresh) the frame index.
  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      try {
        const next = await fetchFrames(product, windowHours);
        if (cancelled) return;
        setColdStart(false);
        setError(null);
        setFrames(prev => {
          if (prev.length === 0 && next.length > 1 && playingRef.current) {
            // The first list for this product and window, with playback on: start
            // at the oldest frame so the run actually plays through to now. The
            // branch below would otherwise pin straight to the newest frame, and
            // since the loop stops there rather than wrapping, the animation
            // would never run at all — the app would open on a still.
            setIndexState(0);
            setLive(false);
          } else if (liveRef.current && next.length > 0) {
            // Pin to the newest frame only if the view was already there.
            setIndexState(next.length - 1);
          } else if (prev.length > 0 && next.length > 0) {
            // Otherwise keep showing the same instant even though its position
            // moved as older frames aged out of the window. Yanking a viewer who
            // scrubbed back to an hour ago forward to now would lose their place
            // every minute.
            const currentStamp = prev[Math.min(indexRef.current, prev.length - 1)]?.stamp;
            const moved = next.findIndex(f => f.stamp === currentStamp);
            if (moved >= 0) setIndexState(moved);
          }
          return next;
        });
      } catch (err) {
        if (cancelled) return;
        if (err instanceof NotReadyError) {
          setColdStart(true);
          setError(null);
        } else {
          setError(err instanceof Error ? err.message : String(err));
        }
      }
    };

    load();
    const timer = window.setInterval(load, INDEX_REFRESH_MS);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [product, windowHours]);

  // Preload the frames of the current product and palette, oldest first so the
  // loop becomes playable in order.
  useEffect(() => {
    if (frames.length === 0) return;
    let cancelled = false;
    let cursor = 0;
    let done = 0;

    const next = () => {
      if (cancelled || cursor >= frames.length) return;
      const frame = frames[cursor++];
      const img = new Image();
      const finish = () => {
        if (cancelled) return;
        done += 1;
        setLoadedCount(done);
        next();
      };
      img.onload = finish;
      // A frame that fails to load must not stall the queue behind it.
      img.onerror = finish;
      img.src = frameUrl(product, frame.stamp, palette);
    };

    for (let i = 0; i < PRELOAD_CONCURRENCY; i++) next();
    return () => {
      cancelled = true;
    };
  }, [frames, product, palette]);

  // Restart the loaded count when the palette changes, since every URL changes
  // with it. Same during-render adjustment as the selection reset above.
  const [prevPalette, setPrevPalette] = useState(palette);
  if (palette !== prevPalette) {
    setPrevPalette(palette);
    setLoadedCount(0);
  }

  // The animation tick.
  useEffect(() => {
    if (!playing || frames.length < 2) return;
    const timer = window.setInterval(() => {
      setIndexState(prev => {
        const last = frames.length - 1;
        if (prev >= last) {
          // Hold on the newest frame, then stop. The animation does not wrap:
          // running it forever means the map spends most of its time showing the
          // past while looking like a live view, and the wrap from "now" back to
          // an hour ago is the moment a viewer is most likely to misread. Ending
          // on the newest frame leaves the map showing the current weather, which
          // is the answer they came for; play restarts the run from the beginning.
          if (dwellRef.current < END_DWELL_FRAMES) {
            dwellRef.current += 1;
            return prev;
          }
          dwellRef.current = 0;
          setPlaying(false);
          return prev;
        }
        const nextIndex = prev + 1;
        if (nextIndex >= last) setLive(true);
        return nextIndex;
      });
    }, 1000 / PLAYBACK_FPS);
    return () => window.clearInterval(timer);
  }, [playing, frames.length]);

  const setIndex = useCallback(
    (i: number) => {
      const clamped = Math.max(0, Math.min(i, Math.max(frames.length - 1, 0)));
      setIndexState(clamped);
      setLive(clamped === frames.length - 1);
      dwellRef.current = 0;
    },
    [frames.length]
  );

  // Play from a finished run starts over rather than doing nothing: the loop
  // stops on the newest frame, so "play" there can only sensibly mean "run it
  // again from the start".
  const togglePlaying = useCallback(() => {
    if (playing) {
      setPlaying(false);
      return;
    }
    if (frames.length > 1 && index >= frames.length - 1) {
      setIndexState(0);
      setLive(false);
      dwellRef.current = 0;
    }
    setPlaying(true);
  }, [playing, index, frames.length]);

  const safeIndex = Math.min(index, Math.max(frames.length - 1, 0));

  return useMemo(
    () => ({
      frames,
      index: safeIndex,
      current: frames[safeIndex],
      playing,
      loadedCount,
      ready: frames.length > 0,
      coldStart,
      error,
      live,
      setIndex,
      setPlaying,
      togglePlaying,
    }),
    [
      frames,
      safeIndex,
      playing,
      loadedCount,
      coldStart,
      error,
      live,
      setIndex,
      togglePlaying,
    ]
  );
}
