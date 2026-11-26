import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Settings } from '../../lib/services/settings';
import { friends, type FriendsPage } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { friendsPage } from '../../lib/stores/social';
import { friendsView } from '../../lib/stores/ui';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { user } from '../../lib/testing/dom';
import Friends from './Friends.svelte';
import { NoResize, friend } from './friendsTesting';

// A friend whose presence carries a game is at the top of the list, under "Playing now" with their
// count, whatever the sorting; the others follow under their own heading. Nothing new is read: the
// presence is the one the friends page brings.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));

const playing = (id: string, name: string, title: string) => friend(id, name, 'online', { presence: { status: 'online', gameId: 7, gameTitle: title } });

const rowNames = () => [...document.querySelectorAll('[data-friends-row] .name')].map((name) => name.textContent);
const headingTexts = () => [...document.querySelectorAll('h2')].map((h) => h.textContent?.replace(/\s+/g, ' ').trim());

function seed(page: FriendsPage) {
  friendsPage.set(page);
  vi.mocked(friends).mockResolvedValue(page);
}

async function mount() {
  render(Friends, { target: document.getElementById('app') as HTMLElement, props: {} });
  await vi.waitFor(() => expect(friends).toHaveBeenCalled());
  return user();
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  settings.set({ accountSync: true } as Settings);
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  friendsPage.set({ friends: [], incoming: [], outgoing: [] });
  authState.set('bootstrapping');
  settings.set(null);
  friendsView.set('list');
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends: playing now', () => {
  it.each(['list', 'grid'] as const)('puts the friends in a game first under their heading and count, sorted by name too (%s)', async (view) => {
    friendsView.set(view);
    seed({
      friends: [friend('u-ana', 'Ana', 'online'), playing('u-vera', 'Vera', 'Hades'), friend('u-gleb', 'Gleb', 'offline'), playing('u-boris', 'Boris', 'Celeste')],
      incoming: [],
      outgoing: [],
    });
    const keys = await mount();
    await screen.findByText('Vera');

    await keys.click(screen.getByRole('button', { name: new RegExp(msg('social.friendsSortStatus')) }));
    await keys.click(screen.getByRole('menuitemradio', { name: msg('social.friendsSortName') }));

    expect(rowNames()).toEqual(['Boris', 'Vera', 'Ana', 'Gleb']);
    expect(headingTexts()).toEqual([`${msg('social.friendsPlayingNowHeading')} 2`, `${msg('social.friendsOthersHeading')} 2`]);
    const nowHeading = screen.getByRole('heading', { name: `${msg('social.friendsPlayingNowHeading')} 2` });
    expect(nowHeading.compareDocumentPosition(screen.getByText('Boris')) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('names the rows with the tab’s own words and count when nobody is in a game', async () => {
    seed({ friends: [friend('u-ana', 'Ana', 'online'), friend('u-gleb', 'Gleb', 'offline')], incoming: [], outgoing: [] });
    await mount();
    await screen.findByText('Ana');

    expect(headingTexts()).toEqual([`${msg('social.friendsTabAll')} 2`]);
    expect(rowNames()).toEqual(['Ana', 'Gleb']);
  });
});
