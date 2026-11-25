import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { route } from '../../lib/stores/router';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import Installed from './Installed.svelte';

// The rows on the base parts: the title is the game's link, the picture beside it a repeat of it;
// a size not fully measured is said in words, in the list and in the grid.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/compat', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/compat')>()),
  compatStatuses: vi.fn(async () => new Map()),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(),
}));
vi.mock('../../lib/stores/missingImages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/missingImages')>()),
  refreshMissingImages: vi.fn(async () => {}),
}));

const GB = 1024 ** 3;

function game(id: string, title: string, extra: Partial<LibraryGame> = {}): LibraryGame {
  return { id, title, installDir: `C:\\Games\\${title}`, executable: 'game.exe', sizeBytes: 0, uninstalled: false, tags: [], lastPlayed: null, ...extra } as unknown as LibraryGame;
}

const row = (id: string) => document.querySelector(`[data-game-row="${id}"]`) as HTMLElement;

beforeEach(() => {
  libraryLoaded.set(true);
  libraryGames.set([
    game('g-exact', 'Exact', { sizeBytes: 3 * GB }),
    game('g-least', 'Least', { sizeBytes: 2 * GB, sizeUnknown: true }),
    game('g-unknown', 'Unknown', { sizeUnknown: true }),
  ]);
});

afterEach(() => {
  cleanup();
  libraryGames.set([]);
  libraryLoaded.set(false);
  installedView.set('list');
  document.body.replaceChildren();
});

describe('Installed: a list row on the base parts', () => {
  it('its title is the game’s text link, which opens the game; the picture is a repeat out of the Tab order', async () => {
    installedView.set('list');
    const { user } = renderDom(Installed);
    const link = within(row('g-exact')).getByRole('button', { name: 'Exact' });

    expect(link.classList.contains('text-link')).toBe(true);
    const cover = row('g-exact').querySelector('.thumb button') as HTMLElement;
    expect(cover.getAttribute('tabindex')).toBe('-1');
    expect(cover.getAttribute('aria-hidden')).toBe('true');

    await user.click(link);
    expect(get(route)).toMatchObject({ name: 'game', params: { id: 'g-exact' } });
  });
});

describe('Installed: sizes in words', () => {
  it.each(['list', 'grid'] as const)('in the %s: measured, “at least”, “size unknown”', (view) => {
    installedView.set(view);
    renderDom(Installed);

    expect(row('g-exact').textContent).toContain(bytesSize(3 * GB));
    expect(row('g-least').textContent).toContain(msg('common.meterAtLeast', { size: bytesSize(2 * GB) }));
    expect(row('g-unknown').textContent).toContain(msg('common.meterSizeUnknown'));
  });
});
