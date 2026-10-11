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

export async function refreshProfile() {
  const id = ++seq;
  try {
    const snap = await getProfileSnapshot();
    if (id !== seq) return;
    profileSnapshot.set(snap);
    profileFailed.set(false);
  } catch {
    if (id !== seq) return;
    profileFailed.set(true);
  }
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
