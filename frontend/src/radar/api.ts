// Typed access to /api/tutka. The backend mirrors these shapes in
// internal/tutka/handlers.go and internal/tutka/models.go — change one, change
// both.

/** Grid geometry. `corners` is top-left, top-right, bottom-right, bottom-left as
 * [lon, lat], which is exactly the order a MapLibre image source wants. */
export interface GridInfo {
  width: number;
  height: number;
  resolution_m: number;
  bbox_epsg3857: [number, number, number, number];
  corners: [[number, number], [number, number], [number, number], [number, number]];
}

export interface ProductInfo {
  id: string;
  label: string;
  unit: string;
  step_minutes: number;
  palette: string;
  retain_hours: number;
  palettes: string[];
  frames: number;
  latest?: string;
}

export interface ProductsResponse {
  attribution: string;
  grid: GridInfo;
  products: ProductInfo[];
}

export interface FrameRef {
  /** RFC3339 instant the frame was measured. */
  time: string;
  /** Compact UTC stamp used in the frame URL, e.g. 20260725T1305Z. */
  stamp: string;
}

export interface LegendStop {
  value: number;
  label: string;
  color: string;
}

export interface LegendResponse {
  product: string;
  palette: string;
  label: string;
  unit: string;
  stops: LegendStop[];
  attribution: string;
}

/**
 * Why a cell is empty. `no_echo` means the radar looked and found nothing;
 * `no_coverage` means it cannot see there at all. They render the same but they
 * are opposite answers to "is it raining here", so the UI must not merge them.
 */
export type SampleState = 'measured' | 'no_echo' | 'no_coverage' | 'outside_grid';

export interface PointSample {
  time: string;
  /** null unless `state` is 'measured'. */
  value: number | null;
  state: SampleState;
}

export interface Motion {
  speed_kmh: number;
  bearing_deg: number;
  /** Normalised correlation of the best match, 0..1. Low means the field is not
   * moving as one body — a rotating low, or two systems going different ways. */
  confidence: number;
  valid: boolean;
}

export interface ExtrapolationStep {
  minutes_ahead: number;
  value: number | null;
  state: SampleState;
}

export interface Nowcast {
  motion: Motion;
  steps: ExtrapolationStep[];
  /** Minutes until rain reaches the point, or -1 for none inside the horizon. */
  arrival_minutes: number;
  method: string;
}

export interface PointResponse {
  lat: number;
  lon: number;
  product: string;
  unit: string;
  current: PointSample;
  series: PointSample[];
  /** Absent unless two recent frames gave a usable motion estimate. */
  nowcast?: Nowcast;
}

/** Finnish compass point for a bearing, for describing which way rain is heading. */
const POINTS = ['pohjoiseen', 'koilliseen', 'itään', 'kaakkoon', 'etelään', 'lounaaseen', 'länteen', 'luoteeseen'];

export function bearingText(deg: number): string {
  const normalized = ((deg % 360) + 360) % 360;
  return POINTS[Math.round(normalized / 45) % 8];
}

/** Raised when the backend has no data yet (HTTP 503). Callers show a loading
 * state for this rather than an error — it is the documented cold start. */
export class NotReadyError extends Error {
  constructor() {
    super('data not available yet');
    this.name = 'NotReadyError';
  }
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (res.status === 503) throw new NotReadyError();
  if (!res.ok) throw new Error(`${url}: ${res.status}`);
  return (await res.json()) as T;
}

export function fetchProducts(): Promise<ProductsResponse> {
  return getJSON<ProductsResponse>('/api/tutka/products');
}

/** Fetches the animation index. `sinceHours` bounds it to the recent past, which
 * is the common case — the full archive is a week of five-minute frames. */
export async function fetchFrames(product: string, sinceHours?: number): Promise<FrameRef[]> {
  const params = new URLSearchParams({ product });
  if (sinceHours !== undefined) {
    params.set('from', new Date(Date.now() - sinceHours * 3600_000).toISOString());
  }
  const res = await getJSON<{ frames: FrameRef[] }>(`/api/tutka/frames?${params}`);
  return res.frames ?? [];
}

export function frameUrl(product: string, stamp: string, palette: string): string {
  return `/api/tutka/frame/${product}/${stamp}.png?palette=${encodeURIComponent(palette)}`;
}

export function fetchLegend(product: string, palette: string): Promise<LegendResponse> {
  return getJSON<LegendResponse>(`/api/tutka/legend/${product}?palette=${encodeURIComponent(palette)}`);
}

export function fetchPoint(lat: number, lon: number, product: string): Promise<PointResponse> {
  const params = new URLSearchParams({ lat: String(lat), lon: String(lon), product });
  return getJSON<PointResponse>(`/api/tutka/point?${params}`);
}

/** Formats an instant as Finnish local wall-clock time, which is what a viewer
 * comparing the radar to their own window cares about. */
export function clockTime(iso: string): string {
  return new Date(iso).toLocaleTimeString('fi-FI', { hour: '2-digit', minute: '2-digit' });
}

/** The day under the frame clock's time. The weekday is there because the
 * look-back runs to a week, and "18.07." alone does not tell a viewer scrubbing
 * through it which day they have landed on nearly as quickly. */
export function clockDate(iso: string): string {
  return new Date(iso).toLocaleDateString('fi-FI', {
    weekday: 'short',
    day: '2-digit',
    month: '2-digit',
  });
}

/** Renders a sample for display, keeping the no-echo / no-coverage distinction
 * visible rather than printing a reassuring zero for both. */
export function sampleText(sample: PointSample | undefined, unit: string): string {
  if (!sample) return '–';
  switch (sample.state) {
    case 'measured':
      return `${sample.value} ${unit}`;
    case 'no_echo':
      return 'ei sadetta';
    case 'no_coverage':
      return 'tutkan ulkopuolella';
    default:
      return '–';
  }
}
