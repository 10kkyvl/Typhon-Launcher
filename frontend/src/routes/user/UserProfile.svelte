<script lang="ts">
  import { Lock, LogIn, UserRound } from '@lucide/svelte';
  import ProfileCanvas from '../../lib/components/ProfileCanvas.svelte';
  import ProfileCover from '../../lib/components/ProfileCover.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
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
  import { blockPrompt, unfriendPrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import { showcaseLabel } from '../../lib/profile/view';
  import { wideArt } from '../../lib/social/art';
  import { openGameByIGDB } from '../../lib/social/openGame';
  import { navigate } from '../../lib/stores/router';
  import { openChat } from '../../lib/stores/messaging';
  import { toast } from '../../lib/stores/toasts';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import { msg } from '../../lib/i18n';
  import ProfilePlaying from '../profile/ProfilePlaying.svelte';
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
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);

  const isGuest = $derived($authState === 'guest');
  const name = $derived(data ? data.displayName || data.username : '');
  const stranger = $derived(!!data && data.relation !== 'friend' && data.relation !== 'self');
  const closed = $derived(!!data && data.visibility === 'private' && data.relation !== 'self');
  const restricted = $derived(!!data && data.visibility === 'friends' && stranger);
  const common = $derived(data && !closed && data.common && data.common.count > 0 ? data.common : null);
  const recent = $derived(data && !closed ? data.recentlyPlayed : []);
  const activity = $derived(data && !closed ? data.recentActivity : []);
  const mutual = $derived(data && !closed && data.mutualCount > 0 ? data.mutualFriends : []);

  const presenceGame = $derived.by(() => {
    if (!data || closed) return null;
    const gameId = data.presence?.gameId;
    if (gameId == null) return null;
    const known =
      data.recentlyPlayed.find((g) => g.igdbId === gameId) ??
      data.favorites.find((g) => g.igdbId === gameId) ??
      data.common?.games.find((g) => g.igdbId === gameId);
    return {
      igdbId: gameId,
      title: data.presence?.gameTitle ?? '',
      coverUrl: known?.coverUrl ?? '',
      heroUrl: known?.heroUrl ?? '',
    };
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

  function act(id: string) {
    const current = data;
    if (!current || busy) return;
    if (id !== 'unfriend' && id !== 'block') {
      void run(id);
      return;
    }
    const label = current.displayName || current.username;
    pending = {
      prompt: id === 'unfriend' ? unfriendPrompt(label) : blockPrompt(label, current.relation === 'friend'),
      run: () => run(id),
    };
  }

  async function run(id: string) {
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
    <ProfileCanvas appearance={data.appearance}>
    <ProfileCover appearance={data.appearance} />
    <UserHeader profile={data} {busy} onaction={act} onmessage={() => data && openChat(data)} />
    {#if closed}
      <p class="notice"><Lock size="1.8rem" strokeWidth={1.6} />{msg('social.userProfileClosed')}</p>
    {:else if restricted}
      <p class="notice"><Lock size="1.8rem" strokeWidth={1.6} />{msg('social.userRestToFriends')}</p>
    {:else}
      <div class="columns">
        <div class="main">
          {#if presenceGame}
            <ProfilePlaying
              title={presenceGame.title}
              art={wideArt(presenceGame)}
              onopen={() => openGameByIGDB(presenceGame.igdbId, presenceGame.title)}
            />
          {/if}
          {#each data.showcase ?? [] as block (block.kind)}
            <UserCovers title={showcaseLabel(block.kind)} games={block.games} hearts={block.kind === 'favorites'} />
          {/each}
          {#if recent.length > 0}
            <UserRecent games={recent} />
          {/if}
          {#if common}
            <UserCommon {common} {name} />
          {/if}
          {#if mutual.length > 0}
            <UserMutual friends={mutual} count={data.mutualCount} />
          {/if}
          {#if activity.length > 0}
            <UserActivity items={activity} />
          {/if}
        </div>
      </div>
    {/if}
    </ProfileCanvas>
  </div>
{/if}

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}

<style>
  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .notice {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .notice :global(svg) {
    flex-shrink: 0;
    color: var(--text-3);
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

  .main {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    min-width: 0;
  }
</style>
