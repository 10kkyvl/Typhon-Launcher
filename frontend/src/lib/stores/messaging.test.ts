import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get, writable } from 'svelte/store';

const handlers = new Map<string, (event: { data: unknown }) => void>();
const api: Record<string, any> = {
  conversations: vi.fn(async () => []),
  messages: vi.fn(async () => ({ messages: [], next: '', canSend: true })),
  notify: vi.fn(async () => false),
  read: vi.fn(async () => {}),
  send: vi.fn(),
  edit: vi.fn(),
  react: vi.fn(async () => {}),
  unreact: vi.fn(async () => {}),
  start: vi.fn(async () => {}),
  stop: vi.fn(async () => {}),
  typing: vi.fn(async () => {}),
};

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn((name: string, handler: (event: { data: unknown }) => void) => {
    handlers.set(name, handler);
    return vi.fn();
  }) },
}));
vi.mock('../services/backend', () => ({ inWails: true }));
vi.mock('../services/messaging', () => ({
  HISTORY_DAYS: 7,
  isMessageAlive: (message: { createdAt: string; expiresAt: string }, now = Date.now()) => {
    const created = Date.parse(message.createdAt);
    const expires = Date.parse(message.expiresAt);
    return (Number.isNaN(expires) || expires > now) && (Number.isNaN(created) || created >= now - 7 * 24 * 60 * 60 * 1000);
  },
  normalizeMessage: (message: unknown) => {
    const value = message as { reactions?: Array<{ emoji: string; userIds?: string[] | null }> | null };
    return { ...(message as object), reactions: (value.reactions ?? []).map((reaction) => ({ ...reaction, userIds: reaction.userIds ?? [] })) };
  },
  conversations: api.conversations,
  messages: api.messages,
  notify: api.notify,
  read: api.read,
  send: api.send,
  edit: api.edit,
  react: api.react,
  unreact: api.unreact,
  start: api.start,
  stop: api.stop,
  typing: api.typing,
}));
vi.mock('./social', () => ({ needsSocialConsent: writable(false) }));
vi.mock('./presence', () => ({ presenceStatus: writable('online') }));
vi.mock('./user', () => ({ authState: writable('unauthenticated'), currentUser: writable(null) }));

const peer = { id: 'peer-1', username: 'friend', displayName: 'Friend', avatarUrl: '' };

function message(patch: Record<string, unknown> = {}) {
  return {
    id: 'message-1',
    clientId: 'client-1',
    senderId: 'peer-1',
    recipientId: 'me',
    text: 'hello',
    createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(),
    reactions: [],
    ...patch,
  };
}

async function flush(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

async function load() {
  vi.resetModules();
  handlers.clear();
  for (const mock of Object.values(api)) mock.mockReset();
  api.conversations.mockResolvedValue([{ peer, lastMessage: null, unread: 0, canSend: true }]);
  api.messages.mockResolvedValue({ messages: [], next: '', canSend: true });
  api.start.mockResolvedValue(undefined);
  api.stop.mockResolvedValue(undefined);
  api.read.mockResolvedValue(undefined);
  api.notify.mockResolvedValue(false);
  const user = await import('./user');
  const messaging = await import('./messaging');
  user.currentUser.set({ id: 'me' } as never);
  user.authState.set('authenticated');
  messaging.initMessaging();
  await flush();
  return { user, messaging };
}

async function emit(name: string, data: unknown): Promise<void> {
  await handlers.get(name)?.({ data });
  await flush();
}

beforeEach(() => {
  vi.useRealTimers();
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: { visibilityState: 'visible', hasFocus: () => true, documentElement: { lang: '' } },
  });
});

