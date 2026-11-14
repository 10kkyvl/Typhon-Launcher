import { get, writable, type Readable } from 'svelte/store';

/** The pointer must rest on a card this long before the backdrop flows to its game. */
export const HOVER_REST_MS = 300;
/** Leaving a card keeps its game this long, so crossing the gap to a neighbour does not bounce back. */
export const HOVER_LEAVE_MS = 250;
/** How many claimed games are remembered for the screens that claim none. */
const HISTORY = 16;

export interface AmbientState {
  /** The newest live claim. */
  claim: string | null;
  /** The library card the pointer has rested on. */
  hover: string | null;
  /** The newest live game of a page's resting picture (the library's carousel): below a rested card. */
  resting: string | null;
  /** Games claimed this session, newest first. */
  history: readonly string[];
}

interface Claim {
  token: symbol;
  id: string;
}

let claims: Claim[] = [];
let rests: Claim[] = [];
const state = writable<AmbientState>({ claim: null, hover: null, resting: null, history: [] });

/** What screens and cards ask of the backdrop; `pickAmbient` turns it into the game to show. */
export const ambientState: Readable<AmbientState> = { subscribe: state.subscribe };

function publish(update: Partial<AmbientState>) {
  state.update((current) => ({ ...current, ...update }));
}

/**
 * The extension point for anything that shows a game (a screen, a slide, a plugin view): while the
 * claim is live the window backdrop shows `gameId`, the newest live claim winning over older ones and
 * over hovered cards. The caller must call the returned release once it stops showing the game,
 * usually as an effect's cleanup: `$effect(() => claimAmbient(id))`. A release removes only its own
 * claim, in any order and at most once; `null` claims nothing and leaves other claims alone.
 */
export function claimAmbient(gameId: string | null): () => void {
  if (!gameId) return () => {};
  const token = Symbol(gameId);
  claims = [...claims, { token, id: gameId }];
  const history = [gameId, ...get(state).history.filter((id) => id !== gameId)].slice(0, HISTORY);
  publish({ claim: gameId, history });
  return () => {
    const before = claims.length;
    claims = claims.filter((claim) => claim.token !== token);
    if (claims.length !== before) publish({ claim: claims.at(-1)?.id ?? null });
  };
}

/**
 * A page's resting picture (the slide a carousel shows): the backdrop shows `gameId` while no screen
 * claims a game and no card is rested on, the newest live one winning. Unlike `claimAmbient` it is not
 * remembered: a slide that passed by on its own does not become the game of the screens that claim
 * none. The returned release removes only its own claim, at most once; it is published after the
 * current task, so a release followed by the next slide's claim never shows the window without one.
 */
export function restAmbient(gameId: string | null): () => void {
  if (!gameId) return () => {};
  const token = Symbol(gameId);
  rests = [...rests, { token, id: gameId }];
  publish({ resting: gameId });
  return () => {
    const before = rests.length;
    rests = rests.filter((rest) => rest.token !== token);
    if (rests.length === before) return;
    queueMicrotask(() => {
      const resting = rests.at(-1)?.id ?? null;
      if (get(state).resting !== resting) publish({ resting });
    });
  };
}

/**
 * The game the backdrop shows: the newest live claim, else the rested card, else the page's resting
 * game while its art resolves, else the last claimed game whose art resolves (one without art, or one
 * no longer in the library, is skipped), else the suggestion: the last played library game with art.
 */
export function pickAmbient(current: AmbientState, suggestion: string | null, hasArt: (id: string) => boolean): string | null {
  const resting = current.resting !== null && hasArt(current.resting) ? current.resting : null;
  return current.claim ?? current.hover ?? resting ?? current.history.find(hasArt) ?? suggestion;
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
  if (get(state).hover !== gameId) {
    clearTimeout(enterTimer);
    enterId = gameId;
    enterTimer = setTimeout(() => {
      enterId = null;
      publish({ hover: gameId });
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
      if (pointerOn === null) publish({ hover: null });
    }, HOVER_LEAVE_MS);
  };
}
