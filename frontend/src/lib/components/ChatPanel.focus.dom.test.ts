import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { get, writable } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', () => ({ currentUser: writable({ id: 'me' }), authState: writable('authenticated') }));
vi.mock('../stores/social', () => ({ needsSocialConsent: writable(false) }));
vi.mock('../stores/presence', () => ({ presenceStatus: writable('online') }));
vi.mock('../services/messaging', async (actual) => ({
  ...(await actual<typeof import('../services/messaging')>()),
  messages: vi.fn(),
}));
// The calls that reach the backend; what the store does around them is its own tests'.
vi.mock('../stores/messaging', async (actual) => ({
  ...(await actual<typeof import('../stores/messaging')>()),
  retryMessaging: vi.fn(),
  retryMessage: vi.fn(),
  editMessage: vi.fn(),
  sendMessage: vi.fn(),
}));

import { locale, msg } from '../i18n';
import { messages as fetchMessages, type Message, type MessagePage } from '../services/messaging';
import * as chat from '../stores/messaging';
import { user } from '../testing/dom';
import ChatPanel from './ChatPanel.svelte';

const peer = { id: 'peer', username: 'friend', displayName: 'Friend', avatarUrl: '' };

function message(id: string, clientId: string, senderId = 'peer'): Message {
  return {
    id, clientId, senderId, recipientId: senderId === 'me' ? 'peer' : 'me', text: 'Hello', createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(), reactions: [],
  } as Message;
}

function held<T>() {
  let resolve: (value: T) => void = () => {};
  const promise = new Promise<T>((ok) => (resolve = ok));
  return { promise, resolve };
}

let app: HTMLDivElement;

beforeEach(() => {
  locale.set('en');
  vi.mocked(fetchMessages).mockReset();
  vi.mocked(chat.retryMessaging).mockReset();
  vi.mocked(chat.retryMessage).mockReset();
  vi.mocked(chat.editMessage).mockReset();
  vi.mocked(chat.sendMessage).mockReset();
  chat.chatOpen.set(true);
  chat.activePeer.set(peer);
  chat.chatConnected.set(true);
  chat.chatLoading.set(false);
  chat.chatLoadingMore.set(false);
  chat.chatHistoryError.set('');
  chat.chatConversationsError.set('');
  chat.chatToast.set(null);
  chat.nextByPeer.set({});
  chat.failedMessages.set(new Set());
  chat.pendingMessages.set(new Set());
  chat.attachmentDrafts.set({});
  chat.editingMessageId.set(null);
  chat.conversations.set([{ peer, unread: 0, canSend: true }]);
  chat.messagesByPeer.set({ peer: [message('m1', 'c1')] });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  chat.messagesByPeer.set({});
  chat.activePeer.set(null);
  chat.chatOpen.set(false);
  chat.chatToast.set(null);
  document.body.replaceChildren();
});

function setup() {
  render(ChatPanel, { target: app });
  return user();
}

describe('ChatPanel: the history\'s Retry', () => {
  it('asks for the newest messages again when they failed, even with older ones still to load', async () => {
    const page = held<MessagePage>();
    vi.mocked(fetchMessages).mockReturnValue(page.promise);
    chat.nextByPeer.set({ peer: 'cursor' });
    chat.chatHistoryError.set('Could not load history.');
    const keys = setup();

    await keys.click(within(screen.getByRole('alert').closest('.notice') as HTMLElement).getByRole('button', { name: msg('social.chatRetry') }));

    expect(vi.mocked(fetchMessages).mock.calls).toEqual([['peer', '']]);
    page.resolve({ messages: [], next: 'cursor' } as unknown as MessagePage);
  });

  it('asks for the older page again when that page failed, and moves focus into the history on success', async () => {
    const again = held<MessagePage>();
    vi.mocked(fetchMessages).mockRejectedValueOnce(new Error('offline')).mockReturnValueOnce(again.promise);
    chat.nextByPeer.set({ peer: 'cursor' });
    const keys = setup();
    await keys.click(screen.getByRole('button', { name: msg('social.chatEarlier') }));
    const notice = (await screen.findByRole('alert')).closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('social.chatRetry') });

    await keys.click(retry);

    expect(vi.mocked(fetchMessages).mock.calls).toEqual([['peer', 'cursor'], ['peer', 'cursor']]);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    again.resolve({ messages: [message('m0', 'c0')], next: 'older' } as unknown as MessagePage);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('social.chatEarlier') }));
  });
});

