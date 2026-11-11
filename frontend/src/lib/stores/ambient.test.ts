import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

type Ambient = typeof import('./ambient');
let ambient: Ambient;

beforeEach(async () => {
  vi.useFakeTimers();
  vi.resetModules();
  ambient = await import('./ambient');
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

let withArt = new Set<string>();
let suggestion: string | null = null;
const target = () => ambient.pickAmbient(get(ambient.ambientState), suggestion, (id) => withArt.has(id));

beforeEach(() => {
  withArt = new Set(['last-played', 'home', 'elden', 'a', 'b', 'card', 'page']);
  suggestion = null;
});

describe('which game the backdrop follows', () => {
  it('shows nothing before any game is known', () => {
    expect(target()).toBeNull();
  });

  it('a screen claim wins over a rested card, which wins over the suggestion', () => {
    const { HOVER_REST_MS, claimAmbient, hoverAmbient } = ambient;
    suggestion = 'last-played';
    const leave = hoverAmbient('card');
    vi.advanceTimersByTime(HOVER_REST_MS);
    expect(target()).toBe('card');
    const release = claimAmbient('page');
    expect(target()).toBe('page');
    release();
    expect(target()).toBe('card');
    leave();
    vi.advanceTimersByTime(ambient.HOVER_LEAVE_MS);
    expect(target()).toBe('page');
  });

  it('a claim wins even before its art is known: the page shows its own game or nothing', () => {
    suggestion = 'last-played';
    const release = ambient.claimAmbient('catalog-new');
    expect(target()).toBe('catalog-new');
    release();
    expect(target()).toBe('last-played');
  });

  it('keeps the last claimed game when a screen without a game of its own takes over', () => {
    suggestion = 'last-played';
    ambient.claimAmbient('elden')();
    expect(target()).toBe('elden');
  });

  it('never keeps a claimed game that has no art, or one that is gone', () => {
    suggestion = 'last-played';
    ambient.claimAmbient('elden')();
    ambient.claimAmbient('sideloaded-no-art')();
    expect(target()).toBe('elden');
    withArt.delete('elden');
    expect(target()).toBe('last-played');
  });

  it('remembers a claimed game once its art arrives', () => {
    suggestion = 'last-played';
    ambient.claimAmbient('late-art')();
    expect(target()).toBe('last-played');
    withArt.add('late-art');
    expect(target()).toBe('late-art');
  });
});

describe('overlapping claims', () => {
  it('releases in any order leave no claim behind', () => {
    const { claimAmbient } = ambient;
    suggestion = 'home';
    const first = claimAmbient('a');
    const second = claimAmbient('b');
    first();
    expect(get(ambient.ambientState).claim).toBe('b');
    second();
    expect(get(ambient.ambientState).claim).toBeNull();
    const leave = ambient.hoverAmbient('card');
    vi.advanceTimersByTime(ambient.HOVER_REST_MS);
    expect(target()).toBe('card');
    leave();
  });

  it('the newest live claim wins and an older one takes over when it is released', () => {
    const { claimAmbient } = ambient;
    const first = claimAmbient('a');
    const second = claimAmbient('b');
    expect(target()).toBe('b');
    second();
    expect(target()).toBe('a');
    first();
    expect(get(ambient.ambientState).claim).toBeNull();
  });

  it('a release runs once: calling it again does not remove another claim of the same game', () => {
    const { claimAmbient } = ambient;
    const first = claimAmbient('a');
    const second = claimAmbient('a');
    first();
    first();
    expect(get(ambient.ambientState).claim).toBe('a');
    second();
    expect(get(ambient.ambientState).claim).toBeNull();
  });

  it('a claim of no game never wipes another claimant', () => {
    const release = ambient.claimAmbient('a');
    const none = ambient.claimAmbient(null);
    expect(target()).toBe('a');
    none();
    expect(target()).toBe('a');
    release();
  });
});

describe('hover intent on the backdrop', () => {
  it('follows a card only after the pointer has rested on it', () => {
    const { HOVER_REST_MS, hoverAmbient } = ambient;
    suggestion = 'home';
    const leave = hoverAmbient('a');
    vi.advanceTimersByTime(HOVER_REST_MS - 1);
    expect(target()).toBe('home');
    vi.advanceTimersByTime(1);
    expect(target()).toBe('a');
    leave();
  });

  it('a fast pass over several cards changes nothing', () => {
    const { HOVER_LEAVE_MS, HOVER_REST_MS, hoverAmbient } = ambient;
    suggestion = 'home';
    for (const id of ['a', 'b', 'c', 'd', 'e']) {
      const leave = hoverAmbient(id);
      vi.advanceTimersByTime(60);
      leave();
    }
    vi.advanceTimersByTime(HOVER_REST_MS + HOVER_LEAVE_MS);
    expect(target()).toBe('home');
  });

  it('crossing the gap to a neighbour does not bounce back, leaving the grid does', () => {
    const { HOVER_LEAVE_MS, HOVER_REST_MS, hoverAmbient } = ambient;
    suggestion = 'home';
    const leaveA = hoverAmbient('a');
    vi.advanceTimersByTime(HOVER_REST_MS);
    expect(target()).toBe('a');

    leaveA();
    vi.advanceTimersByTime(HOVER_LEAVE_MS - 50);
    const leaveB = hoverAmbient('b');
    vi.advanceTimersByTime(HOVER_REST_MS - 1);
    expect(target()).toBe('a');
    vi.advanceTimersByTime(1);
    expect(target()).toBe('b');

    leaveB();
    vi.advanceTimersByTime(HOVER_LEAVE_MS - 1);
    expect(target()).toBe('b');
    vi.advanceTimersByTime(1);
    expect(target()).toBe('home');
  });

  it('a quick pass after a rested card still lets that card go once the pointer leaves the grid', () => {
    const { HOVER_LEAVE_MS, HOVER_REST_MS, hoverAmbient } = ambient;
    suggestion = 'home';
    const leaveA = hoverAmbient('a');
    vi.advanceTimersByTime(HOVER_REST_MS);
    leaveA();
    vi.advanceTimersByTime(HOVER_LEAVE_MS - 50);
    const leaveB = hoverAmbient('b');
    vi.advanceTimersByTime(HOVER_REST_MS - 100);
    leaveB();
    vi.advanceTimersByTime(HOVER_REST_MS + HOVER_LEAVE_MS);
    expect(target()).toBe('home');
  });

  it('a late release of the old card does not cancel the new card (newest wins)', () => {
    const { HOVER_REST_MS, hoverAmbient } = ambient;
    const leaveA = hoverAmbient('a');
    const leaveB = hoverAmbient('b');
    leaveA();
    vi.advanceTimersByTime(HOVER_REST_MS);
    expect(target()).toBe('b');
    leaveB();
  });

  it('does nothing on a device without hover', () => {
    const matchMedia = vi.fn((query: string) => ({ matches: query === '(hover: none)' }));
    vi.stubGlobal('matchMedia', matchMedia);
    suggestion = 'home';
    const leave = ambient.hoverAmbient('a');
    vi.advanceTimersByTime(ambient.HOVER_REST_MS * 2);
    expect(target()).toBe('home');
    expect(matchMedia).toHaveBeenCalledWith('(hover: none)');
    leave();
    vi.advanceTimersByTime(ambient.HOVER_LEAVE_MS * 2);
    expect(target()).toBe('home');
  });
});
