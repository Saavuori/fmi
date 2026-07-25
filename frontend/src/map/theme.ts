// Theme identity shared by the CSS variables (via the data-theme attribute the
// app shell sets on <html>) and the MapLibre basemap. The shell owns the toggle
// and persistence; this module only maps the theme to the map's own colours.

export type Theme = 'dark' | 'light';

export const BASEMAP_STYLES: Record<Theme, string> = {
  dark: 'https://basemaps.cartocdn.com/gl/dark-matter-gl-style/style.json',
  light: 'https://basemaps.cartocdn.com/gl/positron-gl-style/style.json',
};

/**
 * Default overlay opacity per theme. Radar sits on top of the basemap, so the
 * two have to share the frame: the dark basemap is quiet enough to take a nearly
 * opaque overlay, while Positron's white land needs a little more transparency
 * for coastlines and place names to stay readable through the rain.
 */
export const DEFAULT_OPACITY: Record<Theme, number> = {
  dark: 0.85,
  light: 0.75,
};

/** Colour of the pin marking the point being read out. */
export const PROBE_COLORS: Record<Theme, string> = {
  dark: '#f8fafc',
  light: '#0f172a',
};
