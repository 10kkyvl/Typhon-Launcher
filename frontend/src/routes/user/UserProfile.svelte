<script lang="ts">
  import { LogIn, UserRound } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { AccountError } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import type { PublicProfile } from '../../lib/services/social';
  import {
    accept,
    block,
    decline,
    profile as fetchProfile,
    refresh,
    sendRequest,
    unfriend,
  } from '../../lib/services/social';
  import { openGameByIGDB } from '../../lib/social/openGame';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import { msg } from '../../lib/i18n';
  import UserActivity from './UserActivity.svelte';
  import UserCommon from './UserCommon.svelte';
  import UserCovers from './UserCovers.svelte';
  import UserHeader from './UserHeader.svelte';
  import UserMutual from './UserMutual.svelte';
  import UserRecent from './UserRecent.svelte';

  let { username }: { username?: string } = $props();

  let data = $state<PublicProfile | null>(null);
  let loading = $state(true);
  let refreshing = $state(false);
  let failure = $state('');
  let missing = $state(false);
  let busy = $state(false);

  const isGuest = $derived($authState === 'guest');
  const name = $derived(data ? data.displayName || data.username : '');
  const stranger = $derived(!!data && data.relation !== 'friend' && data.relation !== 'self');
  const closed = $derived(!!data && data.visibility === 'private' && data.relation !== 'self');
  const restricted = $derived(!!data && data.visibility === 'friends' && stranger);
  const common = $derived(data && !closed && data.common && data.common.count > 0 ? data.common : null);
  const recent = $derived(data && !closed ? data.recentlyPlayed : []);
  const activity = $derived(data && !closed ? data.recentActivity : []);
  const favorites = $derived(data && !closed ? data.favorites : []);
  const mutual = $derived(data && !closed && data.mutualCount > 0 ? data.mutualFriends : []);

  const presenceGame = $derived.by(() => {
    if (!data || closed) return null;
    const gameId = data.presence?.gameId;
    if (gameId == null) return null;
    const cover =
      data.recentlyPlayed.find((g) => g.igdbId === gameId)?.coverUrl ??
      data.favorites.find((g) => g.igdbId === gameId)?.coverUrl ??
      data.common?.games.find((g) => g.igdbId === gameId)?.coverUrl ??
      '';
    return { igdbId: gameId, title: data.presence?.gameTitle ?? '', coverUrl: cover };
  });

  async function load(target: string, quiet = false) {
    if (quiet) {
      refreshing = true;
    } else {
      loading = true;
      failure = '';
      missing = false;
    }
    try {
      const loaded = await fetchProfile(target);
      if (target !== username) return;
      if (loaded.relation === 'self') {
        navigate('profile');
        return;
      }
      data = loaded;
      failure = '';
      missing = false;
    } catch (err) {
      if (target !== username) return;
      if (quiet) {
        toast(accountErrorText(err, msg('social.userRefreshFailed')), 'danger');
        return;
      }
      data = null;
      missing = err instanceof AccountError && err.code === 'user_not_found';
      failure = accountErrorText(err, msg('social.userLoadFailed'));
    } finally {
      if (target === username) {
        loading = false;
        refreshing = false;
      }
    }
  }

  async function act(id: string) {
    const current = data;
    if (!current || busy) return;
    busy = true;
    try {
      let done = '';
      if (id === 'add') {
        const result = await sendRequest(current.username);
        done = result.accepted ? msg('social.friendsNowFriends') : msg('social.relationOutgoing');
      } else if (id === 'cancel') {
        await decline(current.id);
        done = msg('social.requestCancelled');
      } else if (id === 'decline') {
        await decline(current.id);
        done = msg('social.requestDeclined');
      } else if (id === 'accept') {
        await accept(current.id);
        done = msg('social.friendsNowFriends');
      } else if (id === 'unfriend') {
        await unfriend(current.id);
        done = msg('social.friendsUnfriended');
      } else if (id === 'block') {
        await block(current.id);
        await refresh();
        toast(msg('social.userBlocked'), 'success');
        navigate('friends', { tab: 'blocked' });
        return;
      } else {
        return;
      }
      toast(done, 'success');
      await refresh();
      await load(current.username, true);
    } catch (err) {
      toast(accountErrorText(err, msg('social.actionFailed')), 'danger');
    } finally {
      busy = false;
    }
  }

  function signIn(view: 'login' | 'register') {
    leaveGuest(view).catch((err) => toast(accountErrorText(err, msg('social.signInFailed')), 'danger'));
  }

  $effect(() => {
    const target = username ?? '';
    if (isGuest) {
      data = null;
      loading = false;
      missing = false;
      failure = '';
      return;
    }
    if (!target) {
      data = null;
      loading = false;
      missing = true;
      failure = '';
      return;
    }
    void load(target);
  });
