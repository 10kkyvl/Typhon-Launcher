import { get, writable } from 'svelte/store';
import {
  DEFAULT_PRESENCE,
  setStatus,
  status as fetchStatus,
  type PresenceStatus,
} from '../services/online';

export const presenceStatus = writable<PresenceStatus>(DEFAULT_PRESENCE);

export async function initPresence(): Promise<void> {
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
