import { Service as ProfileService } from '../../../bindings/typhon/internal/profile';
import { inWails } from './backend';

export interface GameRef {
  id: string;
  title: string;
  cover: string;
  canonicalGameId?: string;
  playtimeSeconds: number;
  status: string;
  statusAt?: string | null;
}

export interface ProfileStats {
  games: number;
  hours: number;
  completed: number;
  playing: number;
  monthSeconds: number;
  monthGames: number;
  monthCompleted: number;
}

export interface PlayingEntry {
  game: GameRef;
  recentSeconds: number;
}

export interface ActivityEntry {
  game: GameRef;
  seconds: number;
}

export interface ActivityDay {
  date: string;
  entries: ActivityEntry[];
}

export interface ShowcaseBlock {
  kind: string;
  games: GameRef[];
}

export interface ProfileSnapshot {
  stats: ProfileStats;
  playing: PlayingEntry[];
  activity: ActivityDay[];
  running: GameRef[];
  showcase: ShowcaseBlock[];
}

export const EMPTY_SNAPSHOT: ProfileSnapshot = {
  stats: { games: 0, hours: 0, completed: 0, playing: 0, monthSeconds: 0, monthGames: 0, monthCompleted: 0 },
  playing: [],
  activity: [],
  running: [],
  showcase: [],
};

export async function getProfileSnapshot(): Promise<ProfileSnapshot> {
  if (!inWails) return EMPTY_SNAPSHOT;
  return (await ProfileService.Snapshot()) as unknown as ProfileSnapshot;
}
