import { cleanup, screen } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { rescan, scanProgress, scanning } from '../../lib/stores/discovery';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { toasts } from '../../lib/stores/toasts';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

// The header's own controls: Find games and its progress, the list/grid switch kept between visits.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
const backend = vi.hoisted(() => ({ inWails: false }));
vi.mock('../../lib/services/backend', () => backend);
vi.mock('../../lib/services/compat', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/compat')>()),
  compatStatuses: vi.fn(async () => new Map()),
}));
vi.mock('../../lib/stores/discovery', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/discovery')>()),
  rescan: vi.fn(),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(),
}));
vi.mock('../../lib/stores/missingImages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/missingImages')>()),
  refreshMissingImages: vi.fn(async () => {}),
}));

const games = [{ id: 'g-hades', title: 'Hades' }].map(
  (game) => ({ ...game, installDir: 'C:\\Games\\Hades', executable: 'C:\\Games\\Hades\\game.exe', sizeBytes: 0, uninstalled: false, tags: [], lastPlayed: null }) as unknown as LibraryGame,
);

const toastTexts = () => get(toasts).map((item) => `${item.kind}:${item.message}`);

beforeEach(() => {
  libraryGames.set(games);
  libraryLoaded.set(true);
});

afterEach(() => {
  cleanup();
  libraryGames.set([]);
  libraryLoaded.set(false);
  installedView.set('list');
  scanning.set(false);
  scanProgress.set({ processed: 0, total: 0 });
  backend.inWails = false;
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

describe('Installed: Find games', () => {
  it('runs the scan, says its progress on the button, then says what it found', async () => {
    let answer!: (result: Awaited<ReturnType<typeof rescan>>) => void;
    vi.mocked(rescan).mockImplementation(() => {
      scanning.set(true);
      return new Promise((resolve) => (answer = resolve));
    });
    const { user } = renderDom(Installed);
    backend.inWails = true;

    await user.click(screen.getByRole('button', { name: msg('games.installedFindGames') }));
    scanProgress.set({ processed: 3, total: 10 });

    await vi.waitFor(() => expect(screen.getByRole('button', { name: msg('games.installedScanProgress', { processed: 3, total: 10 }) })).toBeTruthy());
    expect(rescan).toHaveBeenCalledTimes(1);

    scanning.set(false);
    answer({ candidates: 2, added: 1, updated: 0, known: 1, skipped: 0, errors: 0, cancelled: false } as Awaited<ReturnType<typeof rescan>>);
    await vi.waitFor(() => expect(toastTexts().some((text) => text.startsWith('success:'))).toBe(true));
  });

  it('outside the desktop app says it is not available there and scans nothing', async () => {
    const { user } = renderDom(Installed);

    await user.click(screen.getByRole('button', { name: msg('games.installedFindGames') }));

    expect(rescan).not.toHaveBeenCalled();
    expect(toastTexts()).toContain(`info:${msg('games.installedScanDesktopOnly')}`);
  });
});

describe('Installed: the list/grid switch', () => {
  it('shows the view picked and keeps it when the page is opened again', async () => {
    const first = renderDom(Installed);

    await first.user.click(screen.getByRole('radio', { name: msg('games.viewGrid') }));
    expect(get(installedView)).toBe('grid');
    expect(document.querySelector('.grid')).not.toBeNull();
    first.unmount();

    renderDom(Installed);
    expect(screen.getByRole('radio', { name: msg('games.viewGrid') }).getAttribute('aria-checked')).toBe('true');
    expect(document.querySelector('.grid')).not.toBeNull();
  });
});
