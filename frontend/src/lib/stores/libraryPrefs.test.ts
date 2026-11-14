import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

class MemoryStorage {
  readonly items = new Map<string, string>();
  getItem(key: string) {
    return this.items.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    this.items.set(key, value);
  }
  removeItem(key: string) {
    this.items.delete(key);
  }
}

class BrokenStorage {
  getItem(): string | null {
    throw new Error('SecurityError');
  }
  setItem() {
    throw new Error('QuotaExceededError');
  }
}

async function fresh(local: object, session: object) {
  vi.stubGlobal('localStorage', local);
  vi.stubGlobal('sessionStorage', session);
  vi.resetModules();
  const prefs = await import('./libraryPrefs');
  const ui = await import('./ui');
  return { ...prefs, libraryView: ui.libraryView };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('library view choices', () => {
  it('start from the defaults with empty storage', async () => {
    const { libraryChoices, librarySort, libraryView, DEFAULT_CHOICES } = await fresh(new MemoryStorage(), new MemoryStorage());

    expect(get(libraryChoices)).toEqual(DEFAULT_CHOICES);
    expect(DEFAULT_CHOICES).toEqual({ filter: 'all', source: 'all', status: 'all', collection: '', search: '', heroHidden: false });
    expect(get(librarySort)).toBe('title');
    expect(get(libraryView)).toBe('grid');
  });

  it('keep grid or list and the sort across a restart, the rest for the session', async () => {
    const local = new MemoryStorage();
    const session = new MemoryStorage();
    const first = await fresh(local, session);
    first.libraryView.set('list');
    first.librarySort.set('playtime');
    first.libraryChoices.set({ filter: 'favorites', source: 'steam', status: 'playing', collection: 'k1', search: 'hades', heroHidden: true });

    const reload = await fresh(local, session);
    expect(get(reload.libraryView)).toBe('list');
    expect(get(reload.librarySort)).toBe('playtime');
    expect(get(reload.libraryChoices)).toEqual({ filter: 'favorites', source: 'steam', status: 'playing', collection: 'k1', search: 'hades', heroHidden: true });

    const restart = await fresh(local, new MemoryStorage());
    expect(get(restart.libraryView)).toBe('list');
    expect(get(restart.librarySort)).toBe('playtime');
    expect(get(restart.libraryChoices)).toEqual(restart.DEFAULT_CHOICES);
  });

  it('fall back to the default for a value they do not know, field by field', async () => {
    const local = new MemoryStorage();
    const session = new MemoryStorage();
    const first = await fresh(local, session);
    first.libraryView.set('list');
    for (const [key] of local.items) local.items.set(key, key.includes('view') ? '"tiles"' : '{not json');
    const choicesKey = [...session.items.keys()][0];
    session.items.set(choicesKey, JSON.stringify({ filter: 'uninstalled', source: 'origin', status: 'won', collection: 7, search: 'x'.repeat(5000), heroHidden: 'yes' }));

    const next = await fresh(local, session);

    expect(get(next.libraryView)).toBe('grid');
    expect(get(next.librarySort)).toBe('title');
    expect(get(next.libraryChoices)).toEqual(next.DEFAULT_CHOICES);
  });

  it('work in memory when storage throws on every read and write', async () => {
    const { libraryChoices, librarySort, libraryView, DEFAULT_CHOICES } = await fresh(new BrokenStorage(), new BrokenStorage());

    expect(get(libraryChoices)).toEqual(DEFAULT_CHOICES);
    libraryView.set('list');
    librarySort.set('size');
    libraryChoices.update((choices) => ({ ...choices, filter: 'hidden' }));

    expect(get(libraryView)).toBe('list');
    expect(get(librarySort)).toBe('size');
    expect(get(libraryChoices).filter).toBe('hidden');
  });

  it('work when there is no storage at all', async () => {
    const { libraryChoices, libraryView } = await fresh(undefined as unknown as object, undefined as unknown as object);

    libraryView.set('list');
    expect(get(libraryView)).toBe('list');
    expect(get(libraryChoices).filter).toBe('all');
  });
});
