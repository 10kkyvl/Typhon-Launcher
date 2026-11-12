import { render, screen, within } from '@testing-library/svelte';
import { writable } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', () => ({ currentUser: writable({ id: 'me' }), authState: writable('authenticated') }));
vi.mock('../stores/social', () => ({ needsSocialConsent: writable(false) }));
vi.mock('../stores/presence', () => ({ presenceStatus: writable('online') }));
vi.mock('../services/messaging', async (actual) => ({
  ...(await actual<typeof import('../services/messaging')>()),
  react: vi.fn(async () => {}),
  unreact: vi.fn(async () => {}),
  messages: vi.fn(),
}));

import { locale, msg } from '../i18n';
import { messages as fetchMessages, type MessagePage } from '../services/messaging';
import * as chat from '../stores/messaging';
import { user } from '../testing/dom';
import ChatPanel from './ChatPanel.svelte';

const peer = { id: 'peer', username: 'friend', displayName: 'Friend', avatarUrl: '' };

function message(id: string, clientId = 'c1', senderId = 'peer', reactions: { emoji: string; userIds: string[] }[] = []) {
  return {
    id, clientId, senderId, recipientId: senderId === 'me' ? 'peer' : 'me', text: 'Hello', createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(), reactions,
  };
}

let app: HTMLDivElement;

beforeEach(() => {
  vi.mocked(fetchMessages).mockReset();
  locale.set('en');
  chat.chatOpen.set(true);
  chat.activePeer.set(peer);
  chat.chatConnected.set(true);
  chat.chatLoading.set(false);
  chat.chatLoadingMore.set(false);
  chat.chatHistoryError.set('');
  chat.chatConversationsError.set('');
  chat.nextByPeer.set({});
  chat.failedMessages.set(new Set());
  chat.attachmentDrafts.set({});
  chat.editingMessageId.set(null);
  chat.conversations.set([{ peer, unread: 0, canSend: true }]);
  chat.pendingMessages.set(new Set());
  chat.messagesByPeer.set({ peer: [message('m1')] });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  chat.messagesByPeer.set({});
  chat.activePeer.set(null);
  chat.chatOpen.set(false);
  document.body.replaceChildren();
});

function setup() {
  render(ChatPanel, { target: app });
  return user();
}

const isIconButton = (el: HTMLElement) => el.classList.contains('icon-btn');
const isButton = (el: HTMLElement) => el.classList.contains('btn');

describe('ChatPanel: buttons', () => {
  it('goes back to the conversations through a base icon button the keyboard presses', async () => {
    const keys = setup();
    const back = screen.getByRole('button', { name: msg('social.chatBack') });
    expect(isIconButton(back)).toBe(true);
    back.focus();

    await keys.keyboard('{Enter}');

    expect(screen.getByRole('button', { name: new RegExp(peer.displayName) })).toBeTruthy();
    expect(screen.queryByRole('button', { name: msg('social.chatBack') })).toBeNull();
  });

  it('edits an own message and opens the reactions through base icon buttons', async () => {
    chat.messagesByPeer.set({ peer: [message('m-own', 'c-own', 'me')] });
    const keys = setup();
    const edit = screen.getByRole('button', { name: msg('social.chatEdit') });
    const reaction = screen.getByRole('button', { name: msg('social.chatReaction') });
    expect(isIconButton(edit)).toBe(true);
    expect(isIconButton(reaction)).toBe(true);
    edit.focus();

    await keys.keyboard('{Enter}');

    expect(screen.getByRole('button', { name: msg('social.chatSave') })).toBeTruthy();
  });

  it('draws the reactions under a message as pressed base chips', () => {
    chat.messagesByPeer.set({ peer: [message('m-r', 'c-r', 'peer', [{ emoji: 'heart', userIds: ['peer', 'me'] }])] });
    setup();
    const heart = screen.getByRole('button', { name: 'Heart, 2' });
    expect(heart.classList.contains('chip')).toBe(true);
    expect(heart.getAttribute('aria-pressed')).toBe('true');
  });

  it('retries a failed message and loads earlier ones with base buttons, the latter busy while it loads', () => {
    chat.failedMessages.set(new Set(['c1']));
    chat.nextByPeer.set({ peer: 'cursor' });
    chat.chatLoadingMore.set(true);
    setup();
    expect(isButton(screen.getByRole('button', { name: msg('social.chatRetry') }))).toBe(true);
    const earlier = screen.getByRole('button', { name: msg('social.chatLoadingHistory') });
    expect(isButton(earlier)).toBe(true);
    expect(earlier.getAttribute('aria-busy')).toBe('true');
  });

  it('removes an attached image through a base icon button', () => {
    chat.attachmentDrafts.set({ peer: [{ key: 'k1', name: 'shot.png', bytes: 1024, encoded: '', previewUrl: 'blob:shot' }] });
    setup();
    expect(isIconButton(screen.getByRole('button', { name: msg('social.chatAttachRemove') }))).toBe(true);
  });

  it('writes in fields with the shared field look', async () => {
    chat.messagesByPeer.set({ peer: [message('m-own', 'c-own', 'me')] });
    const keys = setup();
    expect(screen.getByPlaceholderText(msg('social.chatPlaceholder')).classList.contains('input')).toBe(true);
    await keys.click(screen.getByRole('button', { name: msg('social.chatEdit') }));
    expect(document.querySelector('textarea.edit-input')?.classList.contains('input')).toBe(true);
  });
});

