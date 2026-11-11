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
}));

import { locale } from '../i18n';
import { react } from '../services/messaging';
import * as chat from '../stores/messaging';
import { user } from '../testing/dom';
import { description, hintOnFocus } from '../testing/hints';
import ChatPanel from './ChatPanel.svelte';

const peer = { id: 'peer', username: 'friend', displayName: 'Friend', avatarUrl: '' };
const REACTIONS = ['Fire', 'Salute', 'Heart', 'Applause', 'Skull', 'Party', 'Eyes', 'Laugh'];

function message(id: string, clientId = 'c1') {
  return {
    id, clientId, senderId: 'peer', recipientId: 'me', text: 'Hello', createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(), reactions: [],
  };
}

let app: HTMLDivElement;

beforeEach(() => {
  vi.mocked(react).mockReset();
  vi.mocked(react).mockResolvedValue(undefined);
  locale.set('en');
  chat.chatOpen.set(true);
  chat.activePeer.set(peer);
  chat.chatConnected.set(true);
  chat.conversations.set([{ peer, unread: 0, canSend: true }]);
  chat.pendingMessages.set(new Set());
  chat.messagesByPeer.set({ peer: [message('m1')] });
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  chat.messagesByPeer.set({});
  chat.chatOpen.set(false);
  document.body.replaceChildren();
});

function setup() {
  render(ChatPanel, { target: app });
  const keys = user();
  const opener = () => screen.getByRole('button', { name: 'Reaction' });
  const picker = () => screen.queryByRole('dialog', { name: 'Reaction' });
  return { keys, opener, picker };
}

describe('ChatPanel reaction picker', () => {
  it('the reaction control opens a picker out of the chat panel, each reaction a button with its name', async () => {
    const { keys, opener, picker } = setup();
    expect(opener().getAttribute('aria-haspopup')).toBe('dialog');

    await keys.click(opener());

    const opened = picker() as HTMLElement;
    expect(opened).not.toBeNull();
    expect(opener().getAttribute('aria-controls')).toBe(opened.id);
    expect(app.contains(opened)).toBe(false);
    expect(within(opened).getAllByRole('button').map((button) => button.getAttribute('aria-label'))).toEqual(REACTIONS);
    expect(document.activeElement).toBe(within(opened).getByRole('button', { name: 'Fire' }));
  });

  it('picking a reaction sends it, closes the picker and puts focus back on the control', async () => {
    const { keys, opener, picker } = setup();
    await keys.click(opener());

    await keys.click(within(picker() as HTMLElement).getByRole('button', { name: 'Heart' }));

    expect(react).toHaveBeenCalledExactlyOnceWith('peer', 'm1', 'heart');
    expect(picker()).toBeNull();
    expect(document.activeElement).toBe(opener());
  });

  it('a reaction that fails to send says so in the chat', async () => {
    vi.mocked(react).mockRejectedValue(new Error('offline'));
    const { keys, opener, picker } = setup();
    await keys.click(opener());

    await keys.click(within(picker() as HTMLElement).getByRole('button', { name: 'Fire' }));

    expect(react).toHaveBeenCalledOnce();
    expect(await screen.findByText('Could not update the reaction.')).toBeTruthy();
  });

  it('Escape closes the picker, focus on the control', async () => {
    const { keys, opener, picker } = setup();
    await keys.click(opener());
    expect(picker()).not.toBeNull();

    await keys.keyboard('{Escape}');

    expect(picker()).toBeNull();
    expect(document.activeElement).toBe(opener());
  });

  it('a press outside closes the picker', async () => {
    const { keys, opener, picker } = setup();
    await keys.click(opener());
    expect(picker()).not.toBeNull();

    await keys.click(screen.getByText('Hello'));

    expect(picker()).toBeNull();
    expect(react).not.toHaveBeenCalled();
  });

  it('a pending message offers no reaction control', () => {
    chat.messagesByPeer.set({ peer: [message('local:c2', 'c2')] });
    chat.pendingMessages.set(new Set(['c2']));
    render(ChatPanel, { target: app });
    expect(screen.queryByRole('button', { name: 'Reaction' })).toBeNull();
  });
});

describe('ChatPanel composer', () => {
  it('the attach button says how many images of what size it takes', async () => {
    render(ChatPanel, { target: app });
    const attach = screen.getByRole('button', { name: 'Attach an image' });
    expect(attach.hasAttribute('title')).toBe(false);
    expect(description(attach)).toBe('Up to 4 images of 8 MB each, you can paste them from the clipboard');
    expect(await hintOnFocus(attach)).toBe('Up to 4 images of 8 MB each, you can paste them from the clipboard');
  });
});
