import { get, writable } from 'svelte/store';
import { inWails } from '../services/backend';
import {
  DEFAULT_PRESENCE,
  kick,
  setStatus,
  status as fetchStatus,
  type PresenceStatus,
} from '../services/online';
import { authState } from './user';

export const presenceStatus = writable<PresenceStatus>(DEFAULT_PRESENCE);

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
  }

  try {
    presenceStatus.set(await fetchStatus());
  } catch (err) {
    console.warn('presence status request failed', err);
  }
}

export async function updatePresenceStatus(next: PresenceStatus): Promise<void> {
  const previous = get(presenceStatus);
  if (previous === next) return;
  presenceStatus.set(next);
  try {
    await setStatus(next);
  } catch (err) {
    presenceStatus.set(previous);
    throw err;
  }
}
