import { describe, expect, it } from 'vitest';
import { filterHistory } from './historyFilter';
import { Kind, type Record } from '../../../bindings/typhon/internal/history';

const record = (over: Partial<Record> = {}): Record => ({
  id: over.id ?? Math.random().toString(36),
  kind: Kind.KindInstalled,
  at: '2026-01-01T00:00:00Z',
  title: 'Cyberpunk 2077',
  bytesKnown: false,
  ...over,
});

describe('filterHistory', () => {
  const records = [
    record({ id: 'a', kind: Kind.KindInstalled, title: 'Cyberpunk 2077' }),
    record({ id: 'b', kind: Kind.KindRemoved, title: 'Hades' }),
    record({ id: 'c', kind: Kind.KindUpdated, title: 'Doom Eternal' }),
  ];

  it('returns everything when no filter is given', () => {
    expect(filterHistory(records)).toEqual(records);
  });

  it('filters by a set of kinds', () => {
    expect(filterHistory(records, { kinds: [Kind.KindInstalled, Kind.KindRemoved] }).map((r) => r.id)).toEqual([
      'a',
      'b',
    ]);
  });

  it('an empty kinds array behaves like no filter', () => {
    expect(filterHistory(records, { kinds: [] })).toEqual(records);
  });

  it('filters by a case-insensitive query', () => {
    expect(filterHistory(records, { query: 'hades' }).map((r) => r.id)).toEqual(['b']);
    expect(filterHistory(records, { query: 'HADES' }).map((r) => r.id)).toEqual(['b']);
  });

  it('matches Cyrillic case-insensitively', () => {
    const cyrillic = [record({ id: 'x', title: 'Ведьмак 3' })];
    expect(filterHistory(cyrillic, { query: 'ВЕДЬМАК' }).map((r) => r.id)).toEqual(['x']);
  });

  it('combines kind and query filters', () => {
    expect(filterHistory(records, { kinds: [Kind.KindUpdated], query: 'doom' }).map((r) => r.id)).toEqual(['c']);
  });

  it('trims whitespace from the query', () => {
    expect(filterHistory(records, { query: '  hades  ' }).map((r) => r.id)).toEqual(['b']);
  });

  it('returns nothing when the query matches no title', () => {
    expect(filterHistory(records, { query: 'no such game' })).toEqual([]);
  });
});