describe('ChatPanel: the conversation list\'s Retry', () => {
  it('stays busy with the focus while the chats reload, then moves focus to the first conversation', async () => {
    const done = held<void>();
    vi.mocked(chat.retryMessaging).mockReturnValue(done.promise);
    chat.activePeer.set(null);
    chat.chatConversationsError.set('Could not load chats.');
    const keys = setup();
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    const retry = within(notice).getByRole('button', { name: msg('social.chatRetry') });

    await keys.click(retry);

    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    chat.chatConversationsError.set('');
    done.resolve();
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
    expect(document.activeElement).toBe(screen.getByRole('button', { name: /Friend/ }));
  });
});

describe('ChatPanel: Reconnect', () => {
  it('stays busy with the focus while the connection is tried', async () => {
    const done = held<void>();
    vi.mocked(chat.retryMessaging).mockReturnValue(done.promise);
    chat.chatConnected.set(false);
    const keys = setup();
    const reconnect = screen.getByRole('button', { name: msg('social.chatReconnect') });

    await keys.click(reconnect);

    expect(chat.retryMessaging).toHaveBeenCalledOnce();
    expect(reconnect.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(reconnect);
    done.resolve();
    await vi.waitFor(() => expect(reconnect.hasAttribute('aria-busy')).toBe(false));
  });
});

describe('ChatPanel: focus stays with the user', () => {
  it('Edit puts focus in a named editor; Cancel and Save bring it back to the message\'s Edit', async () => {
    chat.messagesByPeer.set({ peer: [message('m-own', 'c-own', 'me')] });
    vi.mocked(chat.editMessage).mockImplementation(async () => chat.editingMessageId.set(null));
    const keys = setup();
    screen.getByRole('button', { name: msg('social.chatEdit') }).focus();

    await keys.keyboard('{Enter}');
    const editor = screen.getByRole('textbox', { name: msg('social.chatEditField') });
    expect(document.activeElement).toBe(editor);
    await keys.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('social.chatCancel') }));
    await keys.keyboard('{Enter}');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('social.chatEdit') }));

    await keys.keyboard('{Enter}');
    await keys.type(screen.getByRole('textbox', { name: msg('social.chatEditField') }), ' again');
    await keys.tab();
    await keys.tab();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('social.chatSave') }));
    await keys.keyboard('{Enter}');
    expect(chat.editMessage).toHaveBeenCalledWith('peer', 'm-own', 'Hello again');
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('social.chatEdit') })));
  });

  it('Retry on a failed message stays busy with the focus, then hands it to the sent message', async () => {
    const sent = held<Message>();
    chat.messagesByPeer.set({ peer: [message('local:c-f', 'c-f', 'me')] });
    chat.failedMessages.set(new Set(['c-f']));
    vi.mocked(chat.retryMessage).mockImplementation(() => {
      chat.failedMessages.set(new Set());
      chat.pendingMessages.set(new Set(['c-f']));
      return sent.promise;
    });
    const keys = setup();
    const retry = screen.getByRole('button', { name: msg('social.chatRetry') });
    retry.focus();

    await keys.keyboard('{Enter}');

    expect(retry.isConnected).toBe(true);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    chat.pendingMessages.set(new Set());
    chat.messagesByPeer.set({ peer: [message('m-sent', 'c-f', 'me')] });
    sent.resolve(message('m-sent', 'c-f', 'me'));
    await vi.waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('social.chatEdit') })));
  });

  it('Back lands on the conversation it came from', async () => {
    const keys = setup();
    screen.getByRole('button', { name: msg('social.chatBack') }).focus();

    await keys.keyboard('{Enter}');

    expect(document.activeElement).toBe(screen.getByRole('button', { name: /Friend/ }));
  });

  it('a conversation opened from the list takes focus where its message is written', async () => {
    chat.activePeer.set(null);
    const keys = setup();
    screen.getByRole('button', { name: /Friend/ }).focus();

    await keys.keyboard('{Enter}');

    expect(document.activeElement).toBe(screen.getByPlaceholderText(msg('social.chatPlaceholder')));
  });

  it('Back from a conversation that is not in the list lands on the list\'s heading', async () => {
    chat.conversations.set([]);
    const keys = setup();
    screen.getByRole('button', { name: msg('social.chatBack') }).focus();

    await keys.keyboard('{Enter}');

    expect(document.activeElement).toBe(screen.getByRole('heading', { name: msg('social.chatMessages') }));
  });
});
