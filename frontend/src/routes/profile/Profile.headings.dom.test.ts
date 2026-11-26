import { cleanup, screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import type { ProfileBlock } from '../../lib/services/account';
import { EMPTY_SNAPSHOT, getProfileSnapshot } from '../../lib/services/profile';
import { profileSnapshot } from '../../lib/stores/profile';
import { navigate, resetHistory } from '../../lib/stores/router';
import { authState, currentUser } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Profile from './Profile.svelte';
import { player, SNAPSHOT } from './profileFixtures';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));

// The own profile's headings, as another player's page has them: the page's h1 is the player's
// @username (the one the shell focuses on arrival), the display name under it an h2, the cards of the
// blocks h3 (the kit's Card fixes its title there). The markup read from the source is in
// Profile.markup.test.ts.

const block = (type: string): ProfileBlock => ({ id: `b-${type}`, type, width: 'full', config: {} });
const headings = () => screen.getAllByRole('heading').map((heading) => [heading.tagName, heading.textContent?.trim()]);

beforeEach(() => {
  locale.set('en');
  resetBindings();
  navigate('profile');
  vi.mocked(getProfileSnapshot).mockResolvedValue(SNAPSHOT);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  vi.mocked(getProfileSnapshot).mockReset();
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  profileSnapshot.set(EMPTY_SNAPSHOT);
  document.body.replaceChildren();
});

describe('the own profile headings', () => {
  it('the page is titled by the @username, the display name is an h2 and the blocks are h3', async () => {
    authState.set('authenticated');
    currentUser.set(player({ layout: { version: 1, blocks: [block('stats'), block('recent')] } }));
    renderDom(Profile);
    await screen.findByText(msg('profile.blockStats'));

    expect(headings()).toEqual([
      ['H1', '@ann'],
      ['H2', 'Ann'],
      ['H3', msg('profile.blockStats')],
      ['H3', msg('social.recentlyPlayedTitle')],
    ]);
  });

  it("a guest's page is titled Profile, the guest's name under it", () => {
    authState.set('guest');
    renderDom(Profile);

    expect(headings()).toEqual([
      ['H1', msg('social.profileLabel')],
      ['H2', msg('social.guestName')],
    ]);
  });
});
