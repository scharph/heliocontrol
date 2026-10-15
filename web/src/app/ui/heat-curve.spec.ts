import { describe, expect, it } from 'vitest';
import { curveTarget } from './heat-curve';
import { fmtDuration, fromMinutes, toMinutes } from '../core/format';

// Same cases as catalog.TestHeatCurve on the server (values of the real controller).
const p = { limit: 18, atLimit: 24, at0: 28, atMinus15: 33, boost: 1, reduction: -2 };

describe('curveTarget', () => {
  it('matches the server implementation', () => {
    const cases: [number, number][] = [[20, 24], [18, 24], [9, 26], [0, 28], [-7.5, 30.5], [-15, 33], [-20, 33]];
    for (const [at, want] of cases) expect(curveTarget(p, at)).toBeCloseTo(want, 6);
  });
  it('applies the reduction / boost shift', () => {
    expect(curveTarget(p, 0, p.reduction)).toBe(26);
    expect(curveTarget(p, 0, p.boost)).toBe(29);
  });
});

describe('format helpers', () => {
  it('converts clock times', () => {
    expect(toMinutes('24:00')).toBe(1440);
    expect(fromMinutes(390)).toBe('06:30');
    expect(fromMinutes(toMinutes('17:45'))).toBe('17:45');
  });
  it('formats durations', () => {
    expect(fmtDuration(3725)).toBe('01:02:05');
    expect(fmtDuration(90061)).toBe('1d 01:01:01');
    expect(fmtDuration(undefined)).toBe('–');
  });
});
