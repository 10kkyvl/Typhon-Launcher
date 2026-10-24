import { matchesCollection } from '../library/collections';
import { canPlay } from '../library/launch';
import type { LibraryCollection, LibraryGame } from '../services/library';

export type PickSource = 'installed' | 'all' | 'backlog' | 'collection';

export interface PickFilters {
  source: PickSource;
  collectionId: string;
  stale: boolean;
  genre: string;
}

export const STALE_DAYS = 30;
const DAY_MS = 24 * 60 * 60 * 1000;
const SOURCES: readonly PickSource[] = ['installed', 'all', 'backlog', 'collection'];
const STORAGE_KEY = 'typhon.randomPick';

export const defaultFilters: PickFilters = { source: 'installed', collectionId: '', stale: false, genre: '' };

export function isStale(lastPlayed: string | null | undefined, now: number): boolean {
  if (!lastPlayed) return true;
  const time = Date.parse(lastPlayed);
  if (!Number.isFinite(time)) return true;
  return now - time > STALE_DAYS * DAY_MS;
}

export function pickCandidates(
  games: readonly LibraryGame[],
  filters: PickFilters,
  genres: Readonly<Record<string, string[]>>,
  collections: readonly LibraryCollection[],
  now: number,
): LibraryGame[] {
  const collection = filters.source === 'collection' ? collections.find((item) => item.id === filters.collectionId) : undefined;
  if (filters.source === 'collection' && !collection) return [];
  return games.filter((game) => {
    if (game.hidden) return false;
    if (filters.source === 'installed' && !canPlay(game)) return false;
    if (filters.source === 'backlog' && game.status !== 'backlog') return false;
    const own = game.canonicalGameId ? genres[game.canonicalGameId] : undefined;
    if (collection && !matchesCollection(game, own, collection)) return false;
    if (filters.stale && !isStale(game.lastPlayed, now)) return false;
    if (filters.genre && !(own ?? []).includes(filters.genre)) return false;
    return true;
  });
}

export function pickRandom(
  candidates: readonly LibraryGame[],
  previousId: string | null,
  rng: () => number = Math.random,
): LibraryGame | null {
  if (candidates.length === 0) return null;
  const pool = candidates.length > 1 ? candidates.filter((game) => game.id !== previousId) : candidates;
  const index = Math.min(pool.length - 1, Math.max(0, Math.floor(rng() * pool.length)));
  return pool[index];
}

export function readFilters(): PickFilters {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { ...defaultFilters };
    const data = JSON.parse(raw) as Partial<PickFilters>;
    return {
      source: SOURCES.includes(data.source as PickSource) ? (data.source as PickSource) : defaultFilters.source,
      collectionId: typeof data.collectionId === 'string' ? data.collectionId : '',
      stale: data.stale === true,
      genre: typeof data.genre === 'string' ? data.genre : '',
    };
  } catch {
    return { ...defaultFilters };
  }
}

export function writeFilters(filters: PickFilters): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(filters));
  } catch {
    return;
  }
}

export const SPIN_STEPS = 18;

export function spinFrames(
  candidates: readonly LibraryGame[],
  final: LibraryGame,
  rng: () => number = Math.random,
  steps: number = SPIN_STEPS,
): LibraryGame[] {
  const frames: LibraryGame[] = [];
  const others = candidates.filter((game) => game.id !== final.id);
  let last = '';
  for (let i = 0; i < steps - 1; i++) {
    const pool = others.length > 1 ? others.filter((game) => game.id !== last) : others;
    if (pool.length === 0) break;
    const next = pool[Math.min(pool.length - 1, Math.floor(rng() * pool.length))];
    frames.push(next);
    last = next.id;
  }
  frames.push(final);
  return frames;
}

export function spinDelay(step: number, steps: number = SPIN_STEPS): number {
  const progress = steps <= 1 ? 1 : step / (steps - 1);
  return Math.round(60 + 340 * progress * progress * progress);
}

export function reducedMotion(): boolean {
  try {
    return document.documentElement.classList.contains('no-anim') || window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  } catch {
    return false;
  }
}

export function runSpin(
  frames: readonly LibraryGame[],
  onFrame: (game: LibraryGame) => void,
  onDone: (game: LibraryGame) => void,
): () => void {
  let step = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const tick = () => {
    const game = frames[step];
    if (step >= frames.length - 1) {
      onDone(game);
      return;
    }
    onFrame(game);
    timer = setTimeout(tick, spinDelay(step, frames.length));
    step += 1;
  };
  tick();
  return () => clearTimeout(timer);
}
