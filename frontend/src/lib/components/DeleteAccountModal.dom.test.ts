import { render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../i18n';
import { AccountError, deleteAccount, fetchCurrentUser, type CurrentUser } from '../services/account';
import { toasts } from '../stores/toasts';
import { authState, authView, currentUser } from '../stores/user';
import { user } from '../testing/dom';
import { resetBindings } from '../testing/fakeBinding';
import { held, sentToGo } from '../testing/gameDialogsTesting';
import ProfileSettingsModal from '../../routes/profile/ProfileSettingsModal.svelte';
import { player } from '../../routes/profile/profileFixtures';
import DeleteAccountModal from './DeleteAccountModal.svelte';

// Deleting an account is for good: the service call is a mock, and the recording runtime under it
// fails the test if anything else would have reached Go.
vi.mock('@wailsio/runtime', async () => (await import('../testing/gameDialogsTesting')).goRuntime());
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/account', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../services/account')>()),
  deleteAccount: vi.fn(),
  fetchCurrentUser: vi.fn(),
}));

const ANA = { id: 'u-ana', username: 'ana', displayName: 'Ana' } as CurrentUser;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
  currentUser.set(ANA);
  authState.set('authenticated');
});

afterEach(() => {
  const sent = sentToGo();
  resetBindings();
  vi.mocked(deleteAccount).mockReset();
  vi.mocked(fetchCurrentUser).mockReset();
  currentUser.set(null);
  authState.set('bootstrapping');
  authView.set('login');
  toasts.set([]);
  document.body.replaceChildren();
  expect(sent).toEqual([]);
});

const said = () => get(toasts).map((toast) => [toast.message, toast.kind]);

const describedBy = (el: Element) =>
  (el.getAttribute('aria-describedby') ?? '')
    .split(' ')
    .filter(Boolean)
    .map((id) => document.getElementById(id)?.textContent?.trim());

function openDialog(onclose = vi.fn()) {
  const view = render(DeleteAccountModal, { target: app, props: { onclose } });
  const dialog = screen.getByRole('dialog', { name: msg('profile.deleteAccountConfirmTitle') });
  const password = within(dialog).getByLabelText(msg('profile.deleteAccountPassword')) as HTMLInputElement;
  const remove = within(dialog).getByRole('button', { name: msg('profile.deleteAccountConfirmButton') });
  const cancel = within(dialog).getByRole('button', { name: msg('common.cancel') });
  return { ...view, dialog, password, remove, cancel, onclose, keys: user() };
}

async function pressDelete(answer: Promise<void>) {
  vi.mocked(deleteAccount).mockReturnValueOnce(answer);
  const opened = openDialog();
  await opened.keys.type(opened.password, 'secret');
  opened.remove.focus();
  await opened.keys.keyboard('{Enter}');
  return opened;
}