</script>

<PageHeader title={username ? `@${username}` : msg('social.profileLabel')} />

{#if isGuest}
  <EmptyState
    title={msg('social.userGuestTitle')}
    description={msg('social.userGuestDesc')}
  >
    {#snippet icon()}
      <UserRound size="2.2rem" strokeWidth={1.6} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={() => signIn('login')}>
        <LogIn size="1.5rem" strokeWidth={1.8} />
        {msg('social.signInButton')}
      </Button>
      <Button onclick={() => signIn('register')}>{msg('social.createAccountButton')}</Button>
    {/snippet}
  </EmptyState>
{:else if loading}
  <p class="muted">{msg('social.loadingEllipsis')}</p>
{:else if missing}
  <EmptyState title={msg('social.userNotFoundTitle')} description={msg('social.userNotFoundDesc')}>
    {#snippet icon()}
      <UserRound size="2.2rem" strokeWidth={1.6} />
    {/snippet}
  </EmptyState>
{:else if failure}
  <EmptyState title={msg('social.userLoadFailed')} description={failure}>
    {#snippet icon()}
      <UserRound size="2.2rem" strokeWidth={1.6} />
    {/snippet}
    {#snippet actions()}
      <Button variant="primary" onclick={() => load(username ?? '')}>{msg('common.retry')}</Button>
    {/snippet}
  </EmptyState>
{:else if data}
  <div class="profile" class:refreshing>
    <UserHeader profile={data} {busy} onaction={act} />
    {#if closed}
      <p class="muted note">{msg('social.userProfileClosed')}</p>
    {:else if restricted}
      <p class="muted note">{msg('social.userRestToFriends')}</p>
    {:else}
      <div class="columns">
        <div class="main">
          {#if recent.length > 0}
            <UserRecent games={recent} />
          {/if}
          {#if common}
            <UserCommon {common} {name} />
          {/if}
          <div class="pair">
            <div class="pair-left">
              {#if activity.length > 0}
                <UserActivity items={activity} />
              {/if}
            </div>
            <div class="pair-right">
              <UserCovers title={msg('social.favoriteGamesTitle')} games={favorites} hearts />
            </div>
          </div>
        </div>
        <div class="side">
          {#if presenceGame}
            <Card title={msg('social.nowPlayingTitle')}>
              <button class="playing" type="button" onclick={() => openGameByIGDB(presenceGame.igdbId, presenceGame.title)}>
                <span class="cover">
                  <Artwork src={presenceGame.coverUrl} alt={presenceGame.title} ratio="16 / 9" radius="var(--radius-md)" />
                </span>
                <span class="title">{presenceGame.title}</span>
                <StatusBadge kind="success" label={msg('social.playing')} plain />
              </button>
            </Card>
          {/if}
          {#if data.bio}
            <Card title={msg('social.aboutTitle')}>
              <p class="bio">{data.bio}</p>
            </Card>
          {/if}
          {#if mutual.length > 0}
            <UserMutual friends={mutual} count={data.mutualCount} />
          {/if}
        </div>
      </div>
    {/if}
  </div>
{/if}

<style>
  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .note {
    margin-bottom: var(--space-10);
  }

  .profile {
    display: flex;
    flex-direction: column;
    transition: opacity var(--dur) var(--ease);
  }

  .profile.refreshing {
    opacity: 0.6;
  }

  .columns {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
  }

  .main,
  .side {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    min-width: 0;
  }

  .pair {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-6);
    align-items: start;
  }

  .pair-left,
  .pair-right {
    display: contents;
  }

  .pair > :global(*) {
    min-width: 0;
  }

  .bio {
    font-size: var(--font-sm);
    line-height: 1.55;
    color: var(--text-2);
    overflow-wrap: anywhere;
    white-space: pre-wrap;
  }

  .playing {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.8rem;
    width: 100%;
    padding: 0;
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .playing .cover {
    display: block;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    transition: transform var(--dur) var(--ease);
  }

  .playing:hover .cover {
    transform: scale(1.01);
  }

  .playing .title {
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    line-height: 1.3;
  }

  @media (min-width: 1600px) {
    .columns {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 40rem;
      gap: 0 var(--space-6);
      align-items: start;
    }
  }

  @media (max-width: 1200px) {
    .pair {
      grid-template-columns: 1fr;
    }
  }
</style>
