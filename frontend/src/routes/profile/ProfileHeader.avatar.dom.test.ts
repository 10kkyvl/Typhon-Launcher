import { cleanup, screen, waitFor } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { locale, msg } from '../../lib/i18n';
import { pickAvatar, type AvatarImage } from '../../lib/services/account';
import { EMPTY_SNAPSHOT, getProfileSnapshot } from '../../lib/services/profile';
import { profileSnapshot } from '../../lib/stores/profile';
import { navigate, resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState, currentUser, pickingAvatar } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { description } from '../../lib/testing/hints';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import Profile from './Profile.svelte';
import { player, SNAPSHOT } from './profileFixtures';
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

// The header's avatar buttons: "Change avatar" busy while the system picker is open, and both refused
// in the Tab order with the header Save's words when the connection goes, never natively disabled
// (Chromium drops a focus it holds to <body>; jsdom keeps it, so the attributes are read).

const button = (name: string) => screen.getByRole('button', { name });
const refusal = (control: HTMLElement) => ({ ariaDisabled: control.getAttribute('aria-disabled'), disabled: control.matches(':disabled'), reason: description(control) });

beforeEach(() => {
  locale.set('en');
  resetBindings();
  navigate('profile');
  authState.set('authenticated');
  currentUser.set(player());
  vi.mocked(getProfileSnapshot).mockResolvedValue(SNAPSHOT);
});

afterEach(() => {
  cleanup();
  expect(calls).toEqual([]);
  vi.mocked(getProfileSnapshot).mockReset();
  vi.mocked(pickAvatar).mockReset();
  // A picker a failed test left open would hold the next test's button.
  pickingAvatar.set(false);
  resetHistory();
  currentUser.set(null);
  authState.set('bootstrapping');
  profileSnapshot.set(EMPTY_SNAPSHOT);
  toasts.set([]);
  document.body.replaceChildren();
});

describe('"Change avatar" when the connection goes', () => {
  it('keeps the focus it holds, refused with its reason, and a press opens no picker', async () => {
    const { user } = renderDom(Profile);
    const change = screen.getByRole('button', { name: msg('ui.avatarChange') });
    change.focus();

    authState.set('offline');

    await waitFor(() => expect(change.getAttribute('aria-disabled')).toBe('true'));
    expect(change.matches(':disabled')).toBe(false);
    expect(document.activeElement).toBe(change);
    expect(description(change)).toBe(msg('social.editRequiresConnection'));
    await user.keyboard('{Enter}');
    await user.click(change);
    expect(pickAvatar).not.toHaveBeenCalled();
  });
});

describe('"Change avatar" while the system picker is open', () => {
  it('keeps the focus it was pressed with, busy, and a second press opens no second picker', async () => {
    const picked = pending<AvatarImage>();
    vi.mocked(pickAvatar).mockReturnValue(picked.promise);
    const { user } = renderDom(Profile);
    const change = button(msg('ui.avatarChange'));

    await user.click(change);
    await user.click(change);

    expect(document.activeElement).toBe(change);
    expect({ busy: change.getAttribute('aria-busy'), disabled: change.matches(':disabled') }).toEqual({ busy: 'true', disabled: false });
    expect(pickAvatar).toHaveBeenCalledTimes(1);
    picked.resolve({ data: '', mime: '' } as AvatarImage);
    await waitFor(() => expect(change.getAttribute('aria-busy')).toBeNull());
    expect(document.activeElement).toBe(change);
  });
});

describe('"Remove avatar" when the connection goes', () => {
  it('with an avatar, keeps the focus it holds, refused with its reason, and asks nothing', async () => {
    currentUser.set(player({}, { avatarUrl: 'https://cdn.example/a.png' }));
    const { user } = renderDom(Profile);
    const remove = button(msg('ui.avatarRemove'));
    remove.focus();

    authState.set('offline');

    await waitFor(() => expect(remove.getAttribute('aria-disabled')).toBe('true'));
    expect(refusal(remove)).toEqual({ ariaDisabled: 'true', disabled: false, reason: msg('social.editRequiresConnection') });
    expect(document.activeElement).toBe(remove);
    await user.keyboard('{Enter}');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('with no avatar, stays plainly disabled: there is nothing to remove', () => {
    renderDom(Profile);
    authState.set('offline');

    expect(button(msg('ui.avatarRemove')).matches(':disabled')).toBe(true);
  });

  it('online, with an avatar, both buttons are free and say no reason', () => {
    currentUser.set(player({}, { avatarUrl: 'https://cdn.example/a.png' }));
    renderDom(Profile);

    expect([refusal(button(msg('ui.avatarChange'))), refusal(button(msg('ui.avatarRemove')))]).toEqual([
      { ariaDisabled: null, disabled: false, reason: '' },
      { ariaDisabled: null, disabled: false, reason: '' },
    ]);
  });
});
