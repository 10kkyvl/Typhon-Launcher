import { writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import { EMPTY_SNAPSHOT, getProfileSnapshot, type ProfileSnapshot } from '../services/profile';
import { currentUser } from './user';

export const profileSnapshot = writable<ProfileSnapshot>(EMPTY_SNAPSHOT);
export const profileLoading = writable(false);

let started = false;

export async function refreshProfile() {
  profileLoading.set(true);
  try {
    profileSnapshot.set(await getProfileSnapshot());
  } catch (err) {
    console.error('profile snapshot failed', err);
  } finally {
    profileLoading.set(false);
  }
}

export function initProfile() {
  if (started) return;
  started = true;
  void refreshProfile();
  if (!inWails) return;
  for (const name of ['library:updated', 'game:started', 'game:stopped', 'playlog:recorded']) {
    Events.On(name, () => void refreshProfile());
  }
  let seen: string | null = null;
  currentUser.subscribe((user) => {
    const key = user ? JSON.stringify(user.profile?.showcase ?? []) : null;
    if (key === seen) return;
    seen = key;
    void refreshProfile();
  });
}
