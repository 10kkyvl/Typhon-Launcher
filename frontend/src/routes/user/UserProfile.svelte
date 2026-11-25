<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { Lock, LogIn, UserRound } from '@lucide/svelte';
  import ProfileCanvas from '../../lib/components/ProfileCanvas.svelte';
  import ProfileCover from '../../lib/components/ProfileCover.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import AboutBlock from '../../lib/components/profile/AboutBlock.svelte';
  import BlockGrid from '../../lib/components/profile/BlockGrid.svelte';
  import { AccountError } from '../../lib/services/account';
  import { accountErrorText } from '../../lib/services/accountMessages';
  import { chatPeerOf, type PublicProfile } from '../../lib/services/social';
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
  import { publicLayout, resolvePublic, type GridBlock } from '../../lib/profile/layoutView';
  import { wideArt } from '../../lib/social/art';
  import { openGameByIGDB } from '../../lib/social/openGame';
  import { navigate } from '../../lib/stores/router';
  import { openChat } from '../../lib/stores/messaging';
  import { toast } from '../../lib/stores/toasts';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { authState, leaveGuest } from '../../lib/stores/user';
  import { msg } from '../../lib/i18n';
  import type { SocialKey } from '../../lib/i18n/catalog/ru/social';
  import HiddenBadge from '../profile/HiddenBadge.svelte';
  import ProfilePlaying from '../profile/ProfilePlaying.svelte';
  import UserActivity from './UserActivity.svelte';
  import { inviteToPlay } from '../../lib/social/invite';
  import UserCommon from './UserCommon.svelte';
  import UserHeader from './UserHeader.svelte';
  import UserMutual from './UserMutual.svelte';
  import UserRecent from './UserRecent.svelte';
  import UserStats from './UserStats.svelte';

  let { username }: { username?: string } = $props();

  let data = $state<PublicProfile | null>(null);
  let loading = $state(true);
  let refreshing = $state(false);
  let failure = $state('');
  let missing = $state(false);
  type Action = 'add' | 'cancel' | 'accept' | 'decline' | 'unfriend' | 'block';
  let busy = $state<Action | ''>('');
  let pending = $state<{ prompt: ConfirmPrompt; action: Action } | null>(null);

  // What failed, when the answer carries no code of its own.
  const FAILED: Record<Action, SocialKey> = {
    add: 'social.friendsSendRequestFailed',
    cancel: 'social.cancelRequestFailed',
    accept: 'social.acceptRequestFailed',
    decline: 'social.declineRequestFailed',
    unfriend: 'social.friendsUnfriendFailed',
    block: 'social.blockFailed',
  };
  const failedText = (action: Action, err: unknown) => accountErrorText(err, msg(FAILED[action]));

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

  function empty(type: string): boolean {
    if (type === 'playing') return !presenceGame;
    if (type === 'recent') return recent.length === 0;
    if (type === 'activity') return activity.length === 0;
    if (type === 'about') return !data || data.bio.trim() === '';
    return false;
  }

  const gridBlocks = $derived(
    data && !closed
      ? resolvePublic(publicLayout(data), { profile: data, open: (card) => void openGameByIGDB(card.igdbId, card.title), empty })
      : [],
  );
  const autoArt = $derived(wideArt(data?.autoGame) || undefined);

  // A retry keeps the failure on screen, its Retry busy, until the answer replaces it. A quiet read
  // keeps the page as it is and answers its failure, for the caller to say.
  async function load(target: string, quiet = false, retry = false): Promise<unknown> {
    if (quiet) {
      refreshing = true;
    } else {
      loading = true;
      if (!retry) failure = '';
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
      if (quiet) return err;
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

  let retrying = $state(false);
  let missingState = $state<HTMLElement>();
  let profileBox = $state<HTMLElement>();
  const loadFailedTitle = msg('social.userLoadFailed');

  // The notice's retry: it worked when the failure is gone, whether a profile came or the answer is
  // that there is no such user. Focus then goes into the profile, or to the "not found" state.
  async function retryLoad(): Promise<boolean> {
    await load(username ?? '', false, true);
    return missing || !failure;
  }

  function act(id: string) {
    const current = data;
    const action = id as Action;
    if (!current || busy || !(action in FAILED)) return;
    if (action !== 'unfriend' && action !== 'block') {
      const handOff = focusHandOff();
      perform(action).then(
        () => handOff(relationControl),
        (err: unknown) => toast(failedText(action, err), 'danger'),
      );
      return;
    }
    const label = current.displayName || current.username;
    pending = {
      prompt: action === 'unfriend' ? unfriendPrompt(label) : blockPrompt(label, current.relation === 'friend'),
      action,
    };
  }

  async function change(action: Action, current: PublicProfile): Promise<string> {
    if (action === 'add') {
      const result = await sendRequest(current.username);
      return result.accepted ? msg('social.friendsNowFriends') : msg('social.relationOutgoing');
    }
    if (action === 'cancel') {
      await decline(current.id);
      return msg('social.requestCancelled');
    }
    if (action === 'decline') {
      await decline(current.id);
      return msg('social.requestDeclined');
    }
    if (action === 'accept') {
      await accept(current.id);
      return msg('social.friendsNowFriends');
    }
    if (action === 'unfriend') {
      await unfriend(current.id);
      return msg('social.friendsUnfriended');
    }
    await block(current.id);
    return msg('social.userBlocked');
  }

  // Rejects only when the action itself failed: the caller says that where the player looks. Once it is
  // done it is said as done, and a failed re-read after it is said as a failed read.
  async function perform(action: Action): Promise<void> {
    const current = data;
    if (!current || busy) return;
    busy = action;
    try {
      toast(await change(action, current), 'success');
      if (action === 'block') {
        // The blocked players are on the friends screen, and the friends list is what refresh reads.
        await refresh().catch((err) => toast(accountErrorText(err, msg('social.friendsRefreshFailed')), 'danger'));
        navigate('friends', { tab: 'blocked' });
        return;
      }
      const listFailed = await refresh().then(
        () => null,
        (err: unknown) => err ?? new Error('refresh'),
      );
      const failed = (await load(current.username, true)) ?? listFailed;
      if (failed) toast(accountErrorText(failed, msg('social.userRefreshFailed')), 'danger');
    } finally {
      busy = '';
    }
  }

  // UserHeader marks the control that stands for the relation the page now shows.
  const relationControl = () => profileBox?.querySelector<HTMLElement>('[data-relation-control]');

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

{#snippet external(block: GridBlock)}
  {#if data}
    {#if block.type === 'playing' && presenceGame}
      <ProfilePlaying
        title={presenceGame.title}
        art={wideArt(presenceGame)}
        onopen={() => openGameByIGDB(presenceGame.igdbId, presenceGame.title)}
      />
    {:else if block.type === 'recent'}
      <UserRecent games={recent} />
    {:else if block.type === 'activity'}
      <UserActivity items={activity} />
    {:else if block.type === 'stats'}
      <Card title={msg('profile.blockStats')}>
        <UserStats stats={data.stats} />
      </Card>
    {:else if block.type === 'about'}
      <AboutBlock bio={data.bio} />
    {/if}
  {/if}
{/snippet}

{#snippet masked()}
  <HiddenBadge text={msg('social.statsHiddenHint')} />
{/snippet}

<Page>
  <PageHeader title={username ? `@${username}` : msg('social.profileLabel')} />

  {#if isGuest}
    <EmptyState level={2}
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
  {:else if loading && !failure}
    <Spinner label={msg('social.loadingEllipsis')} />
  {:else if missing}
    <!-- Takes the focus when a retry ends here: the heading of an empty state cannot. -->
    <div class="missing" tabindex="-1" bind:this={missingState}>
      <EmptyState level={2} title={msg('social.userNotFoundTitle')} description={msg('social.userNotFoundDesc')}>
        {#snippet icon()}
          <UserRound size="2.2rem" strokeWidth={1.6} />
        {/snippet}
      </EmptyState>
    </div>
  {:else if failure || retrying}
    <!-- A failure without a reason of its own is the one sentence, not a title and the same text. -->
    <Notice
      tone="danger"
      title={failure !== loadFailedTitle ? loadFailedTitle : undefined}
      level={failure !== loadFailedTitle ? 2 : undefined}
      bind:retrying
      retry={{ label: msg('common.retry'), run: retryLoad, focus: () => missingState, within: () => profileBox }}
    >
      {failure}
    </Notice>
  {:else if data}
    <div class="profile" class:refreshing bind:this={profileBox}>
      <ProfileCanvas appearance={data.appearance} {autoArt}>
      <ProfileCover appearance={data.appearance} {autoArt} />
      <UserHeader profile={data} {busy} onaction={act} onmessage={() => data && openChat(chatPeerOf(data))} />
      {#if closed}
        <Notice tone="info" announce={false}>
          {#snippet icon()}<Lock size="1.6rem" strokeWidth={1.8} />{/snippet}
          {msg('social.userProfileClosed')}
        </Notice>
      {:else if restricted}
        <Notice tone="info" announce={false}>
          {#snippet icon()}<Lock size="1.6rem" strokeWidth={1.8} />{/snippet}
          {msg('social.userRestToFriends')}
        </Notice>
      {:else}
        <div class="columns">
          <div class="main">
            <BlockGrid blocks={gridBlocks} {external} {masked} />
            {#if common}
              <UserCommon
                {common}
                {name}
                username={data.username}
                oninvite={data.relation === 'friend' ? (game) => inviteToPlay(data!, game.title) : undefined}
              />
            {/if}
            {#if mutual.length > 0}
              <UserMutual friends={mutual} count={data.mutualCount} />
            {/if}
          </div>
        </div>
      {/if}
      </ProfileCanvas>
    </div>
  {/if}

  {#if pending}
    {@const action = pending.action}
    <ConfirmModal
      prompt={pending.prompt}
      onconfirm={() => perform(action)}
      onclose={() => (pending = null)}
      errorText={(err) => failedText(action, err)}
      returnFocus={relationControl}
    />
  {/if}
</Page>

<style>
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
