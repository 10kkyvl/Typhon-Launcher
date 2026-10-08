import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get, writable } from 'svelte/store';

const handlers = new Map<string, (event: { data: unknown }) => void>();
const api: Record<string, any> = {
  conversations: vi.fn(),
  messages: vi.fn(),
  notify: vi.fn(),
  read: vi.fn(),
  send: vi.fn(),
  edit: vi.fn(),
  react: vi.fn(),
  unreact: vi.fn(),
  start: vi.fn(),
  stop: vi.fn(),
  typing: vi.fn(),
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

function message() {
  return {
    id: 'message-1',
    clientId: 'client-1',
    senderId: 'peer-1',
    recipientId: 'me',
    text: 'hello',
    createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(),
    reactions: [],
  };
}

async function flush(): Promise<void> {
  for (let i = 0; i < 8; i += 1) await Promise.resolve();
}

async function load(passive: boolean) {
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
  messaging.initMessaging(passive ? { passive: true } : undefined);
  await flush();
  return { user, messaging };
}

async function emit(name: string, data: unknown): Promise<void> {
  await handlers.get(name)?.({ data });
  await flush();
}

beforeEach(() => {
  vi.useFakeTimers();
  const drafts = new Map<string, string>();
  Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: {
    getItem: (key: string) => drafts.get(key) ?? null,
    setItem: (key: string, value: string) => { drafts.set(key, value); },
    removeItem: (key: string) => { drafts.delete(key); },
  } });
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: { visibilityState: 'visible', hasFocus: () => true, documentElement: { lang: '' } },
  });
});

describe('messaging store passive mode', () => {
  it('does not start or stop the shared service and keeps no timers', async () => {
    const { user } = await load(true);
    expect(api.start).not.toHaveBeenCalled();
    expect(api.conversations).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);

    user.authState.set('unauthenticated');
    await flush();
    expect(api.stop).not.toHaveBeenCalled();
    expect(api.start).not.toHaveBeenCalled();
  });

  it('stores an incoming message without notifying or showing a toast', async () => {
    const { messaging } = await load(true);
    await emit('chat:event', { ownerId: 'me', kind: 'message', peerId: peer.id, message: message() });

    expect(get(messaging.messagesByPeer)[peer.id]?.map((item) => item.id)).toEqual(['message-1']);
    expect(get(messaging.conversations)[0].unread).toBe(1);
    expect(api.notify).not.toHaveBeenCalled();
    expect(get(messaging.chatToast)).toBeNull();
  });

  it('follows the connection state reported by the service', async () => {
    const { messaging } = await load(true);
    await emit('chat:event', { ownerId: 'me', kind: 'connection', peerId: '', connected: false });
    expect(get(messaging.chatConnected)).toBe(false);
  });
});

describe('messaging store regular mode', () => {
  it('starts the service, keeps its timers and notifies on an incoming message', async () => {
    const { messaging } = await load(false);
    expect(api.start).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(2);

    await emit('chat:event', { ownerId: 'me', kind: 'message', peerId: peer.id, message: message() });
    expect(api.notify).toHaveBeenCalledWith(peer.id, 'Friend', 'hello');
    expect(get(messaging.chatToast)?.message.id).toBe('message-1');
  });
});
