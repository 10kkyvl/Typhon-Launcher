import { Events } from '@wailsio/runtime';
import { cleanup, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Settings } from '../../lib/services/settings';
import { blocks, friends, type FriendsPage } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { initSocial } from '../../lib/stores/social';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { user } from '../../lib/testing/dom';
import Friends from './Friends.svelte';
import { NoResize, friend } from './friendsTesting';

// A friend who starts a game moves from "Other friends" to "Playing now": the two groups are two
// lists, so the row is built anew in the other one, and focus that was on one of its controls follows
// to the same control of the new row (the row rule finds it by the friend's id), not to a neighbour.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: true }));
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));

const ANA = friend('u-ana', 'Ana', 'online');
const GLEB = friend('u-gleb', 'Gleb', 'online');
const playing = (f: ReturnType<typeof friend>) => ({ ...f, presence: { status: 'online', gameId: 7, gameTitle: 'Quake' } });

let friendsEvent: ((event: { data: unknown }) => void) | undefined;
const emitFriends = (page: FriendsPage) => friendsEvent?.({ data: page });
const NAMES: Record<string, string> = { 'u-ana': 'Ana', 'u-gleb': 'Gleb' };
const rowOf = (id: string) => screen.getByText(NAMES[id], { selector: '.name' }).closest('[data-friends-row]') as HTMLElement;
const chatOf = (id: string) => rowOf(id).querySelector('[data-row-control="chat"]') as HTMLElement;

async function mount(page: FriendsPage) {
  vi.mocked(friends).mockResolvedValue(page);
  render(Friends, { target: document.getElementById('app') as HTMLElement, props: {} });
  const keys = user();
  await screen.findByText(page.friends[0].displayName);
  await vi.waitFor(() => expect(blocks).toHaveBeenCalled());
  await tick();
  return keys;
}

beforeAll(async () => {
  settings.set({ accountSync: true } as Settings);
  await initSocial();
  // Started signed out: each test then signs in, a change of account that clears what the start read.
  authState.set('unauthenticated');
  friendsEvent = vi.mocked(Events.On).mock.calls.find(([name]) => name === 'social:friends')?.[1] as typeof friendsEvent;
});

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  settings.set({ accountSync: true } as Settings);
  authState.set('authenticated');
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  authState.set('unauthenticated');
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends: focus when a friend changes group', () => {
  it('P5 follows a friend who starts a game into "Playing now" (the row moves between groups)', async () => {
    await mount({ friends: [playing(ANA), GLEB], incoming: [], outgoing: [] });
    const before = chatOf('u-gleb');
    before.focus();
    emitFriends({ friends: [playing(ANA), playing(GLEB)], incoming: [], outgoing: [] });
    // Checked once the change has shown: before it, the old row and its focused button are still there.
    await tick();
    await vi.waitFor(() => expect(document.activeElement).toBe(chatOf('u-gleb')));
    expect(before.isConnected).toBe(false);
  });

  it('P6 follows the first friend who starts a game (the groups appear)', async () => {
    await mount({ friends: [ANA, GLEB], incoming: [], outgoing: [] });
    const before = chatOf('u-gleb');
    before.focus();
    emitFriends({ friends: [ANA, playing(GLEB)], incoming: [], outgoing: [] });
    await tick();
    await vi.waitFor(() => expect(document.activeElement).toBe(chatOf('u-gleb')));
    expect(before.isConnected).toBe(false);
  });
});
