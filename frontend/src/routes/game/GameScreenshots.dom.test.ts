import { screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { listShots, screenshotsStatus, type Shot } from '../../lib/services/screenshots';
import { lastSavedShot } from '../../lib/stores/screenshots';
import { toast, toasts } from '../../lib/stores/toasts';
import { renderDom } from '../../lib/testing/dom';
import { dateTime } from '../../lib/utils/format';
import GameScreenshots from './GameScreenshots.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/screenshots', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/screenshots')>()),
  listShots: vi.fn(),
  screenshotsStatus: vi.fn(),
}));
// The store takes a repeated message into the toast already shown, so the calls are what tell a
// repeated report apart.
vi.mock('../../lib/stores/toasts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/stores/toasts')>();
  return { ...actual, toast: vi.fn(actual.toast) };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(screenshotsStatus).mockResolvedValue({ enabled: true, hotkey: 'F12' } as Awaited<ReturnType<typeof screenshotsStatus>>);
});

afterEach(() => {
  vi.mocked(listShots).mockReset();
  vi.mocked(screenshotsStatus).mockReset();
  vi.mocked(toast).mockClear();
  lastSavedShot.set(null);
  toasts.set([]);
  vi.restoreAllMocks();
});

function shot(name: string, at: string): Shot {
  return { name, url: `https://img.test/${name}`, at } as Shot;
}

describe('GameScreenshots: the thumbnails', () => {
  it('each has a name of its own, even for two shots taken in the same second', async () => {
    const at = '2026-11-14T10:00:00Z';
    vi.mocked(listShots).mockResolvedValue([shot('a.png', at), shot('b.png', at), shot('c.png', '2026-11-14T11:00:00Z')]);
    renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });

    await vi.waitFor(() => expect(document.querySelectorAll('button.thumb')).toHaveLength(3));
    const names = [...document.querySelectorAll('button.thumb')].map((button) => button.getAttribute('aria-label'));

    expect(new Set(names).size).toBe(3);
    expect(names[0]).toBe(msg('games.blocks.screenshotOpen', { index: 1, count: 3, name: dateTime(new Date(at)) }));
  });
});

describe('GameScreenshots: loading and nothing yet', () => {
  it('shows the base spinner named for what loads, then the empty state', async () => {
    let answer!: (shots: Shot[]) => void;
    vi.mocked(listShots).mockReturnValue(new Promise((resolve) => (answer = resolve)));
    renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });

    const status = screen.getByRole('status');
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('screenshots.loading'));

    answer([]);
    const empty = (await screen.findByText(msg('screenshots.empty', { hotkey: 'F12' }))).closest('.empty');
    expect(empty?.classList.contains('inline')).toBe(true);
    expect(screen.queryByRole('status')).toBeNull();
  });
});

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => (resolve = res));
  return { promise, resolve };
}

const saved = (name: string, gameId = 'g1') => ({ gameId, title: 'Hades', path: `/shots/${name}`, name, at: '2026-11-14T10:00:00Z' });

describe('GameScreenshots: a list that failed to load', () => {
  it('can be retried: the notice and the focus stay on its busy Retry, then focus goes to the first shot', async () => {
    const again = deferred<Shot[]>();
    vi.mocked(listShots).mockRejectedValueOnce(new Error('typhon:screenshots.read_failed: нет доступа')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    expect(notice.textContent).toContain(msg('screenshots.loadError'));
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve([shot('a.png', '2026-11-14T10:00:00Z')]);
    await vi.waitFor(() => expect(document.querySelector('button.thumb')).not.toBeNull());
    await vi.waitFor(() => expect(document.activeElement).toBe(document.querySelector('button.thumb')));
  });

  it('keeps the notice when the retry fails again, says it again, and can be retried once more', async () => {
    vi.mocked(listShots).mockRejectedValue(new Error('typhon:screenshots.read_failed: нет доступа'));
    const { user } = renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(msg('screenshots.loadError')));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    expect(retry.hasAttribute('aria-busy')).toBe(false);

    await user.keyboard('{Enter}');
    await vi.waitFor(() => expect(listShots).toHaveBeenCalledTimes(3));
  });
});

// Both reads of `n` loads have settled, and the block has taken their answers.
async function loadsDone(n: number) {
  const settled = (results: { type: string }[]) => results.filter((result) => result.type !== 'incomplete').length;
  await vi.waitFor(() => {
    expect(settled(vi.mocked(listShots).mock.settledResults)).toBe(n);
    expect(settled(vi.mocked(screenshotsStatus).mock.settledResults)).toBe(n);
  });
  await tick();
}

