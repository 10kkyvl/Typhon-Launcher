import { screen, within } from '@testing-library/svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { msg } from '../../lib/i18n';
import { AccountError } from '../../lib/services/account';
import {
  accept,
  decline,
  profile as fetchProfile,
  refresh,
  sendRequest,
  unfriend,
  type PublicProfile,
  type SendResult,
} from '../../lib/services/social';
import { resetHistory } from '../../lib/stores/router';
import { toasts } from '../../lib/stores/toasts';
import { authState } from '../../lib/stores/user';
import { renderDom } from '../../lib/testing/dom';
import { calls, resetBindings } from '../../lib/testing/fakeBinding';
import { description } from '../../lib/testing/hints';
import UserProfile from './UserProfile.svelte';
import { player } from './userTesting';

vi.mock('@wailsio/runtime', async () => (await import('../../lib/testing/dom')).wailsRuntime());
vi.mock('../../../bindings/typhon/internal/social', async (original) => (await import('./userTesting')).fakeService(await original(), 'social'));
vi.mock('../../../bindings/typhon/internal/messaging', async (original) => (await import('./userTesting')).fakeService(await original(), 'messaging'));
vi.mock('../../lib/services/social', async (original) => (await import('./userTesting')).socialService(await original()));
vi.mock('../../lib/stores/messaging', async (original) => (await import('./userTesting')).messagingStore(await original()));

const button = (name: string) => screen.getByRole('button', { name });

function pending<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => (resolve = res));
  return { promise, resolve };
}

async function mount(first: PublicProfile, ...after: PublicProfile[]) {
  const mock = vi.mocked(fetchProfile);
  for (const next of [first, ...after]) mock.mockResolvedValueOnce(next);
  mock.mockResolvedValue(after.at(-1) ?? first);
  const rendered = renderDom(UserProfile, { username: first.username });
  await screen.findByText(`@${first.username}`, { selector: '.username' });
  return rendered;
}

async function press(user: ReturnType<typeof renderDom>['user'], name: string) {
  const control = button(name);
  control.focus();
  await user.keyboard('{Enter}');
  return control;
}

beforeEach(() => {
  authState.set('authenticated');
  resetBindings();
  toasts.set([]);
  vi.mocked(refresh).mockResolvedValue(undefined);
});

afterEach(() => {
  expect(calls).toEqual([]);
  for (const mock of [fetchProfile, refresh, sendRequest, accept, decline, unfriend]) vi.mocked(mock).mockReset();
  document.body.replaceChildren();
  authState.set('bootstrapping');
  resetHistory();
});

