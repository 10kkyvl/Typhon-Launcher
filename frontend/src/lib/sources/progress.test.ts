import { describe, expect, it } from 'vitest';
import {
  isIndeterminate,
  overallPercent,
  progressPercent,
  stagePercent,
  stageWeights,
  type SourceProgress,
} from './progress';

function entry(stage: SourceProgress['stage'], done: number, total: number, sawMatch = false): SourceProgress {
  return { stage, done, total, startedAt: 0, sawMatch };
}

describe('stageWeights', () => {
  it('sum to 100', () => {
    expect(Object.values(stageWeights).reduce((a, b) => a + b, 0)).toBe(100);
  });
});

describe('stagePercent', () => {
  it.each([
    ['fetch', 0, 1, 0],
    ['fetch', 1, 1, 5],
    ['parse', 0, 4000, 5],
    ['parse', 2000, 4000, 15],
    ['parse', 4000, 4000, 25],
    ['match', 0, 10, 25],
    ['match', 5, 10, 55],
    ['match', 10, 10, 85],
    ['apply', 0, 20, 85],
    ['apply', 20, 20, 95],
    ['save', 0, 1, 95],
    ['save', 1, 1, 100],
  ] as const)('%s %i/%i is %f%%', (stage, done, total, want) => {
    expect(stagePercent(stage, done, total)).toBeCloseTo(want, 6);
  });

  it('treats total 0 as the start of the stage', () => {
    expect(stagePercent('apply', 0, 0)).toBeCloseTo(85, 6);
    expect(stagePercent('match', 3, 0)).toBeCloseTo(25, 6);
  });

  it('clamps done above total and below zero', () => {
    expect(stagePercent('parse', 9000, 4000)).toBeCloseTo(25, 6);
    expect(stagePercent('parse', -5, 4000)).toBeCloseTo(5, 6);
  });

  it('redistributes the match weight when matching is absent', () => {
    expect(stagePercent('apply', 0, 20, false)).toBeCloseTo(62.5, 6);
    expect(stagePercent('apply', 20, 20, false)).toBeCloseTo(87.5, 6);
    expect(stagePercent('save', 1, 1, false)).toBeCloseTo(100, 6);
  });
});

describe('progressPercent', () => {
  it('uses the full weights until apply when match was not seen', () => {
    expect(progressPercent(entry('parse', 4000, 4000))).toBeCloseTo(25, 6);
  });

  it('keeps match weight after the match stage was seen', () => {
    expect(progressPercent(entry('apply', 0, 20, true))).toBeCloseTo(85, 6);
  });

  it('skips match weight in apply and save when match never came', () => {
    expect(progressPercent(entry('apply', 0, 20, false))).toBeCloseTo(62.5, 6);
    expect(progressPercent(entry('save', 1, 1, false))).toBeCloseTo(100, 6);
  });

  it('never decreases when match is absent', () => {
    const steps = [
      entry('fetch', 0, 1),
      entry('fetch', 1, 1),
      entry('parse', 0, 800),
      entry('parse', 800, 800),
      entry('apply', 0, 0),
      entry('apply', 5, 5),
      entry('save', 0, 1),
      entry('save', 1, 1),
    ].map(progressPercent);
    for (let i = 1; i < steps.length; i++) expect(steps[i]).toBeGreaterThanOrEqual(steps[i - 1]);
  });
});

describe('overallPercent', () => {
  it('is zero without sources', () => {
    expect(overallPercent([])).toBe(0);
  });

  it('averages the sources', () => {
    expect(overallPercent([entry('fetch', 1, 1), entry('match', 10, 10, true)])).toBeCloseTo(45, 6);
  });

  it('handles a source with total 0', () => {
    expect(overallPercent([entry('apply', 0, 0, true), entry('save', 1, 1, true)])).toBeCloseTo(92.5, 6);
  });
});

describe('isIndeterminate', () => {
  it('is true only when total is zero', () => {
    expect(isIndeterminate(entry('apply', 0, 0))).toBe(true);
    expect(isIndeterminate(entry('apply', 0, 3))).toBe(false);
  });
});
