import { render, screen } from '@testing-library/svelte';
import { get, writable } from 'svelte/store';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

// The real messaging store over stubbed services and a user store the test signs in and out: what a
// request started by one account does once another account has signed in.
const api = vi.hoisted(() => ({
  start: vi.fn(async () => {}),
  stop: vi.fn(async () => {}),
  conversations: vi.fn(),
  messages: vi.fn(),
  read: vi.fn(async () => {}),
  notify: vi.fn(async () => false),
  typing: vi.fn(async () => {}),
  send: vi.fn(),
  edit: vi.fn(),
  react: vi.fn(),
  unreact: vi.fn(),
  uploadChatImage: vi.fn(),
}));

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', () => ({ currentUser: writable<{ id: string } | null>(null), authState: writable('unauthenticated') }));
vi.mock('../stores/social', () => ({ needsSocialConsent: writable(false) }));
vi.mock('../stores/presence', () => ({ presenceStatus: writable('online') }));
vi.mock('../services/messaging', async (actual) => ({ ...(await actual<typeof import('../services/messaging')>()), ...api }));

import { locale, msg } from '../i18n';
import type { Attachment, Message } from '../services/messaging';
import * as chat from '../stores/messaging';
import { toasts } from '../stores/toasts';
import * as account from '../stores/user';
import { user } from '../testing/dom';
import ChatPanel from './ChatPanel.svelte';

const peer = { id: 'peer', username: 'friend', displayName: 'Friend', avatarUrl: '' };
const currentUser = account.currentUser as unknown as ReturnType<typeof writable<{ id: string } | null>>;
const authState = account.authState as unknown as ReturnType<typeof writable<string>>;

function message(id: string, clientId: string, senderId: string, patch: Partial<Message> = {}): Message {
  return {
    id, clientId, senderId, recipientId: senderId === 'peer' ? 'a' : 'peer', text: 'Hello', createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(), reactions: [], ...patch,
  } as Message;
}

function held<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (err: unknown) => void = () => {};
  const promise = new Promise<T>((ok, fail) => {
    resolve = ok;
    reject = fail;
  });
  return { promise, resolve, reject };
}

let starts = 0;

async function signIn(id: string) {
  currentUser.set({ id });
  authState.set('authenticated');
  starts += 1;
  await vi.waitFor(() => expect(api.start).toHaveBeenCalledTimes(starts));
  await vi.waitFor(() => expect(get(chat.conversations)).toHaveLength(1));
}

async function signOut() {
  currentUser.set(null);
  authState.set('unauthenticated');
  await vi.waitFor(() => expect(get(chat.conversations)).toEqual([]));
}

// What the next account must find untouched by a request of the one before it.
function expectUntouched() {
  expect(get(chat.attachmentDrafts)).toEqual({});
  expect(get(chat.chatErrors)).toEqual({});
  expect(get(chat.chatSends)).toEqual({});
  expect(get(toasts)).toEqual([]);
}

let app: HTMLDivElement;

beforeAll(() => {
  chat.initMessaging();
});

beforeEach(async () => {
  locale.set('en');
  for (const mock of Object.values(api)) mock.mockClear();
  starts = 0;
  api.conversations.mockResolvedValue([{ peer, lastMessage: null, unread: 0, canSend: true }]);
  api.messages.mockResolvedValue({ messages: [], next: '', canSend: true });
  toasts.set([]);
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(async () => {
  await signOut();
  toasts.set([]);
  document.body.replaceChildren();
});

// Account A in the conversation with `peer`, the panel drawn.
async function conversationOfA(history: Message[] = []) {
  await signIn('a');
  chat.openChat(peer);
  await vi.waitFor(() => expect(api.messages).toHaveBeenCalled());
  chat.messagesByPeer.set({ peer: history });
  render(ChatPanel, { target: app });
  return user();
}

async function nextAccount() {
  await signOut();
  await signIn('b');
}

describe('ChatPanel: a request that outlives its account', () => {
  it('a send whose images were going up posts nothing and writes nothing once another account is in', async () => {
    const up = held<Attachment>();
    api.uploadChatImage.mockReturnValue(up.promise);
    api.send.mockRejectedValue(new Error('offline'));
    const keys = await conversationOfA();
    chat.attachmentDrafts.set({ peer: [{ key: 'k1', name: 'shot.png', bytes: 1024, encoded: 'data', previewUrl: 'blob:shot' }] });
    await keys.click(await screen.findByRole('button', { name: msg('social.chatSend') }));
    expect(api.uploadChatImage).toHaveBeenCalledTimes(1);

    await nextAccount();
    up.resolve({ id: 'a1', url: 'https://img/a1', width: 10, height: 10, bytes: 1024 });
    await up.promise;
    await new Promise((done) => setTimeout(done, 0));

    expect(api.send).not.toHaveBeenCalled();
    expectUntouched();
  });

  it('an upload that fails after its account has signed out says nothing on the sign-in screen', async () => {
    const up = held<Attachment>();
    api.uploadChatImage.mockReturnValue(up.promise);
    const keys = await conversationOfA();
    chat.attachmentDrafts.set({ peer: [{ key: 'k1', name: 'shot.png', bytes: 1024, encoded: 'data', previewUrl: 'blob:shot' }] });
    await keys.click(await screen.findByRole('button', { name: msg('social.chatSend') }));

    await signOut();
    up.reject(new Error('offline'));
    await up.promise.catch(() => undefined);
    await new Promise((done) => setTimeout(done, 0));

    expect(api.send).not.toHaveBeenCalled();
    expectUntouched();
  });

  it('an edit that fails after another account is in leaves no error with it', async () => {
    const saving = held<Message>();
    api.edit.mockReturnValue(saving.promise);
    const keys = await conversationOfA([message('m-own', 'c-own', 'a')]);
    await keys.click(await screen.findByRole('button', { name: msg('social.chatEdit') }));
    await keys.click(screen.getByRole('button', { name: msg('social.chatSave') }));
    expect(api.edit).toHaveBeenCalledTimes(1);

    await nextAccount();
    saving.reject(new Error('offline'));
    await saving.promise.catch(() => undefined);
    await new Promise((done) => setTimeout(done, 0));

    expectUntouched();
  });

  it('a retried message that fails after another account is in leaves no error with it', async () => {
    const sending = held<Message>();
    api.send.mockReturnValue(sending.promise);
    const keys = await conversationOfA([message('local:c-f', 'c-f', 'a')]);
    chat.failedMessages.set(new Set(['c-f']));
    await keys.click(await screen.findByRole('button', { name: msg('social.chatRetry') }));
    expect(api.send).toHaveBeenCalledTimes(1);

    await nextAccount();
    sending.reject(new Error('offline'));
    await sending.promise.catch(() => undefined);
    await new Promise((done) => setTimeout(done, 0));

    expectUntouched();
  });

  it('a reaction that fails after another account is in leaves no error with it', async () => {
    const reacting = held<void>();
    api.react.mockReturnValue(reacting.promise);
    const keys = await conversationOfA([message('m1', 'c1', 'peer', { reactions: [{ emoji: 'fire', userIds: ['peer'] }] } as Partial<Message>)]);
    await keys.click(await screen.findByRole('button', { name: 'Fire, 1' }));
    expect(api.react).toHaveBeenCalledTimes(1);

    await nextAccount();
    reacting.reject(new Error('offline'));
    await reacting.promise.catch(() => undefined);
    await new Promise((done) => setTimeout(done, 0));

    expectUntouched();
  });
});
