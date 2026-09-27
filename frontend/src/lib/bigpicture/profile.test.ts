import { describe, expect, it } from 'vitest';
import type { LibraryGame } from '../services/library';
import { recentLocalGames } from './profile';

const game = (id: string, overrides: Partial<LibraryGame> = {}): LibraryGame => ({
  id,
  title: id,
  executable: '',
  installDir: '',
  cover: '',
  version: '',
  sizeBytes: 0,
  lastPlayed: null,
  playtimeSeconds: 0,
  installedAt: '',
  ...overrides,
});

describe('local Big Picture profile', () => {
  it('keeps uninstalled game history visible instead of dropping its playtime record', () => {
    const games = [
      game('recent', { playtimeSeconds: 5400, lastPlayed: '2026-09-20T12:00:00Z', status: 'completed' }),
      game('older', { playtimeSeconds: 7200, lastPlayed: '2026-09-19T12:00:00Z' }),
      game('uninstalled', { uninstalled: true, playtimeSeconds: 36000, lastPlayed: '2026-09-18T12:00:00Z', status: 'completed' }),
    ];

    expect(recentLocalGames(games).map(({ id }) => id)).toEqual(['recent', 'older', 'uninstalled']);
  });
});
