// Mirrors internal/havainnot/models.go — change one, change both.

export interface Parameter {
  code: string;
  label: string;
  unit: string;
}

export interface Station {
  fmisid: string;
  name: string;
  region?: string;
  latitude: number;
  longitude: number;
  timestamp: number;
  /**
   * Keyed by FMI parameter code, containing only what the station actually
   * reported. A missing key means "not measured here", which is different from a
   * zero — most stations measure a subset.
   */
  values: Record<string, number>;
}

export interface Reading {
  timestamp: number;
  values: Record<string, number>;
}

export interface StationDetail extends Station {
  series: Reading[];
}

export interface Snapshot {
  stations: Station[];
  parameters: Parameter[];
  updated: number;
  attribution: string;
}

/**
 * What the layer knows and the panel hosting its toggle has to show. It lives
 * here rather than beside the component so the component file exports nothing
 * but the component (which is what keeps fast refresh working).
 */
export interface HavainnotState {
  snapshot: Snapshot | null;
  coldStart: boolean;
  error: string | null;
}

export const EMPTY_HAVAINNOT_STATE: HavainnotState = {
  snapshot: null,
  coldStart: true,
  error: null,
};

/**
 * Colour scale per parameter. Each is a list of [value, colour] stops that
 * MapLibre interpolates between, so a station's marker encodes its reading
 * directly and the map is readable without opening anything.
 *
 * Temperature is diverging around freezing, because 0 °C is the threshold that
 * actually changes behaviour in Finland. The rest are sequential.
 */
export interface Scale {
  stops: [number, string][];
  /** Rounding for the marker label; wind to one decimal, temperature to whole. */
  decimals: number;
}

export const SCALES: Record<string, Scale> = {
  t2m: {
    decimals: 0,
    stops: [
      [-30, '#4c1d95'],
      [-20, '#4338ca'],
      [-10, '#2563eb'],
      [-2, '#38bdf8'],
      [0, '#a5f3fc'],
      [5, '#34d399'],
      [12, '#a3e635'],
      [18, '#fbbf24'],
      [24, '#f97316'],
      [30, '#dc2626'],
    ],
  },
  ws_10min: {
    decimals: 1,
    stops: [
      [0, '#bae6fd'],
      [4, '#38bdf8'],
      [8, '#2563eb'],
      [14, '#f59e0b'],
      [21, '#dc2626'],
      [28, '#7c2d12'],
    ],
  },
  wg_10min: {
    decimals: 1,
    stops: [
      [0, '#bae6fd'],
      [6, '#38bdf8'],
      [12, '#2563eb'],
      [18, '#f59e0b'],
      [25, '#dc2626'],
      [33, '#7c2d12'],
    ],
  },
  rh: {
    decimals: 0,
    stops: [
      [20, '#fbbf24'],
      [50, '#a3e635'],
      [75, '#34d399'],
      [90, '#38bdf8'],
      [100, '#2563eb'],
    ],
  },
  r_1h: {
    decimals: 1,
    stops: [
      [0, '#bae6fd'],
      [0.5, '#38bdf8'],
      [2, '#22c55e'],
      [5, '#fbbf24'],
      [10, '#f97316'],
      [20, '#dc2626'],
    ],
  },
  td: {
    decimals: 0,
    stops: [
      [-30, '#4c1d95'],
      [-10, '#2563eb'],
      [0, '#a5f3fc'],
      [10, '#34d399'],
      [20, '#f97316'],
    ],
  },
  p_sea: {
    decimals: 0,
    stops: [
      [960, '#7c3aed'],
      [990, '#38bdf8'],
      [1013, '#a3e635'],
      [1030, '#fbbf24'],
      [1050, '#dc2626'],
    ],
  },
  vis: {
    decimals: 0,
    stops: [
      [0, '#dc2626'],
      [1000, '#f97316'],
      [5000, '#fbbf24'],
      [20000, '#34d399'],
      [50000, '#38bdf8'],
    ],
  },
  snow_aws: {
    decimals: 0,
    stops: [
      [0, '#e0f2fe'],
      [10, '#7dd3fc'],
      [30, '#38bdf8'],
      [60, '#2563eb'],
      [100, '#1e3a8a'],
    ],
  },
  wd_10min: {
    // Direction is a compass bearing, not a magnitude: a colour ramp would imply
    // that 359° and 1° are opposites. It gets a neutral marker and an arrow-free
    // textual readout instead.
    decimals: 0,
    stops: [
      [0, '#94a3b8'],
      [360, '#94a3b8'],
    ],
  },
};

export const DEFAULT_PARAMETER = 't2m';

/** Colour for a value, matching what the map paints. Used by the legend so the
 * two cannot disagree. */
export function colorFor(code: string, value: number): string {
  const scale = SCALES[code];
  if (!scale) return '#94a3b8';
  const { stops } = scale;
  if (value <= stops[0][0]) return stops[0][1];
  if (value >= stops[stops.length - 1][0]) return stops[stops.length - 1][1];
  for (let i = 1; i < stops.length; i++) {
    if (value > stops[i][0]) continue;
    const [loV, loC] = stops[i - 1];
    const [hiV, hiC] = stops[i];
    const t = (value - loV) / (hiV - loV);
    return mixHex(loC, hiC, t);
  }
  return stops[stops.length - 1][1];
}

function mixHex(a: string, b: string, t: number): string {
  const pa = parseInt(a.slice(1), 16);
  const pb = parseInt(b.slice(1), 16);
  const mix = (shift: number) => {
    const ca = (pa >> shift) & 0xff;
    const cb = (pb >> shift) & 0xff;
    return Math.round(ca + (cb - ca) * t);
  };
  const to2 = (n: number) => n.toString(16).padStart(2, '0');
  return `#${to2(mix(16))}${to2(mix(8))}${to2(mix(0))}`;
}

/** Formats a reading for display, or '–' when the station does not report it. */
export function readingText(
  values: Record<string, number> | undefined,
  param: Parameter
): string {
  const v = values?.[param.code];
  if (v === undefined) return '–';
  const decimals = SCALES[param.code]?.decimals ?? 1;
  if (param.code === 'wd_10min') return `${Math.round(v)}° ${compass(v)}`;
  return `${v.toFixed(decimals)} ${param.unit}`;
}

// Finnish compass points from north, clockwise: pohjoinen, koillinen, itä,
// kaakko, etelä, lounas, länsi, luode.
const POINTS = ['P', 'KOI', 'I', 'KA', 'E', 'LO', 'L', 'LU'];

/**
 * Finnish compass point for a bearing. Note this is the direction the wind blows
 * *from*, which is the meteorological convention FMI reports in — a "southerly"
 * wind comes out of the south.
 */
export function compass(deg: number): string {
  const normalized = ((deg % 360) + 360) % 360;
  return POINTS[Math.round(normalized / 45) % 8];
}

export function observationTime(timestamp: number): string {
  return new Date(timestamp * 1000).toLocaleTimeString('fi-FI', {
    hour: '2-digit',
    minute: '2-digit',
  });
}
