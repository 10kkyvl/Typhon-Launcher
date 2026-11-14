import { describe, expect, it } from 'vitest';
import { STATUS_FILTERS, matchesStatus } from './statuses';

describe('matchesStatus', () => {
  it('lets every game through on all', () => {
    expect(matchesStatus('', 'all')).toBe(true);
    expect(matchesStatus('dropped', 'all')).toBe(true);
    expect(matchesStatus(undefined, 'all')).toBe(true);
  });

  it('matches the exact status', () => {
    expect(matchesStatus('mastered', 'mastered')).toBe(true);
    expect(matchesStatus('completed', 'mastered')).toBe(false);
  });

  it('treats an empty or unknown status as not started', () => {
    expect(matchesStatus('', 'none')).toBe(true);
    expect(matchesStatus(undefined, 'none')).toBe(true);
    expect(matchesStatus('something', 'none')).toBe(true);
    expect(matchesStatus('playing', 'none')).toBe(false);
  });

  it('offers every status exactly once, not started last', () => {
    expect(STATUS_FILTERS).toEqual(['all', 'playing', 'backlog', 'paused', 'completed', 'mastered', 'dropped', 'none']);
  });
});
