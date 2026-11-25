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

async function start() {
  const { toast } = await import('./toasts');
  vi.mocked(toast).mockClear();
  const quiet = vi.spyOn(console, 'error').mockImplementation(() => {});
  const social = await import('./social');
  await social.initSocial();
  quiet.mockRestore();
  return vi.mocked(toast);
}

describe('the friends list when Go answers outside the account contract', () => {
  it('says the list could not be loaded, in the surface\'s own words', async () => {
    answer('social.Friends', () => {
      throw new Error('social: service is not running');
    });

    const toast = await start();

    expect(toast).toHaveBeenCalledWith(msg('state.socialLoadFriendsFailed'), 'danger');
    expect(toast).not.toHaveBeenCalledWith(msg('state.accountErrorServerError'), 'danger');
  });

  it('still names a server error Go reports as one', async () => {
    answer('social.Friends', () => {
      throw new Error('server_error');
    });

    const toast = await start();

    expect(toast).toHaveBeenCalledWith(msg('state.accountErrorServerError'), 'danger');
  });
});
