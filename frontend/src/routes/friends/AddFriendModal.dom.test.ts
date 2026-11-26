import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
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

describe('AddFriendModal: a send that fails after the player typed on', () => {
  it('is not said under the player found since, whose request can still be sent', async () => {
    const mira = { ...lena, username: 'mira', displayName: 'Mira' } as PublicProfile;
    let failed!: (err: unknown) => void;
    vi.mocked(sendRequest).mockReturnValueOnce(new Promise<SendResult>((_resolve, reject) => (failed = reject)));
    const { dialog, keys } = await search(async () => lena);
    await keys.click(await within(dialog).findByRole('button', { name: msg('social.friendsSendRequest') }));
    vi.mocked(profile).mockImplementationOnce(async () => mira);
    const field = within(dialog).getByRole('textbox', { name: msg('social.friendsSearchLabel') });

    await keys.clear(field);
    await keys.type(field, 'mira');
    expect(await within(dialog).findByText('@mira')).toBeTruthy();
    failed(new Error('server said no'));
    await vi.waitFor(() => expect(within(dialog).getByRole('button', { name: msg('social.friendsSendRequest') }).getAttribute('aria-busy')).not.toBe('true'));

    expect(within(dialog).queryByText(msg('social.friendsSendRequestFailed'))).toBeNull();
    expect(within(dialog).getByText('@mira')).toBeTruthy();
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
