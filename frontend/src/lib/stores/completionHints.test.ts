import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) } }));
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../services/library', () => ({ setStatus: vi.fn() }));
vi.mock('../services/achievements', () => ({ fetchAchievements: vi.fn(), onAchievementsUnlocked: vi.fn(() => () => {}) }));

import { PLAYING_HINT_DELAY_MS, cancelPlayingHint, schedulePlayingHint } from './completionHints';
import { libraryGames } from './library';
import { toasts } from './toasts';

function seed(id: string) {
  libraryGames.set([{ id, title: 'Game', status: '', playtimeSeconds: 0 }] as never);
}

describe('playing hint', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    toasts.set([]);
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('does not suggest right after the start', () => {
    seed('a');
    schedulePlayingHint('a');
    vi.advanceTimersByTime(PLAYING_HINT_DELAY_MS - 1);
    expect(get(toasts)).toHaveLength(0);
  });

  it('suggests once the session lasts long enough', () => {
    seed('b');
    schedulePlayingHint('b');
    vi.advanceTimersByTime(PLAYING_HINT_DELAY_MS);
    expect(get(toasts)).toHaveLength(1);
  });

  it('never suggests when the session ended before the delay', () => {
    seed('c');
    schedulePlayingHint('c');
    vi.advanceTimersByTime(2000);
    cancelPlayingHint('c');
    vi.advanceTimersByTime(PLAYING_HINT_DELAY_MS * 2);
    expect(get(toasts)).toHaveLength(0);
  });
});
