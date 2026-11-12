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
import { react, unreact } from '../services/messaging';
import * as chat from '../stores/messaging';
import { user } from '../testing/dom';
import { description, hintOnFocus } from '../testing/hints';
import { ruleBody, styleOf } from '../testing/css';
import ChatPanel from './ChatPanel.svelte';
import chatSource from './ChatPanel.svelte?raw';
import iconButtonSource from './IconButton.svelte?raw';

const peer = { id: 'peer', username: 'friend', displayName: 'Friend', avatarUrl: '' };
const REACTIONS = ['Fire', 'Salute', 'Heart', 'Applause', 'Skull', 'Party', 'Eyes', 'Laugh'];

function message(id: string, clientId = 'c1', reactions: { emoji: string; userIds: string[] }[] = []) {
  return {
    id, clientId, senderId: 'peer', recipientId: 'me', text: 'Hello', createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 86400000).toISOString(), reactions,
  };
}

let app: HTMLDivElement;

beforeEach(() => {
  vi.mocked(react).mockReset();
  vi.mocked(react).mockResolvedValue(undefined);
  vi.mocked(unreact).mockReset();
  vi.mocked(unreact).mockResolvedValue(undefined);
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

  // The user already reacted to the message with a heart (and so did the friend): the picker says so,
  // and picking it again takes the user's heart back.
  function reacted(id: string) {
    chat.messagesByPeer.set({ peer: [message(id, `c-${id}`, [{ emoji: 'heart', userIds: ['peer', 'me'] }])] });
    return setup();
  }

  it('shows the user\'s own reaction pressed, and only that one', async () => {
    const { keys, opener, picker } = reacted('m-pressed');
    await keys.click(opener());

    const pressed = within(picker() as HTMLElement)
      .getAllByRole('button')
      .filter((button) => button.getAttribute('aria-pressed') === 'true')
      .map((button) => button.getAttribute('aria-label'));
    expect(pressed).toEqual(['Heart']);
  });

  it('names each reaction under a message with its count and says which is the user\'s own', async () => {
    chat.messagesByPeer.set({
      peer: [message('m-chips', 'c-chips', [{ emoji: 'heart', userIds: ['peer', 'me'] }, { emoji: 'fire', userIds: ['peer'] }])],
    });
    setup();

    const heart = screen.getByRole('button', { name: 'Heart, 2' });
    const fire = screen.getByRole('button', { name: 'Fire, 1' });
    expect(heart.getAttribute('aria-pressed')).toBe('true');
    expect(fire.getAttribute('aria-pressed')).toBe('false');
  });

  it('picking the user\'s own reaction takes it back, closes the picker and puts focus back on the control', async () => {
    const { keys, opener, picker } = reacted('m-undo');
    await keys.click(opener());

    await keys.click(within(picker() as HTMLElement).getByRole('button', { name: 'Heart' }));

    expect(unreact).toHaveBeenCalledExactlyOnceWith('peer', 'm-undo', 'heart');
    expect(react).not.toHaveBeenCalled();
    expect(picker()).toBeNull();
    expect(document.activeElement).toBe(opener());
  });

  it('taking a reaction back that fails says so in the chat', async () => {
    vi.mocked(unreact).mockRejectedValue(new Error('offline'));
    const { keys, opener, picker } = reacted('m-undo-fails');
    await keys.click(opener());

    await keys.click(within(picker() as HTMLElement).getByRole('button', { name: 'Heart' }));

    expect(unreact).toHaveBeenCalledOnce();
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

  it('the send button is named, unavailable with an empty draft and available once there is text', async () => {
    const { keys } = setup();
    const send = screen.getByRole('button', { name: 'Send' }) as HTMLButtonElement;
    expect(send.disabled).toBe(true);

    await keys.type(screen.getByRole('textbox'), 'Hi');

    expect(send.disabled).toBe(false);
  });
});

describe('ChatPanel counts', () => {
  const badgeIn = (element: HTMLElement) => element.querySelector('[data-part="count-badge"] [aria-hidden="true"]')?.textContent;

  it('the closed chat opener is filled in the accent and says how many messages are unread in its name', async () => {
    chat.chatOpen.set(false);
    chat.conversations.set([
      { peer, unread: 2, canSend: true },
      { peer: { ...peer, id: 'other', username: 'other' }, unread: 1, canSend: true },
    ]);
    const { keys } = setup();
    const opener = screen.getByRole('button', { name: 'Open chats, 3 unread' });
    expect(badgeIn(opener)).toBe('3');
    expect(opener.classList.contains('primary')).toBe(true);

    await keys.click(opener);

    expect(screen.getByRole('region', { name: 'Chats' })).toBeTruthy();
  });

  it('with nothing unread the opener is named plainly and shows no count', () => {
    chat.chatOpen.set(false);
    setup();
    const opener = screen.getByRole('button', { name: 'Open chats' });
    expect(opener.querySelector('[data-part="count-badge"]')).toBeNull();
  });

  it('a conversation with unread messages says how many as part of its name', () => {
    chat.activePeer.set(null);
    chat.conversations.set([{ peer, unread: 4, canSend: true }]);
    setup();
    const row = screen.getByRole('button', { name: /Friend .* 4 unread$/ });
    expect(badgeIn(row)).toBe('4');
  });
});

describe('ChatPanel opener look', () => {
  // The shadow lifts the button off the page: drawn on its wrapper, it must follow the button's own
  // corners, which a theme sets, or a round shadow lies under a squarer button.
  it('draws its shadow with the corners of the button it lifts', () => {
    const wrapper = ruleBody(styleOf(chatSource, 'ChatPanel.svelte'), '.chat-toggle');
    const button = ruleBody(styleOf(iconButtonSource, 'IconButton.svelte'), '.icon-btn');
    expect(wrapper['box-shadow']).toBe('var(--shadow-pop)');
    expect(wrapper['border-radius']).toBe(button['border-radius']);
  });
});
