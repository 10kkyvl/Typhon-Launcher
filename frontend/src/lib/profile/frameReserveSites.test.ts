import { describe, expect, it } from 'vitest';
import activity from '../../routes/activity/Activity.svelte?raw';
import overlayFriends from '../../overlay/OverlayFriends.svelte?raw';
import overlayChat from '../../overlay/OverlayChat.svelte?raw';
import chatConversations from '../components/ChatConversations.svelte?raw';
import gameFriends from '../../routes/game/GameFriendsPanel.svelte?raw';

function tags(source: string, name: string): string[] {
  return source.replace(/\r\n/g, '\n').match(new RegExp(`<${name}\\b[^]*?>`, 'g')) ?? [];
}

// The friends screen, every component of it: a row drawn in a part the page mounts is still on the page.
const friendsScreen = Object.fromEntries(
  Object.entries(import.meta.glob<string>('../../routes/friends/*.svelte', { query: '?raw', import: 'default', eager: true })).map(([path, text]) => [
    path.replace('../../routes/friends/', ''),
    text,
  ]),
);
const friends = friendsScreen['Friends.svelte'];
const friendRow = friendsScreen['FriendRow.svelte'];

describe('lists that stack avatars give every row the same frame reserve', () => {
  it('passes a reserve to every FriendRow on the friends page', () => {
    expect(Object.keys(friendsScreen).sort()).toEqual([
      'AddFriendModal.svelte',
      'FriendCodeCard.svelte',
      'FriendRow.svelte',
      'Friends.svelte',
      'FriendsList.svelte',
      'RequestCards.svelte',
    ]);
    const rows = Object.fromEntries(Object.entries(friendsScreen).map(([file, source]) => [file, tags(source, 'FriendRow')]));
    expect(Object.fromEntries(Object.entries(rows).map(([file, found]) => [file, found.length]))).toEqual({
      'AddFriendModal.svelte': 0,
      'FriendCodeCard.svelte': 0,
      'FriendRow.svelte': 0,
      'Friends.svelte': 0,
      'FriendsList.svelte': 2,
      'RequestCards.svelte': 1,
    });
    // A part hands on the reserve the page gives it, and the page gives one computed over a whole list:
    // the friends of the tab, the incoming and the sent requests, the blocked users.
    for (const row of [...rows['FriendsList.svelte'], ...rows['RequestCards.svelte']]) expect(row).toContain('{reserve}');
    expect(tags(friends, 'FriendsList').map((tag) => /reserve=\{(\w+)\}/.exec(tag)?.[1])).toEqual(['listReserve']);
    expect(tags(friends, 'RequestCards').map((tag) => /reserve=\{(\w+)\}/.exec(tag)?.[1])).toEqual([
      'incomingReserve',
      'outgoingReserve',
      'blockedReserve',
    ]);
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
    ['ChatConversations', chatConversations],
    ['OverlayChat', overlayChat],
  ])('%s reserves the conversation list by its widest frame', (_name, source) => {
    expect(source).toContain('const conversationReserve = $derived(frameReserve($conversations.map((item) => fieldsFrame(item.peer))));');
    const rows = tags(source, 'Avatar').filter((tag) => tag.includes('item.peer'));
    expect(rows).toHaveLength(1);
    expect(rows[0]).toContain('reserve={conversationReserve}');
  });
});
