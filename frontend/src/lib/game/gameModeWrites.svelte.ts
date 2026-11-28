import { gameModeKeys, overrideState, withOverride, type GameModeKey, type OverrideState } from '../gamemode/gamemode';
import type { GameModeOverride } from '../services/library';

interface Chosen {
  state: OverrideState;
  /** The write that set it: an older one's answer does not touch a newer choice. */
  n: number;
  /** Go stored it; shown until the library's copy comes back. */
  done: boolean;
}

/**
 * The game mode changes of the properties dialog, sent to Go one at a time in the order they were
 * made: a change made while another is written waits its turn instead of being dropped. Go stores the
 * whole override a call carries, so each is built on the last one Go took for that game until the
 * library's copy (`stored`) holds it too, else on that copy.
 *
 * `chosen(key)` is what a row shows instead of the library's copy: the change waiting or being sent,
 * then the one Go stored until `settle()` (the library's copy changed). A refused change goes back to
 * the stored value. `reset()` starts an opening clean; writes already queued still go out, in order,
 * and their answers touch nothing the new opening shows.
 */
export function createGameModeWrites(
  send: (gameId: string, override: GameModeOverride) => Promise<unknown>,
  stored: (gameId: string) => GameModeOverride | null | undefined,
) {
  let shown = $state<Partial<Record<GameModeKey, Chosen>>>({});
  let tail: Promise<void> = Promise.resolve();
  const taken = new Map<string, GameModeOverride>();
  const waiting = new Map<string, number>();
  let n = 0;
  let opening = 0;

  const without = (key: GameModeKey) => Object.fromEntries(Object.entries(shown).filter(([k]) => k !== key)) as typeof shown;

  function finish(gameId: string, key: GameModeKey, mine: number, at: number, ok: boolean) {
    const left = (waiting.get(gameId) ?? 1) - 1;
    if (left > 0) waiting.set(gameId, left);
    else waiting.delete(gameId);
    if (at !== opening || shown[key]?.n !== mine) return;
    shown = ok ? { ...shown, [key]: { ...shown[key], done: true } } : without(key);
  }

  return {
    chosen: (key: GameModeKey): OverrideState | undefined => shown[key]?.state,

    /** How many of this game's changes are still to be answered. */
    waiting: (gameId: string) => waiting.get(gameId) ?? 0,

    /** Resolves once Go stored the change; rejects with its refusal. */
    write(gameId: string, key: GameModeKey, state: OverrideState): Promise<void> {
      const mine = ++n;
      const at = opening;
      shown = { ...shown, [key]: { state, n: mine, done: false } };
      waiting.set(gameId, (waiting.get(gameId) ?? 0) + 1);
      const run = tail.then(async () => {
        const next = withOverride(taken.get(gameId) ?? stored(gameId), key, state);
        await send(gameId, next);
        taken.set(gameId, next);
      });
      // The refusal reaches the caller through `run`; the next change still goes.
      tail = run.catch(() => undefined);
      return run.then(
        () => finish(gameId, key, mine, at, true),
        (err: unknown) => {
          finish(gameId, key, mine, at, false);
          throw err;
        },
      );
    },

    /** The library's copy changed: the rows show it again, and it is the base once it holds what Go took. */
    settle() {
      for (const [gameId, override] of taken) {
        const copy = stored(gameId);
        if (gameModeKeys.every((key) => overrideState(copy, key) === overrideState(override, key))) taken.delete(gameId);
      }
      shown = Object.fromEntries(Object.entries(shown).filter(([, chosen]) => !chosen.done)) as typeof shown;
    },

    reset() {
      opening++;
      shown = {};
    },
  };
}
