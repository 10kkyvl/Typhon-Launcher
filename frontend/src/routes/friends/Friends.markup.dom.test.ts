import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import type { Settings } from '../../lib/services/settings';
import { friends, type FriendsPage } from '../../lib/services/social';
import { settings } from '../../lib/stores/settings';
import { friendsPage } from '../../lib/stores/social';
import { authState } from '../../lib/stores/user';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Friends from './Friends.svelte';
import { NoResize, friend, request } from './friendsTesting';

const SCREEN = import.meta.glob<string>('./*.svelte', { query: '?raw', import: 'default', eager: true });

// The screen's headings come in order under its h1: each section is an h2 of the kit's section
// heading, its count part of its name, and the section is named by it. Spacing comes from the scale.

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));

const PAGE: FriendsPage = {
  friends: [friend('u-ana', 'Ana', 'online')],
  incoming: [request('r-inna', 'Inna'), request('r-kirill', 'Kirill')],
  outgoing: [request('r-lev', 'Lev')],
};

const headings = () => [...document.querySelectorAll('h1, h2, h3, h4, h5, h6')].map((h) => `${h.tagName} ${h.textContent?.replace(/\s+/g, ' ').trim()}`);

async function mount(tab?: string) {
  render(Friends, { target: document.getElementById('app') as HTMLElement, props: { tab } });
  await screen.findByText('Inna');
}

beforeEach(() => {
  vi.stubGlobal('ResizeObserver', NoResize);
  const app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  authState.set('authenticated');
  settings.set({ accountSync: true } as Settings);
  friendsPage.set(PAGE);
  vi.mocked(friends).mockResolvedValue(PAGE);
  resetBindings();
});

afterEach(() => {
  expect(calls).toEqual([]);
  cleanup();
  document.body.replaceChildren();
  friendsPage.set({ friends: [], incoming: [], outgoing: [] });
  authState.set('bootstrapping');
  settings.set(null);
  vi.resetAllMocks();
  vi.unstubAllGlobals();
});

describe('Friends: headings', () => {
  it('names the incoming requests over the list with an h2 and its count, and the section by it', async () => {
    await mount();

    expect(headings()).toEqual([`H1 ${msg('social.friendsTitle')}`, `H2 ${msg('social.friendsIncomingHeading')} 2`]);
    expect(screen.getByRole('region', { name: `${msg('social.friendsIncomingHeading')} 2` })).toBeTruthy();
  });

  it('gives the requests tab h2 sections with counts, the safety advice one level under the page', async () => {
    await mount('requests');

    expect(headings()).toEqual([
      `H1 ${msg('social.friendsTitle')}`,
      `H2 ${msg('social.friendsIncomingHeading')} 2`,
      `H2 ${msg('social.friendsSentHeading')} 1`,
      `H2 ${msg('social.friendsSafetyTitle')}`,
    ]);
    expect(screen.getByRole('region', { name: `${msg('social.friendsSentHeading')} 1` })).toBeTruthy();
  });
});

describe('Friends: spacing', () => {
  // The avatar's own width, which a compact row's second line steps past: a size, not a spacing.
  const SIZES = ['margin-left: calc(3.2rem * (1 + var(--reserve-l, 0) + var(--reserve-r, 0)) + var(--space-3));'];

  it('takes every padding, margin and gap of the screen from the scale', () => {
    expect(Object.keys(SCREEN)).toContain('./FriendRow.svelte');
    const literal = Object.entries(SCREEN)
      .flatMap(([path, text]) =>
        (text.match(/^\s*(?:padding|margin|gap)[\w-]*:[^;]*\d(?:\.\d+)?rem[^;]*;/gm) ?? [])
          .map((line) => line.trim())
          .filter((line) => !SIZES.includes(line))
          .map((line) => `${path}: ${line}`),
      );
    expect(literal).toEqual([]);
  });
});
