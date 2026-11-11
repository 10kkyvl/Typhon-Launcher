import { derived, get, writable } from 'svelte/store';

/** The pointer must rest on a card this long before the backdrop flows to its game. */
export const HOVER_REST_MS = 300;
/** Leaving a card keeps its game this long, so crossing the gap to a neighbour does not bounce back. */
export const HOVER_LEAVE_MS = 250;

const claimed = writable<string | null>(null);
const hovered = writable<string | null>(null);
const remembered = writable<string | null>(null);
let claimedOnce = false;

claimed.subscribe((id) => {
  if (!id) return;
  claimedOnce = true;
  remembered.set(id);
});

/**
 * The game whose art the window backdrop shows: the game a screen claims, else the library card the
 * pointer rests on, else the remembered game (the last one a screen claimed, or before any claim the
 * suggested one: the last played game that has art). Ids are library ids or canonical game ids.
 */
export const ambientTarget = derived([claimed, hovered, remembered], ([claim, hover, last]) => claim ?? hover ?? last);

/**
 * Claims the backdrop for a screen's game and returns the release, which restores the previous claim
 * unless someone claimed since. From an effect: `$effect(() => claimAmbient(id))`.
 */
export function claimAmbient(gameId: string | null): () => void {
  const previous = get(claimed);
  claimed.set(gameId);
  return () => {
    if (get(claimed) === gameId) claimed.set(previous === gameId ? null : previous);
  };
}

/** The game to start from; it follows the library until a screen has claimed a game. */
export function rememberAmbient(gameId: string | null) {
  if (!claimedOnce) remembered.set(gameId);
}

let enterTimer: ReturnType<typeof setTimeout> | undefined;
let enterId: string | null = null;
let leaveTimer: ReturnType<typeof setTimeout> | undefined;
let pointerOn: string | null = null;

/**
 * Hover intent for a library card or row: the backdrop moves to `gameId` once the pointer has rested on
 * it for HOVER_REST_MS. The returned release (pointer leave) lets the hovered game go once the pointer
 * has been on no card for HOVER_LEAVE_MS. Devices without hover never take part.
 */
export function hoverAmbient(gameId: string): () => void {
  if (globalThis.matchMedia?.('(hover: none)').matches) return () => {};
  pointerOn = gameId;
  clearTimeout(leaveTimer);
  if (get(hovered) !== gameId) {
    clearTimeout(enterTimer);
    enterId = gameId;
    enterTimer = setTimeout(() => {
      enterId = null;
      hovered.set(gameId);
    }, HOVER_REST_MS);
  }
  return () => {
    // A neighbour's enter may already have replaced ours; only undo what is ours.
    if (enterId === gameId) {
      clearTimeout(enterTimer);
      enterId = null;
    }
    if (pointerOn !== gameId) return;
    pointerOn = null;
    clearTimeout(leaveTimer);
    leaveTimer = setTimeout(() => {
      if (pointerOn === null) hovered.set(null);
    }, HOVER_LEAVE_MS);
  };
}
