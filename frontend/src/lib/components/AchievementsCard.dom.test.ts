import { screen } from '@testing-library/svelte';
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

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  toasts.set([]);
  vi.restoreAllMocks();
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
