/** A write of the properties dialog that changes what a game starts or its files. */
export type FileWrite = { kind: 'compat' } | { kind: 'exe'; how: 'pick' | 'browse' } | { kind: 'saves'; how: 'browse' | 'reset' };

/** A write that runs, and the dialog that began it (`by`), which alone says its outcome. */
export type RunningWrite = FileWrite & { by: object };

/**
 * The file writes of the properties dialog, by game: one at a time per game. Go's Enable takes the
 * executable's folder at its start and places the wrapper files there, so a file set, or a saves
 * folder chosen, while it runs lands beside the wrong files. Kept by game, not by opening: a write
 * goes on when its dialog closes (a file dialog has no owner window), so a dialog of that game opened
 * again holds the others until it ends; a dialog of another game is free. `fileWrites` is module
 * state, like a store: what runs is a fact about the game, and a dialog torn down (a language switch,
 * Big Picture, a new session) leaves its writes running for every dialog mounted after it. A write
 * that never answers holds its game for the session.
 */
export function createFileWrites() {
  let running = $state.raw<Record<string, RunningWrite>>({});

  return {
    /** The write that runs for the game, if any. */
    of: (gameId: string | undefined): RunningWrite | null => (gameId !== undefined && gameId in running ? running[gameId] : null),

    /** Takes the game for `write` begun by `by`; returns how to give it back, or null when another write holds it. */
    start(gameId: string, write: FileWrite, by: object): (() => void) | null {
      if (gameId in running) return null;
      const mine = { ...write, by };
      running = { ...running, [gameId]: mine };
      return () => {
        if (running[gameId] !== mine) return;
        running = Object.fromEntries(Object.entries(running).filter(([id]) => id !== gameId));
      };
    },
  };
}

export const fileWrites = createFileWrites();
