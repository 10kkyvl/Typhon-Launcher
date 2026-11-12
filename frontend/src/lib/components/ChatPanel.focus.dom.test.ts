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
  uploadChatImage: vi.fn(),
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
import { messages as fetchMessages, uploadChatImage, type Attachment, type Message, type MessagePage } from '../services/messaging';
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

describe('ChatPanel: a Retry that fails again says so again', () => {
  // The notice holding `text`, its live region as it is now, and its Retry; after a press that fails
  // again, the words come into the region as a new node while the notice keeps its focused Retry.
  async function pressAndFail(text: string, keys: ReturnType<typeof user>) {
    const notice = screen.getByText(text).closest('.notice') as HTMLElement;
    const region = notice.querySelector('.body') as HTMLElement;
    const before = region.firstElementChild;
    const retry = within(notice).getByRole('button', { name: msg('social.chatRetry') });
    await keys.click(retry);
    await vi.waitFor(() => expect(retry.hasAttribute('aria-busy')).toBe(false));
    expect(notice.isConnected).toBe(true);
    expect(document.activeElement).toBe(retry);
    await vi.waitFor(() => expect(region.firstElementChild).not.toBe(before));
    expect(region.firstElementChild?.textContent).toContain(text);
  }

  it('the history', async () => {
    vi.mocked(fetchMessages).mockRejectedValue(new Error('offline'));
    chat.chatHistoryError.set(msg('social.chatHistoryError'));
    const keys = setup();

    await pressAndFail(msg('social.chatHistoryError'), keys);
  });

  it('the chat list', async () => {
    vi.mocked(chat.retryMessaging).mockResolvedValue(undefined);
    chat.activePeer.set(null);
    chat.chatConversationsError.set(msg('social.chatConversationsError'));
    const keys = setup();

    await pressAndFail(msg('social.chatConversationsError'), keys);
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

describe('ChatPanel: the composer', () => {
  it('does not send on the Enter that ends a composition', async () => {
    setup();
    const field = screen.getByPlaceholderText(msg('social.chatPlaceholder'));
    await fireEvent.input(field, { target: { value: 'こんにちは' } });

    await fireEvent.keyDown(field, { key: 'Enter', isComposing: true });
    expect(chat.sendMessage).not.toHaveBeenCalled();

    await fireEvent.keyDown(field, { key: 'Enter' });
    expect(chat.sendMessage).toHaveBeenCalledOnce();
  });

  it('does not send on the Enter a WebKit view reports as part of a composition (key code 229)', async () => {
    setup();
    const field = screen.getByPlaceholderText(msg('social.chatPlaceholder'));
    await fireEvent.input(field, { target: { value: 'こんにちは' } });

    await fireEvent.keyDown(field, { key: 'Enter', keyCode: 229 });

    expect(chat.sendMessage).not.toHaveBeenCalled();
  });
});

describe('ChatPanel: keyboard paths that replace the pressed control', () => {
  it('the opener puts focus into the panel it opens', async () => {
    chat.chatOpen.set(false);
    chat.activePeer.set(null);
    const keys = setup();
    screen.getByRole('button', { name: msg('social.chatOpen') }).focus();

    await keys.keyboard('{Enter}');

    expect(document.activeElement).toBe(screen.getByRole('button', { name: /Friend/ }));
  });

  it('the panel\'s Close brings focus back to the opener', async () => {
    chat.activePeer.set(null);
    const keys = setup();
    screen.getByRole('button', { name: msg('social.chatClose') }).focus();
    await keys.keyboard('{Enter}');
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('social.chatOpen') }));
  });

  it('"Earlier" that loads the last older page hands focus to the first message', async () => {
    const older = { ...message('m0', 'c0'), createdAt: new Date(Date.now() - 3_600_000).toISOString() };
    vi.mocked(fetchMessages).mockResolvedValue({ messages: [older], next: '' } as unknown as MessagePage);
    chat.nextByPeer.set({ peer: 'cursor' });
    const keys = setup();
    screen.getByRole('button', { name: msg('social.chatEarlier') }).focus();

    await keys.keyboard('{Enter}');

    await vi.waitFor(() => expect(screen.queryByRole('button', { name: msg('social.chatEarlier') })).toBeNull());
    const first = document.querySelector('[data-client="c0"]') as HTMLElement;
    expect(first.contains(document.activeElement)).toBe(true);
  });

  it('the message field keeps focus while its message is sent: it refuses edits without turning disabled', async () => {
    vi.mocked(chat.sendMessage).mockReturnValue(new Promise(() => {}) as never);
    const keys = setup();
    const field = screen.getByPlaceholderText(msg('social.chatPlaceholder')) as HTMLTextAreaElement;

    await keys.type(field, 'Hi{Enter}');

    expect(chat.sendMessage).toHaveBeenCalledOnce();
    expect(field.disabled).toBe(false);
    expect(field.readOnly).toBe(true);
    expect(document.activeElement).toBe(field);
    await keys.type(field, '!');
    expect(field.value).toBe('Hi');
  });
});

describe('ChatPanel: a send in flight', () => {
  const uploaded = { id: 'a1', url: 'https://img/a1', width: 10, height: 10, bytes: 1024 };

  it('refuses a second Send while the first runs', async () => {
    vi.mocked(chat.sendMessage).mockReturnValue(new Promise(() => {}) as never);
    chat.attachmentDrafts.set({ peer: [{ key: 'k1', name: 'shot.png', bytes: 1024, encoded: '', previewUrl: 'blob:shot', uploaded }] });
    const keys = setup();
    const send = screen.getByRole('button', { name: msg('social.chatSend') });

    await keys.click(send);

    expect(send.hasAttribute('disabled')).toBe(true);
    await keys.click(send);
    expect(chat.sendMessage).toHaveBeenCalledTimes(1);
  });

  it('belongs to its conversation: after Back and reopening it, Send is still refused and the images go once', async () => {
    const sent = held<void>();
    vi.mocked(chat.sendMessage).mockReturnValue(sent.promise as never);
    chat.attachmentDrafts.set({ peer: [{ key: 'k1', name: 'shot.png', bytes: 1024, encoded: '', previewUrl: 'blob:shot', uploaded }] });
    const keys = setup();

    await keys.click(screen.getByRole('button', { name: msg('social.chatSend') }));
    expect(chat.sendMessage).toHaveBeenCalledTimes(1);
    await keys.click(screen.getByRole('button', { name: msg('social.chatBack') }));
    await keys.click(screen.getByRole('button', { name: /Friend/ }));

    const send = screen.getByRole('button', { name: msg('social.chatSend') });
    expect(send.hasAttribute('disabled')).toBe(true);
    await keys.click(send);
    expect(chat.sendMessage).toHaveBeenCalledTimes(1);
    sent.resolve();
    await vi.waitFor(() => expect(screen.queryByRole('button', { name: msg('social.chatAttachRemove') })).toBeNull());
  });

  it('shows the upload\'s progress again when its conversation is reopened while the images go up', async () => {
    const up = held<Attachment>();
    vi.mocked(uploadChatImage).mockReturnValue(up.promise);
    chat.attachmentDrafts.set({ peer: [{ key: 'k1', name: 'shot.png', bytes: 1024, encoded: 'data', previewUrl: 'blob:shot' }] });
    const keys = setup();

    await keys.click(screen.getByRole('button', { name: msg('social.chatSend') }));
    expect(screen.getByText(msg('social.chatUploading'))).toBeTruthy();
    await keys.click(screen.getByRole('button', { name: msg('social.chatBack') }));
    await keys.click(screen.getByRole('button', { name: /Friend/ }));

    expect(screen.getByText(msg('social.chatUploading'))).toBeTruthy();
    expect(screen.getByRole('button', { name: msg('social.chatSend') }).hasAttribute('disabled')).toBe(true);
    up.resolve(uploaded);
    await vi.waitFor(() => expect(chat.sendMessage).toHaveBeenCalledTimes(1));
  });
});

describe('ChatPanel: the new message toast', () => {
  const toast = () => ({ id: 1, peer, message: message('m9', 'c9') });

  // jsdom has no Web Animations: the toast's fade is given one that never ends, as a slow fade would.
  beforeEach(() => {
    Object.defineProperty(Element.prototype, 'animate', { value: () => ({ cancel: () => {}, onfinish: null }), configurable: true, writable: true });
  });
  afterEach(() => {
    Reflect.deleteProperty(Element.prototype, 'animate');
  });

  it('is announced, and its own button hides it without opening the chat', async () => {
    chat.chatOpen.set(false);
    chat.activePeer.set(null);
    const keys = setup();
    const region = document.querySelector('.toast-announce') as HTMLElement;
    expect(region.getAttribute('role')).toBe('status');
    expect(region.textContent?.trim()).toBe('');

    chat.chatToast.set(toast());
    await vi.waitFor(() => expect(region.textContent).toContain(msg('social.chatToastAnnounce', { name: 'Friend', text: 'Hello' })));
    const hide = screen.getByRole('button', { name: msg('social.chatToastDismiss') });
    hide.focus();
    await keys.keyboard('{Enter}');

    expect(get(chat.chatToast)).toBeNull();
    expect(screen.queryByRole('region', { name: msg('social.chatPanelLabel') })).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole('button', { name: msg('social.chatOpen') }));
  });
});
