import { describe, it, expect } from 'vitest';
import { AGE_BANDS, ageBandIndex, countByBand, type Strike } from './strikes';

const NOW = 1_784_986_400;

function strike(minutesAgo: number): Strike {
  return {
    latitude: 61,
    longitude: 25,
    timestamp: NOW - minutesAgo * 60,
    multiplicity: 1,
    peakCurrent: -14.5,
    cloudFlash: false,
  };
}

describe('ageBandIndex', () => {
  it('places strikes in the band matching their age', () => {
    expect(ageBandIndex(strike(0), NOW)).toBe(0);
    expect(ageBandIndex(strike(4.9), NOW)).toBe(0);
    expect(ageBandIndex(strike(5.1), NOW)).toBe(1);
    expect(ageBandIndex(strike(19), NOW)).toBe(1);
    expect(ageBandIndex(strike(21), NOW)).toBe(2);
    expect(ageBandIndex(strike(59), NOW)).toBe(2);
    expect(ageBandIndex(strike(61), NOW)).toBe(3);
    expect(ageBandIndex(strike(1000), NOW)).toBe(3);
  });

  it('never returns an index outside the band table', () => {
    // A clock skew between browser and server can make a strike look like it is
    // from the future; that must land in the newest band rather than off the end.
    const index = ageBandIndex(strike(-10), NOW);
    expect(index).toBeGreaterThanOrEqual(0);
    expect(index).toBeLessThan(AGE_BANDS.length);
  });
});

describe('countByBand', () => {
  it('returns one count per band, summing to the strike total', () => {
    const strikes = [strike(1), strike(3), strike(10), strike(30), strike(90), strike(120)];
    const counts = countByBand(strikes, NOW);
    expect(counts).toHaveLength(AGE_BANDS.length);
    expect(counts).toEqual([2, 1, 1, 2]);
    expect(counts.reduce((a, b) => a + b, 0)).toBe(strikes.length);
  });

  it('returns zeros rather than an empty array for no strikes', () => {
    // Zero strikes is the normal state in Finland, and the panel renders a row per
    // band unconditionally.
    expect(countByBand([], NOW)).toEqual([0, 0, 0, 0]);
  });
});

describe('AGE_BANDS', () => {
  it('is ordered newest-first with ascending bounds and descending prominence', () => {
    for (let i = 1; i < AGE_BANDS.length; i++) {
      expect(AGE_BANDS[i].maxMinutes).toBeGreaterThan(AGE_BANDS[i - 1].maxMinutes);
      expect(AGE_BANDS[i].radius).toBeLessThanOrEqual(AGE_BANDS[i - 1].radius);
      expect(AGE_BANDS[i].opacity).toBeLessThanOrEqual(AGE_BANDS[i - 1].opacity);
    }
  });

  it('ends with an unbounded band so no strike is unclassifiable', () => {
    expect(AGE_BANDS[AGE_BANDS.length - 1].maxMinutes).toBe(Number.POSITIVE_INFINITY);
  });
});
