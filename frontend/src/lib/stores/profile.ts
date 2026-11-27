import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { EMPTY_SNAPSHOT, getProfileSnapshot, type ProfileSnapshot } from '../services/profile';
import { currentUser } from './user';
import type { CurrentUser, ProfileSettings } from '../services/account';

export const profileSnapshot = writable<ProfileSnapshot>(EMPTY_SNAPSHOT);
export const profileFailed = writable(false);

export interface ProfileDraft {
  owner: string;
  draft: ProfileSettings;
  reset: boolean;
}

export const profileDraft = writable<ProfileDraft | null>(null);

let started = false;
let seq = 0;
let newest: Promise<boolean> = Promise.resolve(true);

/**
 * Reads the snapshot again. Answers whether the read came (true) or failed (false); a read that a newer
 * one replaced answers with the newest read's result, so a caller waits for what the page will show.
 */
export function refreshProfile(): Promise<boolean> {
  const id = ++seq;
  const read = (async (): Promise<boolean> => {
    try {
      const snap = await getProfileSnapshot();
      if (id !== seq) return newest;
      profileSnapshot.set(snap);
      profileFailed.set(false);
      return true;
    } catch {
      if (id !== seq) return newest;
      profileFailed.set(true);
      return false;
    }
  })();
  newest = read;
  return read;
}

function snapshotKey(user: CurrentUser | null): string | null {
  return user ? JSON.stringify([user.profile?.showcase ?? [], user.profile?.layout ?? null]) : null;
}

export function initProfile() {
  void refreshProfile();
  if (started) return;
  started = true;
  if (!inWails) return;
  for (const name of ['library:updated', 'game:started', 'game:stopped', 'playlog:recorded']) {
    Events.On(name, () => void refreshProfile());
  }
  let seen = snapshotKey(get(currentUser));
  currentUser.subscribe((user) => {
    const key = snapshotKey(user);
    if (key === seen) return;
    seen = key;
    void refreshProfile();
  });
}
