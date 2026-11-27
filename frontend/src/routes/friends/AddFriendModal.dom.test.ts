import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { msg } from '../../lib/i18n';
import { accountErrorText } from '../../lib/services/accountMessages';
import { profile, sendRequest, type PublicProfile, type SendResult } from '../../lib/services/social';
import { toasts } from '../../lib/stores/toasts';
import { user } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import AddFriendModal from './AddFriendModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/social', async (original) => (await import('./friendsTesting')).socialService(await original()));
vi.mock('../../lib/services/messaging', async (original) => (await import('./friendsTesting')).messagingService(await original()));
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./friendsTesting')).fakeService(await original(), 'messaging'));

const lena = { username: 'lena', displayName: 'Lena', avatarUrl: '', relation: 'none', mutualCount: 0 } as unknown as PublicProfile;

let app: HTMLDivElement;

beforeEach(() => {
  resetBindings();
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  expect(calls).toEqual([]);
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

async function search(result: () => Promise<PublicProfile>) {
  vi.mocked(profile).mockImplementationOnce(() => result());
  render(AddFriendModal, { target: app, props: { open: true } });
  const dialog = screen.getByRole('dialog', { name: msg('social.friendsAddFriend') });
  const keys = user();
  await keys.type(within(dialog).getByRole('textbox', { name: msg('social.friendsSearchLabel') }), 'lena{Enter}');
  return { dialog, keys };
}

describe('AddFriendModal: Enter while an input method composes', () => {
  it.each([
    ['isComposing', { isComposing: true }],
    ['key code 229 (WebKit after the composition)', { keyCode: 229 }],
  ])('commits the text and looks nothing up (%s)', async (_how, init) => {
    vi.useFakeTimers();
    try {
      render(AddFriendModal, { target: app, props: { open: true } });
      const field = screen.getByRole('textbox', { name: msg('social.friendsSearchLabel') });
      await fireEvent.input(field, { target: { value: 'lena' } });

      await fireEvent.keyDown(field, { key: 'Enter', ...init });
      expect(profile).not.toHaveBeenCalled();

      await fireEvent.keyDown(field, { key: 'Enter' });
      expect(vi.mocked(profile).mock.calls).toEqual([['lena']]);
    } finally {
      vi.clearAllTimers();
      vi.useRealTimers();
    }
  });
});

type Shown = { unmount: () => void; rerender: (props: { open: boolean }) => Promise<void> };

describe('AddFriendModal: a send that fails after the player typed on', () => {
  const mira = { ...lena, username: 'mira', displayName: 'Mira' } as PublicProfile;
  const toLena = msg('social.friendsSendRequestFailedTo', { name: 'Lena' });
  const sendButton = (dialog: HTMLElement) => within(dialog).getByRole('button', { name: msg('social.friendsSendRequest') });
  const dangerNotices = (dialog: HTMLElement) => dialog.querySelectorAll('.notice[data-tone="danger"]');

  /** Finds Lena, sends to her, looks Mira up while the send runs; `fail` then answers the send. */
  async function sendThenTypeOn() {
    let fail!: (err: unknown) => void;
    vi.mocked(sendRequest).mockReturnValueOnce(new Promise<SendResult>((_resolve, reject) => (fail = reject)));
    const { dialog, keys } = await search(async () => lena);
    await keys.click(await within(dialog).findByRole('button', { name: msg('social.friendsSendRequest') }));
    vi.mocked(profile).mockImplementationOnce(async () => mira);
    const field = within(dialog).getByRole('textbox', { name: msg('social.friendsSearchLabel') });
    await keys.clear(field);
    await keys.type(field, 'mira');
    expect(await within(dialog).findByText('@mira')).toBeTruthy();
    return { dialog, keys, field, fail };
  }

  async function failed(dialog: HTMLElement, fail: (err: unknown) => void, err: unknown) {
    fail(err);
    await vi.waitFor(() => expect(sendButton(dialog).getAttribute('aria-busy')).not.toBe('true'));
  }

  it('P9 is said once in the dialog’s notice with the name of the player it was sent to', async () => {
    const { dialog, fail } = await sendThenTypeOn();

    await failed(dialog, fail, new Error('server said no'));

    const alert = within(dialog).getByRole('alert');
    expect(alert.textContent?.trim()).toBe(toLena);
    expect(dangerNotices(dialog)).toHaveLength(1);
    expect(within(dialog).getByText('@mira')).toBeTruthy();
    expect(get(toasts)).toEqual([]);
  });

  it('a failure with a code of its own says the name, then the code’s words', async () => {
    const { AccountError } = await import('../../lib/services/account');
    const { dialog, fail } = await sendThenTypeOn();

    await failed(dialog, fail, new AccountError('rate_limited'));

    expect(within(dialog).getByRole('alert').textContent?.trim()).toBe(
      msg('social.friendsFailedWhy', { what: toLena, why: accountErrorText(new AccountError('rate_limited'), '') }),
    );
  });

  it('stays through a new lookup, the same notice, Lena found again or nobody', async () => {
    const { dialog, keys, field, fail } = await sendThenTypeOn();
    await failed(dialog, fail, new Error('server said no'));
    const notice = within(dialog).getByRole('alert').closest('.notice') as HTMLElement;

    vi.mocked(profile).mockImplementationOnce(async () => lena);
    await keys.clear(field);
    await keys.type(field, 'lena');
    expect(await within(dialog).findByText('@lena')).toBeTruthy();
    expect(notice.isConnected).toBe(true);
    expect(notice.textContent?.trim()).toBe(toLena);

    vi.mocked(profile).mockImplementationOnce(() => Promise.reject(new Error('offline')));
    await keys.clear(field);
    await keys.type(field, 'nobody{Enter}');
    expect(await within(dialog).findByText(msg('social.friendsFindUserFailed'))).toBeTruthy();
    expect(notice.isConnected).toBe(true);
    expect(dangerNotices(dialog)).toHaveLength(2);
  });

  it('goes when the player dismisses it', async () => {
    const { dialog, keys, fail } = await sendThenTypeOn();
    await failed(dialog, fail, new Error('server said no'));
    const notice = within(dialog).getByText(toLena).closest('.notice') as HTMLElement;

    await keys.click(within(notice).getByRole('button', { name: msg('common.close') }));

    expect(within(dialog).queryByText(toLena)).toBeNull();
    expect(within(dialog).getByText('@mira')).toBeTruthy();
  });

  it('goes when the player sends again', async () => {
    const { dialog, keys, fail } = await sendThenTypeOn();
    await failed(dialog, fail, new Error('server said no'));
    expect(within(dialog).getByText(toLena)).toBeTruthy();
    vi.mocked(sendRequest).mockReturnValueOnce(new Promise<SendResult>(() => {}));

    await keys.click(sendButton(dialog));

    expect(vi.mocked(sendRequest).mock.calls).toEqual([['lena'], ['mira']]);
    expect(within(dialog).queryByText(toLena)).toBeNull();
  });

  it('goes with the dialog: opened again, it starts clean', async () => {
    const { dialog, keys, fail } = await sendThenTypeOn();
    await failed(dialog, fail, new Error('server said no'));
    expect(within(dialog).getByText(toLena)).toBeTruthy();

    await keys.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));
    await vi.waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());

    expect(screen.queryByText(toLena)).toBeNull();
  });

  it.each([
    ['unmounted', (shown: Shown) => shown.unmount()],
    ['closed by its owner', (shown: Shown) => shown.rerender({ open: false })],
  ])('is one toast with the name when the dialog is gone before the answer (%s)', async (_how, gone) => {
    let fail!: (err: unknown) => void;
    vi.mocked(sendRequest).mockReturnValueOnce(new Promise<SendResult>((_resolve, reject) => (fail = reject)));
    vi.mocked(profile).mockImplementationOnce(async () => lena);
    const shown = render(AddFriendModal, { target: app, props: { open: true } });
    const dialog = screen.getByRole('dialog', { name: msg('social.friendsAddFriend') });
    const keys = user();
    await keys.type(within(dialog).getByRole('textbox', { name: msg('social.friendsSearchLabel') }), 'lena{Enter}');
    await keys.click(await within(dialog).findByRole('button', { name: msg('social.friendsSendRequest') }));

    await gone(shown);
    fail(new Error('server said no'));

    await vi.waitFor(() => expect(get(toasts)).toHaveLength(1));
    expect(get(toasts).map((t) => [t.message, t.kind])).toEqual([[toLena, 'danger']]);
  });
});

describe('AddFriendModal', () => {
  it('says a failed lookup in a danger notice', async () => {
    const { dialog } = await search(() => Promise.reject(new Error('offline')));

    const alert = await within(dialog).findByRole('alert');
    expect(alert.textContent?.trim()).toBe(msg('social.friendsFindUserFailed'));
    expect(alert.closest('[data-tone="danger"]')).not.toBeNull();
  });

  it('keeps the focus on "send request", busy, while it is sent', async () => {
    let sent!: (value: SendResult) => void;
    vi.mocked(sendRequest).mockReturnValueOnce(new Promise<SendResult>((resolve) => (sent = resolve)));
    const { dialog, keys } = await search(async () => lena);
    const send = await within(dialog).findByRole('button', { name: msg('social.friendsSendRequest') });
    send.focus();

    await keys.keyboard('{Enter}');

    expect(send.getAttribute('aria-busy')).toBe('true');
    expect(send).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(send);
    sent({ accepted: false } as SendResult);
  });
});
