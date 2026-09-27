import { render } from 'svelte/server';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { writable } from 'svelte/store';
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', () => ({ currentUser: writable({ id: 'me' }), authState: writable('authenticated') }));
vi.mock('../stores/social', () => ({ needsSocialConsent: writable(false) }));
vi.mock('../stores/presence', () => ({ presenceStatus: writable('online') }));
import ChatPanel from './ChatPanel.svelte';
import * as chat from '../stores/messaging';
import { locale } from '../i18n';

const peer = { id: 'peer', username: 'friend', displayName: 'Friend', avatarUrl: '' };

afterEach(() => vi.useRealTimers());

beforeEach(() => {
  locale.set('en');
  chat.chatOpen.set(true);
  chat.activePeer.set(peer);
  chat.chatConnected.set(true);
  chat.chatHistoryError.set('');
  chat.chatConversationsError.set('');
  chat.conversations.set([{ peer, unread: 0, canSend: true }]);
  chat.messagesByPeer.set({});
  chat.pendingMessages.set(new Set());
});

describe('chat panel recovery', () => {
  it('keeps the composer when the event stream disconnects', () => {
    chat.chatConnected.set(false);
    const html = render(ChatPanel).body;
    expect(html).toContain('Reconnecting…');
    expect(html).toContain('<textarea');
    expect(html).toContain('aria-label="Send"');
  });

  it('shows the cached list alongside its refresh error', () => {
    chat.activePeer.set(null);
    chat.chatConversationsError.set('Refresh failed');
    const html = render(ChatPanel).body;
    expect(html).toContain('Refresh failed');
    expect(html).toContain('Friend');
    expect(html).toContain('class="conversation ');
  });

  it('shows sending status without edit or reaction actions on a pending message', () => {
    chat.messagesByPeer.set({ peer: [{
      id: 'local:client', clientId: 'client', senderId: 'me', recipientId: 'peer',
      text: 'Pending text', createdAt: new Date().toISOString(),
      expiresAt: new Date(Date.now() + 86400000).toISOString(), reactions: [],
    }] });
    chat.pendingMessages.set(new Set(['client']));
    const html = render(ChatPanel).body;
    expect(html).toContain('Sending…');
    expect(html).toContain('aria-busy="true"');
    expect(html).not.toContain('title="Edit"');
    expect(html).not.toContain('title="Reaction"');
  });
});

describe('chat message dates', () => {
  it.each([
    { name: 'today', now: new Date(2026, 8, 26, 23, 30), sent: new Date(2026, 8, 26, 23, 11), day: '' },
    { name: 'yesterday across midnight', now: new Date(2026, 8, 26, 0, 1), sent: new Date(2026, 8, 25, 23, 11), day: 'Вчера, ' },
    { name: 'yesterday more than 24 hours ago', now: new Date(2026, 8, 26, 23, 30), sent: new Date(2026, 8, 25, 0, 11), day: 'Вчера, ' },
    { name: 'older message', now: new Date(2026, 8, 26, 12), sent: new Date(2026, 8, 24, 23, 11), day: '24 сент., ' },
    { name: 'previous year', now: new Date(2027, 0, 1, 12), sent: new Date(2026, 11, 31, 23, 11), day: 'Вчера, ' },
    { name: 'daylight saving boundary', now: new Date(2026, 2, 30, 0, 1), sent: new Date(2026, 2, 29, 0, 11), day: 'Вчера, ' },
  ])('renders $name in the message metadata', ({ now, sent, day }) => {
    vi.useFakeTimers();
    vi.setSystemTime(now);
    locale.set('ru');
    const iso = sent.toISOString();
    chat.messagesByPeer.set({ peer: [{
      id: 'message', clientId: 'client', senderId: 'me', recipientId: 'peer',
      text: 'Message', createdAt: iso, expiresAt: '', reactions: [],
    }] });
    const clock = `${String(sent.getHours()).padStart(2, '0')}:${String(sent.getMinutes()).padStart(2, '0')}`;
    const html = render(ChatPanel).body;
    expect(html).toContain(`datetime="${iso}"`);
    expect(html).toContain(`>${day}${clock}</time>`);
  });

  it('localizes yesterday and older dates in English', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(2026, 8, 26, 12));
    chat.messagesByPeer.set({ peer: [25, 24].map((day) => ({
      id: `message-${day}`, clientId: `client-${day}`, senderId: 'me', recipientId: 'peer',
      text: 'Message', createdAt: new Date(2026, 8, day, 23, 11).toISOString(), expiresAt: '', reactions: [],
    })) });
    const html = render(ChatPanel).body;
    expect(html).toContain('>Yesterday, 11:11 PM</time>');
    expect(html).toContain('>Sep 24, 11:11 PM</time>');
  });
});
