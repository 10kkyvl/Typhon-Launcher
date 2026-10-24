import { describe, expect, it } from 'vitest';
import { candidateGroups, candidateLabel, familyIds, relativeCandidate, showSimilar } from './similar';

const base = { releasesLoading: false, releasesFailed: false, releaseCount: 0, relatedSettled: true, hasGameId: true };

describe('relativeCandidate', () => {
  it('is null when the candidate is this game', () => {
    expect(relativeCandidate({ matchedCandidate: { id: 'g1', title: 'A', year: 2000 } }, 'g1')).toBeNull();
  });
  it('is null when there is no candidate', () => {
    expect(relativeCandidate({ matchedCandidate: { id: '', title: '', year: 0 } }, 'g1')).toBeNull();
  });
  it('keeps a relative and drops an unknown year', () => {
    expect(relativeCandidate({ matchedCandidate: { id: 'g2', title: 'B', year: 0 } }, 'g1')).toEqual({ title: 'B', year: 0 });
    expect(relativeCandidate({ matchedCandidate: { id: 'g2', title: 'B', year: 2010 } }, 'g1')).toEqual({ title: 'B', year: 2010 });
  });
});

describe('candidateLabel', () => {
  it('adds the year only when known', () => {
    expect(candidateLabel({ title: 'B', year: 2010 })).toBe('B 2010');
    expect(candidateLabel({ title: 'B', year: 0 })).toBe('B');
  });
});

describe('showSimilar', () => {
  it('shows only for a settled game without own releases', () => {
    expect(showSimilar(base)).toBe(true);
    expect(showSimilar({ ...base, releaseCount: 1 })).toBe(false);
    expect(showSimilar({ ...base, releasesLoading: true })).toBe(false);
    expect(showSimilar({ ...base, releasesFailed: true })).toBe(false);
    expect(showSimilar({ ...base, relatedSettled: false })).toBe(false);
    expect(showSimilar({ ...base, hasGameId: false })).toBe(false);
  });
});

describe('familyIds and candidateGroups', () => {
  it('takes game ids and skips empty ones', () => {
    expect(familyIds([{ game: { id: 'a' } }, { game: { id: '' } }, { game: { id: 'b' } }])).toEqual(['a', 'b']);
  });
  it('puts server candidates first group, the rest local', () => {
    const groups = candidateGroups([{ source: 'local' }, { source: 'server' }, {}]);
    expect(groups.server).toEqual([{ source: 'server' }]);
    expect(groups.local).toEqual([{ source: 'local' }, {}]);
  });
});
