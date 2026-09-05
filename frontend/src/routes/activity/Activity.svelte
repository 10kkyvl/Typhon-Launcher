<script lang="ts">
  import { onMount } from 'svelte';
  import { LogIn, Users } from '@lucide/svelte';
  import Avatar from '../../lib/components/Avatar.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import FeedRow from '../../lib/components/FeedRow.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SocialConsentScreen from '../../lib/components/SocialConsentScreen.svelte';
  import StatTile from '../../lib/components/StatTile.svelte';
  import { AccountError } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import type { FriendView } from '../../lib/services/social';
  import { weekSummary } from '../../lib/profile/week';
  import { feedDayGroups } from '../../lib/social/feed';
  import { popularGames } from '../../lib/social/popular';
  import { isPlaying, presenceDot, sortFriends, statusDot } from '../../lib/social/presence';
  import { feedCursor, feedEvents, feedLoading, loadFeed, moreFeed, noteEvent, reactToEvent } from '../../lib/stores/feed';
  import { loadArt } from '../../lib/stores/metadata';
  import { presenceStatus } from '../../lib/stores/presence';
  import { initProfile, profileSnapshot } from '../../lib/stores/profile';
  import { navigate } from '../../lib/stores/router';
  import { friendsPage, needsSocialConsent } from '../../lib/stores/social';
  import { toast } from '../../lib/stores/toasts';
  import { authState, currentUser, leaveGuest } from '../../lib/stores/user';
  import { formatCount } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import FriendRow from '../friends/FriendRow.svelte';
  import PopularGames from './PopularGames.svelte';
  import WeekActivity from './WeekActivity.svelte';

  const PLAYING_LIMIT = 6;

  let consentOpen = $state(false);

  const isGuest = $derived($authState === 'guest');
  const feedGroups = $derived(feedDayGroups($feedEvents));

  const user = $derived($currentUser);
  const displayName = $derived(user?.displayName || user?.username || '');
  const stats = $derived($profileSnapshot.stats);
  const ownDot = $derived(statusDot($presenceStatus));

  const playingFriends = $derived(
    sortFriends($friendsPage.friends).filter((friend) => isPlaying(friend.presence)),
  );
  const popular = $derived(popularGames($feedEvents, $friendsPage.friends, user?.id ?? ''));
  const week = $derived(weekSummary($profileSnapshot.activity));

  function report(err: unknown, fallback: string) {
    if (err instanceof AccountError && err.code === 'unauthenticated') return;
    toast(accountErrorText(err, fallback), 'danger');
  }

  function signIn(view: 'login' | 'register') {
    leaveGuest(view).catch((err) => report(err, msg('transfers.activityOpenSignInError')));
  }

  function openUser(friend: FriendView) {
    navigate('user', { username: friend.username });
  }

  function playingGame(friend: FriendView): { igdbId: number; title: string } | null {
    const gameId = friend.presence?.gameId;
    if (!gameId) return null;
    return { igdbId: gameId, title: friend.presence?.gameTitle ?? '' };
  }

  $effect(() => {
    if (!isGuest && !$needsSocialConsent) void loadFeed(true);
  });

  $effect(() => {
    const ids = week.games.map((item) => item.game.canonicalGameId).filter((id): id is string => Boolean(id));
    if (ids.length > 0) void loadArt(ids);
  });

  onMount(() => {
    initProfile();
  });
</script>

<PageHeader title={msg('transfers.activityTitle')} subtitle={msg('transfers.activitySubtitle')} />

