import { render, screen, within } from '@testing-library/svelte';
import { writable } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@wailsio/runtime', async () => (await import('../testing/dom')).wailsRuntime());
vi.mock('../services/backend', () => ({ inWails: false }));
vi.mock('../stores/user', () => ({ currentUser: writable({ id: 'me' }), authState: writable('authenticated') }));
vi.mock('../stores/social', () => ({ needsSocialConsent: writable(false) }));
vi.mock('../stores/presence', () => ({ presenceStatus: writable('online') }));

import { locale } from '../i18n';
import * as chat from '../stores/messaging';
import { user } from '../testing/dom';
import { description, hintOnFocus } from '../testing/hints';
import ChatPanel from './ChatPanel.svelte';

const peer = { id: 'peer', username: 'friend', displayName: 'Friend', avatarUrl: '' };

let app: HTMLDivElement;

beforeEach(() => {
  locale.set('en');
  chat.chatOpen.set(true);
  chat.activePeer.set(peer);
  chat.chatConnected.set(true);
  chat.conversations.set([{ peer, unread: 0, canSend: true }]);
  chat.pendingMessages.set(new Set());
  chat.messagesByPeer.set({ peer: [{
    id: 'm1', clientId: 'c1', senderId: 'peer', recipientId: 'me', text: 'Hello', createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(), reactions: [],
  }] });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  chat.messagesByPeer.set({});
  chat.chatOpen.set(false);
  document.body.replaceChildren();
});

describe('ChatPanel reaction choices', () => {
  it('each reaction offered for a message is a button with a name', async () => {
    render(ChatPanel, { target: app });
    await user().click(screen.getByRole('button', { name: 'Reaction' }));
    const article = screen.getByText('Hello').closest('article') as HTMLElement;
    const unnamed = within(article).getAllByRole('button').filter((button) => !button.getAttribute('aria-label') && !button.textContent?.trim());
    expect(unnamed).toEqual([]);
    expect(within(article).getAllByRole('button').length).toBeGreaterThan(2);
  });

  it('the attach button says how many images of what size it takes', async () => {
    render(ChatPanel, { target: app });
    const attach = screen.getByRole('button', { name: 'Attach an image' });
    expect(attach.hasAttribute('title')).toBe(false);
    expect(description(attach)).toBe('Up to 4 images of 8 MB each, you can paste them from the clipboard');
    expect(await hintOnFocus(attach)).toBe('Up to 4 images of 8 MB each, you can paste them from the clipboard');
  });
});
