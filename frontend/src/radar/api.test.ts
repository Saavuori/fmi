import { describe, it, expect } from 'vitest';
import { bearingText, frameUrl, sampleText, type PointSample } from './api';

describe('sampleText', () => {
  const measured: PointSample = { time: '2026-07-25T14:45:00Z', value: 24.5, state: 'measured' };

  it('prints a measured value with its unit', () => {
    expect(sampleText(measured, 'dBZ')).toBe('24.5 dBZ');
  });

  // The distinction this guards is the point of the whole SampleState type: an
  // empty pixel because the radar saw nothing is not the same claim as an empty
  // pixel outside radar range, and only the first is evidence of dry weather.
  it('keeps "no rain" and "no radar coverage" as different answers', () => {
    const noEcho: PointSample = { time: '', value: null, state: 'no_echo' };
    const noCoverage: PointSample = { time: '', value: null, state: 'no_coverage' };
    expect(sampleText(noEcho, 'dBZ')).toBe('ei sadetta');
    expect(sampleText(noCoverage, 'dBZ')).toBe('tutkan ulkopuolella');
    expect(sampleText(noEcho, 'dBZ')).not.toBe(sampleText(noCoverage, 'dBZ'));
  });

  it('never prints a zero for a missing reading', () => {
    for (const state of ['no_echo', 'no_coverage', 'outside_grid'] as const) {
      expect(sampleText({ time: '', value: null, state }, 'mm/h')).not.toMatch(/0/);
    }
    expect(sampleText(undefined, 'dBZ')).toBe('–');
  });
});

describe('bearingText', () => {
  it('names the direction the rain is heading towards', () => {
    expect(bearingText(0)).toBe('pohjoiseen');
    expect(bearingText(45)).toBe('koilliseen');
    expect(bearingText(90)).toBe('itään');
    expect(bearingText(135)).toBe('kaakkoon');
    expect(bearingText(180)).toBe('etelään');
    expect(bearingText(225)).toBe('lounaaseen');
    expect(bearingText(270)).toBe('länteen');
    expect(bearingText(315)).toBe('luoteeseen');
  });

  it('wraps around north', () => {
    expect(bearingText(360)).toBe('pohjoiseen');
    expect(bearingText(358)).toBe('pohjoiseen');
    expect(bearingText(-45)).toBe('luoteeseen');
  });
});

describe('frameUrl', () => {
  it('builds the immutable frame path with an escaped palette', () => {
    expect(frameUrl('dbz', '20260725T1305Z', 'dbz-cvd')).toBe(
      '/api/tutka/frame/dbz/20260725T1305Z.png?palette=dbz-cvd'
    );
  });
});
