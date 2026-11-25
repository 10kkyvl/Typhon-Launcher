<script lang="ts">
  import { ArrowDownUp, ChevronDown, EllipsisVertical, LayoutGrid, List, MessageCircle, UserPlus, Users } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import type { FrameReserve } from '../../lib/profile/frameLook';
  import { chatPeerOf, type FriendView, type UserCard } from '../../lib/services/social';
  import { presenceDot, presenceLine, sortFriends } from '../../lib/social/presence';
  import { friendsView } from '../../lib/stores/ui';
  import { openChat } from '../../lib/stores/messaging';
  import { msg } from '../../lib/i18n';
  import FriendRow from './FriendRow.svelte';

  let {
    friends,
    group,
    tab,
    reserve,
    search = $bindable(),
    sortBy = $bindable(),
    onadd,
    onmenu,
    onopen,
  }: {
    /** Every friend: the toolbar and "no friends yet" follow it. */
    friends: FriendView[];
    /** The friends of the chosen presence tab, before the search. */
    group: FriendView[];
    tab: string;
    reserve: FrameReserve;
    search: string;
    sortBy: 'name' | 'status';
    onadd: () => void;
    onmenu: (user: UserCard, item: string) => void;
    onopen: (user: UserCard) => void;
  } = $props();

  const sortLabels: Record<'name' | 'status', string> = {
    name: msg('social.friendsSortName'),
    status: msg('social.friendsSortStatus'),
  };

  const groupEmptyCopy: Record<string, { title: string; description: string }> = {
    online: { title: msg('social.friendsEmptyOnlineTitle'), description: msg('social.friendsEmptyOnlineDesc') },
    away: { title: msg('social.friendsEmptyAwayTitle'), description: msg('social.friendsEmptyAwayDesc') },
    offline: { title: msg('social.friendsEmptyOfflineTitle'), description: msg('social.friendsEmptyOfflineDesc') },
  };

  function sortByName(list: FriendView[]): FriendView[] {
    return [...list].sort((a, b) => (a.displayName || a.username).localeCompare(b.displayName || b.username, 'ru'));
  }

  function matchesSearch(user: UserCard, query: string): boolean {
    return (user.displayName || '').toLowerCase().includes(query) || user.username.toLowerCase().includes(query);
  }

  const visibleFriends = $derived.by(() => {
    const query = search.trim().toLowerCase();
    const filtered = query ? group.filter((f) => matchesSearch(f, query)) : group;
    return sortBy === 'status' ? sortFriends(filtered) : sortByName(filtered);
  });

  function playingGame(friend: FriendView): { igdbId: number; title: string } | null {
    const presence = friend.presence;
    if (!presence || presenceDot(presence) === 'offline' || presence.gameId == null) return null;
    return { igdbId: presence.gameId, title: presence.gameTitle ?? '' };
  }

  const menuItems = [
    { id: 'profile', label: msg('social.profileLabel') },
    { id: 'unfriend', label: msg('social.unfriendLabel'), danger: true, separator: true },
    { id: 'block', label: msg('social.blockLabel'), danger: true },
  ];

  function openMessage(user: UserCard) {
    openChat(chatPeerOf(user));
  }
</script>

