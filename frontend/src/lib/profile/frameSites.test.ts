import { describe, expect, it } from 'vitest';

const files = import.meta.glob(
  [
    '../bigpicture/components/ProfilePage.svelte',
    '../bigpicture/components/SocialPage.svelte',
    '../components/ChatPanel.svelte',
    '../components/FeedRow.svelte',
    '../components/PlayerCard.svelte',
    '../components/Sidebar.svelte',
    '../../overlay/OverlayChat.svelte',
    '../../routes/activity/Activity.svelte',
    '../../routes/friends/AddFriendModal.svelte',
    '../../routes/friends/FriendRow.svelte',
    '../../routes/game/GameFriendsPanel.svelte',
    '../../routes/game/ReviewCard.svelte',
    '../../routes/profile/ProfileHeader.svelte',
    '../../routes/user/UserHeader.svelte',
    '../../routes/user/UserMutual.svelte',
  ],
  { query: '?raw', import: 'default', eager: true },
) as Record<string, string>;

function avatars(source: string): string[] {
  return source.replace(/\r\n/g, '\n').match(/<Avatar\b[^]*?\/>/g) ?? [];
}

describe('avatar render sites', () => {
  it('finds every site', () => {
    expect(Object.keys(files)).toHaveLength(15);
  });
  it.each(Object.entries(files))('%s passes the frame to every Avatar', (path, source) => {
    const list = avatars(source);
    expect(list.length).toBeGreaterThan(0);
    for (const tag of list) expect(tag, path).toMatch(/\{\.\.\.[A-Za-z]*[Ff]rame\w*\(|\{\.\.\.avatarFrame\}/);
  });
});
