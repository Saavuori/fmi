import { describe, it, expect } from 'vitest';
import { SCALES, type Parameter, colorFor, compass, readingText } from './observations';

// The parameter list is served by the backend (internal/havainnot/models.go), not
// declared here, so the tests describe the ones they exercise.
const temp: Parameter = { code: 't2m', label: 'Lämpötila', unit: '°C' };
const wind: Parameter = { code: 'ws_10min', label: 'Tuuli', unit: 'm/s' };
const dir: Parameter = { code: 'wd_10min', label: 'Tuulen suunta', unit: '°' };

describe('compass', () => {
  it('maps the eight cardinal and intercardinal bearings', () => {
    expect(compass(0)).toBe('P'); // pohjoinen
    expect(compass(45)).toBe('KOI'); // koillinen
    expect(compass(90)).toBe('I'); // itä
    expect(compass(135)).toBe('KA'); // kaakko
    expect(compass(180)).toBe('E'); // etelä
    expect(compass(225)).toBe('LO'); // lounas
    expect(compass(270)).toBe('L'); // länsi
    expect(compass(315)).toBe('LU'); // luode
  });

  it('wraps around north rather than falling off the table', () => {
    expect(compass(360)).toBe('P');
    expect(compass(359)).toBe('P');
    expect(compass(1)).toBe('P');
    // A negative bearing should still resolve; FMI does not send them, but a
    // computed difference elsewhere could.
    expect(compass(-90)).toBe('L');
    expect(compass(-45)).toBe('LU');
  });

  it('rounds to the nearest point at the boundaries', () => {
    expect(compass(22)).toBe('P');
    expect(compass(23)).toBe('KOI');
  });
});

describe('colorFor', () => {
  it('returns the exact stop colour on a stop', () => {
    expect(colorFor('t2m', 0)).toBe('#a5f3fc');
    expect(colorFor('t2m', 30)).toBe('#dc2626');
  });

  it('clamps outside the scale rather than wrapping', () => {
    // A -40 C reading in Lapland must not come back as the hot end of the ramp.
    expect(colorFor('t2m', -40)).toBe(colorFor('t2m', -30));
    expect(colorFor('t2m', 45)).toBe(colorFor('t2m', 30));
  });

  it('interpolates between stops', () => {
    const mid = colorFor('t2m', 2.5); // between the 0 and 5 stops
    expect(mid).toMatch(/^#[0-9a-f]{6}$/);
    expect(mid).not.toBe(colorFor('t2m', 0));
    expect(mid).not.toBe(colorFor('t2m', 5));
  });

  it('falls back to a neutral colour for an unknown parameter', () => {
    expect(colorFor('not_a_parameter', 5)).toBe('#94a3b8');
  });
});

describe('readingText', () => {
  it('formats a value with its unit at the parameter’s precision', () => {
    expect(readingText({ t2m: 15.74 }, temp)).toBe('16 °C');
    expect(readingText({ ws_10min: 4.25 }, wind)).toBe('4.3 m/s');
  });

  it('renders wind direction as a bearing plus a compass point', () => {
    expect(readingText({ wd_10min: 180 }, dir)).toBe('180° E');
  });

  it('distinguishes a real zero from a station that does not measure it', () => {
    // A missing key means "not measured here", which is not the same claim as 0.
    expect(readingText({ t2m: 0 }, temp)).toBe('0 °C');
    expect(readingText({}, temp)).toBe('–');
    expect(readingText(undefined, temp)).toBe('–');
  });
});

describe('scale table', () => {
  it('covers the parameters the backend serves', () => {
    // If the backend's Parameters table grows, a scale has to be added here or the
    // marker falls back to neutral grey and the legend disappears.
    for (const code of [
      't2m',
      'ws_10min',
      'wg_10min',
      'wd_10min',
      'rh',
      'r_1h',
      'td',
      'p_sea',
      'vis',
      'snow_aws',
    ]) {
      expect(SCALES[code], `missing scale for ${code}`).toBeDefined();
    }
  });

  it('keeps every scale’s stops in ascending order', () => {
    for (const [code, scale] of Object.entries(SCALES)) {
      for (let i = 1; i < scale.stops.length; i++) {
        expect(scale.stops[i][0], `${code} stop ${i}`).toBeGreaterThanOrEqual(scale.stops[i - 1][0]);
      }
    }
  });
});
