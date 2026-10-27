import { describe, expect, it } from 'vitest';
import friends from '../../routes/friends/Friends.svelte?raw';
import friendRow from '../../routes/friends/FriendRow.svelte?raw';
import activity from '../../routes/activity/Activity.svelte?raw';
import overlayFriends from '../../overlay/OverlayFriends.svelte?raw';
import overlayChat from '../../overlay/OverlayChat.svelte?raw';
import chatPanel from '../components/ChatPanel.svelte?raw';
import gameFriends from '../../routes/game/GameFriendsPanel.svelte?raw';

function tags(source: string, name: string): string[] {
  return source.replace(/\r\n/g, '\n').match(new RegExp(`<${name}\\b[^]*?>`, 'g')) ?? [];
}

describe('lists that stack avatars give every row the same frame reserve', () => {
  it('passes a reserve to every FriendRow on the friends page', () => {
    const rows = tags(friends, 'FriendRow');
    expect(rows).toHaveLength(5);
    for (const row of rows) expect(row).toMatch(/reserve=\{\w+Reserve\}/);
  });

  it('computes the friends page reserves over whole lists, not the search result', () => {
    expect(friends).toContain('const listReserve = $derived(reserveOf(groupFor));');
    expect(friends).toContain('const incomingReserve = $derived(reserveOf(page.incoming));');
    expect(friends).toContain('const outgoingReserve = $derived(reserveOf(page.outgoing));');
    expect(friends).toContain('const blockedReserve = $derived(reserveOf(blocked));');
  });

  it('hands the reserve from FriendRow to its Avatar and to the compact meta offset', () => {
    expect(tags(friendRow, 'Avatar')[0]).toContain('{reserve}');
    expect(friendRow).toContain('margin-left: calc(3.2rem * (1 + var(--reserve-l, 0) + var(--reserve-r, 0)) + var(--space-3));');
  });

  it.each([
    ['Activity', activity, 'FriendRow', 'reserve={playingReserve}'],
    ['OverlayFriends', overlayFriends, 'FriendRow', '{reserve}'],
    ['GameFriendsPanel', gameFriends, 'Avatar', '{reserve}'],
  ])('%s passes its list reserve', (_name, source, tag, expected) => {
    const found = tags(source, tag);
    expect(found.length).toBeGreaterThan(0);
    for (const item of found) expect(item).toContain(expected);
  });

  it.each([
    ['ChatPanel', chatPanel],
    ['OverlayChat', overlayChat],
  ])('%s reserves the conversation list by its widest frame', (_name, source) => {
    expect(source).toContain('const conversationReserve = $derived(frameReserve($conversations.map((item) => fieldsFrame(item.peer))));');
    const rows = tags(source, 'Avatar').filter((tag) => tag.includes('item.peer'));
    expect(rows).toHaveLength(1);
    expect(rows[0]).toContain('reserve={conversationReserve}');
  });
});