describe('DeleteAccountModal: what it does today', () => {
  it('opens on the password field, never on Delete', async () => {
    const { password } = openDialog();
    await tick();
    expect(document.activeElement).toBe(password);
  });

  it('keeps the words that say what is deleted', () => {
    const { dialog } = openDialog();
    expect(within(dialog).getByText(msg('profile.deleteAccountConfirmText'))).toBeTruthy();
  });

  it('sends one deletion per press, with the typed password, while the press runs', async () => {
    const deletion = held<void>();
    const { remove, password, keys } = await pressDelete(deletion.promise);

    expect(remove.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(remove);
    await keys.keyboard('{Enter}');
    await keys.click(remove);
    // Enter in the field submits the form even while the field is read-only.
    password.focus();
    await keys.keyboard('{Enter}');

    expect(vi.mocked(deleteAccount).mock.calls).toEqual([['secret']]);
    deletion.resolve();
  });

  it('lets nothing close it while the deletion runs: Escape and the close button do nothing', async () => {
    const deletion = held<void>();
    const { dialog, keys, onclose } = await pressDelete(deletion.promise);

    await keys.keyboard('{Escape}');
    await keys.click(within(dialog).getByRole('button', { name: msg('common.close') }));

    expect(onclose).not.toHaveBeenCalled();
    expect(dialog.isConnected).toBe(true);
    deletion.reject(new AccountError('invalid_credentials'));
    expect((await within(dialog).findByRole('alert')).textContent?.trim()).toBe(msg('profile.deleteAccountWrongPassword'));
    expect(onclose).not.toHaveBeenCalled();
  });

  it('says a wrong password by its field, marked wrong', async () => {
    const deletion = held<void>();
    const { dialog, password } = await pressDelete(deletion.promise);

    deletion.reject(new AccountError('invalid_credentials'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('profile.deleteAccountWrongPassword'));
    expect(password.getAttribute('aria-invalid')).toBe('true');
    expect(describedBy(password)).toContain(msg('profile.deleteAccountWrongPassword'));
  });

  it('says another refusal by its code in a danger notice, the field not marked', async () => {
    const deletion = held<void>();
    const { dialog, password } = await pressDelete(deletion.promise);

    deletion.reject(new AccountError('network_error'));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('state.accountErrorNetwork'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
    expect(password.getAttribute('aria-invalid')).not.toBe('true');
  });

  it('after a deletion: one success toast, the sign-in screen, and the dialog closes', async () => {
    const deletion = held<void>();
    const { onclose } = await pressDelete(deletion.promise);

    deletion.resolve();

    await vi.waitFor(() => expect(onclose).toHaveBeenCalledTimes(1));
    expect(said()).toEqual([[msg('profile.deleteAccountDone'), 'success']]);
    expect([get(authState), get(authView), get(currentUser)]).toEqual(['unauthenticated', 'login', null]);
  });

  it('a refusal after which the account is gone: said in a toast, and the dialog closes', async () => {
    vi.mocked(fetchCurrentUser).mockRejectedValueOnce(new AccountError('unauthenticated'));
    const deletion = held<void>();
    const { onclose } = await pressDelete(deletion.promise);

    deletion.reject(new AccountError('server_error'));

    await vi.waitFor(() => expect(onclose).toHaveBeenCalledTimes(1));
    expect(said()).toEqual([[msg('state.accountErrorServerError'), 'danger']]);
    expect(get(authState)).toBe('unauthenticated');
  });
});

describe('DeleteAccountModal on the kit', () => {
  it('tells a password manager to fill the current password', () => {
    const { password } = openDialog();
    expect(password.getAttribute('autocomplete')).toBe('current-password');
    expect(password.type).toBe('password');
  });

  it('holds Delete with a reason until a password is typed, in the Tab order', async () => {
    const { remove, password, keys } = openDialog();

    expect(remove).toHaveProperty('disabled', false);
    expect(remove.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(remove)).toContain(msg('profile.deleteAccountPasswordNeeded'));
    await keys.click(remove);
    expect(deleteAccount).not.toHaveBeenCalled();

    await keys.type(password, 's');
    expect(remove.getAttribute('aria-disabled')).toBeNull();
  });

  it('while the deletion runs: the field refuses edits and Cancel is held with a reason, nothing disabled under focus', async () => {
    const deletion = held<void>();
    const { password, cancel, keys, onclose } = await pressDelete(deletion.promise);

    expect(password.readOnly).toBe(true);
    expect(cancel).toHaveProperty('disabled', false);
    expect(cancel.getAttribute('aria-disabled')).toBe('true');
    expect(describedBy(cancel)).toContain(msg('modals.actionRunning'));
    await keys.click(cancel);
    expect(onclose).not.toHaveBeenCalled();
    deletion.resolve();
  });

  it('a wrong password: focus goes to the field, which takes edits and keeps the typed text', async () => {
    const deletion = held<void>();
    const { dialog, password, keys } = await pressDelete(deletion.promise);

    deletion.reject(new AccountError('invalid_credentials'));
    await within(dialog).findByRole('alert');
    await tick();

    expect(document.activeElement).toBe(password);
    expect(password.readOnly).toBe(false);
    expect(password.value).toBe('secret');
    await keys.type(password, '2');
    expect(password.value).toBe('secret2');
  });

  it('another refusal: focus goes to the notice that says it', async () => {
    const deletion = held<void>();
    const { dialog } = await pressDelete(deletion.promise);

    deletion.reject(new AccountError('rate_limited'));
    const alert = await within(dialog).findByRole('alert');
    await tick();

    expect(alert.closest('[data-tone="danger"]')?.contains(document.activeElement)).toBe(true);
  });
});

describe('DeleteAccountModal: an answer after the dialog is gone', () => {
  it('a deletion that lands says its toast once and calls nothing back', async () => {
    const deletion = held<void>();
    const { unmount, onclose } = await pressDelete(deletion.promise);
    unmount();

    deletion.resolve();
    await vi.waitFor(() => expect(get(authState)).toBe('unauthenticated'));
    await tick();

    expect(onclose).not.toHaveBeenCalled();
    expect(said()).toEqual([[msg('profile.deleteAccountDone'), 'success']]);
  });

  it('a refusal says once, in a toast naming the account that pressed, with its reason by code', async () => {
    const deletion = held<void>();
    const { unmount, onclose } = await pressDelete(deletion.promise);
    unmount();

    deletion.reject(new AccountError('invalid_credentials'));
    await vi.waitFor(() => expect(said()).toHaveLength(1));

    expect(onclose).not.toHaveBeenCalled();
    expect(said()).toEqual([[msg('profile.deleteAccountFailedFor', { name: 'ana', reason: msg('profile.deleteAccountWrongPassword') }), 'danger']]);
  });
});

describe('DeleteAccountModal from the profile settings', () => {
  it('Cancel gives focus back to "Delete account", which opened it', async () => {
    const me = player();
    currentUser.set(me);
    render(ProfileSettingsModal, { target: app, props: { open: true, settings: me.profile } });
    const keys = user();
    const settings = screen.getByRole('dialog', { name: msg('social.profileSettingsTitle') });
    const opener = within(settings).getByRole('button', { name: msg('profile.deleteAccountButton') });
    await keys.click(opener);
    const dialog = await screen.findByRole('dialog', { name: msg('profile.deleteAccountConfirmTitle') });

    await keys.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(document.activeElement).toBe(opener);
    expect(deleteAccount).not.toHaveBeenCalled();
  });
});
