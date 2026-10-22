import { Events } from '@wailsio/runtime';
import { Service as AchievementsService } from '../../../bindings/typhon/internal/achievements';
import { msg } from '../i18n';
import { inWails } from './backend';

export type AchievementsSource = 'none' | 'local';

export interface AchievementItem {
  key: string;
  name: string;
  description: string;
  hidden: boolean;
  icon: string;
  iconGray: string;
  unlocked: boolean;
  unlockedAt: string;
}

export interface AchievementsView {
  appId: string;
  total: number;
  unlocked: number;
  items: AchievementItem[];
  source: AchievementsSource;
  schemaError: string;
  partial: boolean;
}

export interface AchievementsUnlocked {
  gameId: string;
  keys: string[];
  names: string[];
}

export const EVENT_UNLOCKED = 'achievements:unlocked';
export const CODE_NO_STEAM_ID = 'achievements.no_steam_id';

const unavailable = () => new Error('unavailable in browser');

function normalize(raw: AchievementsView): AchievementsView {
  return { ...raw, items: raw.items ?? [] };
}

export async function fetchAchievements(gameId: string): Promise<AchievementsView> {
  if (!inWails) throw unavailable();
  return normalize((await AchievementsService.GameAchievements(gameId)) as unknown as AchievementsView);
}

export async function rescanAchievements(gameId: string): Promise<AchievementsView> {
  if (!inWails) throw unavailable();
  return normalize((await AchievementsService.Rescan(gameId)) as unknown as AchievementsView);
}

export function onAchievementsUnlocked(handler: (event: AchievementsUnlocked) => void): () => void {
  if (!inWails) return () => {};
  return Events.On(EVENT_UNLOCKED, (event) => {
    const data = event.data as Partial<AchievementsUnlocked> | null;
    handler({ gameId: data?.gameId ?? '', keys: data?.keys ?? [], names: data?.names ?? [] });
  });
}

export function unlockMessage(event: AchievementsUnlocked): string {
  const total = Math.max(event.keys.length, event.names.length);
  const first = event.names.find((name) => name !== '') ?? event.keys[0] ?? '';
  if (total <= 1) return msg('games.achievementsUnlocked', { name: first });
  return msg('games.achievementsUnlockedMore', { name: first, count: total - 1 });
}
