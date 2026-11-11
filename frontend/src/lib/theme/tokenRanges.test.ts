import { describe, expect, it } from 'vitest';
import { goSources } from '../testing/sources';
import { TOKEN_RANGES, tokenRangeProblem } from './tokenRanges';

function goRanges(): Record<string, [number, number]> {
  const source = goSources['internal/theme/tokens.go'];
  expect(source, 'internal/theme/tokens.go').toBeTypeOf('string');
  const block = /var numberRanges = map\[string\]\[2\]float64\{([\s\S]*?)\n\}/.exec(source)?.[1];
  expect(block, 'numberRanges block').toBeTypeOf('string');
  return Object.fromEntries(
    [...(block as string).matchAll(/"(--[a-z0-9-]+)":\s*\{\s*([-\d.]+),\s*([-\d.]+)\s*\}/g)].map((m) => [m[1], [Number(m[2]), Number(m[3])]]),
  );
}

describe('token ranges', () => {
  it('are the ranges Go enforces, token for token', () => {
    const go = goRanges();
    expect(Object.keys(go).length).toBeGreaterThan(0);
    expect(Object.fromEntries(Object.entries(TOKEN_RANGES).map(([name, r]) => [name, [r.min, r.max]]))).toEqual(go);
  });

  it('give every ranged token a step that lands on both ends', () => {
    for (const [name, r] of Object.entries(TOKEN_RANGES)) {
      expect(r.step, name).toBeGreaterThan(0);
      expect(Number.isInteger(Math.round(((r.max - r.min) / r.step) * 1e6) / 1e6), name).toBe(true);
    }
  });

  it('find the problem with a value outside the range or not a plain number', () => {
    expect(tokenRangeProblem('--ambient-dim', '0.6')).toBeNull();
    expect(tokenRangeProblem('--ambient-dim', '0')).toBeNull();
    expect(tokenRangeProblem('--ambient-dim', ' 1 ')).toBeNull();
    for (const value of ['2', '-0.1', '1.01', 'abc', '', '1e0', '50%']) {
      expect(tokenRangeProblem('--ambient-dim', value), value).toEqual({ name: '--ambient-dim', min: 0, max: 1 });
    }
  });

  it('leave tokens without a range alone', () => {
    expect(tokenRangeProblem('--radius-md', '2')).toBeNull();
    expect(tokenRangeProblem('--bg', 'red')).toBeNull();
  });
});
