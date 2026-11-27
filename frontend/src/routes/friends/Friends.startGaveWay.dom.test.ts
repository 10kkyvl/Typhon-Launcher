import { cleanup, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Events } from '@wailsio/runtime';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import type { Settings } from '../../lib/services/settings';
import { friends, kick, refresh, type FriendsPage } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { initSocial } from '../../lib/stores/social';
import { toast } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Friends from './Friends.svelte';
import { NoResize, friend } from './friendsTesting';

// The Friends screen as the first route of a signed-in start whose read gives way to a page Go sent
// while the stored session was checked: that page is dropped, and the start's answer, older than it,
// brings none. The screen then reads once itself, so a failure there is its notice with Retry, not a
// spinner until Go's next good poll. One file, one start: the store starts once per module graph.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/stores/toasts', async (original) => ({
  ...(await original<typeof import('../../lib/stores/toasts')>()),
  toast: vi.fn(),
}));

const HEARD: FriendsPage = { friends: [friend('u-ana', 'Ana', 'online')], incoming: [], outgoing: [] };
const START: FriendsPage = { friends: [friend('u-bo', 'Bo', 'online')], incoming: [], outgoing: [] };

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ accountSync: true } as Settings);
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  vi.unstubAllGlobals();
});

describe('Friends: the first route of a start whose read gave way', () => {
  it('reads once itself, and its failure is the notice with Retry, not a toast', async () => {
    let answer!: (value: FriendsPage) => void;
    vi.mocked(friends).mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    vi.mocked(refresh).mockRejectedValueOnce(new AccountError('network_error'));
    const started = initSocial();
    const heard = vi.mocked(Events.On).mock.calls.find(([name]) => name === 'social:friends')?.[1] as (event: { data: unknown }) => void;

    heard({ data: HEARD });
    authState.set('authenticated');
    render(Friends, { target: document.getElementById('app') as HTMLElement, props: {} });
    await tick();
    answer(START);
    await started;

    expect(await screen.findByRole('button', { name: msg('common.retry') })).toBeTruthy();
    expect(screen.queryByText('Bo')).toBeNull();
    expect([kick, refresh, friends].map((call) => vi.mocked(call).mock.calls.length)).toEqual([1, 1, 1]);
    expect(toast).not.toHaveBeenCalled();
  });
});
