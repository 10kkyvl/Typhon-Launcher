import { describe, expect, it } from 'vitest';
import { matchesCollection } from './collections';

describe('matchesCollection', () => {
  it('requires every collection tag, ignoring case', () => {
    const collection = { tags: ['Co-op', 'cozy'] };
    expect(matchesCollection({ tags: ['cozy', 'CO-OP', 'extra'] }, undefined, collection)).toBe(true);
    expect(matchesCollection({ tags: ['cozy'] }, undefined, collection)).toBe(false);
    expect(matchesCollection({}, undefined, collection)).toBe(false);
  });

  it('matches the status only when the collection sets one', () => {
    expect(matchesCollection({ status: 'playing' }, undefined, { status: 'playing' })).toBe(true);
    expect(matchesCollection({ status: 'paused' }, undefined, { status: 'playing' })).toBe(false);
    expect(matchesCollection({}, undefined, { status: 'playing' })).toBe(false);
    expect(matchesCollection({ status: 'paused' }, undefined, {})).toBe(true);
  });

  it('needs at least one shared genre when genres are set', () => {
    const collection = { genres: ['RPG', 'Strategy'] };
    expect(matchesCollection({}, ['rpg', 'Indie'], collection)).toBe(true);
    expect(matchesCollection({}, ['Indie'], collection)).toBe(false);
    expect(matchesCollection({}, undefined, collection)).toBe(false);
    expect(matchesCollection({}, [], collection)).toBe(false);
  });

  it('combines tags, status and genres with AND', () => {
    const collection = { tags: ['a'], status: 'backlog', genres: ['RPG'] };
    expect(matchesCollection({ tags: ['a'], status: 'backlog' }, ['rpg'], collection)).toBe(true);
    expect(matchesCollection({ tags: ['a'], status: 'backlog' }, ['indie'], collection)).toBe(false);
    expect(matchesCollection({ tags: [], status: 'backlog' }, ['rpg'], collection)).toBe(false);
  });

  it('matches every game when the collection has no criteria', () => {
    expect(matchesCollection({ tags: ['x'], status: 'paused' }, ['Indie'], {})).toBe(true);
  });
});
