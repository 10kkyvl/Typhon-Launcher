import { screen } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import { profile as fetchProfile } from '../../lib/services/social';
import { resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import UserProfile from './UserProfile.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));

beforeEach(() => {
  authState.set('authenticated');
  resetBindings();
  toasts.set([]);
});

afterEach(() => {
  expect(calls).toEqual([]);
  vi.mocked(fetchProfile).mockReset();
  document.body.replaceChildren();
  authState.set('bootstrapping');
  resetHistory();
});

// The server answers a profile of a player either of the two has blocked as "user not found"
// (typhon-backend social.Service.profile → ErrNotFound → 404 user_not_found): the page cannot tell
// it from a name nobody has, so it says both.
describe("a user's page: a player it cannot show", () => {
  it('"not found" from the server says the name may be wrong or one of the two blocked the other', async () => {
    vi.mocked(fetchProfile).mockRejectedValue(new AccountError('user_not_found'));
    renderDom(UserProfile, { username: 'ann' });

    expect(await screen.findByRole('heading', { level: 2, name: msg('social.userNotFoundTitle') })).toBeTruthy();
    expect(screen.getByText(msg('social.userNotFoundHiddenDesc'))).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('a page opened without a name says only that there is no such player', async () => {
    renderDom(UserProfile, { username: '' });

    expect(await screen.findByRole('heading', { level: 2, name: msg('social.userNotFoundTitle') })).toBeTruthy();
    expect(screen.getByText(msg('social.userNotFoundDesc'))).toBeTruthy();
    expect(vi.mocked(fetchProfile)).not.toHaveBeenCalled();
  });
});
