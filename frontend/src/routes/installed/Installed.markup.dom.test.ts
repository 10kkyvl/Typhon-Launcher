import { cleanup, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import type { MoveJob } from '../../lib/services/relocate';
import { libraryGames, libraryLoaded } from '../../lib/stores/library';
import { moves } from '../../lib/stores/relocate';
import { route } from '../../lib/stores/router';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import { bytesSize } from '../../lib/utils/format';
import Installed from './Installed.svelte';
import listSource from './InstalledList.svelte?raw';

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
  moves.set([]);
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

// jsdom applies no layout and no media query: these pin the rules by their text; the narrow row and a
// long path pushing nothing out are seen only in a real browser.
const styles = (/<style>([\s\S]*)<\/style>/.exec(listSource)?.[1] ?? '').replace(/\/\*[\s\S]*?\*\//g, '');
const narrow = /@media \(max-width: 1400px\) \{([\s\S]*)\}\s*$/.exec(styles)?.[1] ?? '';
const ruleOf = (css: string, selector: string) => new RegExp(`(?:^|\\})\\s*${selector.replace('.', '\\.')}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';

describe('Installed: a long install path', () => {
  it('wraps at any character in the list, never cut', () => {
    const rule = ruleOf(styles, '.path');
    expect(rule).toMatch(/overflow-wrap:\s*anywhere/);
    expect(rule).not.toMatch(/text-overflow|nowrap|overflow:\s*hidden/);
  });
});

describe('Installed: a game being moved, at every width', () => {
  it('says “Moving” beside its size where the status column is hidden, and nothing else comes back there', () => {
    libraryGames.set([game('g-exact', 'Exact', { sizeBytes: 3 * GB }), game('g-least', 'Least', { sizeBytes: 2 * GB })]);
    moves.set([{ id: 'm1', scope: 'game', stage: 'copy', gameId: 'g-exact', title: 'Exact' } as unknown as MoveJob]);
    installedView.set('list');
    renderDom(Installed);

    const titles = (id: string) => row(id).querySelector('.titles')?.textContent ?? '';
    expect(titles('g-exact')).toContain(msg('installed.moving'));
    expect(titles('g-least')).not.toContain(msg('installed.moving'));
    expect(ruleOf(narrow, '.status')).toMatch(/display:\s*none/);
    expect(ruleOf(styles, '.row-moving')).toMatch(/display:\s*none/);
    expect(ruleOf(narrow, '.row-moving')).toMatch(/display:\s*inline-flex/);
  });
});
