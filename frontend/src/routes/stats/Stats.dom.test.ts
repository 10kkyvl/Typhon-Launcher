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

import { msg } from '../../lib/i18n';
import type { LibraryGame } from '../../lib/services/library';
import { getStats } from '../../lib/services/playlog';
import { libraryGames } from '../../lib/stores/library';
import { toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import Stats from './Stats.svelte';

afterEach(() => libraryGames.set([]));

describe('Stats: most played', () => {
  // The bar under each game draws its share of the period's playtime; the row's text says it.
  it('says each game’s share of the playtime in the row, the bar only draws it', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const { findByRole, container } = renderDom(Stats);

    const named = async (title: string) => {
      let found = '';
      await findByRole('button', {
        name: (name) => {
          if (name.includes(title)) found = name;
          return name.includes(title);
        },
      });
      return found;
    };
    expect(await named('Hades')).toMatch(/ · 75%$/);
    expect(await named('Celeste')).toMatch(/ · 25%$/);
    for (const share of container.querySelectorAll('.share')) expect(share.getAttribute('aria-hidden')).toBe('true');
  });

  // A game without art shows initials in its place: they are a picture, not part of the row's name.
  it('names each row by the game first, the cover says nothing', async () => {
    libraryGames.set([{ id: 'g1', title: 'Hades' } as LibraryGame, { id: 'g2', title: 'Celeste' } as LibraryGame]);
    const { findByRole } = renderDom(Stats);

    expect(await findByRole('button', { name: (name) => name.startsWith('Hades ') })).toBeTruthy();
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
