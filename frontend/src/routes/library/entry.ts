import type { GameSource } from '../../lib/library/source';

export interface Entry {
  id: string;
  title: string;
  cover: string;
  hero: string;
  installed: boolean;
  playable: boolean;
  playtimeSeconds: number;
  sizeBytes: number;
  lastPlayed: string | null;
  subtitle: string;
  hidden: boolean;
  inLibrary: boolean;
  tags?: string[];
  status?: string;
  completedAt?: string | null;
  canonicalGameId?: string;
  watched?: boolean;
  source?: GameSource | null;
  platform?: string;
}

export type Sort = 'alpha' | 'recent' | 'playtime' | 'size' | 'status' | 'completed';
