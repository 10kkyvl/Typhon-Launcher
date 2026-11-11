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

const target = () => get(ambient.ambientTarget);

describe('which game the backdrop follows', () => {
  it('shows nothing before any game is known', () => {
    expect(target()).toBeNull();
  });

  it('a screen claim wins over a rested card and over the remembered game', () => {
    const { HOVER_REST_MS, claimAmbient, hoverAmbient, rememberAmbient } = ambient;
    rememberAmbient('last-played');
    const leave = hoverAmbient('card');
    vi.advanceTimersByTime(HOVER_REST_MS);
    expect(target()).toBe('card');
    const release = claimAmbient('page');
    expect(target()).toBe('page');
    release();
    expect(target()).toBe('card');
    leave();
  });

  it('a rested card wins over the remembered game', () => {
    ambient.rememberAmbient('last-played');
    const leave = ambient.hoverAmbient('card');
    vi.advanceTimersByTime(ambient.HOVER_REST_MS);
    expect(target()).toBe('card');
    leave();
    vi.advanceTimersByTime(ambient.HOVER_LEAVE_MS);
    expect(target()).toBe('last-played');
  });

  it('follows the suggested game until a screen has claimed one, then keeps the last claimed', () => {
    const { claimAmbient, rememberAmbient } = ambient;
    rememberAmbient('older');
    rememberAmbient('newer');
    expect(target()).toBe('newer');
    rememberAmbient(null);
    expect(target()).toBeNull();
    rememberAmbient('newer');
    claimAmbient('elden')();
    rememberAmbient('last-played');
    expect(target()).toBe('elden');
  });

  it('keeps the last game when a screen without a game of its own takes over', () => {
    const release = ambient.claimAmbient('elden');
    release();
    expect(target()).toBe('elden');
  });

  it('a release restores the claim it replaced, and only its own', () => {
    const { claimAmbient } = ambient;
    const first = claimAmbient('a');
    const second = claimAmbient('b');
    first();
    expect(target()).toBe('b');
    second();
    expect(target()).toBe('a');
  });

  it('a claim of no game leaves the backdrop to the cards and the remembered game', () => {
    ambient.rememberAmbient('last-played');
    const release = ambient.claimAmbient(null);
    expect(target()).toBe('last-played');
    release();
  });
});

describe('hover intent on the backdrop', () => {
  it('follows a card only after the pointer has rested on it', () => {
    const { HOVER_REST_MS, hoverAmbient, rememberAmbient } = ambient;
    rememberAmbient('home');
    const leave = hoverAmbient('a');
    vi.advanceTimersByTime(HOVER_REST_MS - 1);
    expect(target()).toBe('home');
    vi.advanceTimersByTime(1);
    expect(target()).toBe('a');
    leave();
  });

  it('a fast pass over several cards changes nothing', () => {
    const { HOVER_LEAVE_MS, HOVER_REST_MS, hoverAmbient, rememberAmbient } = ambient;
    rememberAmbient('home');
    for (const id of ['a', 'b', 'c', 'd', 'e']) {
      const leave = hoverAmbient(id);
      vi.advanceTimersByTime(60);
      leave();
    }
    vi.advanceTimersByTime(HOVER_REST_MS + HOVER_LEAVE_MS);
    expect(target()).toBe('home');
  });

  it('crossing the gap to a neighbour does not bounce back, leaving the grid does', () => {
    const { HOVER_LEAVE_MS, HOVER_REST_MS, hoverAmbient, rememberAmbient } = ambient;
    rememberAmbient('home');
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
    const { HOVER_LEAVE_MS, HOVER_REST_MS, hoverAmbient, rememberAmbient } = ambient;
    rememberAmbient('home');
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
    ambient.rememberAmbient('home');
    const leave = ambient.hoverAmbient('a');
    vi.advanceTimersByTime(ambient.HOVER_REST_MS * 2);
    expect(target()).toBe('home');
    expect(matchMedia).toHaveBeenCalledWith('(hover: none)');
    leave();
    vi.advanceTimersByTime(ambient.HOVER_LEAVE_MS * 2);
    expect(target()).toBe('home');
  });
});
