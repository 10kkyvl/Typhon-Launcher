import { render } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import type { CloudGameStatus, CloudStatus } from '../services/cloudsaves';
import type { LibraryGame } from '../services/library';
import { closeCloudConflict, cloudStatus, showConflictOnPage } from '../stores/cloudsaves';
import { libraryGames } from '../stores/library';
import { liveIsland, liveIslandMessages } from '../stores/liveIsland';
import { resetHistory } from '../stores/router';
import { holdToasts, toasts } from '../stores/toasts';
import IslandShellHarness from '../testing/IslandShellHarness.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));

// What the island says is session memory: every test gets copies of its own.
let copiesMade = 0;
function conflict(gameId: string): CloudGameStatus {
  const copies = `page-copy-${++copiesMade}`;
  return {
    gameId,
    state: 'conflict',
    code: '',
    message: '',
    at: '',
    syncedAt: '',
    conflict: { localDigest: copies, localUpdatedAt: '2026-11-01T10:00:00Z', localSize: 10, remoteDigest: `remote-${copies}`, remoteUpdatedAt: '2026-11-02T10:00:00Z', remoteSize: 20, remoteDevice: 'Deck' },
  };
}

const cloud = (...games: CloudGameStatus[]): CloudStatus => ({ enabled: true, signedIn: true, usedBytes: 0, quotaBytes: 0, maxBytes: 0, error: '', games });
const conflictMessages = () => get(liveIslandMessages).filter((m) => m.kind === 'conflict');

let app: HTMLDivElement;

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  toasts.set([]);
  holdToasts(false);
  liveIsland.clear();
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
  resetHistory();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  closeCloudConflict();
  cloudStatus.set(null);
  libraryGames.set([]);
  toasts.set([]);
  liveIsland.clear();
  resetHistory();
  vi.unstubAllGlobals();
});

function setup() {
  render(IslandShellHarness, { target: app });
  return { row: () => app.querySelector<HTMLElement>('[data-cloud]') };
}

describe('a save conflict the game page shows', () => {
  it('is not said in the island, and the status line leaves the game to the page until the page lets go', async () => {
    const { row } = setup();
    const release = showConflictOnPage('g1');
    cloudStatus.set(cloud(conflict('g1')));
    await tick();

    expect(conflictMessages()).toEqual([]);
    expect(get(toasts)).toEqual([]);
    expect(row()?.dataset.cloud).not.toBe('conflict');

    release();
    await tick();

    expect(conflictMessages()).toEqual([]);
    expect(row()?.dataset.cloud).toBe('conflict');
    expect(row()?.textContent).toContain(msg('ui.statusCloudConflict', { title: 'Hades' }));
  });

  it('takes the island\'s message back once the page shows it', async () => {
    setup();
    cloudStatus.set(cloud(conflict('g1')));
    await tick();
    expect(conflictMessages()).toHaveLength(1);

    const release = showConflictOnPage('g1');
    await tick();

    expect(conflictMessages()).toEqual([]);
    release();
  });
});
