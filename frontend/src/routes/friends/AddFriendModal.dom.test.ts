import { render, screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { profile, sendRequest, type PublicProfile, type SendResult } from '../../lib/services/social';
import { toasts } from '../../lib/stores/toasts';
import { user } from '../../lib/testing/dom';
import AddFriendModal from './AddFriendModal.svelte';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../lib/services/social', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/services/social')>()),
  profile: vi.fn(),
  profileByCode: vi.fn(),
  sendRequest: vi.fn(),
}));

const lena = { username: 'lena', displayName: 'Lena', avatarUrl: '', relation: 'none', mutualCount: 0 } as unknown as PublicProfile;

let app: HTMLDivElement;

beforeEach(() => {
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  toasts.set([]);
  vi.clearAllMocks();
  document.body.replaceChildren();
});

async function search(result: () => Promise<PublicProfile>) {
  vi.mocked(profile).mockImplementationOnce(result);
  render(AddFriendModal, { target: app, props: { open: true } });
  const dialog = screen.getByRole('dialog', { name: msg('social.friendsAddFriend') });
  const keys = user();
  await keys.type(within(dialog).getByRole('textbox', { name: msg('social.friendsSearchLabel') }), 'lena{Enter}');
  return { dialog, keys };
}

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
