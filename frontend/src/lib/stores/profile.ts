import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { EMPTY_SNAPSHOT, getProfileSnapshot, type ProfileSnapshot } from '../services/profile';
import { currentUser } from './user';
import type { CurrentUser } from '../services/account';

export const profileSnapshot = writable<ProfileSnapshot>(EMPTY_SNAPSHOT);

let started = false;
let seq = 0;

export async function refreshProfile() {
  const id = ++seq;
  try {
    const snap = await getProfileSnapshot();
    if (id !== seq) return;
    profileSnapshot.set(snap);
  } catch (err) {
    if (id !== seq) return;
    console.error('profile snapshot failed', err);
  }
}

function showcaseKey(user: CurrentUser | null): string | null {
  return user ? JSON.stringify(user.profile?.showcase ?? []) : null;
}

export function initProfile() {
  void refreshProfile();
  if (started) return;
  started = true;
  if (!inWails) return;
  for (const name of ['library:updated', 'game:started', 'game:stopped', 'playlog:recorded']) {
    Events.On(name, () => void refreshProfile());
  }
  let seen = showcaseKey(get(currentUser));
  currentUser.subscribe((user) => {
    const key = showcaseKey(user);
    if (key === seen) return;
    seen = key;
    void refreshProfile();
  });
}
