import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writable } from 'svelte/store';

const saveProfile = vi.fn();
const toast = vi.fn();

vi.mock('../theme/apply', () => ({ displayedAccent: writable('#4F6BFF') }));
vi.mock('./toasts', () => ({ toast: (...args: unknown[]) => toast(...args) }));
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('./user', () => ({
  currentUser: writable(null),
  authState: writable('authenticated'),
  saveProfile: (...args: unknown[]) => saveProfile(...args),
}));

import { displayedAccent } from '../theme/apply';
import { confirmedAccent } from './confirmedAccent';
import { authState, currentUser } from './user';
import { appearanceOf } from '../profile/appearance';
import { ACCENT_SYNC_DELAY_MS, initProfileAccentSync, launcherAccent } from './profileAccent';
import type { CurrentUser } from '../services/account';

function user(accentSource: 'custom' | 'theme', accent = '#67d8ef'): CurrentUser {
  return {
    id: 'u1',
    profile: {
      visibility: 'friends', showOnline: true, showPlaying: true, showPlaytime: true, showLibrary: true,
      showActivity: true, showStats: true, showcase: ['favorites'],
      appearance: { ...appearanceOf(), accent, accentSource },
    },
  } as CurrentUser;
}

let stop: (() => void) | undefined;
beforeEach(() => {
  vi.useFakeTimers();
  saveProfile.mockReset().mockResolvedValue(true);
  toast.mockReset();
  displayedAccent.set('#4F6BFF');
  confirmedAccent.set('#4F6BFF');
  authState.set('authenticated');
  currentUser.set(user('theme'));
});
afterEach(() => {
  stop?.();
  vi.useRealTimers();
});

describe('profile accent follows the launcher theme', () => {
  it('reads the accent that is on screen as lowercase hex', () => {
    expect(launcherAccent()).toBe('#4f6bff');
    confirmedAccent.set('');
    displayedAccent.set('not a colour');
    expect(launcherAccent()).toBeNull();
  });

  it('saves any spelling of the accent as #rrggbb', async () => {
    confirmedAccent.set('rgb(79, 107, 255)');
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    expect(saveProfile.mock.calls[0][0].profile.appearance.accent).toBe('#4f6bff');
  });

  it('normalises hsl and short hex', () => {
    confirmedAccent.set('hsl(0, 100%, 50%)');
    expect(launcherAccent()).toBe('#ff0000');
    confirmedAccent.set('#abc');
    expect(launcherAccent()).toBe('#aabbcc');
  });

  it('tells the user once when the theme accent cannot be read', async () => {
    confirmedAccent.set('oklch(0.7 0.1 200)');
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    currentUser.set(user('theme'));
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS * 3);
    expect(saveProfile).not.toHaveBeenCalled();
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
  });

  it('saves only the appearance accent once the theme settles', async () => {
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    expect(saveProfile).toHaveBeenCalledTimes(1);
    const sent = saveProfile.mock.calls[0][0].profile;
    expect(sent.appearance.accent).toBe('#4f6bff');
    expect(sent.appearance.accentSource).toBe('theme');
    expect(sent.layout).toBeUndefined();
    expect(sent.showcase).toBeUndefined();
  });

  it('sends nothing while a theme is only previewed', async () => {
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    saveProfile.mockClear();
    for (const color of ['#111111', '#222222', '#333333']) {
      displayedAccent.set(color);
      await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS * 2);
    }
    expect(saveProfile).not.toHaveBeenCalled();
  });

  it('joins confirmations that arrive back to back into one save', async () => {
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    saveProfile.mockClear();
    for (const color of ['#111111', '#222222', '#333333']) {
      confirmedAccent.set(color);
      await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS - 100);
    }
    expect(saveProfile).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(100);
    expect(saveProfile).toHaveBeenCalledTimes(1);
    expect(saveProfile.mock.calls[0][0].profile.appearance.accent).toBe('#333333');
  });

  it('sends nothing when the accent is unchanged', async () => {
    currentUser.set(user('theme', '#4f6bff'));
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    expect(saveProfile).not.toHaveBeenCalled();
  });

  it('sends nothing when the accent is chosen by hand', async () => {
    currentUser.set(user('custom'));
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    expect(saveProfile).not.toHaveBeenCalled();
  });

  it('sends nothing for a guest or while offline', async () => {
    authState.set('guest');
    currentUser.set(null);
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    authState.set('offline');
    currentUser.set(user('theme'));
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    expect(saveProfile).not.toHaveBeenCalled();
  });

  it('shows a failed save and does not retry the same accent in a loop', async () => {
    saveProfile.mockRejectedValue(new Error('offline'));
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    expect(toast).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls[0][1]).toBe('danger');
    currentUser.set(user('theme'));
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS * 3);
    expect(saveProfile).toHaveBeenCalledTimes(1);
    confirmedAccent.set('#abcdef');
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS);
    expect(saveProfile).toHaveBeenCalledTimes(2);
  });

  it('retries later when another save is in flight', async () => {
    saveProfile.mockResolvedValueOnce(false).mockResolvedValue(true);
    stop = initProfileAccentSync();
    await vi.advanceTimersByTimeAsync(ACCENT_SYNC_DELAY_MS * 2);
    expect(saveProfile).toHaveBeenCalledTimes(2);
  });
});