describe('ChatPanel: messages', () => {
  it('keeps a failed history and its focused Retry on screen while the history reloads', async () => {
    let settle: (page: MessagePage) => void = () => {};
    vi.mocked(fetchMessages).mockReturnValue(new Promise((ok) => (settle = ok)));
    chat.chatHistoryError.set('Could not load the messages.');
    const keys = setup();
    const alert = screen.getByRole('alert');
    const notice = alert.closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');
    const retry = within(notice).getByRole('button', { name: msg('social.chatRetry') });

    await keys.click(retry);

    expect(notice.isConnected).toBe(true);
    expect(retry.getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(retry);
    settle({ messages: [], next: '' } as unknown as MessagePage);
    await vi.waitFor(() => expect(notice.isConnected).toBe(false));
  });

  it('shows a failed conversation list as a danger notice with its Retry', () => {
    chat.activePeer.set(null);
    chat.chatConversationsError.set('Could not load the conversations.');
    setup();
    const notice = screen.getByRole('alert').closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('danger');
    expect(isButton(within(notice).getByRole('button', { name: msg('social.chatRetry') }))).toBe(true);
  });

  it('says it is reconnecting in a warning notice with a base Reconnect button', () => {
    chat.chatConnected.set(false);
    setup();
    const status = screen.getAllByRole('status').find((el) => el.closest('.notice')) as HTMLElement;
    const notice = status.closest('.notice') as HTMLElement;
    expect(notice.getAttribute('data-tone')).toBe('warning');
    expect(isButton(within(notice).getByRole('button', { name: msg('social.chatReconnect') }))).toBe(true);
  });

  it('states that a conversation cannot be written to in a notice that is not announced', () => {
    chat.conversations.set([{ peer, unread: 0, canSend: false }]);
    setup();
    const text = screen.getByText(msg('social.chatDisabled'));
    expect(text.closest('.notice')).toBeTruthy();
    expect(text.closest('[role="status"], [role="alert"]')).toBeNull();
  });

  it('marks a message being sent with the base spinner named "Sending"', () => {
    chat.pendingMessages.set(new Set(['c1']));
    setup();
    const status = screen.getAllByRole('status').find((el) => el.classList.contains('spinner')) as HTMLElement;
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('social.chatSending'));
  });

  it('shows the history loading as the base spinner and an empty history as the base empty state', () => {
    chat.messagesByPeer.set({ peer: [] });
    chat.chatLoading.set(true);
    setup();
    const status = screen.getAllByRole('status').find((el) => el.classList.contains('spinner')) as HTMLElement;
    expect(status.classList.contains('spinner')).toBe(true);
    expect(status.textContent?.trim()).toBe(msg('social.chatLoadingHistory'));

    chat.chatLoading.set(false);
    return vi.waitFor(() => expect(screen.getByText(msg('social.chatNoHistory')).closest('.empty')).toBeTruthy());
  });
});
