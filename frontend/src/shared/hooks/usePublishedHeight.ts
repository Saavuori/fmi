import { useLayoutEffect, useState } from 'react';

/**
 * Publishes an element's height to a custom property on <html>, so bottom-anchored
 * map controls can clear a docked bar whose height they cannot know (it depends on
 * the mode, on whether the bar is mounted at all, and on how its rows wrap).
 *
 * Returns a callback ref rather than a RefObject: the element comes and goes with
 * the component that renders it, and a callback ref makes that a state change the
 * effect can depend on instead of a mutation it would have to poll for. The
 * property is set to 0 whenever the element is absent, so a mode without a bar
 * doesn't inherit the last one's height.
 */
export function usePublishedHeight(cssVar: string): (el: HTMLElement | null) => void {
  const [el, setEl] = useState<HTMLElement | null>(null);

  useLayoutEffect(() => {
    const root = document.documentElement;
    const publish = (px: number) => root.style.setProperty(cssVar, `${Math.round(px)}px`);

    if (!el) {
      publish(0);
      return;
    }
    publish(el.offsetHeight);
    // The bar's height is content-dependent — a wrapped row, a loading note
    // appearing — and none of that fires a window resize.
    const ro = new ResizeObserver(() => publish(el.offsetHeight));
    ro.observe(el);
    return () => {
      ro.disconnect();
      publish(0);
    };
  }, [el, cssVar]);

  return setEl;
}
