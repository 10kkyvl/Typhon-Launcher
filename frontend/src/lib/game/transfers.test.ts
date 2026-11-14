import { describe, expect, it, vi } from 'vitest';
import type { Download } from '../services/downloads';
import type { Installation } from '../services/install';
import { activeDownload, activeInstall, gameDownloads, gameInstalls, ownsDownload, terminalDownload } from './transfers';

vi.mock('../services/backend', () => ({ inWails: false }));

function download(id: string, patch: Partial<Download> = {}): Download {
  return { id, name: id, status: 'downloading', progress: 0, addedAt: '2026-01-01T00:00:00Z', origin: { gameId: 'c1' }, ...patch } as Download;
}

function install(id: string, patch: Partial<Installation> = {}): Installation {
  return { id, downloadId: '', gameId: '', status: 'extracting', startedAt: '2026-01-01T00:00:00Z', ...patch } as Installation;
}

const owner = { gameIds: ['c1', '', undefined], libraryId: 'g1' };

describe("a game's downloads", () => {
  it('belong to it by a catalog id it is known by or by its library card, never by an empty id', () => {
    expect(ownsDownload(download('a', { origin: { gameId: 'c1' } }), owner)).toBe(true);
    expect(ownsDownload(download('b', { origin: { libraryId: 'g1' } }), owner)).toBe(true);
    expect(ownsDownload(download('c', { origin: { gameId: 'c2' } }), owner)).toBe(false);
    expect(ownsDownload(download('d', { origin: { gameId: '' } }), { gameIds: [''] })).toBe(false);
    expect(ownsDownload(download('e', { origin: {} }), { gameIds: ['c1'] })).toBe(false);
  });

  it('come newest first, so the terminal one is the last the player started, not the first listed', () => {
    const owned = gameDownloads(
      [
        download('old-failed', { status: 'failed', addedAt: '2026-01-01T00:00:00Z' }),
        download('other', { origin: { gameId: 'c9' }, addedAt: '2026-05-01T00:00:00Z' }),
        download('new-done', { status: 'completed', addedAt: '2026-03-01T00:00:00Z' }),
      ],
      owner,
    );
    expect(owned.map((item) => item.id)).toEqual(['new-done', 'old-failed']);
    expect(terminalDownload(owned)?.id).toBe('new-done');
  });

  it('name as active the newest one that has not finished: running, queued or paused', () => {
    const owned = gameDownloads(
      [
        download('paused', { status: 'paused', addedAt: '2026-01-01T00:00:00Z' }),
        download('queued', { status: 'queued', addedAt: '2026-02-01T00:00:00Z' }),
        download('done', { status: 'completed', addedAt: '2026-03-01T00:00:00Z' }),
      ],
      owner,
    );
    expect(activeDownload(owned)?.id).toBe('queued');
    expect(activeDownload(gameDownloads([download('done', { status: 'completed' })], owner))).toBeUndefined();
  });
});

describe("a game's installs", () => {
  it("are those of its downloads or its card, newest first; the active one is the newest still running", () => {
    const owned = [download('d1')];
    const installs = gameInstalls(
      [
        install('of-download', { downloadId: 'd1', status: 'completed', startedAt: '2026-04-01T00:00:00Z' }),
        install('of-card', { gameId: 'g1', status: 'extracting', startedAt: '2026-02-01T00:00:00Z' }),
        install('other', { downloadId: 'd9', gameId: 'g9', startedAt: '2026-05-01T00:00:00Z' }),
      ],
      owned,
      'g1',
    );
    expect(installs.map((item) => item.id)).toEqual(['of-download', 'of-card']);
    expect(activeInstall(installs)?.id).toBe('of-card');
  });
});
