import { writable } from 'svelte/store';
import { inWails } from '../services/backend';
import { onAchievementsUnlocked, unlockMessage, type AchievementsUnlocked } from '../services/achievements';
import { toast } from './toasts';

export const lastUnlock = writable<AchievementsUnlocked | null>(null);
export const achievementsStamp = writable<Record<string, number>>({});

let started = false;

export function initAchievements(options: { notify?: boolean } = {}): () => void {
  if (!inWails || started) return () => {};
  started = true;
  const notify = options.notify ?? true;
  const off = onAchievementsUnlocked((event) => {
    achievementsStamp.update((stamps) => ({ ...stamps, [event.gameId]: (stamps[event.gameId] ?? 0) + 1 }));
    lastUnlock.set(event);
    if (notify) toast(unlockMessage(event), 'success');
  });
  return () => {
    off();
    started = false;
  };
}