describe("a user's page: the pressed control", () => {
  it('Add stays focused and busy, not disabled, until the answer', async () => {
    vi.mocked(sendRequest).mockReturnValue(new Promise(() => {}));
    const { user } = await mount(player());

    const add = await press(user, msg('social.relationNone'));

    expect(add.getAttribute('aria-busy')).toBe('true');
    expect(add.hasAttribute('disabled')).toBe(false);
    expect(document.activeElement).toBe(add);
  });

  it('Accept is busy while Decline waits beside it, and a second press asks nothing more', async () => {
    const answer = pending<void>();
    vi.mocked(accept).mockReturnValue(answer.promise);
    const { user } = await mount(player({ relation: 'incoming' }));

    const acceptButton = await press(user, msg('social.relationIncoming'));
    await user.keyboard('{Enter}');

    expect(acceptButton.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(acceptButton);

    await user.click(button(msg('social.declineButton')));

    expect(vi.mocked(accept)).toHaveBeenCalledTimes(1);
    expect(vi.mocked(decline)).not.toHaveBeenCalled();
  });

  it('Cancel request pressed: Cancel is the busy one and keeps the focus', async () => {
    vi.mocked(decline).mockReturnValue(new Promise(() => {}));
    const { user } = await mount(player({ relation: 'outgoing' }));

    const cancel = await press(user, msg('social.cancelRequestButton'));

    expect(cancel.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(cancel);
  });

  it('Decline pressed: Decline is the busy one, Accept is not', async () => {
    vi.mocked(decline).mockReturnValue(new Promise(() => {}));
    const { user } = await mount(player({ relation: 'incoming' }));

    const declineButton = await press(user, msg('social.declineButton'));

    expect(declineButton.getAttribute('aria-busy')).toBe('true');
    expect(button(msg('social.relationIncoming')).hasAttribute('aria-busy')).toBe(false);
  });

  it('a failed Add leaves the focus on Add, no longer busy', async () => {
    vi.mocked(sendRequest).mockRejectedValue(new AccountError('request_blocked'));
    const { user } = await mount(player());

    const add = await press(user, msg('social.relationNone'));

    await vi.waitFor(() => expect(add.hasAttribute('aria-busy')).toBe(false));
    expect(document.activeElement).toBe(add);
  });
});

describe("a user's page: focus when the buttons change", () => {
  it('a sent request: focus lands on Cancel request', async () => {
    vi.mocked(sendRequest).mockResolvedValue({ accepted: false } as SendResult);
    const { user } = await mount(player(), player({ relation: 'outgoing' }));

    await press(user, msg('social.relationNone'));

    await vi.waitFor(() => expect(document.activeElement).toBe(button(msg('social.cancelRequestButton'))));
  });

  it('an accepted request: focus lands on Write', async () => {
    vi.mocked(accept).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'incoming' }), player({ relation: 'friend' }));

    await press(user, msg('social.relationIncoming'));

    await vi.waitFor(() => expect(document.activeElement).toBe(button(msg('social.chatWrite'))));
  });

  it('a declined request: focus lands on Add', async () => {
    vi.mocked(decline).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'incoming' }), player());

    await press(user, msg('social.declineButton'));

    await vi.waitFor(() => expect(document.activeElement).toBe(button(msg('social.relationNone'))));
  });

  it('a cancelled request: focus lands on Add', async () => {
    vi.mocked(decline).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'outgoing' }), player());

    await press(user, msg('social.cancelRequestButton'));

    await vi.waitFor(() => expect(document.activeElement).toBe(button(msg('social.relationNone'))));
  });

  it('the connection drops while Cancel runs: Cancel keeps the focus and says why, then focus lands on Add, which says it too', async () => {
    const answer = pending<void>();
    vi.mocked(decline).mockReturnValue(answer.promise);
    const { user } = await mount(player({ relation: 'outgoing' }), player());
    const cancel = await press(user, msg('social.cancelRequestButton'));

    authState.set('offline');
    await vi.waitFor(() => expect(description(cancel)).toBe(msg('social.userActionsNeedConnection')));
    expect(cancel.hasAttribute('disabled')).toBe(false);
    expect(cancel.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(cancel);
    answer.resolve(undefined);

    const add = await screen.findByRole('button', { name: msg('social.relationNone') });
    await vi.waitFor(() => expect(document.activeElement).toBe(add));
    expect(add.getAttribute('aria-disabled')).toBe('true');
    expect(description(add)).toBe(msg('social.userActionsNeedConnection'));
  });

  it('a confirmed unfriend: the dialog closes and focus lands on Add', async () => {
    vi.mocked(unfriend).mockResolvedValue(undefined);
    const { user } = await mount(player({ relation: 'friend' }), player());

    await press(user, msg('social.moreLabel'));
    await user.click(screen.getByRole('menuitem', { name: msg('social.unfriendLabel') }));
    const dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('social.unfriendLabel') }));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    await vi.waitFor(() => expect(document.activeElement).toBe(button(msg('social.relationNone'))));
  });

  it('an unfriend that failed and was cancelled gives the focus back to the menu button', async () => {
    vi.mocked(unfriend).mockRejectedValue(new AccountError('not_friends'));
    const { user } = await mount(player({ relation: 'friend' }));

    const more = await press(user, msg('social.moreLabel'));
    await user.click(screen.getByRole('menuitem', { name: msg('social.unfriendLabel') }));
    const dialog = screen.getByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: msg('social.unfriendLabel') }));
    await within(dialog).findByRole('button', { name: msg('common.retry') });
    await user.click(within(dialog).getByRole('button', { name: msg('common.cancel') }));

    await vi.waitFor(() => expect(dialog.isConnected).toBe(false));
    expect(document.activeElement).toBe(more);
  });
});