describe('GameScreenshots: how often a failure is reported', () => {
  const listFailure = () => new Error('typhon:screenshots.read_failed: нет доступа');
  const statusFailure = () => new Error('typhon:screenshots.status_failed: нет');

  it('a list that keeps failing is reported once while its notice stands, and again after a success', async () => {
    vi.mocked(listShots)
      .mockRejectedValueOnce(listFailure())
      .mockRejectedValueOnce(listFailure())
      .mockRejectedValueOnce(listFailure())
      .mockResolvedValueOnce([])
      .mockRejectedValueOnce(listFailure());
    renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });
    await loadsDone(1);
    expect(toast).toHaveBeenCalledTimes(1);

    lastSavedShot.set(saved('a.png'));
    await loadsDone(2);
    lastSavedShot.set(saved('b.png'));
    await loadsDone(3);
    expect(toast).toHaveBeenCalledTimes(1);

    lastSavedShot.set(saved('c.png'));
    await loadsDone(4);
    lastSavedShot.set(saved('d.png'));
    await loadsDone(5);
    expect(toast).toHaveBeenCalledTimes(2);
  });

  it('a retry that fails again says it in the notice and toasts nothing', async () => {
    vi.mocked(listShots).mockRejectedValue(listFailure());
    const { user } = renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    await loadsDone(1);
    expect(toast).toHaveBeenCalledTimes(1);

    await user.click(retry);
    await loadsDone(2);

    expect(retry.isConnected).toBe(true);
    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('a status read that succeeds re-arms its one report', async () => {
    vi.mocked(listShots).mockResolvedValue([]);
    vi.mocked(screenshotsStatus)
      .mockRejectedValueOnce(statusFailure())
      .mockResolvedValueOnce({ enabled: true, hotkey: 'F12' } as Awaited<ReturnType<typeof screenshotsStatus>>)
      .mockRejectedValueOnce(statusFailure());
    renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });
    await loadsDone(1);
    expect(toast).toHaveBeenCalledTimes(1);

    lastSavedShot.set(saved('a.png'));
    await loadsDone(2);
    expect(toast).toHaveBeenCalledTimes(1);
    lastSavedShot.set(saved('b.png'));
    await loadsDone(3);

    expect(toast).toHaveBeenCalledTimes(2);
  });
});

describe('GameScreenshots: reloads', () => {
  it('a status read that keeps failing is reported once, not again on every saved screenshot', async () => {
    vi.mocked(screenshotsStatus).mockRejectedValue(new Error('typhon:screenshots.status_failed: нет'));
    vi.mocked(listShots).mockResolvedValue([]);
    renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });
    await vi.waitFor(() => expect(toast).toHaveBeenCalledTimes(1));

    lastSavedShot.set(saved('a.png'));
    await vi.waitFor(() => expect(listShots).toHaveBeenCalledTimes(2));
    lastSavedShot.set(saved('b.png'));
    await vi.waitFor(() => expect(listShots).toHaveBeenCalledTimes(3));
    await vi.waitFor(() => expect(screenshotsStatus).toHaveBeenCalledTimes(3));
    await new Promise((resolve) => setTimeout(resolve, 20));

    expect(toast).toHaveBeenCalledTimes(1);
  });

  it('loads once on mount, though the last saved screenshot is of this game', async () => {
    lastSavedShot.set(saved('a.png'));
    vi.mocked(listShots).mockResolvedValue([shot('a.png', '2026-11-14T10:00:00Z')]);
    renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });

    await vi.waitFor(() => expect(document.querySelector('button.thumb')).not.toBeNull());
    await new Promise((resolve) => setTimeout(resolve, 20));

    expect(listShots).toHaveBeenCalledTimes(1);
  });

  it("reloads for a screenshot of this game saved later, not for another game's", async () => {
    vi.mocked(listShots).mockResolvedValue([]);
    renderDom(GameScreenshots, { gameId: 'g1', title: 'Hades' });
    await vi.waitFor(() => expect(listShots).toHaveBeenCalledTimes(1));

    lastSavedShot.set(saved('x.png', 'g2'));
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(listShots).toHaveBeenCalledTimes(1);
    lastSavedShot.set(saved('a.png'));

    await vi.waitFor(() => expect(listShots).toHaveBeenCalledTimes(2));
  });
});
