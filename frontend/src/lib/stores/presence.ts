import { get, writable } from 'svelte/store';
import { Events } from '@wailsio/runtime';
import { inWails } from '../services/backend';
import {
  DEFAULT_PRESENCE,
  kick,
  setStatus,
  status as fetchStatus,
  toPresenceStatus,
  type PresenceStatus,
} from '../services/online';
import { authState } from './user';

export const presenceStatus = writable<PresenceStatus>(DEFAULT_PRESENCE);
export const shownPresence = writable<PresenceStatus>(DEFAULT_PRESENCE);
export const autoAway = writable(false);

let started = false;

export async function initPresence(): Promise<void> {
  if (started) return;
  started = true;

  if (inWails) {
    let previous: string | undefined;
    authState.subscribe((state) => {
      const changed = previous !== state;
      previous = state;
      if (!changed || state !== 'authenticated') return;
      kick().catch((err) => console.warn('presence kick failed', err));
    });
    Events.On('presence:status', (event) => {
      const data = (event.data ?? {}) as { status?: string; chosen?: string; auto?: boolean };
      presenceStatus.set(toPresenceStatus(data.chosen ?? get(presenceStatus)));
      shownPresence.set(toPresenceStatus(data.status ?? get(presenceStatus)));
      autoAway.set(data.auto === true);
    });
  }

  try {
    const current = await fetchStatus();
    presenceStatus.set(current);
    shownPresence.set(current);
  } catch (err) {
    console.warn('presence status request failed', err);
  }
}

export async function updatePresenceStatus(next: PresenceStatus): Promise<void> {
  const previous = get(presenceStatus);
  const previousShown = get(shownPresence);
  const previousAuto = get(autoAway);
  if (previous === next && !previousAuto) return;
  presenceStatus.set(next);
  shownPresence.set(next);
  autoAway.set(false);
  try {
    await setStatus(next);
  } catch (err) {
    presenceStatus.set(previous);
    shownPresence.set(previousShown);
    autoAway.set(previousAuto);
    throw err;
  }
}
