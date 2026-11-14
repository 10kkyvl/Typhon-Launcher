import { screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { fetchAchievements, rescanAchievements, type AchievementsView } from '../services/achievements';
import { toasts } from '../stores/toasts';
import { renderDom } from '../testing/dom';
import AchievementsCard from './AchievementsCard.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/achievements', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/achievements')>()),
  fetchAchievements: vi.fn(),
  rescanAchievements: vi.fn(),
}));

function view(patch: Partial<AchievementsView> = {}): AchievementsView {
  return {
    appId: '1145360',
    total: 2,
    unlocked: 1,
    items: [
      { key: 'a', name: 'First blood', description: '', hidden: false, icon: '', iconGray: '', unlocked: true, unlockedAt: '' },
      { key: 'b', name: 'Escape', description: '', hidden: false, icon: '', iconGray: '', unlocked: false, unlockedAt: '' },
    ],
    source: 'local',
    schemaError: '',
    partial: false,
    ...patch,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

// Said again after a failed retry: the notice's message comes into its live region again, as new nodes.
// Read before the press; the getter answers the words of a message that came in since, '' until then.
function sayingAgain(notice: Element): () => string {
  const region = notice.querySelector('.body');
  const before = region?.firstElementChild;
  return () => {
    const now = region?.firstElementChild;
    if (!region?.getAttribute('role') || !now || now === before) return '';
    return (now.textContent ?? '').replace(/\s+/g, ' ').trim();
  };
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  vi.mocked(fetchAchievements).mockReset();
  vi.mocked(rescanAchievements).mockReset();
  toasts.set([]);
  vi.restoreAllMocks();
});

describe('AchievementsCard: what the list lacks', () => {
  it('states missing names and unread files as warning notices read with the card', async () => {
    vi.mocked(fetchAchievements).mockResolvedValue(view({ schemaError: 'unavailable', partial: true }));
    renderDom(AchievementsCard, { gameId: 'g1' });

    const names = (await screen.findByText(msg('games.achievementsSchemaUnavailable'))).closest('.notice');
    const partial = screen.getByText(msg('games.achievementsPartial')).closest('.notice');

    expect([names?.getAttribute('data-tone'), partial?.getAttribute('data-tone')]).toEqual(['warning', 'warning']);
    expect(screen.queryByRole('status')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });
});

describe('AchievementsCard: retrying a failed load', () => {
  it('keeps the notice and the focus on its busy Retry until the answer, then focuses the refresh', async () => {
    const again = deferred<AchievementsView>();
    vi.mocked(fetchAchievements).mockRejectedValueOnce(new Error('disk offline')).mockReturnValueOnce(again.promise);
    const { user } = renderDom(AchievementsCard, { gameId: 'g1' });
    const notice = (await screen.findByRole('button', { name: msg('common.retry') })).closest('.notice') as HTMLElement;
    expect(notice).toBeInstanceOf(HTMLElement);
    const retry = within(notice).getByRole('button', { name: msg('common.retry') });
    retry.focus();

    await user.keyboard('{Enter}');

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);

    again.resolve(view());
    const refresh = await screen.findByRole('button', { name: msg('games.achievementsRefresh') });
    expect(notice.isConnected).toBe(false);
    await vi.waitFor(() => expect(document.activeElement).toBe(refresh));
  });

  it('keeps the notice when the retry fails again, and says it again', async () => {
    vi.mocked(fetchAchievements).mockRejectedValue(new Error('disk offline'));
    const { user } = renderDom(AchievementsCard, { gameId: 'g1' });
    const retry = await screen.findByRole('button', { name: msg('common.retry') });
    const notice = retry.closest('.notice') as HTMLElement;
    const words = notice.querySelector('.text')?.textContent?.trim() ?? '';
    const said = sayingAgain(notice);
    retry.focus();

    await user.keyboard('{Enter}');

    await vi.waitFor(() => expect(said()).toBe(words));
    expect(words).not.toBe('');
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
  });
});

describe('AchievementsCard: refreshing', () => {
  it('keeps focus on its busy refresh button while the rescan runs', async () => {
    vi.mocked(fetchAchievements).mockResolvedValue(view());
    const rescan = deferred<AchievementsView>();
    vi.mocked(rescanAchievements).mockReturnValue(rescan.promise);
    const { user } = renderDom(AchievementsCard, { gameId: 'g1' });
    const refresh = await screen.findByRole('button', { name: msg('games.achievementsRefresh') });
    refresh.focus();

    await user.keyboard('{Enter}');

    expect(refresh.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(refresh);
    await user.keyboard('{Enter}');
    expect(rescanAchievements).toHaveBeenCalledTimes(1);

    rescan.resolve(view({ unlocked: 2 }));
    await vi.waitFor(() => expect(refresh.hasAttribute('aria-busy')).toBe(false));
    expect(document.activeElement).toBe(refresh);
  });
});

// The page passes another game to the same card when it moves on without a remount.
describe('AchievementsCard: another game in the same card', () => {
  it('a refresh still running for the last game leaves the new game free to refresh, and its answer is dropped', async () => {
    const name = { name: msg('games.achievementsRefresh') };
    vi.mocked(fetchAchievements).mockImplementation(async (id) => view({ appId: id }));
    const late = deferred<AchievementsView>();
    const own = deferred<AchievementsView>();
    vi.mocked(rescanAchievements).mockReturnValueOnce(late.promise).mockReturnValueOnce(own.promise);
    const { user, rerender } = renderDom(AchievementsCard, { gameId: 'g1' });
    await user.click(await screen.findByRole('button', name));

    await rerender({ gameId: 'g2' });
    await vi.waitFor(() => expect(fetchAchievements).toHaveBeenLastCalledWith('g2'));
    const refresh = await screen.findByRole('button', name);
    await vi.waitFor(() => expect(refresh.hasAttribute('aria-busy')).toBe(false));

    await user.click(refresh);
    expect(rescanAchievements).toHaveBeenLastCalledWith('g2');
    expect(refresh.getAttribute('aria-busy')).toBe('true');

    late.resolve(view({ appId: 'g1', unlocked: 0 }));
    await Promise.resolve();
    await Promise.resolve();
    expect(refresh.getAttribute('aria-busy')).toBe('true');
    expect(screen.getByText(msg('games.achievementsProgress', { unlocked: 1, total: 2 }))).toBeTruthy();

    own.resolve(view({ appId: 'g2', unlocked: 2 }));
    await vi.waitFor(() => expect(refresh.hasAttribute('aria-busy')).toBe(false));
    expect(screen.getByText(msg('games.achievementsProgress', { unlocked: 2, total: 2 }))).toBeTruthy();
  });
});

describe('AchievementsCard: what a failure says', () => {
  const noticeText = (button: HTMLElement) => button.closest('.notice')?.querySelector('.text')?.textContent?.trim();

  it('a load failure whose code has no text says the fallback alone, never the Go words or the code', async () => {
    vi.mocked(fetchAchievements).mockRejectedValue(new Error('typhon:achievements.scan_failed: чтение файла'));
    renderDom(AchievementsCard, { gameId: 'g1' });

    const retry = await screen.findByRole('button', { name: msg('common.retry') });

    expect(noticeText(retry)).toBe(msg('games.achievementsLoadError'));
  });

  it('a load failure whose code has a text says that text', async () => {
    vi.mocked(fetchAchievements).mockRejectedValue(new Error('typhon:achievements.store_broken: файл повреждён'));
    renderDom(AchievementsCard, { gameId: 'g1' });

    const retry = await screen.findByRole('button', { name: msg('common.retry') });

    expect(noticeText(retry)).toBe(msg('achievements.store_broken'));
  });

  it('a refresh failure whose code has no text toasts the fallback alone', async () => {
    vi.mocked(fetchAchievements).mockResolvedValue(view());
    vi.mocked(rescanAchievements).mockRejectedValue(new Error('typhon:achievements.scan_failed: чтение файла'));
    const { user } = renderDom(AchievementsCard, { gameId: 'g1' });

    await user.click(await screen.findByRole('button', { name: msg('games.achievementsRefresh') }));

    await vi.waitFor(() => expect(get(toasts).map((item) => item.message)).toEqual([msg('games.achievementsRefreshError')]));
  });
});
