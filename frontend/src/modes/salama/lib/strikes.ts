// Mirrors internal/salama/models.go — change one, change both.

export interface Strike {
  latitude: number;
  longitude: number;
  /** Unix seconds. Strikes are faded by age, so the instant matters. */
  timestamp: number;
  multiplicity: number;
  /** Kiloamperes, negative for the common negative cloud-to-ground polarity. */
  peakCurrent: number;
  /** True for an intra-cloud discharge; only cloud-to-ground is a ground hazard. */
  cloudFlash: boolean;
}

export interface StrikesResponse {
  strikes: Strike[];
  windowMinutes: number;
  updated: number;
  attribution: string;
}

export type Theme = 'dark' | 'light';

export const BASEMAP_STYLES: Record<Theme, string> = {
  dark: 'https://basemaps.cartocdn.com/gl/dark-matter-gl-style/style.json',
  light: 'https://basemaps.cartocdn.com/gl/positron-gl-style/style.json',
};

/**
 * Age bands, newest first. Lightning is only interesting in relation to now, so
 * strikes are coloured by recency rather than by intensity: the eye should be
 * drawn to where the storm *is*, and the older strikes should read as its track.
 */
export interface AgeBand {
  /** Upper bound of the band, in minutes. */
  maxMinutes: number;
  label: string;
  color: Record<Theme, string>;
  radius: number;
  opacity: number;
}

export const AGE_BANDS: AgeBand[] = [
  {
    maxMinutes: 5,
    label: 'alle 5 min',
    color: { dark: '#fef08a', light: '#b45309' },
    radius: 7,
    opacity: 1,
  },
  {
    maxMinutes: 20,
    label: '5–20 min',
    color: { dark: '#fb923c', light: '#c2410c' },
    radius: 5.5,
    opacity: 0.9,
  },
  {
    maxMinutes: 60,
    label: '20–60 min',
    color: { dark: '#f472b6', light: '#a21caf' },
    radius: 4.5,
    opacity: 0.75,
  },
  {
    maxMinutes: Number.POSITIVE_INFINITY,
    label: 'yli tunti',
    color: { dark: '#818cf8', light: '#4338ca' },
    radius: 3.5,
    opacity: 0.55,
  },
];

/** Index of the age band a strike falls into, given the current time. */
export function ageBandIndex(strike: Strike, nowSeconds: number): number {
  const minutes = (nowSeconds - strike.timestamp) / 60;
  for (let i = 0; i < AGE_BANDS.length; i++) {
    if (minutes < AGE_BANDS[i].maxMinutes) return i;
  }
  return AGE_BANDS.length - 1;
}

export function strikeTime(timestamp: number): string {
  return new Date(timestamp * 1000).toLocaleTimeString('fi-FI', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

/** Counts strikes per age band, for the panel's summary. */
export function countByBand(strikes: Strike[], nowSeconds: number): number[] {
  const counts = AGE_BANDS.map(() => 0);
  for (const s of strikes) counts[ageBandIndex(s, nowSeconds)] += 1;
  return counts;
}
