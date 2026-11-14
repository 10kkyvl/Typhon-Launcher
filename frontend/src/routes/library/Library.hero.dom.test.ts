import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { DEFAULT_CHOICES, libraryChoices } from '../../lib/stores/libraryPrefs';
import { initLibrary, libraryGames, runningGames } from '../../lib/stores/library';
import { libraryView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Library from './Library.svelte';

// The library store subscribed as in the app, its events raised in the order Go sends them.
const bridge = vi.hoisted(() => ({
  handlers: new Map<string, (event: { data: unknown }) => void>(),
  games: [] as LibraryGame[],
}));

vi.mock('@wailsio/runtime', async () => {
  const runtime = (await import('../../lib/testing/dom')).wailsRuntime();
  runtime.Events.On = vi.fn((name: string | undefined, cb: (event: unknown) => void) => {
    if (name) bridge.handlers.set(name, cb as (event: { data: unknown }) => void);
    return () => {};
  });
  return runtime;
});
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../../bindings/typhon/internal/library', () => ({
  Service: {
    GetGames: vi.fn(async () => bridge.games),
    GetRunningGames: vi.fn(async () => []),
    StopGame: vi.fn(async () => {}),
  },
}));
vi.mock('../../../bindings/typhon/internal/metadata', () => ({ Service: { SetLanguage: vi.fn(async () => {}) } }));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  requestArt: vi.fn(),
}));
vi.mock('../../lib/stores/genres', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/genres')>()),
  requestLibraryGenres: vi.fn(async () => {}),
}));

function game(id: string, title: string, lastPlayed: string | null): LibraryGame {
  return { id, title, executable: 'game.exe', installDir: `C:\\Games\\${id}`, cover: '', version: '', sizeBytes: 0, lastPlayed, playtimeSeconds: 0, installedAt: '', tags: [] };
}

const emit = async (name: string, data: unknown) => {
  bridge.handlers.get(name)?.({ data });
  await tick();
};
const heroRegion = () => screen.getByRole('region', { name: msg('games.libraryContinuePlayingTitle') });
const recentTitles = () =>
  [...(screen.getAllByRole('heading', { level: 2, name: msg('games.recentLabel') })[0].closest('section')?.querySelectorAll('.recent-item') ?? [])].map(
    (item) => (['Hades', 'Celeste', 'Inside'].find((title) => item.textContent?.includes(title)) ?? '?'),
  );

const hades = game('g-hades', 'Hades', '2026-10-01T10:00:00Z');
const celeste = game('g-celeste', 'Celeste', '2026-11-01T10:00:00Z');
const inside = game('g-inside', 'Inside', '2026-09-01T10:00:00Z');
const hadesPlayed = { ...hades, lastPlayed: '2026-11-14T10:00:00Z', playtimeSeconds: 600 };
const stopped = { gameId: 'g-hades', sessionSeconds: 30, playtimeSeconds: 600, crashed: false, emulatorQuickExit: false };

beforeEach(async () => {
  libraryChoices.set({ ...DEFAULT_CHOICES });
  libraryView.set('list');
  bridge.games = [hades, celeste, inside];
  await initLibrary();
});

afterEach(() => {
  libraryGames.set([]);
  runningGames.set(new Set());
  document.body.replaceChildren();
});

describe('Library: the hero of a running game that stops', () => {
  it.each([
    ['in the order Go sends them: stopped, then the list', [['game:stopped', stopped], ['library:updated', [hadesPlayed, celeste, inside]]]],
    ['in the reverse order: the list, then stopped', [['library:updated', [hadesPlayed, celeste, inside]], ['game:stopped', stopped]]],
  ] as const)('stays that game, %s, and Stop turns into Play with focus on it', async (_name, events) => {
    renderDom(Library);
    await emit('game:started', { gameId: 'g-hades', sessionSeconds: 0, playtimeSeconds: 0 });
    expect(within(heroRegion()).getByRole('heading', { level: 2 }).textContent).toBe('Hades');
    const control = within(heroRegion()).getByRole('button', { name: msg('games.stop') });
    control.focus();
    expect(recentTitles()).toEqual(['Celeste', 'Inside']);

    for (const [name, data] of events) {
      await emit(name, data);
      expect(within(heroRegion()).getByRole('heading', { level: 2 }).textContent).toBe('Hades');
      expect(document.activeElement).toBe(control);
      expect(recentTitles()).toEqual(['Celeste', 'Inside']);
    }

    expect(control.isConnected).toBe(true);
    expect(control.textContent).toContain(msg('common.continue'));
  });
});
