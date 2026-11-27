import { cleanup, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Settings } from '../../lib/services/settings';
import { blocks, friends, kick, refresh, type FriendsPage } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { initSocial } from '../../lib/stores/social';
import { toast } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Friends from './Friends.svelte';
import { NoResize, friend } from './friendsTesting';

// The Friends screen as the first route of a signed-in start: the store reads the friends while the
// stored session is checked behind the splash, and the screen, mounted when the splash goes, takes
// that read as its own while it runs. One file, one start: the store starts once per module graph.

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

const PAGE: FriendsPage = { friends: [friend('u-ana', 'Ana', 'online')], incoming: [], outgoing: [] };

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

describe('Friends: the first route of a signed-in start', () => {
  it('shows the start’s read and reads nothing more: one kick, one Friends(), no refresh, one blocked list', async () => {
    let answer!: (value: FriendsPage) => void;
    vi.mocked(friends).mockReturnValueOnce(new Promise((resolve) => (answer = resolve)));
    const started = initSocial();

    authState.set('authenticated');
    render(Friends, { target: document.getElementById('app') as HTMLElement, props: {} });
    await tick();
    answer(PAGE);

    expect(await screen.findByText('Ana')).toBeTruthy();
    await started;
    await vi.waitFor(() => expect(blocks).toHaveBeenCalled());
    await tick();
    expect([kick, refresh, friends, blocks].map((call) => vi.mocked(call).mock.calls.length)).toEqual([1, 0, 1, 1]);
    expect(toast).not.toHaveBeenCalled();
  });
});
