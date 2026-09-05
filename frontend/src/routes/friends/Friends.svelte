<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import {
    ArrowDownUp,
    ChevronDown,
    EllipsisVertical,
    LayoutGrid,
    List,
    LogIn,
    ShieldCheck,
    UserPlus,
    Users,
  } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import SocialConsentScreen from '../../lib/components/SocialConsentScreen.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { AccountError } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import {
    accept,
    block,
    blocks as fetchBlocks,
    decline,
    refresh,
    unblock,
    unfriend,
    type FriendView,
    type RequestView,
    type UserCard,
  } from '../../lib/services/social';
  import { presenceDot, presenceLine, sortFriends } from '../../lib/social/presence';
  import { commonLine, sentAt } from '../../lib/social/view';
  import { friendsView } from '../../lib/stores/ui';
  import { navigate } from '../../lib/stores/router';
  import { friendsPage, incomingCount, needsSocialConsent } from '../../lib/stores/social';
  import { toast } from '../../lib/stores/toasts';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import { relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import AddFriendModal from './AddFriendModal.svelte';
  import FriendCodeCard from './FriendCodeCard.svelte';
  import FriendRow from './FriendRow.svelte';

  let { tab: initialTab }: { tab?: string } = $props();

  let tab = $state('all');
  let addOpen = $state(false);
  let codeOpen = $state(false);
  let consentOpen = $state(false);
  let blocked = $state<UserCard[]>([]);
  let busy = $state('');
  let myCode = $state('');
  let search = $state('');
  let sortBy = $state<'name' | 'status'>('name');

  const isGuest = $derived($authState === 'guest');
  const page = $derived($friendsPage);

  const onlineFriends = $derived(
    page.friends.filter((f) => {
      const dot = presenceDot(f.presence);
      return dot === 'online' || dot === 'busy';
    }),
  );
  const awayFriends = $derived(page.friends.filter((f) => presenceDot(f.presence) === 'away'));
  const offlineFriends = $derived(
    page.friends.filter((f) => {
      const dot = presenceDot(f.presence);
      return dot !== 'online' && dot !== 'busy' && dot !== 'away';
    }),
  );

  const presenceTabs = new Set(['all', 'online', 'away', 'offline']);

  const sortLabels: Record<'name' | 'status', string> = {
    name: msg('social.friendsSortName'),
    status: msg('social.friendsSortStatus'),
  };

  const groupEmptyCopy: Record<string, { title: string; description: string }> = {
    online: { title: msg('social.friendsEmptyOnlineTitle'), description: msg('social.friendsEmptyOnlineDesc') },
    away: { title: msg('social.friendsEmptyAwayTitle'), description: msg('social.friendsEmptyAwayDesc') },
    offline: { title: msg('social.friendsEmptyOfflineTitle'), description: msg('social.friendsEmptyOfflineDesc') },
  };

  const groupFor = $derived.by(() => {
    if (tab === 'online') return onlineFriends;
    if (tab === 'away') return awayFriends;
    if (tab === 'offline') return offlineFriends;
    return page.friends;
  });

  function sortByName(list: FriendView[]): FriendView[] {
    return [...list].sort((a, b) => (a.displayName || a.username).localeCompare(b.displayName || b.username, 'ru'));
  }

  function matchesSearch(user: UserCard, query: string): boolean {
    return (user.displayName || '').toLowerCase().includes(query) || user.username.toLowerCase().includes(query);
  }

  const visibleFriends = $derived.by(() => {
    const query = search.trim().toLowerCase();
    const filtered = query ? groupFor.filter((f) => matchesSearch(f, query)) : groupFor;
    return sortBy === 'status' ? sortFriends(filtered) : sortByName(filtered);
  });

  function playingGame(friend: FriendView): { igdbId: number; title: string } | null {
    const presence = friend.presence;
    if (!presence || presenceDot(presence) === 'offline' || presence.gameId == null) return null;
    return { igdbId: presence.gameId, title: presence.gameTitle ?? '' };
  }

  function receivedAt(iso: string | null): string {
    const when = relativeDate(iso);
    return when === '—' ? when : msg('friends.receivedAt', { when: when.toLowerCase() });
  }

  function incomingStats(request: RequestView): string[] {
    const stats: string[] = [];
    if (request.mutualCount > 0) stats.push(commonLine(request.mutualCount, 0));
    if (request.commonCount > 0) stats.push(commonLine(0, request.commonCount));
    stats.push(receivedAt(request.createdAt));
    return stats;
  }

  function outgoingStats(request: RequestView): string[] {
    const stats: string[] = [];
    if (request.commonCount > 0) stats.push(commonLine(0, request.commonCount));
    stats.push(`${msg('friends.pendingReply')} · ${sentAt(request.createdAt)}`);
    return stats;
  }

  const tabs = $derived([
    { id: 'all', label: msg('social.friendsTabAll'), count: page.friends.length },
    { id: 'online', label: msg('social.presenceOnline'), count: onlineFriends.length },
    { id: 'away', label: msg('social.friendsTabAway'), count: awayFriends.length },
    { id: 'offline', label: msg('social.presenceOffline'), count: offlineFriends.length },
    { id: 'requests', label: msg('social.friendsTabRequests'), count: $incomingCount + page.outgoing.length },
    { id: 'blocked', label: msg('social.friendsTabBlocked'), count: blocked.length },
  ]);

  const menuItems = [
    { id: 'profile', label: msg('social.profileLabel') },
    { id: 'unfriend', label: msg('social.unfriendLabel'), danger: true, separator: true },
    { id: 'block', label: msg('social.blockLabel'), danger: true },
  ];

  function report(err: unknown, fallback: string) {
    if (err instanceof AccountError && err.code === 'unauthenticated') return;
    toast(accountErrorText(err, fallback), 'danger');
  }

  function reload() {
    refresh().catch((err) => report(err, msg('social.friendsRefreshFailed')));
  }

  async function loadBlocks() {
    try {
      blocked = await fetchBlocks();
    } catch (err) {
      report(err, msg('social.friendsBlockedLoadFailed'));
    }
  }

  async function run(id: string, action: () => Promise<void>, fallback: string, done?: string) {
    if (busy) return;
    busy = id;
    try {
      await action();
      if (done) toast(done, 'success');
      await refresh();
      if (id.startsWith('block:') || id.startsWith('unblock:')) await loadBlocks();
    } catch (err) {
      report(err, fallback);
    } finally {
      busy = '';
    }
  }

  function onMenu(user: UserCard, item: string) {
    if (item === 'profile') {
      navigate('user', { username: user.username });
      return;
    }
    if (item === 'unfriend') {
      run(`unfriend:${user.id}`, () => unfriend(user.id), msg('social.friendsUnfriendFailed'), msg('social.friendsUnfriended'));
      return;
    }
    run(`block:${user.id}`, () => block(user.id), msg('social.blockFailed'), msg('social.userBlocked'));
  }

  function signIn(view: 'login' | 'register') {
    leaveGuest(view).catch((err) => report(err, msg('social.signInFailed')));
  }

  function openProfile(user: UserCard) {
    navigate('user', { username: user.username });
  }

  $effect(() => {
    const next = initialTab;
    untrack(() => {
      if (next && tabs.some((t) => t.id === next)) tab = next;
    });
  });

  $effect(() => {
    if (!isGuest && !$needsSocialConsent) loadBlocks();
  });

  onMount(() => {
    if (!isGuest && !$needsSocialConsent) reload();
  });
</script>

<Card surface="panel">
  <PageHeader title={msg('social.friendsTitle')} subtitle={msg('social.friendsSubtitle')}>
    {#snippet actions()}
      {#if !isGuest && !$needsSocialConsent}
        <Button onclick={() => (codeOpen = !codeOpen)} pressed={codeOpen}>{msg('social.friendsMyCodeButton')}</Button>
        <Button variant="primary" onclick={() => (addOpen = true)}>
          <UserPlus size="1.5rem" strokeWidth={1.8} />
          {msg('social.friendsAddFriend')}
        </Button>
      {/if}
    {/snippet}
  </PageHeader>

  {#if isGuest}
    <EmptyState
      title={msg('social.friendsGuestTitle')}
      description={msg('social.friendsGuestDesc')}
    >
      {#snippet icon()}
        <Users size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => signIn('login')}>
          <LogIn size="1.5rem" strokeWidth={1.8} />
          {msg('social.signInButton')}
        </Button>
        <Button onclick={() => signIn('register')}>{msg('social.createAccountButton')}</Button>
      {/snippet}
    </EmptyState>
  {:else if $needsSocialConsent}
    <EmptyState
      title={msg('social.friendsConsentTitle')}
      description={msg('social.friendsConsentDesc')}
    >
      {#snippet icon()}
        <Users size="2.2rem" strokeWidth={1.6} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => (consentOpen = true)}>{msg('social.friendsEnableSync')}</Button>
      {/snippet}
    </EmptyState>
  {:else}
    {#if codeOpen}
      <div class="code">
        <FriendCodeCard bind:code={myCode} />
      </div>
    {/if}

    <Tabs {tabs} bind:value={tab} variant="pill" />

    {#if presenceTabs.has(tab)}
      <div class="toolbar">
        <div class="search-wrap">
          <SearchInput bind:value={search} placeholder={msg('social.friendsSearchFriendsPlaceholder')} />
        </div>
        <div class="toolbar-right">
          <DropdownMenu
            items={[
              { id: 'name', label: sortLabels.name },
              { id: 'status', label: sortLabels.status },
            ]}
            onselect={(id) => (sortBy = id as 'name' | 'status')}
          >
            {#snippet trigger({ open, toggle })}
              <button class="sort" class:open onclick={toggle}>
                <ArrowDownUp size="1.4rem" strokeWidth={1.8} />
                {msg('social.friendsSortLabel', { value: sortLabels[sortBy] })}
                <ChevronDown size="1.4rem" strokeWidth={1.8} />
              </button>
            {/snippet}
          </DropdownMenu>
          <SegmentedControl
            bind:value={$friendsView}
            options={[
              { id: 'list', label: msg('social.friendsViewList') },
              { id: 'grid', label: msg('social.friendsViewGrid') },
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

      {#if visibleFriends.length === 0}
        {#if page.friends.length === 0}
          <EmptyState title={msg('social.friendsEmptyNobodyTitle')} description={msg('social.friendsEmptyNobodyDesc')} />
        {:else if search.trim()}
          <EmptyState title={msg('social.friendsEmptySearchTitle')} description={msg('social.friendsEmptySearchDesc')} />
        {:else if groupEmptyCopy[tab]}
          <EmptyState title={groupEmptyCopy[tab].title} description={groupEmptyCopy[tab].description} />
        {:else}
          <EmptyState title={msg('social.friendsEmptyNobodyTitle')} description={msg('social.friendsEmptyNobodyDesc')} />
        {/if}
      {:else if $friendsView === 'grid'}
        <div class="grid">
          {#each visibleFriends as friend (friend.id)}
            <FriendRow
              user={friend}
              status={presenceDot(friend.presence)}
              meta={presenceLine(friend.presence)}
              game={playingGame(friend)}
              variant="grid"
              onopen={() => openProfile(friend)}
            >
              {#snippet actions()}
                <DropdownMenu items={menuItems} onselect={(item) => onMenu(friend, item)}>
                  {#snippet trigger({ toggle })}
                    <IconButton label={msg('social.moreLabel')} size="sm" onclick={toggle}>
                      <EllipsisVertical size="1.7rem" strokeWidth={1.8} />
                    </IconButton>
                  {/snippet}
                </DropdownMenu>
              {/snippet}
            </FriendRow>
          {/each}
        </div>
      {:else}
        <div class="list">
          {#each visibleFriends as friend (friend.id)}
            <FriendRow
              user={friend}
              status={presenceDot(friend.presence)}
              meta={presenceLine(friend.presence)}
              game={playingGame(friend)}
              variant="list"
              onopen={() => openProfile(friend)}
            >
              {#snippet actions()}
                <DropdownMenu items={menuItems} onselect={(item) => onMenu(friend, item)}>
                  {#snippet trigger({ toggle })}
                    <IconButton label={msg('social.moreLabel')} size="sm" onclick={toggle}>
                      <EllipsisVertical size="1.7rem" strokeWidth={1.8} />
                    </IconButton>
                  {/snippet}
                </DropdownMenu>
              {/snippet}
            </FriendRow>
          {/each}
        </div>
      {/if}
    {:else if tab === 'requests'}
      {#if page.incoming.length === 0 && page.outgoing.length === 0}
        <EmptyState title={msg('social.friendsEmptyRequestsTitle')} description={msg('social.friendsEmptyRequestsDesc')} />
      {:else}
        {#if page.incoming.length > 0}
          <h4 class="eyebrow">{msg('social.friendsIncomingHeading')}</h4>
          <div class="cards">
            {#each page.incoming as request (request.id)}
              <FriendRow user={request} variant="card" stats={incomingStats(request)} onopen={() => openProfile(request)}>
                {#snippet actions()}
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={!!busy}
                    onclick={() =>
                      run(`accept:${request.id}`, () => accept(request.id), msg('social.acceptRequestFailed'), msg('social.friendsNowFriends'))}
                  >
                    {msg('social.relationIncoming')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!!busy}
                    onclick={() => run(`decline:${request.id}`, () => decline(request.id), msg('social.declineRequestFailed'))}
                  >
                    {msg('social.declineButton')}
                  </Button>
                {/snippet}
              </FriendRow>
            {/each}
          </div>
        {/if}

        {#if page.outgoing.length > 0}
          <h4 class="eyebrow">{msg('social.friendsSentHeading')}</h4>
          <div class="cards">
            {#each page.outgoing as request (request.id)}
              <FriendRow user={request} variant="card" stats={outgoingStats(request)} onopen={() => openProfile(request)}>
                {#snippet actions()}
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!!busy}
                    onclick={() => run(`cancel:${request.id}`, () => decline(request.id), msg('social.cancelRequestFailed'))}
                  >
                    {msg('social.cancelRequestButton')}
                  </Button>
                {/snippet}
              </FriendRow>
            {/each}
          </div>
        {/if}
      {/if}

      <div class="safety">
        <ShieldCheck size="2rem" strokeWidth={1.6} />
        <div class="safety-text">
          <h4>{msg('social.friendsSafetyTitle')}</h4>
          <p>{msg('social.friendsSafetyDesc')}</p>
        </div>
      </div>
    {:else if blocked.length === 0}
      <EmptyState
        title={msg('social.friendsEmptyBlockedTitle')}
        description={msg('social.friendsEmptyBlockedDesc')}
      />
    {:else}
      <div class="cards">
        {#each blocked as user (user.id)}
          <FriendRow {user} variant="card">
            {#snippet actions()}
              <Button
                size="sm"
                disabled={!!busy}
                onclick={() =>
                  run(`unblock:${user.id}`, () => unblock(user.id), msg('social.unblockFailed'), msg('social.userUnblocked'))}
              >
                {msg('social.unblockButton')}
              </Button>
            {/snippet}
          </FriendRow>
        {/each}
      </div>
    {/if}
  {/if}
</Card>

{#if !isGuest && !$needsSocialConsent}
  <AddFriendModal bind:open={addOpen} onsent={reload} />
{/if}
<SocialConsentScreen bind:open={consentOpen} />

<style>
  .code {
    margin-bottom: var(--space-5);
  }

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

  .sort {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: var(--control-sm);
    padding: 0 1.1rem;
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
    white-space: nowrap;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .sort:hover,
  .sort.open {
    background: var(--hover);
    color: var(--text);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .list :global(.row + .row) {
    border-top: 1px solid var(--border);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(22rem, 1fr));
    gap: var(--space-4);
  }

  .cards {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .eyebrow {
    margin: var(--space-6) 0 var(--space-3);
    padding: 0 0.2rem;
    font-size: 1.2rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-3);
  }

  .safety {
    display: flex;
    align-items: flex-start;
    gap: var(--space-4);
    margin-top: var(--space-6);
    padding: var(--space-4) var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text-3);
  }

  .safety-text h4 {
    font-size: var(--font-sm);
    font-weight: 600;
    color: var(--text);
  }

  .safety-text p {
    margin-top: 0.3rem;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }
</style>
