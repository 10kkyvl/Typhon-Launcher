import { beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { answer, resetBindings } from '../testing/fakeBinding';

// The friends list as Go answers it when the social service has not started (the successor of a
// seamless restart before its services are let through, a launcher shutting down): an error outside
// the account contract. The player is told the list could not be loaded, not that the server answered
// strangely: no server was asked.

const FAKE = '../testing/fakeBinding';

vi.mock('@wailsio/runtime', () => ({ Events: { On: vi.fn(() => () => {}) } }));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('./toasts', () => ({ toast: vi.fn() }));
vi.mock('../../../bindings/typhon/internal/social', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  Service: (await import(FAKE)).fakeBinding('social'),
}));

beforeEach(() => {
  resetBindings();
  vi.resetModules();
});

// A first read that fails is kept as state for the screens to show; a later one is said in a toast.
// Both word an error outside the contract the same way.
async function started() {
  const { toast } = await import('./toasts');
  const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});
  const social = await import('./social');
  await social.initSocial();
  quiet.mockRestore();
  vi.mocked(toast).mockClear();
  return { social, toast: vi.mocked(toast) };
}

const NOT_RUNNING = () => {
  throw new Error('social: service is not running');
};

describe('the friends list when Go answers outside the account contract', () => {
  it('keeps a failed first read for the screens, worded as the list not loading', async () => {
    answer('social.Friends', NOT_RUNNING);
    const { social, toast } = await started();
    const { get } = await import('svelte/store');
    const { accountErrorText } = await import('../services/accountMessages');

    expect(get(social.friendsLoaded)).toBe(false);
    expect(accountErrorText(get(social.friendsLoadError), msg('state.socialLoadFriendsFailed'))).toBe(msg('state.socialLoadFriendsFailed'));
    expect(toast).not.toHaveBeenCalled();
  });

  it('says a later failed read in the surface\'s own words', async () => {
    answer('social.Friends', () => ({ friends: [], incoming: [], outgoing: [] }));
    const { social, toast } = await started();
    answer('social.Friends', NOT_RUNNING);
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});
    await social.loadFriends();
    quiet.mockRestore();

    expect(toast).toHaveBeenCalledWith(msg('state.socialLoadFriendsFailed'), 'danger');
    expect(toast).not.toHaveBeenCalledWith(msg('state.accountErrorServerError'), 'danger');
  });

  it('still names a server error Go reports as one', async () => {
    answer('social.Friends', () => ({ friends: [], incoming: [], outgoing: [] }));
    const { social, toast } = await started();
    answer('social.Friends', () => {
      throw new Error('server_error');
    });
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});
    await social.loadFriends();
    quiet.mockRestore();

    expect(toast).toHaveBeenCalledWith(msg('state.accountErrorServerError'), 'danger');
  });
});
