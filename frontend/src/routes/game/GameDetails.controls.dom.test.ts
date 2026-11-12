import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { getMetadataView, type MetadataView } from '../../lib/services/metadata';
import { libraryGames } from '../../lib/stores/library';
import { resetHistory, route } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import GameDetails from './GameDetails.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/metadata', async (importOriginal) => {
  const original = await importOriginal<typeof import('../../lib/services/metadata')>();
  return { ...original, getMetadataView: vi.fn(original.getMetadataView) };
});

function view(patch: Partial<MetadataView>): MetadataView {
  return {
    game: { id: 'c1', title: 'Hades' },
    cover: '',
    hero: '',
    screenshots: [],
    resolved: true,
    stale: false,
    provider: 'igdb',
    match: 'matched',
    ...patch,
  } as MetadataView;
}

const hades = {
  id: 'g1',
  title: 'Hades',
  canonicalGameId: 'c1',
  installDir: '/games/hades',
  executable: 'Hades.exe',
  installedAt: '2026-01-01T00:00:00Z',
  uninstalled: false,
  playtimeSeconds: 0,
  sizeBytes: 0,
  version: '',
} as unknown as LibraryGame;

beforeEach(() => {
  // jsdom has no ResizeObserver; the page's tabs and segmented controls measure with one.
  vi.stubGlobal('ResizeObserver', class {
    observe() {}
    unobserve() {}
    disconnect() {}
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  libraryGames.set([hades]);
});

afterEach(() => {
  libraryGames.set([]);
  toasts.set([]);
  resetHistory();
  document.body.replaceChildren();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('GameDetails: the breadcrumb', () => {
  it('leads back through a muted text link that the keyboard presses', async () => {
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const crumbs = screen.getByRole('navigation');
    const back = within(crumbs).getByRole('button', { name: msg('games.installedStatusWord') });
    expect(back.classList.contains('text-link')).toBe(true);
    expect(back.classList.contains('muted')).toBe(true);
    back.focus();

    await user.keyboard('{Enter}');

    expect(get(route).name).toBe('installed');
  });
});

describe('GameDetails: a long summary', () => {
  it('opens the rest of its text with a disclosure button tied to the text', async () => {
    const summary = 'A rogue-like dungeon crawler. '.repeat(30).trim();
    vi.mocked(getMetadataView).mockResolvedValue(view({ game: { id: 'c1', title: 'Hades', summary } as MetadataView['game'] }));
    const { user } = renderDom(GameDetails, { id: 'g1' });
    const more = await screen.findByRole('button', { name: msg('games.detailExpandSummary') });
    expect(more.getAttribute('aria-expanded')).toBe('false');
    expect(more.hasAttribute('aria-pressed')).toBe(false);
    const text = document.getElementById(more.getAttribute('aria-controls') ?? '');
    expect(text?.textContent?.endsWith('…')).toBe(true);
    more.focus();

    await user.keyboard('{Enter}');

    expect(more.getAttribute('aria-expanded')).toBe('true');
    expect(text?.textContent).toBe(summary);
  });
});
