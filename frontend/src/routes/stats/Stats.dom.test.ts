import { within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/playlog', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/playlog')>()),
  getStats: vi.fn(async () => ({
    period: 'week',
    from: '2026-11-05',
    to: '2026-11-11',
    totalSeconds: 4000,
    sessions: 3,
    days: [],
    top: [
      { gameId: 'g1', title: 'Hades', seconds: 3000, sessions: 2 },
      { gameId: 'g2', title: 'Celeste', seconds: 1000, sessions: 1 },
    ],
    longest: null,
  })),
}));
vi.mock('../../lib/stores/router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/stores/router')>()),
  navigate: vi.fn(),
}));

import { locale, msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { getStats } from '../../lib/services/playlog';
import { libraryGames } from '../../lib/stores/library';
import { navigate } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { playtime } from '../../lib/utils/format';
import Stats from './Stats.svelte';

afterEach(() => libraryGames.set([]));

/** The text a screen reader reads out of `element`: everything not hidden from it. */
function readText(element: Element): string {
  const parts: string[] = [];
  const walk = (node: Node) => {
    if (node instanceof Element && (node.getAttribute('aria-hidden') === 'true' || node.hasAttribute('hidden'))) return;
    if (node.nodeType === Node.TEXT_NODE) parts.push(node.textContent ?? '');
    node.childNodes.forEach(walk);
  };
  walk(element);
  return parts.join(' ').replace(/\s+/g, ' ').trim();
}

describe('Stats: most played', () => {
  // The bar under each game is its share of the period's playtime, named by the game and said in words.
  it('says each game’s share of the playtime through its bar, named by the game', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const { findByRole, getByRole } = renderDom(Stats);
    await findByRole('button', { name: 'Hades' });

    const share = (title: string) => getByRole('progressbar', { name: title });
    expect(share('Hades').getAttribute('aria-valuenow')).toBe('75');
    expect(share('Hades').getAttribute('aria-valuetext')).toBe(msg('stats.topShare', { share: 75 }));
    expect(share('Celeste').getAttribute('aria-valuetext')).toBe(msg('stats.topShare', { share: 25 }));
  });

  // One catalogued message, so a language can put the time and the sessions in its own order.
  it('says a game’s time and sessions as one sentence of the reader’s language', async () => {
    locale.set('en');
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const { findByRole, getAllByRole } = renderDom(Stats);
    await findByRole('button', { name: 'Hades' });

    const texts = getAllByRole('listitem').map((row) => row.textContent?.replace(/\s+/g, ' ').trim());
    expect(texts[0]).toContain(msg('stats.topLine', { time: playtime(3000), count: 2 }));
    expect(texts[1]).toContain(msg('stats.topLine', { time: playtime(1000), count: 1 }));
    expect(msg('stats.topLine', { time: playtime(1000), count: 1 })).toBe(`${playtime(1000)} in 1 session`);
  });

  // A game without art shows initials in its place: they are a picture, not part of the row's name.
  it('names each row by the game first, the cover says nothing', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const { findByRole, getAllByRole } = renderDom(Stats);

    const link = await findByRole('button', { name: 'Hades' });
    expect(readText(getAllByRole('listitem')[0]).startsWith('Hades ')).toBe(true);
    expect(link.closest('li')).toBe(getAllByRole('listitem')[0]);
  });

  it('the title leads to the game’s page, one Tab stop per game: the cover is for the pointer', async () => {
    vi.mocked(navigate).mockReset();
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const { findByRole, getAllByRole, user } = renderDom(Stats);

    await user.click(await findByRole('button', { name: 'Hades' }));

    expect(navigate).toHaveBeenCalledWith('game', { id: 'g1' });
    const row = getAllByRole('listitem')[0];
    const tabbable = [...row.querySelectorAll<HTMLElement>('button, a[href], [tabindex]')].filter((element) => element.tabIndex >= 0);
    expect(tabbable.map((element) => element.textContent?.trim())).toEqual(['Hades']);
  });

  it('a game gone from the library is named as plain text, with its time, and leads nowhere', async () => {
    libraryGames.set([{ id: 'g2', title: 'Celeste' } as LibraryGame]);
    const { findByRole, getAllByRole, queryByRole } = renderDom(Stats);
    await findByRole('button', { name: 'Celeste' });

    const gone = getAllByRole('listitem').find((row) => readText(row).startsWith('Hades')) as HTMLElement;
    expect(gone).toBeTruthy();
    expect(gone.querySelectorAll('button')).toHaveLength(0);
    expect(queryByRole('button', { name: 'Hades' })).toBeNull();
    expect(readText(gone)).toContain(playtime(3000));
  });
});

describe('Stats: retrying a failed first load', () => {
  afterEach(() => toasts.set([]));

  it('keeps the danger notice and the focus on its busy Retry until the answer', async () => {
    let answer!: (value: Awaited<ReturnType<typeof getStats>>) => void;
    vi.mocked(getStats)
      .mockRejectedValueOnce(new Error('offline'))
      .mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const { findByRole, getByRole, user } = renderDom(Stats);
    const notice = (await findByRole('alert')).closest('.notice') as HTMLElement;
    expect(notice.textContent).toContain(msg('stats.loadError'));
    const retry = within(notice).getByRole('button', { name: msg('stats.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    answer({ period: 'week', from: '2026-11-05', to: '2026-11-11', totalSeconds: 0, sessions: 0, days: [], top: [], longest: null });
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(getByRole('heading', { level: 2, name: msg('stats.title') })).toBeTruthy();
  });

  it('says the failure once, in the notice, not again in a toast', async () => {
    vi.mocked(getStats).mockRejectedValueOnce(new Error('offline'));
    const { findByRole } = renderDom(Stats);

    expect((await findByRole('alert')).textContent).toContain(msg('stats.loadError'));
    expect(get(toasts)).toEqual([]);
  });

  it('on success hands the focus to the first most-played game', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame]);
    vi.mocked(getStats).mockRejectedValueOnce(new Error('offline'));
    const { findByRole, user } = renderDom(Stats);
    const notice = (await findByRole('alert')).closest('.notice') as HTMLElement;
    within(notice).getByRole('button', { name: msg('stats.retry') }).focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement?.textContent).toContain('Hades'));
    expect(document.activeElement?.tagName).toBe('BUTTON');
  });

  it('a retry that fails again keeps the notice and the focus on its Retry', async () => {
    vi.mocked(getStats).mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new Error('still offline'));
    const { findByRole, user } = renderDom(Stats);
    const region = await findByRole('alert');
    const notice = region.closest('.notice') as HTMLElement;
    const before = region.firstElementChild;
    const retry = within(notice).getByRole('button', { name: msg('stats.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    // Said again: the failure's words come into the live region as a new node.
    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.textContent).toContain(msg('stats.loadError'));
  });
});

describe('Stats: the first load', () => {
  it('shows the base spinner named by what is loading', async () => {
    vi.mocked(getStats).mockReturnValueOnce(new Promise(() => {}));
    const { findByText } = renderDom(Stats);

    expect((await findByText(msg('stats.loading'))).closest('.spinner[role="status"]')).not.toBeNull();
  });
});