{#snippet friendActions(friend: FriendView)}
  <Button variant="ghost" size="sm" data-row-control="chat" onclick={() => openMessage(friend)}>
    <MessageCircle size="1.4rem" strokeWidth={1.8} />
    {msg('social.chatWrite')}
  </Button>
  <DropdownMenu items={menuItems} onselect={(item) => onmenu(friend, item)}>
    {#snippet trigger({ props })}
      <IconButton {...props} data-row-control="menu" label={msg('social.friendMenuFor', { name: friend.displayName || friend.username })} size="sm">
        <EllipsisVertical size="1.7rem" strokeWidth={1.8} />
      </IconButton>
    {/snippet}
  </DropdownMenu>
{/snippet}

{#if friends.length > 0}
  <div class="toolbar">
    <div class="search-wrap">
      <SearchInput bind:value={search} label={msg('social.friendsSearchFriendsPlaceholder')} placeholder={msg('social.friendsSearchFriendsPlaceholder')} />
    </div>
    <div class="toolbar-right">
      <DropdownMenu
        items={(['status', 'name'] as const).map((id) => ({ id, label: sortLabels[id], checked: sortBy === id }))}
        onselect={(id) => (sortBy = id as 'name' | 'status')}
      >
        {#snippet trigger({ props })}
          <Chip {...props}>
            <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
            {sortLabels[sortBy]}
            <ChevronDown size="1.4rem" strokeWidth={1.8} />
          </Chip>
        {/snippet}
      </DropdownMenu>
      <SegmentedControl
        label={msg('social.friendsViewLabel')}
        bind:value={$friendsView}
        options={[
          { id: 'grid', label: msg('social.friendsViewGrid') },
          { id: 'list', label: msg('social.friendsViewList') },
        ]}
      >
        {#snippet item(option)}
          {#if option.id === 'list'}
            <List size="1.6rem" strokeWidth={1.8} />
          {:else}
            <LayoutGrid size="1.6rem" strokeWidth={1.8} />
          {/if}
        {/snippet}
      </SegmentedControl>
    </div>
  </div>
{/if}

{#if visibleFriends.length === 0}
  {#if friends.length === 0}
    <EmptyState level={2} title={msg('social.friendsEmptyNobodyTitle')} description={msg('social.friendsEmptyNobodyDesc')}>
      {#snippet icon()}
        <Users size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => onadd()}>
          <UserPlus size="1.5rem" strokeWidth={1.8} />
          {msg('social.friendsAddFriend')}
        </Button>
      {/snippet}
    </EmptyState>
  {:else if search.trim()}
    <EmptyState level={2} title={msg('social.friendsEmptySearchTitle')} description={msg('social.friendsEmptySearchDesc')} />
  {:else if groupEmptyCopy[tab]}
    <EmptyState level={2} title={groupEmptyCopy[tab].title} description={groupEmptyCopy[tab].description} />
  {:else}
    <EmptyState level={2} title={msg('social.friendsEmptyNobodyTitle')} description={msg('social.friendsEmptyNobodyDesc')} />
  {/if}
{:else if $friendsView === 'grid'}
  <div class="grid">
    {#each visibleFriends as friend (friend.id)}
      <div class="item" data-friends-row={friend.id}>
        <FriendRow
          user={friend}
          status={presenceDot(friend.presence)}
          meta={presenceLine(friend.presence)}
          game={playingGame(friend)}
          variant="grid"
          {reserve}
          onopen={() => onopen(friend)}
        >
          {#snippet actions()}
            {@render friendActions(friend)}
          {/snippet}
        </FriendRow>
      </div>
    {/each}
  </div>
{:else}
  <div class="list">
    {#each visibleFriends as friend (friend.id)}
      <div class="item" data-friends-row={friend.id}>
        <FriendRow
          user={friend}
          status={presenceDot(friend.presence)}
          meta={presenceLine(friend.presence)}
          game={playingGame(friend)}
          variant="list"
          {reserve}
          onopen={() => onopen(friend)}
        >
          {#snippet actions()}
            {@render friendActions(friend)}
          {/snippet}
        </FriendRow>
      </div>
    {/each}
  </div>
{/if}

<style>
  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin: var(--space-5) 0;
    flex-wrap: wrap;
  }

  .search-wrap {
    flex: 1;
    min-width: 20rem;
    max-width: 40rem;
  }

  .toolbar-right {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-shrink: 0;
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .list > .item + .item {
    border-top: 1px solid var(--border);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(22rem, 1fr));
    gap: var(--space-4);
  }

  /* A card fills its cell, so the cards of a line are as tall as the tallest. */
  .grid > .item {
    display: grid;
  }
</style>
