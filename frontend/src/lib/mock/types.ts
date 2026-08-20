export interface Game {
  id: string;
  title: string;
  cover: string;
  hero: string;
  genres: string[];
  tagline: string;
  description: string;
  developer: string;
  publisher: string;
  releaseDate: string;
  version: string;
  sizeGb: number;
  playtimeHours: number;
  lastPlayed: string | null;
  installed: boolean;
  favorite: boolean;
  completed: boolean;
  language: string;
  modes: string[];
  controllerSupport: string;
  lastUpdate: string;
}

export type DownloadStage = 'downloading' | 'unpacking' | 'verifying' | 'installing' | 'complete';

export interface ActiveDownload {
  id: string;
  gameId: string;
  label: string;
  doneGb: number;
  totalGb: number;
  speedMbs: number;
  baseMbs: number;
  peers: [number, number];
  stage: DownloadStage;
  stagePct: number;
  paused: boolean;
}

export interface QueuedDownload {
  id: string;
  gameId: string;
  sizeGb: number;
}

export type SourceStatus = 'active' | 'disabled' | 'error';

export interface Source {
  id: string;
  name: string;
  path: string;
  status: SourceStatus;
  lastUpdate: string;
  items: number;
  version: string;
  kind: 'core' | 'store' | 'mods' | 'archive' | 'web';
}

export interface Achievement {
  name: string;
  description: string;
  date: string;
}

export interface GameAchievements {
  earned: number;
  total: number;
  recent: Achievement[];
}

export interface Dlc {
  id: string;
  gameId: string;
  name: string;
  kind: string;
  installed: boolean;
}
