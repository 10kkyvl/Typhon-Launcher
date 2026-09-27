import { render } from 'svelte/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { writable } from 'svelte/store';
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', () => ({ currentUser: writable({ id: 'me' }), authState: writable('authenticated') }));
vi.mock('../stores/social', () => ({ needsSocialConsent: writable(false) }));
vi.mock('../stores/presence', () => ({ presenceStatus: writable('online') }));
import ChatPanel from './ChatPanel.svelte';
import * as chat from '../stores/messaging';
import { locale } from '../i18n';

const peer = { id: 'peer', username: 'friend', displayName: 'Friend', avatarUrl: '' };

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