describe('messaging store session and event races', () => {
  it('keeps a replacement toast for its own five seconds', async () => {
    vi.useFakeTimers();
    const { messaging } = await load();
    await emit('chat:event', { ownerId: 'me', kind: 'message', peerId: peer.id, message: message() });
    await vi.advanceTimersByTimeAsync(4000);
    expect(get(messaging.chatToast)?.message.id).toBe('message-1');
    await emit('chat:event', {
      ownerId: 'me', kind: 'message', peerId: peer.id,
      message: message({ id: 'message-2', clientId: 'client-2' }),
    });
    await vi.advanceTimersByTimeAsync(4999);
    expect(get(messaging.chatToast)?.message.id).toBe('message-2');
    await vi.advanceTimersByTimeAsync(1);
    expect(get(messaging.chatToast)).toBeNull();
  });

  it('ignores a stale notify result after logout', async () => {
    const { user, messaging } = await load();
    messaging.conversations.set([{ peer, lastMessage: null, unread: 0, canSend: true }]);
    let resolveNotify!: (shown: boolean) => void;
    api.notify.mockReturnValueOnce(new Promise<boolean>((resolve) => { resolveNotify = resolve; }));

    const pending = emit('chat:event', { ownerId: 'me', kind: 'message', peerId: peer.id, message: message() });
    await flush();
    user.currentUser.set(null);
    user.authState.set('unauthenticated');
    await flush();
    resolveNotify(false);
    await pending;

    expect(get(messaging.chatToast)).toBeNull();
    expect(get(messaging.conversations)).toEqual([]);
  });

  it('does not clear unread before Read succeeds and keeps it after rejection', async () => {
    const { messaging } = await load();
    api.messages.mockResolvedValue({ messages: [message()], next: '', canSend: true });
    let rejectRead!: (error: Error) => void;
    api.read.mockReturnValueOnce(new Promise<void>((_, reject) => { rejectRead = reject; }));
    messaging.conversations.set([{ peer, lastMessage: null, unread: 2, canSend: true }]);

    messaging.openChat(peer);
    await flush();
    expect(api.read).toHaveBeenCalledTimes(1);
    expect(get(messaging.conversations)[0].unread).toBe(2);
    rejectRead(new Error('offline'));
    await flush();
    expect(api.read).toHaveBeenCalledWith(peer.id, 'message-1');
    expect(get(messaging.conversations)[0].unread).toBe(2);
    await messaging.markPeerRead(peer.id);
    expect(get(messaging.conversations)[0].unread).toBe(0);
  });

  it('deduplicates incoming events and increments unread once', async () => {
    const { messaging } = await load();
    const incoming = message({ id: 'incoming-1', clientId: 'incoming-client' });
    await emit('chat:event', { ownerId: 'me', kind: 'message', peerId: peer.id, message: incoming });
    await emit('chat:event', { ownerId: 'me', kind: 'message', peerId: peer.id, message: incoming });

    expect(get(messaging.conversations)[0].unread).toBe(1);
    expect(api.notify).toHaveBeenCalledTimes(1);
  });

  it('merges the own message from SSE without notifying', async () => {
    const { messaging } = await load();
    const own = message({ senderId: 'me', recipientId: peer.id, id: 'own-1', clientId: 'own-client' });
    await emit('chat:event', { ownerId: 'me', kind: 'message', peerId: peer.id, message: own });

    expect(get(messaging.messagesByPeer)[peer.id]).toEqual([own]);
    expect(api.notify).not.toHaveBeenCalled();
  });

  it('keeps an own SSE echo when the chat was closed before it arrived', async () => {
    const { messaging } = await load();
    messaging.conversations.set([]);
    messaging.openChat(peer);
    await flush();
    messaging.closeChat();

    const own = message({ senderId: 'me', recipientId: peer.id, id: 'own-closed-1', clientId: 'own-closed-client' });
    await emit('chat:event', { ownerId: 'me', kind: 'message', peerId: peer.id, message: own });

    expect(get(messaging.conversations)).toEqual([{ peer, lastMessage: own, unread: 0, canSend: true }]);
    expect(get(messaging.messagesByPeer)[peer.id]).toEqual([own]);
  });

  it('retries a failed send with the same clientId', async () => {
    const { messaging } = await load();
    const sent = message({ senderId: 'me', recipientId: peer.id, id: 'sent-1', clientId: 'same-client' });
    api.send.mockRejectedValueOnce(new Error('network')).mockResolvedValueOnce(sent);

    await expect(messaging.sendMessage(peer.id, 'hello', 'same-client')).rejects.toThrow('network');
    await messaging.retryMessage(peer.id, 'same-client');

    expect(api.send).toHaveBeenNthCalledWith(1, peer.id, 'same-client', 'hello');
    expect(api.send).toHaveBeenNthCalledWith(2, peer.id, 'same-client', 'hello');
    expect(get(messaging.failedMessages).has('same-client')).toBe(false);
  });

  it('adds a conversation immediately after the first message is sent', async () => {
    const { messaging } = await load();
    messaging.conversations.set([]);
    const sent = message({ senderId: 'me', recipientId: peer.id, id: 'sent-1', clientId: 'first-client' });
    api.send.mockResolvedValueOnce(sent);

    messaging.openChat(peer);
    await flush();
    await messaging.sendMessage(peer.id, 'hello', 'first-client');

    expect(get(messaging.conversations)).toEqual([{ peer, lastMessage: sent, unread: 0, canSend: true }]);
  });

  it('keeps the first conversation when sending finishes after switching chats', async () => {
    const { messaging } = await load();
    const secondPeer = { ...peer, id: 'peer-2', username: 'other', displayName: 'Other' };
    messaging.conversations.set([]);
    let resolveSend!: (value: unknown) => void;
    api.send.mockReturnValueOnce(new Promise((resolve) => { resolveSend = resolve; }));
    const sent = message({ senderId: 'me', recipientId: peer.id, id: 'sent-after-switch', clientId: 'switch-client' });

    messaging.openChat(peer);
    await flush();
    const pending = messaging.sendMessage(peer.id, 'hello', 'switch-client');
    messaging.openChat(secondPeer);
    resolveSend(sent);
    await pending;

    expect(get(messaging.conversations).find((item) => item.peer.id === peer.id)?.lastMessage).toEqual(sent);
  });

  it('keeps a retried conversation when retry finishes after closing the panel', async () => {
    const { messaging } = await load();
    messaging.conversations.set([]);
    const sent = message({ senderId: 'me', recipientId: peer.id, id: 'sent-after-retry', clientId: 'retry-switch-client' });
    api.send.mockRejectedValueOnce(new Error('network')).mockResolvedValueOnce(sent);

    messaging.openChat(peer);
    await flush();
    await expect(messaging.sendMessage(peer.id, 'hello', 'retry-switch-client')).rejects.toThrow('network');
    messaging.closeChat();
    await messaging.retryMessage(peer.id, 'retry-switch-client');

    expect(get(messaging.conversations).find((item) => item.peer.id === peer.id)?.lastMessage).toEqual(sent);
  });

  it('keeps history loading while switching chats until the active chat finishes', async () => {
    const { messaging } = await load();
    const secondPeer = { ...peer, id: 'peer-2', username: 'other', displayName: 'Other' };
    messaging.conversations.set([
      { peer, lastMessage: null, unread: 0, canSend: true },
      { peer: secondPeer, lastMessage: null, unread: 0, canSend: true },
    ]);
    const resolveHistory: Record<string, (page: unknown) => void> = {};
    api.messages.mockImplementation((peerId: string) => new Promise((resolve) => {
      resolveHistory[peerId] = resolve;
    }));

    messaging.openChat(peer);
    messaging.openChat(secondPeer);
    await flush();
    resolveHistory[peer.id]({ messages: [], next: '', canSend: true });
    await flush();

    expect(get(messaging.activePeer)?.id).toBe(secondPeer.id);
    expect(get(messaging.chatLoading)).toBe(true);
    resolveHistory[secondPeer.id]({ messages: [], next: '', canSend: true });
    await flush();
    expect(get(messaging.chatLoading)).toBe(false);
  });

  it('does not show an old chat error after switching to another chat', async () => {
    const { messaging } = await load();
    const secondPeer = { ...peer, id: 'peer-2', username: 'other', displayName: 'Other' };
    messaging.conversations.set([
      { peer, lastMessage: null, unread: 0, canSend: true },
      { peer: secondPeer, lastMessage: null, unread: 0, canSend: true },
    ]);
    let rejectFirst!: (error: Error) => void;
    const resolveSecond = vi.fn();
    api.messages
      .mockReturnValueOnce(new Promise((_, reject) => { rejectFirst = reject; }))
      .mockReturnValueOnce(new Promise((resolve) => { resolveSecond.mockImplementation(resolve); }));

    messaging.openChat(peer);
    messaging.openChat(secondPeer);
    rejectFirst(new Error('offline'));
    await flush();

    expect(get(messaging.chatHistoryError)).toBe('');
    resolveSecond({ messages: [], next: '', canSend: true });
    await flush();
  });

  it('does not let an older read-marked history request clear a newer error', async () => {
    const { messaging } = await load();
    messaging.conversations.set([{ peer, lastMessage: null, unread: 1, canSend: true }]);
    api.messages.mockResolvedValueOnce({ messages: [message()], next: '', canSend: true });
    let resolveRead!: () => void;
    api.read.mockReturnValueOnce(new Promise<void>((resolve) => { resolveRead = resolve; }));

    messaging.openChat(peer);
    await flush();
    expect(api.read).toHaveBeenCalledWith(peer.id, 'message-1');

    api.messages.mockRejectedValueOnce(new Error('offline'));
    await expect(messaging.loadMessages(peer.id)).rejects.toThrow('offline');
    const historyError = get(messaging.chatHistoryError);
    resolveRead();
    await flush();

    expect(historyError).toBeTruthy();
    expect(get(messaging.chatHistoryError)).toBe(historyError);
  });

  it('clears pagination loading when the active chat changes or closes', async () => {
    const { messaging } = await load();
    const secondPeer = { ...peer, id: 'peer-2', username: 'other', displayName: 'Other' };
    messaging.conversations.set([
      { peer, lastMessage: null, unread: 0, canSend: true },
      { peer: secondPeer, lastMessage: null, unread: 0, canSend: true },
    ]);
    messaging.openChat(peer);
    await flush();
    messaging.nextByPeer.set({ [peer.id]: 'cursor' });
    let resolveMore!: (page: unknown) => void;
    api.messages.mockReturnValueOnce(new Promise((resolve) => { resolveMore = resolve; }));
    const pending = messaging.loadMore(peer.id);
    await flush();
    expect(get(messaging.chatLoadingMore)).toBe(true);

    messaging.openChat(secondPeer);
    expect(get(messaging.chatLoadingMore)).toBe(false);
    messaging.closeChat();
    expect(get(messaging.chatLoadingMore)).toBe(false);
    resolveMore({ messages: [], next: '', canSend: true });
    await pending;
  });

  it('propagates reaction failures so the panel can show an error', async () => {
    const { messaging } = await load();
    api.react.mockRejectedValueOnce(new Error('offline'));

    await expect(messaging.toggleReaction(peer.id, message(), 'heart')).rejects.toThrow('offline');
  });

  it('keeps optimistic failed messages when history refreshes', async () => {
    const { messaging } = await load();
    api.send.mockRejectedValueOnce(new Error('network'));
    await expect(messaging.sendMessage(peer.id, 'offline', 'retry-client')).rejects.toThrow('network');
    api.messages.mockResolvedValueOnce({ messages: [message({ id: 'server-1' })], next: '', canSend: true });
    await messaging.loadMessages(peer.id);

    const ids = get(messaging.messagesByPeer)[peer.id].map((item) => item.clientId);
    expect(ids).toEqual(expect.arrayContaining(['retry-client', 'client-1']));
  });

  it('drops expired messages and unread counts on the expiry tick', async () => {
    vi.useFakeTimers();
    const { messaging } = await load();
    const expired = message({ expiresAt: new Date(Date.now() - 1).toISOString() });
    messaging.messagesByPeer.set({ [peer.id]: [expired] });
    messaging.conversations.set([{ peer, lastMessage: expired, unread: 2, canSend: true }]);
    vi.advanceTimersByTime(1000);

    expect(get(messaging.messagesByPeer)[peer.id]).toEqual([]);
    expect(get(messaging.conversations)[0].unread).toBe(0);
  });

  it('refreshes unread counts within one minute when an older unread expires', async () => {
    vi.useFakeTimers();
    const { messaging } = await load();
    const expiredIncoming = message({ id: 'expired-incoming', expiresAt: new Date(Date.now() - 1).toISOString() });
    const newerOwn = message({ id: 'newer-own', senderId: 'me', recipientId: peer.id });
    messaging.messagesByPeer.set({ [peer.id]: [expiredIncoming, newerOwn] });
    messaging.conversations.set([{ peer, lastMessage: newerOwn, unread: 2, canSend: true }]);
    api.conversations.mockResolvedValue([{ peer, lastMessage: newerOwn, unread: 0, canSend: true }]);
    vi.advanceTimersByTime(60000);
    await flush();

    expect(get(messaging.conversations)[0].unread).toBe(0);
    expect(api.conversations).toHaveBeenCalledTimes(2);
  });

  it('does not let a stale history response overwrite a newer edited event', async () => {
    const { messaging } = await load();
    let resolveHistory!: (page: unknown) => void;
    api.messages.mockReturnValueOnce(new Promise((resolve) => { resolveHistory = resolve; }));
    const edited = message({ editedAt: new Date(Date.now() + 1000).toISOString(), text: 'edited' });
    const pending = messaging.loadMessages(peer.id);
    messaging.messagesByPeer.set({ [peer.id]: [edited] });
    resolveHistory({ messages: [message({ text: 'stale' })], next: '', canSend: true });
    await pending;

    expect(get(messaging.messagesByPeer)[peer.id][0].text).toBe('edited');
  });

  it('applies refreshed reactions when message timestamps are unchanged', async () => {
    const { messaging } = await load();
    const createdAt = new Date(Date.now() - 1000).toISOString();
    const base = message({ id: 'reaction-1', createdAt, reactions: [] });
    messaging.messagesByPeer.set({ [peer.id]: [base] });
    api.messages.mockResolvedValueOnce({
      messages: [message({ id: 'reaction-1', createdAt, reactions: [{ emoji: 'heart', userIds: ['peer-1'] }] })],
      next: '',
      canSend: true,
    });

    await messaging.loadMessages(peer.id);

    expect(get(messaging.messagesByPeer)[peer.id][0].reactions).toEqual([{ emoji: 'heart', userIds: ['peer-1'] }]);
  });

  it('keeps a reaction event received while history is in flight', async () => {
    const { messaging } = await load();
    const createdAt = new Date(Date.now() - 1000).toISOString();
    const base = message({ id: 'reaction-race', createdAt, reactions: [] });
    messaging.messagesByPeer.set({ [peer.id]: [base] });
    let resolveHistory!: (page: unknown) => void;
    api.messages.mockReturnValueOnce(new Promise((resolve) => { resolveHistory = resolve; }));
    const pending = messaging.loadMessages(peer.id);
    await flush();

    const updated = message({ id: 'reaction-race', createdAt, reactions: [{ emoji: 'fire', userIds: ['peer-1'] }] });
    await emit('chat:event', { ownerId: 'me', kind: 'updated', peerId: peer.id, message: updated });
    resolveHistory({ messages: [base], next: '', canSend: true });
    await pending;

    expect(get(messaging.messagesByPeer)[peer.id][0].reactions).toEqual([{ emoji: 'fire', userIds: ['peer-1'] }]);
  });

  it('refreshes reactions after a reconnect sync', async () => {
    const { messaging } = await load();
    const createdAt = new Date(Date.now() - 1000).toISOString();
    const base = message({ id: 'reaction-sync', createdAt, reactions: [] });
    const refreshed = message({ id: 'reaction-sync', createdAt, reactions: [{ emoji: 'party', userIds: ['peer-1'] }] });
    messaging.messagesByPeer.set({ [peer.id]: [base] });
    messaging.conversations.set([{ peer, lastMessage: base, unread: 0, canSend: true }]);
    api.messages.mockResolvedValueOnce({ messages: [base], next: '', canSend: true });
    messaging.openChat(peer);
    await flush();
    api.messages.mockResolvedValue({ messages: [refreshed], next: '', canSend: true });

    await emit('chat:event', { ownerId: 'me', kind: 'sync', peerId: peer.id });

    expect(get(messaging.messagesByPeer)[peer.id][0].reactions).toEqual([{ emoji: 'party', userIds: ['peer-1'] }]);
  });

  it('does not apply an in-flight history response after logout', async () => {
    const { user, messaging } = await load();
    let resolveHistory!: (page: unknown) => void;
    api.messages.mockReturnValueOnce(new Promise((resolve) => { resolveHistory = resolve; }));
    messaging.openChat(peer);
    await flush();
    user.currentUser.set(null);
    user.authState.set('unauthenticated');
    resolveHistory({ messages: [message({ id: 'stale-history' })], next: '', canSend: true });
    await flush();

    expect(get(messaging.messagesByPeer)).toEqual({});
  });

  it('sends typing false when a chat closes', async () => {
    const { messaging } = await load();
    await messaging.setTyping(peer.id, true);
    messaging.openChat(peer);
    messaging.closeChat();

    expect(api.typing).toHaveBeenCalledWith(peer.id, false);
  });
});
