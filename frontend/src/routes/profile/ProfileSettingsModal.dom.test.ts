import { cleanup, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { AccountError, updateProfile, type CurrentUser } from '../../lib/services/account';
import { accountErrorText } from '../../lib/services/accountMessages';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser, savingProfile } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import ProfileSettingsModal from './ProfileSettingsModal.svelte';
import { player } from './profileFixtures';
import { pending } from './profileTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/backend', () => ({ inWails: false }));
vi.mock('../../../bindings/typhon/internal/account', async (original) => (await import('./profileTesting')).fakeService(await original(), 'account'));
vi.mock('../../../bindings/typhon/internal/profile', async (original) => (await import('./profileTesting')).fakeService(await original(), 'profile'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./profileTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/account', async (original) => (await import('./profileTesting')).accountService(await original()));
vi.mock('../../lib/services/profile', async (original) => (await import('./profileTesting')).profileService(await original()));
vi.mock('../../lib/services/profileCover', async (original) => (await import('./profileTesting')).coverService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./profileTesting')).messagingStore(await original()));

// The profile's privacy dialog on its own: its headings, its Save waiting for the answer, a failure
// said inside it, and the connection gone.

const me: CurrentUser = player({ showOnline: true });
const dialog = () => screen.getByRole('dialog', { name: msg('social.profileSettingsTitle') });
const inDialog = (name: string) => within(dialog()).getByRole('button', { name });

function open() {
  return renderDom(ProfileSettingsModal, { open: true, settings: me.profile });
}

beforeEach(() => {
  locale.set('en');
  resetBindings();
  authState.set('authenticated');
  currentUser.set(me);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  vi.mocked(updateProfile).mockReset();
  currentUser.set(null);
  authState.set('bootstrapping');
  savingProfile.set(false);
  toasts.set([]);
  document.body.replaceChildren();
});

describe('the privacy dialog', () => {
  it('is named by its title, and its three groups are headings under it, in order', () => {
    open();

    const title = within(dialog()).getByRole('heading', { name: msg('social.profileSettingsTitle') });
    const groups = within(dialog()).getAllByRole('heading').filter((heading) => heading !== title);
    expect(title.tagName).toBe('H3');
    expect(groups.map((heading) => [heading.tagName, heading.textContent?.trim()])).toEqual([
      ['H4', msg('social.whatOthersSee')],
      ['H4', msg('social.feedPrivacyTitle')],
      ['H4', msg('profile.dangerZoneTitle')],
    ]);
  });

  it('Save stays focused and busy until the answer and sends once; then the dialog closes and says so', async () => {
    const answer = pending<CurrentUser>();
    vi.mocked(updateProfile).mockReturnValue(answer.promise);
    const { user } = open();
    await user.click(within(dialog()).getByRole('switch', { name: msg('social.flagOnlineLabel') }));
    const save = inDialog(msg('common.save'));

    await user.click(save);
    await user.click(save);

    expect(document.activeElement).toBe(save);
    expect(save.getAttribute('aria-busy')).toBe('true');
    expect(updateProfile).toHaveBeenCalledTimes(1);
    expect(vi.mocked(updateProfile).mock.calls[0][0].profile?.showOnline).toBe(false);

    answer.resolve(me);

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(get(toasts).map((toast) => toast.message)).toContain(msg('social.settingsSaved'));
  });

  it('a failed save is said in the dialog in catalogued words, the dialog stays and Save keeps focus', async () => {
    vi.mocked(updateProfile).mockRejectedValue(new AccountError('network'));
    const { user } = open();
    const save = inDialog(msg('common.save'));

    await user.click(save);

    const alert = await within(dialog()).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(accountErrorText(new AccountError('network'), msg('social.saveFailed')));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(document.activeElement).toBe(save);
  });

  it('offline, Save is refused in the Tab order and the dialog says why; nothing is sent', async () => {
    authState.set('offline');
    const { user } = open();
    const save = inDialog(msg('common.save'));

    const said = within(dialog()).getAllByText(msg('social.settingsRequireConnection'));
    expect(said.some((text) => text.closest('[data-tone="warning"]'))).toBe(true);
    expect(save.getAttribute('aria-disabled')).toBe('true');
    expect(description(save)).toContain(msg('social.settingsRequireConnection'));
    save.focus();
    expect(document.activeElement).toBe(save);

    await user.click(save);

    expect(updateProfile).not.toHaveBeenCalled();
  });

  it('Cancel closes without sending', async () => {
    const { user } = open();

    await user.click(inDialog(msg('common.cancel')));

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(updateProfile).not.toHaveBeenCalled();
  });
});
