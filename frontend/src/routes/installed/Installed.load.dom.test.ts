// First: services/library reaches this store back through stores/cloudsaves, and a store loaded
// inside that cycle would keep the real service instead of the mock below.
import { libraryGames, libraryLoaded, libraryLoadError, runningGames } from '../../lib/stores/library';
import { cleanup, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { compatStatuses, type CompatStatus } from '../../lib/services/compat';
import { getGames, type LibraryGame } from '../../lib/services/library';
import { installedView } from '../../lib/stores/ui';
import { renderDom } from '../../lib/testing/dom';
import Installed from './Installed.svelte';

// The first read of the list: loading until it comes, a failure said with Retry, "no games" only
// after a read that found none; and the compatibility log, whose late or failed answer must not
// land over a newer one.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/compat', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/compat')>()),
  compatStatuses: vi.fn(async () => new Map()),
}));
vi.mock('../../lib/services/library', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/library')>()),
  getGames: vi.fn(async () => []),
}));
vi.mock('../../lib/stores/metadata', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/metadata')>()),
  loadArt: vi.fn(),
}));
vi.mock('../../lib/stores/missingImages', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/missingImages')>()),
  refreshMissingImages: vi.fn(async () => {}),
}));

const hades = { id: 'g-hades', title: 'Hades', installDir: 'C:\\Games\\Hades', executable: 'C:\\Games\\Hades\\game.exe', sizeBytes: 0, uninstalled: false, tags: [], lastPlayed: null } as unknown as LibraryGame;
const broken = (gameId: string): CompatStatus => ({ gameId, state: 'broken', attempts: 2, failures: 2, bestSeconds: 0 });

const heading = (text: string) => screen.queryByRole('heading', { name: text });

beforeEach(() => installedView.set('list'));

afterEach(() => {
  cleanup();
  libraryGames.set([]);
  libraryLoaded.set(false);
  libraryLoadError.set(null);
  runningGames.set(new Set());
  vi.clearAllMocks();
  document.body.replaceChildren();
});

describe('Installed: the first read of the list', () => {
  it('says it is loading until the first list arrives, not that there are no games', () => {
    renderDom(Installed);

    expect(screen.getByText(msg('library.loadingTitle'))).toBeTruthy();
    expect(heading(msg('games.installedNoGamesTitle'))).toBeNull();
    expect(screen.queryByText(msg('games.installedEmptySubtitle'))).toBeNull();
  });

  it('says there are no games once a list without any arrived', () => {
    libraryLoaded.set(true);
    renderDom(Installed);

    expect(heading(msg('games.installedNoGamesTitle'))).not.toBeNull();
    expect(screen.queryByText(msg('library.loadingTitle'))).toBeNull();
  });

  it('a failed first read is a danger notice in the catalogue’s words, never Go’s text', () => {
    libraryLoadError.set(new Error('typhon:library.removing: raw go text'));
    renderDom(Installed);

    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    expect(notice.textContent).toContain(msg('library.loadFailedTitle'));
    expect(notice.textContent).toContain(msg('library.removing'));
    expect(notice.textContent).not.toContain('raw go text');
    expect(heading(msg('games.installedNoGamesTitle'))).toBeNull();
    expect(screen.queryByText(msg('library.loadingTitle'))).toBeNull();
  });

  it('Retry reads the list again; on success the rows replace the notice and focus goes to the first row', async () => {
    libraryLoadError.set(new Error('boom'));
    vi.mocked(getGames).mockResolvedValueOnce([hades]);
    const { user } = renderDom(Installed);
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    expect(notice.textContent).toContain(msg('library.loadFailedText'));
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(getGames).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('button', { name: msg('games.installedMenuFor', { title: 'Hades' }) })).toBeTruthy();
    await vi.waitFor(() => expect(document.activeElement?.closest('[data-game-row="g-hades"]')).not.toBeNull());
  });

  it('a retry that fails again keeps the notice and the focus on Retry', async () => {
    libraryLoadError.set(new Error('boom'));
    vi.mocked(getGames).mockRejectedValueOnce(new Error('again'));
    const { user } = renderDom(Installed);
    const region = screen.getByRole('alert');
    const notice = region.closest('.notice') as HTMLElement;
    const before = region.firstElementChild;
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
  });
});

describe('Installed: the compatibility log', () => {
  beforeEach(() => {
    libraryLoaded.set(true);
    libraryGames.set([hades]);
  });

  const badge = () =>
    screen.getByRole('button', { name: msg('games.installedMenuFor', { title: 'Hades' }) }).closest('.row')?.textContent?.includes(msg('games.compatBrokenBadge'));

  it('an answer to an earlier read that comes last does not replace the newer one', async () => {
    const answers: Array<(map: Map<string, CompatStatus>) => void> = [];
    vi.mocked(compatStatuses).mockImplementation(() => new Promise((resolve) => answers.push(resolve)));
    renderDom(Installed);
    await tick();
    runningGames.set(new Set(['g-hades']));
    await tick();
    runningGames.set(new Set());
    await tick();
    expect(answers).toHaveLength(3);

    answers[2](new Map([['g-hades', broken('g-hades')]]));
    await vi.waitFor(() => expect(badge()).toBe(true));
    answers[0](new Map());
    await tick();
    await tick();

    expect(badge()).toBe(true);
  });

  it('a failed read keeps what the last one said, and nothing is thrown', async () => {
    vi.mocked(compatStatuses).mockResolvedValueOnce(new Map([['g-hades', broken('g-hades')]]));
    renderDom(Installed);
    await vi.waitFor(() => expect(badge()).toBe(true));
    vi.mocked(compatStatuses).mockRejectedValueOnce(new Error('log unreadable'));

    runningGames.set(new Set(['g-hades']));
    await tick();
    await tick();

    expect(compatStatuses).toHaveBeenCalledTimes(2);
    expect(badge()).toBe(true);
  });
});