{#if isGuest}
  <EmptyState
    title={msg('transfers.activityGuestTitle')}
    description={msg('transfers.activityGuestDescription')}
  >
    {#snippet icon()}
      <Users size="2.2rem" strokeWidth={1.6} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={() => signIn('login')}>
        <LogIn size="1.5rem" strokeWidth={1.8} />
        {msg('transfers.activitySignIn')}
      </Button>
      <Button onclick={() => signIn('register')}>{msg('transfers.activityCreateAccount')}</Button>
    {/snippet}
  </EmptyState>
{:else if $needsSocialConsent}
  <EmptyState
    title={msg('transfers.activityConsentTitle')}
    description={msg('transfers.activityConsentDescription')}
  >
    {#snippet icon()}
      <Users size="2.2rem" strokeWidth={1.6} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={() => (consentOpen = true)}>{msg('transfers.activityEnableSync')}</Button>
    {/snippet}
  </EmptyState>
{:else}
  <div class="layout">
    <div class="left">
      <Card>
        <div class="profile">
          <Avatar size="lg" name={displayName} src={user?.avatarUrl} status={ownDot} />
          <h3 class="name">{displayName}</h3>
          {#if user}<span class="handle">@{user.username}</span>{/if}
          {#if user?.bio}<p class="bio">{user.bio}</p>{/if}
        </div>
        <div class="stats">
          <StatTile value={formatCount(stats.games)} label={msg('transfers.activityStatGames')} />
          <StatTile value={formatCount(stats.hours)} label={msg('transfers.activityStatHours')} />
          <StatTile value={formatCount(stats.completed)} label={msg('transfers.activityStatCompleted')} />
        </div>
      </Card>

      <WeekActivity {week} />
    </div>

    <div class="center">
      {#if $feedEvents.length === 0}
        {#if $feedLoading}
          <p class="muted">{msg('transfers.activityLoading')}</p>
        {:else}
          <EmptyState
            title={msg('transfers.activityFeedEmptyTitle')}
            description={msg('transfers.activityFeedEmptyDescription')}
          />
        {/if}
      {:else}
        {#each feedGroups as group (group.key)}
          <h4 class="eyebrow">{group.label}</h4>
          {#each group.events as event (event.id)}
            <Card>
              <FeedRow
                {event}
                own={event.user.id === user?.id}
                onreact={(emoji) => reactToEvent(event.id, emoji)}
                onnote={(note) => noteEvent(event.id, note)}
              />
            </Card>
          {/each}
        {/each}
        {#if $feedCursor > 0}
          <div class="more">
            <Button disabled={$feedLoading} onclick={() => moreFeed()}>{msg('transfers.activityShowMore')}</Button>
          </div>
        {/if}
      {/if}
    </div>

    <div class="right">
      <Card title={msg('transfers.activityPlayingTitle')}>
        {#if playingFriends.length === 0}
          <p class="muted">{msg('transfers.activityNoOnePlaying')}</p>
        {:else}
          <div class="friends">
            {#each playingFriends.slice(0, PLAYING_LIMIT) as friend (friend.id)}
              <FriendRow user={friend} status={presenceDot(friend.presence)} game={playingGame(friend)} onopen={() => openUser(friend)} />
            {/each}
          </div>
        {/if}
      </Card>

      <PopularGames games={popular} />
    </div>
  </div>
{/if}

<SocialConsentScreen bind:open={consentOpen} />

<style>
  .layout {
    display: grid;
    grid-template-columns: 30rem minmax(0, 92rem) 30rem;
    justify-content: space-between;
    gap: var(--space-6);
    align-items: start;
  }

  .left,
  .right {
    position: sticky;
    top: var(--space-4);
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    min-width: 0;
    max-height: calc(100vh - var(--topbar-h) - var(--space-8));
    overflow-y: auto;
  }

  .center {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
  }

  .profile {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.4rem;
    text-align: center;
  }

  .name {
    margin-top: var(--space-2);
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .handle {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .bio {
    margin-top: 0.4rem;
    font-size: var(--font-sm);
    line-height: 1.5;
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .stats {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--space-3);
    margin-top: var(--space-5);
    padding-top: var(--space-5);
    border-top: 1px solid var(--border);
    text-align: center;
  }

  .stats :global(.tile) {
    align-items: center;
  }

  .friends {
    display: flex;
    flex-direction: column;
    margin: 0 calc(var(--space-6) * -1);
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .more {
    display: flex;
    justify-content: center;
    padding: var(--space-5) 0;
  }

  .eyebrow {
    font-size: 1.2rem;
    font-weight: 600;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-3);
  }

  @media (max-width: 1400px) {
    .layout {
      grid-template-columns: 30rem minmax(0, 1fr);
    }

    .right {
      display: none;
    }
  }

  @media (max-width: 1100px) {
    .layout {
      grid-template-columns: minmax(0, 1fr);
    }

    .left {
      display: none;
    }
  }
</style>
