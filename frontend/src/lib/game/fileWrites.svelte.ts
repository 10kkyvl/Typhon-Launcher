/** A write of the properties dialog that changes what a game starts or its files. */
export type FileWrite = { kind: 'compat' } | { kind: 'exe'; how: 'pick' | 'browse' } | { kind: 'saves'; how: 'browse' | 'reset' };

/**
 * The file writes of the properties dialog, by game: one at a time per game. Go's Enable takes the
 * executable's folder at its start and places the wrapper files there, so a file set, or a saves
 * folder chosen, while it runs lands beside the wrong files. Kept by game, not by opening: a write
 * goes on when its dialog closes (a file dialog has no owner window), so a dialog of that game opened
 * again holds the others until it ends; a dialog of another game is free.
 */
export function createFileWrites() {
  let running = $state.raw<Record<string, FileWrite>>({});

  return {
    /** The write that runs for the game, if any. */
    of: (gameId: string | undefined): FileWrite | null => (gameId !== undefined && gameId in running ? running[gameId] : null),

    /** Takes the game for `write`; returns how to give it back, or null when another write holds it. */
    start(gameId: string, write: FileWrite): (() => void) | null {
      if (gameId in running) return null;
      const mine = { ...write };
      running = { ...running, [gameId]: mine };
      return () => {
        if (running[gameId] !== mine) return;
        running = Object.fromEntries(Object.entries(running).filter(([id]) => id !== gameId));
      };
    },
  };
}
