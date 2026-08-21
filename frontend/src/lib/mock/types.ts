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
