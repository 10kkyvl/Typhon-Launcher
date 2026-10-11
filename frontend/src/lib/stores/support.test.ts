import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => vi.fn()) },
  Call: { ByID: vi.fn() },
  CancellablePromise: class {},
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/support', () => ({
  unreadTickets: vi.fn(),
}));
vi.mock('./toasts', () => ({ toast: vi.fn() }));

import { AccountError } from '../services/account';
import { unreadTickets } from '../services/support';
import { ru } from '../i18n/catalog/ru';
import { authState } from './user';
import { toast } from './toasts';
import { FOCUS_REFRESH_MS, initSupport, POLL_INTERVAL_MS, refreshUnread, unreadReplies } from './support';

const unread = vi.mocked(unreadTickets);

async function settle() {
  await vi.advanceTimersByTimeAsync(0);
}

let stop: () => void = () => {};

beforeEach(() => {
  vi.stubGlobal('window', new EventTarget());
  vi.useFakeTimers();
  unread.mockReset();
  unread.mockResolvedValue(0);
  vi.mocked(toast).mockClear();
  authState.set('unauthenticated');
});

afterEach(() => {
  stop();
  stop = () => {};
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function start() {
  stop = initSupport();
}

function messages(): string[] {
  return vi.mocked(toast).mock.calls.map((call) => call[0]);
}

describe('support unread polling', () => {
  it('never asks while the user is signed out', async () => {
    start();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 5);
    window.dispatchEvent(new Event('focus'));
    await refreshUnread();

    expect(unread).not.toHaveBeenCalled();
  });

  it('does not ask for a guest or an offline cached profile', async () => {
    start();
    authState.set('guest');
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 2);
    authState.set('offline');
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 2);

    expect(unread).not.toHaveBeenCalled();
  });

  it('asks as soon as the user signs in and then once a minute', async () => {
    start();
    authState.set('authenticated');
    await settle();
    expect(unread).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    expect(unread).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    expect(unread).toHaveBeenCalledTimes(3);
  });

  it('stops on sign-out and drops the badge', async () => {
    unread.mockResolvedValue(2);
    start();
    authState.set('authenticated');
    await settle();
    expect(get(unreadReplies)).toBe(2);

    authState.set('unauthenticated');
    expect(get(unreadReplies)).toBe(0);
    unread.mockClear();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);
    expect(unread).not.toHaveBeenCalled();
  });

  it('ignores an answer that arrives after the user signed out', async () => {
    let finish: (count: number) => void = () => {};
    unread.mockReturnValue(
      new Promise<number>((resolve) => {
        finish = resolve;
      }) as ReturnType<typeof unreadTickets>,
    );
    start();
    authState.set('authenticated');
    await settle();
    authState.set('unauthenticated');
    finish(4);
    await settle();

    expect(get(unreadReplies)).toBe(0);
    expect(messages()).toEqual([]);
  });

  it('asks again when the window gets focus, but only when signed in', async () => {
    start();
    authState.set('authenticated');
    await settle();
    unread.mockClear();
    await vi.advanceTimersByTimeAsync(FOCUS_REFRESH_MS);

    window.dispatchEvent(new Event('focus'));
    await settle();
    expect(unread).toHaveBeenCalledTimes(1);

    authState.set('unauthenticated');
    unread.mockClear();
    window.dispatchEvent(new Event('focus'));
    await settle();
    expect(unread).not.toHaveBeenCalled();
  });

  it('does not ask on every focus: not more often than the focus interval', async () => {
    start();
    authState.set('authenticated');
    await settle();
    unread.mockClear();

    for (let i = 0; i < 5; i++) {
      window.dispatchEvent(new Event('focus'));
      await vi.advanceTimersByTimeAsync(1000);
    }
    expect(unread).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(FOCUS_REFRESH_MS);
    window.dispatchEvent(new Event('focus'));
    await settle();
    expect(unread).toHaveBeenCalledTimes(1);

    window.dispatchEvent(new Event('focus'));
    await settle();
    expect(unread).toHaveBeenCalledTimes(1);
  });

  it('stops everything when the app tears the store down', async () => {
    start();
    authState.set('authenticated');
    await settle();
    stop();
    stop = () => {};
    unread.mockClear();

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3);
    window.dispatchEvent(new Event('focus'));
    await settle();

    expect(unread).not.toHaveBeenCalled();
  });
});

describe('support reply toast', () => {
  it('stays quiet about what was already waiting at the first load', async () => {
    unread.mockResolvedValue(3);
    start();
    authState.set('authenticated');
    await settle();

    expect(get(unreadReplies)).toBe(3);
    expect(messages()).toEqual([]);
  });

  it('raises a toast when the count goes up', async () => {
    unread.mockResolvedValue(1);
    start();
    authState.set('authenticated');
    await settle();

    unread.mockResolvedValue(2);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(get(unreadReplies)).toBe(2);
    expect(messages()).toEqual([ru['tickets.toastReply']]);
  });

  it('does not raise a toast when the count goes down or stays', async () => {
    unread.mockResolvedValue(3);
    start();
    authState.set('authenticated');
    await settle();

    unread.mockResolvedValue(3);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    unread.mockResolvedValue(1);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(get(unreadReplies)).toBe(1);
    expect(messages()).toEqual([]);
  });

  it('treats a new session as a first load again', async () => {
    unread.mockResolvedValue(1);
    start();
    authState.set('authenticated');
    await settle();
    authState.set('unauthenticated');

    unread.mockResolvedValue(5);
    authState.set('authenticated');
    await settle();

    expect(get(unreadReplies)).toBe(5);
    expect(messages()).toEqual([]);
  });
});

describe('support polling failures', () => {
  it('tells the user once per outage and keeps the last count', async () => {
    unread.mockResolvedValue(2);
    start();
    authState.set('authenticated');
    await settle();

    unread.mockRejectedValue(new AccountError('network_error'));
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(messages()).toEqual([ru['state.accountErrorNetwork']]);
    expect(get(unreadReplies)).toBe(2);

    unread.mockResolvedValue(2);
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
    unread.mockRejectedValue(new AccountError('network_error'));
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);

    expect(messages()).toHaveLength(2);
  });

  it('stays quiet about a failure of a request that a newer one has already replaced', async () => {
    authState.set('authenticated');
    await settle();
    unread.mockReset();
    let failOld: (error: Error) => void = () => {};
    unread.mockReturnValueOnce(new Promise<number>((_resolve, reject) => (failOld = reject)));
    unread.mockResolvedValueOnce(3);

    const old = refreshUnread();
    await refreshUnread();
    failOld(new AccountError('network_error'));
    await old;

    expect(messages()).toEqual([]);
    expect(get(unreadReplies)).toBe(3);
  });

  it('does not nag about an expired session, the auth store handles it', async () => {
    unread.mockRejectedValue(new AccountError('unauthenticated'));
    start();
    authState.set('authenticated');
    await settle();

    expect(messages()).toEqual([]);
  });

  it('falls back to the polling message for an unknown failure', async () => {
    unread.mockRejectedValue(new Error('boom'));
    start();
    authState.set('authenticated');
    await settle();

    expect(messages()).toEqual([ru['tickets.pollFailed']]);
  });
});
