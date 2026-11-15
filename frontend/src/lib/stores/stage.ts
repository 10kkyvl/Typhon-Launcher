import { derived, get, writable, type Readable } from 'svelte/store';
import { wideArt, type WideArt } from '../backdrop/source';
import type { LibraryGame } from '../services/library';
import type { GameArt } from '../services/metadata';
import { libraryGames } from './library';
import { gameArt } from './metadata';

/**
 * The stage: the focused game's wide art, sharp, across the top of the window behind the top bar, the
 * sidebar island and the screen's own header (ShellBackdrop draws it). A screen puts its game there by a
 * claim, the way it claims the backdrop (stores/ambient.ts); unlike the backdrop the stage has no memory:
 * with no live claim it is empty.
 */

interface Claim {
  token: symbol;
  id: string;
}

let claims: Claim[] = [];
const claimed = writable<string | null>(null);

/** The game of the newest live claim; null when no screen stands on the stage. */
export const stageGame: Readable<string | null> = { subscribe: claimed.subscribe };

/**
 * Puts `gameId` on the stage while the claim is live, the newest live claim winning over older ones. The
 * caller calls the returned release once it stops showing the game, usually as an effect's cleanup:
 * `$effect(() => claimStage(id))`. A release removes only its own claim, in any order and at most once;
 * it is published after the current task, so one screen giving way to the next never empties the stage
 * in between. `null` claims nothing and leaves other claims alone.
 */
export function claimStage(gameId: string | null): () => void {
  if (!gameId) return () => {};
  const token = Symbol(gameId);
  claims = [...claims, { token, id: gameId }];
  claimed.set(gameId);
  return () => {
    const before = claims.length;
    claims = claims.filter((claim) => claim.token !== token);
    if (claims.length === before) return;
    queueMicrotask(() => {
      const next = claims.at(-1)?.id ?? null;
      if (get(claimed) !== next) claimed.set(next);
    });
  };
}

// Pictures the stage could not load this session: their games have no wide art to stand on.
const failed = writable<ReadonlySet<string>>(new Set());

/** The stage could not load `src`: the game it belongs to is answered "no wide art" from now on. */
export function stagePictureFailed(src: string): void {
  failed.update((set) => (set.has(src) ? set : new Set([...set, src])));
}

function answer(id: string, games: readonly LibraryGame[], art: Readonly<Record<string, GameArt>>, broken: ReadonlySet<string>): WideArt {
  const wide = wideArt(id, games, art);
  return wide.state === 'yes' && broken.has(wide.src) ? { state: 'no' } : wide;
}

/**
 * Whether `gameId` has wide art the stage can show: yes (with the picture), no (a poster, no hero, a
 * picture that failed to load) or not known yet. A screen whose game has none shows its cover itself;
 * its claim still stands and the stage stays empty.
 */
export function stageWide(gameId: string): Readable<WideArt> {
  return derived([libraryGames, gameArt, failed], ([games, art, broken]) => answer(gameId, games, art, broken));
}

/** What the stage draws: the staged game and its wide picture ('' when it has none); null with no claim. */
export const stagePicture: Readable<{ id: string; src: string } | null> = derived(
  [claimed, libraryGames, gameArt, failed],
  ([id, games, art, broken]) => {
    if (!id) return null;
    const wide = answer(id, games, art, broken);
    return { id, src: wide.state === 'yes' ? wide.src : '' };
  },
);
