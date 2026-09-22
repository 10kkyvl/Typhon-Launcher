<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { ArrowLeft, Check, ChevronRight, Gamepad2, RefreshCw, Search, Users } from '@lucide/svelte';
  import Avatar from '../../components/Avatar.svelte';
  import Artwork from '../../components/Artwork.svelte';
  import { accountErrorText } from '../../services/accountMessages';
  import { openByIGDB } from '../../services/sources';
  import {
    accept,
    decline,
    friends as fetchFriends,
    profile as fetchProfile,
    profileByCode,
    refresh as refreshSocial,
    sendRequest,
    userGames,
    emptyFriendsPage,
    type FriendsPage,
    type PublicProfile,
    type UserCard,
  } from '../../services/social';
  import { PRESENCE_STATUSES, type PresenceStatus } from '../../services/online';
  import type { RequestText } from '../contracts';
  import type { BigPictureCommand } from '../input';
import { createRequestGate, ownsFriendsSnapshot, socialAvailability } from '../social';
  import { presenceDot, presenceLine, sortFriends } from '../../social/presence';
  import { isFriendCode } from '../../social/view';
import { friendsPage as sharedFriendsPage, needsSocialConsent } from '../../stores/social';
  import { presenceStatus, updatePresenceStatus } from '../../stores/presence';
  import { authState, currentUser } from '../../stores/user';
  import { t } from '../../i18n';
  import { playtime } from '../../utils/format';
  import { saveBigPicturePreference } from '../preferences';
  import { focusControl } from '../navigation';

  let { onback, ongame, onaccount, requestText }: {
    onback: () => void;
    ongame: (id: string) => void;
    onaccount: () => void;
    requestText: RequestText;
  } = $props();

  let page: HTMLElement;
  let friendsPage = $state<FriendsPage>(emptyFriendsPage());
  let friendsOwner = $state('');
  let friendsLoading = $state(false);
  let friendsError = $state('');
  let activeTab = $state<'friends' | 'requests'>('friends');
  let actionBusy = $state('');
  let actionMessage = $state('');
  let actionFailed = $state(false);
  let consentBusy = $state(false);
  let consentError = $state('');
  let searchBusy = $state(false);
  let searchError = $state('');
  let presenceBusy = $state(false);
  let presenceError = $state('');

  let viewingProfile = $state(false);
  let viewedProfile = $state<PublicProfile | null>(null);
  let profileLoading = $state(false);
  let profileError = $state('');
  let profileRetry = $state<(() => Promise<PublicProfile>) | null>(null);
  let displayedProfileRequest = $state(0);
  let profileOriginFocus = $state('social-add');
  let profileGames = $state<Awaited<ReturnType<typeof userGames>>['games']>([]);
  let gamesCursor = $state('');
  let gamesLoading = $state(false);
  let gamesError = $state('');
  let openingGame = $state(false);
  let openGameError = $state('');
  let actionRevision = 0;
  let previousOwner = '';
  let lastObservedSharedPage: FriendsPage | null = null;

  const friendRequests = createRequestGate();
  const profileRequests = createRequestGate();
  const gameRequests = createRequestGate();
  const availability = $derived(socialAvailability($authState));
  const needsConsent = $derived($needsSocialConsent || consentBusy);
  const sortedFriends = $derived(sortFriends(friendsPage.friends));
  const currentPresence = $derived($presenceStatus);
  const profilePrivate = $derived(!!viewedProfile && viewedProfile.visibility === 'private' && viewedProfile.relation !== 'self');
  const profileFriendsOnly = $derived(!!viewedProfile && viewedProfile.visibility === 'friends' && viewedProfile.relation !== 'friend' && viewedProfile.relation !== 'self');
  const profileGamesVisible = $derived(!!viewedProfile && !profilePrivate && !profileFriendsOnly);

  function profileCanShowGames(profile: PublicProfile): boolean {
    return profile.visibility !== 'private' && !(profile.visibility === 'friends' && profile.relation !== 'friend' && profile.relation !== 'self');
  }

  function stillOwner(owner: string): boolean {
    return $authState === 'authenticated' && $currentUser?.id === owner;
  }

  async function focusDefault() {
    await tick();
    const target = page?.querySelector<HTMLButtonElement>('[data-bp-default]') ?? page?.querySelector<HTMLButtonElement>('[data-bp-focus]');
    if (target) focusControl(target);
  }

  async function focusByKey(key: string) {
    await tick();
    const target = Array.from(page?.querySelectorAll<HTMLButtonElement>('[data-bp-focus]') ?? [])
      .find((button) => button.dataset.bpFocus === key);
    if (target) focusControl(target);
    else await focusDefault();
  }

  function applyOwnedFriends(owner: string, result: FriendsPage) {
    friendsPage = result;
    friendsOwner = owner;
    lastObservedSharedPage = $sharedFriendsPage;
  }

  async function loadFriends(owner = $currentUser?.id ?? '') {
    if (!owner || availability !== 'available' || needsConsent) return;
    const request = friendRequests.begin();
    friendsOwner = owner;
    lastObservedSharedPage = $sharedFriendsPage;
    friendsLoading = true;
    friendsError = '';
    try {
      const result = await fetchFriends();
      if (!friendRequests.isCurrent(request) || !stillOwner(owner)) return;
      applyOwnedFriends(owner, result);
    } catch (err) {
      if (!friendRequests.isCurrent(request) || !stillOwner(owner)) return;
      friendsError = accountErrorText(err, $t('bp.social.loadFailed'));
    } finally {
      if (friendRequests.isCurrent(request)) friendsLoading = false;
    }
  }

  async function refreshFriendsPage() {
    if (friendsLoading || availability !== 'available' || needsConsent) return;
    const owner = $currentUser?.id ?? '';
    if (!owner) return;
    const request = friendRequests.begin();
    friendsOwner = owner;
    lastObservedSharedPage = $sharedFriendsPage;
    friendsLoading = true;
    friendsError = '';
    try {
      await refreshSocial();
      const result = await fetchFriends();
      if (!friendRequests.isCurrent(request) || !stillOwner(owner)) return;
      applyOwnedFriends(owner, result);
    } catch (err) {
      if (!friendRequests.isCurrent(request) || !stillOwner(owner)) return;
      // Keep the last displayed page so an API failure does not hide usable data.
      friendsError = accountErrorText(err, $t('bp.social.loadFailed'));
    } finally {
      if (friendRequests.isCurrent(request)) friendsLoading = false;
    }
  }

  $effect(() => {
    const mode = availability;
    const owner = $currentUser?.id ?? '';
    const consent = $needsSocialConsent;
    const enabling = consentBusy;
    if (owner !== previousOwner) {
      previousOwner = owner;
      actionRevision += 1;
      actionBusy = '';
      friendRequests.invalidate();
      profileRequests.invalidate();
      gameRequests.invalidate();
      friendsOwner = '';
      friendsPage = emptyFriendsPage();
      friendsLoading = false;
      friendsError = '';
      viewingProfile = false;
      viewedProfile = null;
      displayedProfileRequest = 0;
      profileOriginFocus = 'social-add';
      profileRetry = null;
      profileGames = [];
      profileLoading = false;
      gamesLoading = false;
    }
    if (mode !== 'available' || consent || enabling) {
      actionRevision += 1;
      actionBusy = '';
      friendRequests.invalidate();
      profileRequests.invalidate();
      gameRequests.invalidate();
      friendsPage = emptyFriendsPage();
      friendsOwner = '';
      friendsLoading = false;
      friendsError = '';
      viewingProfile = false;
      viewedProfile = null;
      displayedProfileRequest = 0;
      profileRetry = null;
      profileGames = [];
      profileLoading = false;
      gamesLoading = false;
      return;
    }
    void loadFriends(owner);
    return () => friendRequests.invalidate();
  });

  $effect(() => {
    const owner = $currentUser?.id ?? '';
    const shared = $sharedFriendsPage;
    if (availability !== 'available' || needsConsent || !ownsFriendsSnapshot(owner, friendsOwner)) {
      lastObservedSharedPage = shared;
      return;
    }
    if (shared !== lastObservedSharedPage) {
      lastObservedSharedPage = shared;
      friendsPage = shared;
    }
  });

  async function enableSocial() {
    if (consentBusy || availability !== 'available') return;
    consentBusy = true;
    consentError = '';
    actionMessage = '';
    const result = await saveBigPicturePreference({ accountSync: true });
    consentBusy = false;
    if (result === 'failed') consentError = $t('bp.social.consentFailed');
  }

  async function runAction(id: string, operation: () => Promise<void>, success: string) {
    if (actionBusy || friendsLoading || availability !== 'available' || needsConsent) return;
    const owner = $currentUser?.id ?? '';
    if (!owner) return;
    const revision = ++actionRevision;
    actionBusy = id;
    actionMessage = '';
    actionFailed = false;
    try {
      await operation();
      if (!stillOwner(owner) || actionRevision !== revision) return;
      actionMessage = success;
      await loadFriends(owner);
    } catch (err) {
      if (!stillOwner(owner) || actionRevision !== revision) return;
      actionMessage = accountErrorText(err, $t('bp.social.actionFailed'));
      actionFailed = true;
    } finally {
      if (actionRevision === revision) actionBusy = '';
    }
  }

  async function changePresence(next: PresenceStatus) {
    if (presenceBusy || availability !== 'available') return;
    const owner = $currentUser?.id ?? '';
    presenceBusy = true;
    presenceError = '';
    try {
      await updatePresenceStatus(next);
    } catch (err) {
      if (stillOwner(owner)) presenceError = accountErrorText(err, $t('bp.social.presenceFailed'));
    } finally {
      presenceBusy = false;
    }
  }

  function invalidatePublicProfile() {
    profileRequests.invalidate();
    gameRequests.invalidate();
    displayedProfileRequest = 0;
    profileRetry = null;
    viewingProfile = false;
    viewedProfile = null;
    profileGames = [];
    gamesCursor = '';
    profileLoading = false;
    profileError = '';
    gamesLoading = false;
    gamesError = '';
    openGameError = '';
  }

  function closePublicProfile() {
    const returnFocus = profileOriginFocus;
    invalidatePublicProfile();
    void focusByKey(returnFocus);
  }

  export function handleCommand(command: BigPictureCommand): boolean {
    if (command !== 'back' || !viewingProfile) return false;
    closePublicProfile();
    return true;
  }

  function back() {
    if (viewingProfile) closePublicProfile();
    else onback();
  }

  async function loadUserGames(target: PublicProfile, owner: string, reset = false) {
    if (!stillOwner(owner) || !profileCanShowGames(target) || (!reset && (!gamesCursor || gamesLoading))) return;
    const request = gameRequests.begin();
    const cursor = reset ? '' : gamesCursor;
    gamesLoading = true;
    gamesError = '';
    try {
      const result = await userGames(target.username, cursor);
      if (!gameRequests.isCurrent(request) || !stillOwner(owner) || viewedProfile?.username !== target.username) return;
      profileGames = reset
        ? result.games
        : [...profileGames, ...result.games.filter((game) => !profileGames.some((current) => current.igdbId === game.igdbId))];
      gamesCursor = result.next;
    } catch {
      if (gameRequests.isCurrent(request) && stillOwner(owner) && viewedProfile?.username === target.username) {
        gamesError = $t('bp.social.gamesLoadFailed');
      }
    } finally {
      if (gameRequests.isCurrent(request)) gamesLoading = false;
    }
  }

  async function showProfile(load: () => Promise<PublicProfile>, originFocus = profileOriginFocus) {
    if (availability !== 'available' || needsConsent) return;
    const owner = $currentUser?.id ?? '';
    if (!owner) return;
    const request = profileRequests.begin();
    gameRequests.invalidate();
    displayedProfileRequest = request;
    profileOriginFocus = originFocus;
    profileRetry = load;
    viewingProfile = true;
    viewedProfile = null;
    profileLoading = true;
    profileError = '';
    searchError = '';
    profileGames = [];
    gamesCursor = '';
    gamesError = '';
    openGameError = '';
    void focusDefault();
    try {
      const result = await load();
      if (!profileRequests.isCurrent(request) || !stillOwner(owner)) return;
      viewedProfile = result;
      if (profileCanShowGames(result)) {
        void loadUserGames(result, owner, true);
      }
    } catch (err) {
      if (profileRequests.isCurrent(request) && stillOwner(owner)) profileError = accountErrorText(err, $t('bp.social.profileFailed'));
    } finally {
      if (profileRequests.isCurrent(request)) profileLoading = false;
    }
  }

  async function openUser(user: UserCard, originFocus: string) {
    await showProfile(() => fetchProfile(user.username), originFocus);
  }

  async function findFriend(originFocus = 'social-add') {
    if (searchBusy || availability !== 'available' || needsConsent) return;
    searchBusy = true;
    searchError = '';
    try {
      const input = await requestText({ title: $t('bp.social.searchPrompt'), maxLength: 80 });
      const query = input?.trim();
      const owner = $currentUser?.id ?? '';
      if (!query || !owner || !stillOwner(owner)) return;
      await showProfile(() => isFriendCode(query) ? profileByCode(query) : fetchProfile(query.replace(/^@/, '')), originFocus);
    } catch (err) {
      searchError = accountErrorText(err, $t('bp.social.searchFailed'));
    } finally {
      searchBusy = false;
    }
  }

  async function profileAction(action: 'add' | 'accept' | 'decline' | 'cancel') {
    const target = viewedProfile;
    if (!target || actionBusy || availability !== 'available' || needsConsent) return;
    const owner = $currentUser?.id ?? '';
    const profileRequest = displayedProfileRequest;
    if (!owner || !profileRequest) return;
    const actionId = `profile:${action}`;
    const revision = ++actionRevision;
    actionBusy = actionId;
    actionMessage = '';
    actionFailed = false;
    try {
      let nextRelation: PublicProfile['relation'] = target.relation;
      let successMessage = '';
      if (action === 'add') {
        const sent = await sendRequest(target.username);
        nextRelation = sent.accepted ? 'friend' : 'outgoing';
        successMessage = sent.accepted ? $t('bp.social.friendsNow') : $t('bp.social.requestSent');
      } else if (action === 'accept') {
        await accept(target.id);
        nextRelation = 'friend';
        successMessage = $t('bp.social.requestAccepted');
      } else {
        await decline(target.id);
        nextRelation = 'none';
        successMessage = action === 'cancel' ? $t('bp.social.requestCancelled') : $t('bp.social.requestDeclined');
      }
      if (!stillOwner(owner) || !profileRequests.isCurrent(profileRequest) || viewedProfile?.id !== target.id) return;
      const updatedProfile = { ...target, relation: nextRelation };
      viewedProfile = updatedProfile;
      actionMessage = successMessage;
      if (profileCanShowGames(updatedProfile) && profileGames.length === 0) {
        void loadUserGames(updatedProfile, owner, true);
      }
      await loadFriends(owner);
    } catch (err) {
      if (!stillOwner(owner) || !profileRequests.isCurrent(profileRequest) || viewedProfile?.id !== target.id) return;
      actionMessage = accountErrorText(err, $t('bp.social.actionFailed'));
      actionFailed = true;
    } finally {
      if (actionRevision === revision) actionBusy = '';
    }
  }

  async function openGame(igdbId: number, title: string) {
    if (openingGame || availability !== 'available') return;
    const owner = $currentUser?.id ?? '';
    openingGame = true;
    openGameError = '';
    try {
      const resolved = await openByIGDB(igdbId, title);
      if (!stillOwner(owner)) return;
      if (!resolved?.id) throw new Error('game_not_found');
      ongame(resolved.id);
    } catch {
      if (stillOwner(owner)) openGameError = $t('bp.social.openGameFailed');
    } finally {
      openingGame = false;
    }
  }

  function presenceLabel(status: PresenceStatus): string {
    if (status === 'online') return $t('bp.social.online');
    if (status === 'away') return $t('bp.social.away');
    if (status === 'busy') return $t('bp.social.busy');
    return $t('bp.social.invisible');
  }

  onMount(() => { void focusDefault(); });
</script>

<div class="bp-page social-page" bind:this={page}>
  <header class="bp-header">
    <div class="title-copy">
      <span class="eyebrow">TYPHON · BIG PICTURE</span>
      <h1>{#if viewingProfile}{viewedProfile?.displayName || (viewedProfile ? `@${viewedProfile.username}` : $t('bp.social.profileTitle'))}{:else}{$t('bp.social.title')}{/if}</h1>
      {#if !viewingProfile}<p>{$t('bp.social.subtitle')}</p>{/if}
    </div>
    <div class="bp-actions">
      <button class="bp-button back-button" data-bp-focus="social-back" data-bp-default onclick={back}><ArrowLeft size="1.1em" />{$t('bp.social.back')}</button>
      {#if availability === 'available' && !needsConsent && !viewingProfile}
        <button class="bp-button bp-primary" data-bp-focus="social-add" disabled={searchBusy} onclick={() => void findFriend('social-add')}><Search size="1.05em" />{searchBusy ? $t('bp.social.searching') : $t('bp.social.addFriend')}</button>
        <button class="bp-button" data-bp-focus="social-refresh" disabled={friendsLoading} onclick={() => void refreshFriendsPage()}><RefreshCw size="1.05em" />{$t('bp.social.refresh')}</button>
      {/if}
    </div>
  </header>

  {#if viewingProfile}
    {#if profileLoading}
      <div class="bp-empty"><Users size="2.6rem" /><p>{$t('bp.social.profileLoading')}</p></div>
    {:else if profileError}
      <div class="bp-error" role="alert"><p>{profileError}</p><button class="bp-button" data-bp-focus="social-profile-retry" disabled={!profileRetry} onclick={() => profileRetry && void showProfile(profileRetry)}>{$t('bp.social.retry')}</button></div>
    {:else if viewedProfile}
      <section class="public-identity bp-card">
        <Avatar size="lg" name={viewedProfile.displayName || viewedProfile.username} src={viewedProfile.avatarUrl} status={viewedProfile.presence ? presenceDot(viewedProfile.presence) : undefined} />
        <div class="public-copy">
          <span class="eyebrow">@{viewedProfile.username}</span>
          <h2>{viewedProfile.displayName || viewedProfile.username}</h2>
          {#if viewedProfile.presence}<p>{presenceLine(viewedProfile.presence)}</p>{/if}
          {#if viewedProfile.stats}
            <div class="public-stats">
              <span><strong>{viewedProfile.stats.games}</strong>{$t('bp.social.statsGames')}</span>
              {#if viewedProfile.stats.hours != null}<span><strong>{viewedProfile.stats.hours}</strong>{$t('bp.social.statsHours')}</span>{/if}
              <span><strong>{viewedProfile.stats.completed}</strong>{$t('bp.social.statsCompleted')}</span>
            </div>
          {/if}
        </div>
        {#if availability === 'available' && !needsConsent}
          <div class="relation-actions">
            {#if viewedProfile.relation === 'none'}
              <button class="bp-button bp-primary" data-bp-focus="social-profile-add" disabled={!!actionBusy} onclick={() => void profileAction('add')}>{$t('bp.social.sendRequest')}</button>
            {:else if viewedProfile.relation === 'incoming'}
              <button class="bp-button bp-primary" data-bp-focus="social-profile-accept" disabled={!!actionBusy} onclick={() => void profileAction('accept')}>{$t('bp.social.accept')}</button>
              <button class="bp-button" data-bp-focus="social-profile-decline" disabled={!!actionBusy} onclick={() => void profileAction('decline')}>{$t('bp.social.decline')}</button>
            {:else if viewedProfile.relation === 'outgoing'}
              <button class="bp-button" data-bp-focus="social-profile-cancel" disabled={!!actionBusy} onclick={() => void profileAction('cancel')}>{$t('bp.social.cancel')}</button>
            {:else if viewedProfile.relation === 'friend'}
              <span class="relation-pill"><Check size="1em" />{$t('bp.social.relationFriend')}</span>
            {:else if viewedProfile.relation === 'blocked'}
              <span class="relation-pill">{$t('bp.social.relationBlocked')}</span>
            {/if}
          </div>
        {/if}
      </section>

      {#if actionMessage}<p class:danger={actionFailed} class="inline-message" role={actionFailed ? 'alert' : 'status'}>{actionMessage}</p>{/if}
      {#if profilePrivate}
        <div class="bp-empty"><Users size="2.4rem" /><h2>{$t('bp.social.privateProfile')}</h2></div>
      {:else if profileFriendsOnly}
        <div class="bp-empty"><Users size="2.4rem" /><h2>{$t('bp.social.friendsOnlyProfile')}</h2></div>
      {:else if profileGamesVisible}
        {#if viewedProfile.bio}<section class="bp-card bio-card"><h2>{$t('social.aboutTitle')}</h2><p>{viewedProfile.bio}</p></section>{/if}
        {#if viewedProfile.recentlyPlayed.length > 0}
          <section class="games-section">
            <h2>{$t('bp.social.recentGames')}</h2>
            <div class="game-grid">
              {#each viewedProfile.recentlyPlayed as game (game.igdbId)}
                <button class="game-card bp-card" data-bp-focus={`social-recent-${game.igdbId}`} disabled={openingGame} onclick={() => void openGame(game.igdbId, game.title)} aria-label={game.title}>
                  <span class="cover"><Artwork src={game.coverUrl} alt="" label={game.title} ratio="3 / 4" radius="inherit" /></span>
                  <span class="game-name">{game.title}</span><span class="game-meta">{playtime(game.playtimeSeconds ?? 0)}</span>
                </button>
              {/each}
            </div>
          </section>
        {/if}
        {#if viewedProfile.favorites.length > 0}
          <section class="games-section">
            <h2>{$t('bp.social.favoriteGames')}</h2>
            <div class="game-grid">
              {#each viewedProfile.favorites as game (game.igdbId)}
                <button class="game-card bp-card" data-bp-focus={`social-favorite-${game.igdbId}`} disabled={openingGame} onclick={() => void openGame(game.igdbId, game.title)} aria-label={game.title}>
                  <span class="cover"><Artwork src={game.coverUrl} alt="" label={game.title} ratio="3 / 4" radius="inherit" /></span>
                  <span class="game-name">{game.title}</span>
                </button>
              {/each}
            </div>
          </section>
        {/if}
        {#if profileGames.length > 0 || gamesLoading}
          <section class="games-section">
            <h2>{$t('bp.social.allGames')}</h2>
            <div class="game-grid">
              {#each profileGames as game (game.igdbId)}
                <button class="game-card bp-card" data-bp-focus={`social-all-${game.igdbId}`} disabled={openingGame} onclick={() => void openGame(game.igdbId, game.title)} aria-label={game.title}>
                  <span class="cover"><Artwork src={game.coverUrl} alt="" label={game.title} ratio="3 / 4" radius="inherit" /></span>
                  <span class="game-name">{game.title}</span><span class="game-meta">{playtime(game.playtimeSeconds ?? 0)}</span>
                </button>
              {/each}
            </div>
            {#if gamesError}<p class="error-text" role="alert">{gamesError}</p>{/if}
            {#if gamesCursor}<button class="bp-button" data-bp-focus="social-load-more" disabled={gamesLoading} onclick={() => viewedProfile && void loadUserGames(viewedProfile, $currentUser?.id ?? '')}>{gamesLoading ? $t('bp.social.searching') : $t('bp.social.loadMore')}</button>{/if}
          </section>
        {/if}
        {#if viewedProfile.recentlyPlayed.length === 0 && viewedProfile.favorites.length === 0 && profileGames.length === 0 && !gamesLoading}
          <div class="bp-empty"><Gamepad2 size="2.5rem" /><h2>{$t('bp.social.noGamesTitle')}</h2><p>{$t('bp.social.noGamesDescription')}</p></div>
        {/if}
        {#if openGameError}<p class="error-text" role="alert">{openGameError}</p>{/if}
      {/if}
    {/if}
  {:else if availability === 'loading'}
    <div class="bp-empty"><Users size="2.6rem" /><p>{$t('bp.social.loading')}</p></div>
  {:else if availability === 'guest'}
    <div class="bp-empty"><Users size="2.8rem" /><h2>{$t('bp.social.guestTitle')}</h2><p>{$t('bp.social.guestDescription')}</p><button class="bp-button bp-primary" data-bp-focus="social-account" onclick={onaccount}>{$t('bp.social.account')}</button></div>
  {:else if availability === 'offline'}
    <div class="bp-empty"><Users size="2.8rem" /><h2>{$t('bp.social.offlineTitle')}</h2><p>{$t('bp.social.offlineDescription')}</p></div>
  {:else if availability === 'sign-in'}
    <div class="bp-empty"><Users size="2.8rem" /><h2>{$t('bp.social.signInTitle')}</h2><p>{$t('bp.social.signInDescription')}</p><button class="bp-button bp-primary" data-bp-focus="social-account" onclick={onaccount}>{$t('bp.social.account')}</button></div>
  {:else if needsConsent}
    <section class="bp-card consent-card">
      <h2>{$t('modals.socialConsentTitle')}</h2>
      <p>{$t('modals.socialConsentIntro')}</p>
      <p>{$t('modals.socialConsentListIntro')}</p>
      <ul><li>{$t('modals.socialConsentListGames')}</li><li>{$t('modals.socialConsentListPlaytime')}</li><li>{$t('modals.socialConsentListFavoritesStatus')}</li><li>{$t('modals.socialConsentListDates')}</li></ul>
      <p>{$t('modals.socialConsentOutro')}</p>
      {#if consentError}<p class="error-text" role="alert">{consentError}</p>{/if}
      <button class="bp-button bp-primary" data-bp-focus="social-enable-sync" disabled={consentBusy} onclick={enableSocial}>{consentBusy ? $t('modals.socialConsentEnabling') : $t('modals.socialConsentEnable')}</button>
    </section>
  {:else}
    {#if friendsError}<div class="bp-error" role="alert"><p>{friendsError}</p><button class="bp-button" data-bp-focus="social-retry" disabled={friendsLoading} onclick={() => void refreshFriendsPage()}>{$t('bp.social.retry')}</button></div>{/if}
    {#if searchError}<p class="error-text" role="alert">{searchError}</p>{/if}

    <section class="presence-card bp-card">
      <div class="presence-heading"><h2>{$t('bp.social.yourPresence')}</h2><span class="presence-indicator" class:busy={presenceBusy}></span></div>
      <div class="presence-choices">
        {#each PRESENCE_STATUSES as status (status)}
          <button class="bp-button presence-choice" class:selected={currentPresence === status} aria-pressed={currentPresence === status} data-bp-focus={`social-presence-${status}`} disabled={presenceBusy} onclick={() => void changePresence(status)}>{presenceLabel(status)}</button>
        {/each}
      </div>
      {#if presenceError}<p class="error-text" role="alert">{presenceError}</p>{/if}
    </section>

    <div class="tabs" role="tablist" aria-label={$t('bp.social.title')}>
      <button class="bp-button tab" class:selected={activeTab === 'friends'} role="tab" aria-selected={activeTab === 'friends'} data-bp-focus="social-tab-friends" onclick={() => (activeTab = 'friends')}>{$t('bp.social.tabFriends')} <span>{friendsPage.friends.length}</span></button>
      <button class="bp-button tab" class:selected={activeTab === 'requests'} role="tab" aria-selected={activeTab === 'requests'} data-bp-focus="social-tab-requests" onclick={() => (activeTab = 'requests')}>{$t('bp.social.tabRequests')} <span>{friendsPage.incoming.length + friendsPage.outgoing.length}</span></button>
    </div>

    {#if activeTab === 'friends'}
      {#if friendsLoading && friendsPage.friends.length === 0}
        <div class="bp-empty"><Users size="2.4rem" /><p>{$t('bp.social.loading')}</p></div>
      {:else if sortedFriends.length === 0 && !friendsError}
        <div class="bp-empty"><Users size="2.4rem" /><h2>{$t('bp.social.emptyFriendsTitle')}</h2><p>{$t('bp.social.emptyFriendsDescription')}</p><button class="bp-button bp-primary" data-bp-focus="social-add-empty" onclick={() => void findFriend('social-add-empty')}><Search size="1.05em" />{$t('bp.social.addFriend')}</button></div>
      {:else}
        <section class="friend-section">
          <h2>{$t('bp.social.friendsCount', { count: sortedFriends.length })}</h2>
          <div class="people-grid">
            {#each sortedFriends as friend (friend.id)}
              <button class="person-card bp-card" data-bp-focus={`social-friend-${encodeURIComponent(friend.id)}`} disabled={!!actionBusy} onclick={() => void openUser(friend, `social-friend-${encodeURIComponent(friend.id)}`)}>
                <Avatar size="md" name={friend.displayName || friend.username} src={friend.avatarUrl} status={presenceDot(friend.presence)} />
                <span class="person-copy"><strong>{friend.displayName || friend.username}</strong><span>@{friend.username}</span><span class="person-presence">{presenceLine(friend.presence)}</span></span>
                <ChevronRight class="person-chevron" size="1.4em" />
              </button>
            {/each}
          </div>
        </section>
      {/if}
    {:else}
      {#if friendsLoading && friendsPage.incoming.length + friendsPage.outgoing.length === 0}
        <div class="bp-empty"><Users size="2.4rem" /><p>{$t('bp.social.loading')}</p></div>
      {:else if friendsPage.incoming.length === 0 && friendsPage.outgoing.length === 0 && !friendsError}
        <div class="bp-empty"><Users size="2.4rem" /><h2>{$t('bp.social.emptyIncomingTitle')}</h2><p>{$t('bp.social.emptyIncomingDescription')}</p></div>
      {:else}
        {#if friendsPage.incoming.length > 0}
          <section class="request-section"><h2>{$t('bp.social.incomingCount')} <span>{friendsPage.incoming.length}</span></h2>
            <div class="request-list">
              {#each friendsPage.incoming as request (request.id)}
                <div class="request-card bp-card">
                  <button class="person-link" data-bp-focus={`social-incoming-profile-${encodeURIComponent(request.id)}`} disabled={!!actionBusy} onclick={() => void openUser(request, `social-incoming-profile-${encodeURIComponent(request.id)}`)}><Avatar size="md" name={request.displayName || request.username} src={request.avatarUrl} /><span><strong>{request.displayName || request.username}</strong><small>@{request.username}</small></span><ChevronRight size="1.2em" /></button>
                  <div class="request-actions">
                    <button class="bp-button bp-primary" data-bp-focus={`social-accept-${encodeURIComponent(request.id)}`} disabled={!!actionBusy || friendsLoading} onclick={() => void runAction(`accept:${request.id}`, () => accept(request.id), $t('bp.social.requestAccepted'))}>{actionBusy === `accept:${request.id}` ? $t('bp.social.searching') : $t('bp.social.accept')}</button>
                    <button class="bp-button" data-bp-focus={`social-decline-${encodeURIComponent(request.id)}`} disabled={!!actionBusy || friendsLoading} onclick={() => void runAction(`decline:${request.id}`, () => decline(request.id), $t('bp.social.requestDeclined'))}>{$t('bp.social.decline')}</button>
                  </div>
                </div>
              {/each}
            </div>
          </section>
        {/if}
        {#if friendsPage.outgoing.length > 0}
          <section class="request-section"><h2>{$t('bp.social.outgoingCount')} <span>{friendsPage.outgoing.length}</span></h2>
            <div class="request-list">
              {#each friendsPage.outgoing as request (request.id)}
                <div class="request-card bp-card">
                  <button class="person-link" data-bp-focus={`social-outgoing-profile-${encodeURIComponent(request.id)}`} disabled={!!actionBusy} onclick={() => void openUser(request, `social-outgoing-profile-${encodeURIComponent(request.id)}`)}><Avatar size="md" name={request.displayName || request.username} src={request.avatarUrl} /><span><strong>{request.displayName || request.username}</strong><small>@{request.username}</small></span><ChevronRight size="1.2em" /></button>
                  <div class="request-actions"><button class="bp-button" data-bp-focus={`social-cancel-${encodeURIComponent(request.id)}`} disabled={!!actionBusy || friendsLoading} onclick={() => void runAction(`cancel:${request.id}`, () => decline(request.id), $t('bp.social.requestCancelled'))}>{$t('bp.social.cancel')}</button></div>
                </div>
              {/each}
            </div>
          </section>
        {/if}
      {/if}
    {/if}
    {#if actionMessage}<p class:danger={actionFailed} class="inline-message" role={actionFailed ? 'alert' : 'status'}>{actionMessage}</p>{/if}
  {/if}
</div>

<style>
  .social-page { display: flex; flex-direction: column; gap: clamp(1rem, 1.8vh, 1.8rem); padding-bottom: 2rem; }
  .title-copy { min-width: 0; }
  .eyebrow { display: block; margin-bottom: .35rem; color: var(--text-3); font-size: .72em; font-weight: 700; letter-spacing: .16em; }
  .bp-header h1 { margin: 0; font-size: clamp(2rem, 4vw, 4.6rem); line-height: 1.08; overflow-wrap: anywhere; }
  .bp-header p { margin: .55rem 0 0; color: var(--text-2); font-size: 1.05em; }
  .bp-actions { flex-wrap: wrap; }
  .back-button { min-height: 3.4rem; padding-inline: 1.15rem; }
  .bp-empty { min-height: 14rem; padding: 2rem; }
  .bp-empty h2 { margin: .55rem 0 .15rem; font-size: clamp(1.3rem, 2vw, 2rem); }
  .bp-empty p { margin: .25rem 0 1rem; max-width: 42rem; }
  .bp-empty .bp-button { margin-top: .65rem; min-height: 3.3rem; padding: .7rem 1.25rem; }
  .presence-card { padding: clamp(1.1rem, 1.7vw, 1.8rem); }
  .presence-heading { display: flex; align-items: center; gap: .7rem; }
  .presence-heading h2, .friend-section h2, .request-section h2, .games-section h2 { margin: 0; font-size: clamp(1.2rem, 1.6vw, 1.8rem); }
  .presence-indicator { width: .65rem; height: .65rem; border-radius: 50%; background: var(--success); }
  .presence-indicator.busy { background: var(--warning); }
  .presence-choices { display: flex; flex-wrap: wrap; gap: .55rem; margin-top: .85rem; }
  .presence-choice { min-height: 2.9rem; padding: .55rem .95rem; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-2); }
  .presence-choice.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 24%, var(--surface-2)); }
  .tabs { display: flex; gap: .6rem; border-bottom: 1px solid var(--border); }
  .tab { display: inline-flex; align-items: center; gap: .6rem; min-height: 3.1rem; padding: .5rem 1.1rem; border-radius: var(--radius-md) var(--radius-md) 0 0; color: var(--text-2); }
  .tab.selected { color: var(--text); background: color-mix(in srgb, var(--accent) 18%, var(--surface-2)); box-shadow: inset 0 -3px var(--accent); }
  .tab > span { min-width: 1.7em; padding: .1rem .4rem; border-radius: 99px; background: var(--surface-3); color: var(--text-3); text-align: center; font-size: .8em; }
  .friend-section, .request-section, .games-section { display: flex; flex-direction: column; gap: .8rem; }
  .people-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 22rem), 1fr)); gap: .75rem; }
  .person-card { display: flex; align-items: center; gap: .9rem; min-width: 0; width: 100%; padding: 1rem; border-radius: var(--radius-lg); text-align: left; color: var(--text); }
  .person-copy { display: flex; flex: 1; min-width: 0; flex-direction: column; gap: .13rem; }
  .person-copy strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .person-copy > span { color: var(--text-3); font-size: .82em; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .person-copy > span.person-presence { color: var(--text-2); margin-top: .18rem; }
  .person-card :global(.person-chevron) { flex: none; color: var(--text-3); }
  .request-section h2 > span { color: var(--text-3); font-weight: 500; }
  .request-list { display: flex; flex-direction: column; gap: .7rem; }
  .request-card { display: flex; align-items: center; justify-content: space-between; gap: 1rem; padding: .8rem 1rem; }
  .person-link { flex: 1; min-width: 0; display: flex; align-items: center; gap: .75rem; padding: .1rem; text-align: left; color: var(--text); border-radius: var(--radius-md); }
  .person-link > span { min-width: 0; display: flex; flex-direction: column; gap: .15rem; }
  .person-link strong, .person-link small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .person-link small { color: var(--text-3); }
  .person-link :global(svg:last-child) { margin-left: auto; color: var(--text-3); }
  .request-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: .5rem; }
  .request-actions .bp-button { min-height: 2.9rem; padding: .55rem .9rem; }
  .inline-message, .error-text { margin: 0; color: var(--success); font-size: .92em; }
  .inline-message.danger, .error-text { color: var(--danger); }
  .public-identity { display: flex; align-items: center; gap: clamp(1rem, 2.2vw, 2.4rem); padding: clamp(1.1rem, 1.9vw, 2rem); }
  .public-identity :global(.avatar.lg) { width: clamp(5rem, 8vw, 8.5rem); height: clamp(5rem, 8vw, 8.5rem); }
  .public-copy { min-width: 0; flex: 1; }
  .public-copy h2 { margin: 0; font-size: clamp(1.6rem, 2.7vw, 3rem); line-height: 1.1; overflow-wrap: anywhere; }
  .public-copy > p { margin: .4rem 0 0; color: var(--text-2); }
  .public-stats { display: flex; flex-wrap: wrap; gap: .9rem 1.4rem; margin-top: .65rem; }
  .public-stats span { display: flex; align-items: baseline; gap: .35rem; color: var(--text-3); font-size: .83em; }
  .public-stats strong { color: var(--text); font-size: 1.3em; font-variant-numeric: tabular-nums; }
  .relation-actions { display: flex; flex-wrap: wrap; gap: .5rem; flex: none; }
  .relation-actions .bp-button { min-height: 3.1rem; padding: .6rem 1rem; }
  .relation-pill { display: inline-flex; align-items: center; gap: .4rem; padding: .6rem .95rem; border-radius: 99px; background: var(--surface-2); color: var(--text-2); white-space: nowrap; }
  .bio-card { padding: 1.2rem 1.5rem; }
  .bio-card h2 { margin: 0 0 .3rem; font-size: 1.2em; }
  .bio-card p { margin: 0; color: var(--text-2); line-height: 1.5; white-space: pre-wrap; }
  .game-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 11rem), 1fr)); gap: .8rem; }
  .game-card { display: flex; flex-direction: column; gap: .45rem; width: 100%; padding: .6rem; border-radius: var(--radius-lg); color: var(--text); text-align: left; overflow: hidden; }
  .cover { display: block; width: 100%; aspect-ratio: 3 / 4; overflow: hidden; border-radius: var(--radius-md); }
  .game-name { width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 650; }
  .game-meta { color: var(--text-3); font-size: .8em; }
  .consent-card { max-width: 78rem; padding: clamp(1.2rem, 2vw, 2.4rem); }
  .consent-card h2 { margin: 0 0 .7rem; font-size: clamp(1.4rem, 2.2vw, 2.5rem); }
  .consent-card p, .consent-card li { color: var(--text-2); line-height: 1.5; }
  .consent-card ul { margin: .4rem 0 .8rem; padding-left: 1.5rem; }
  .consent-card .bp-button { min-height: 3.2rem; padding: .7rem 1.2rem; }
  @media (max-width: 760px) {
    .public-identity { align-items: flex-start; flex-wrap: wrap; }
    .public-copy { flex-basis: calc(100% - 7rem); }
    .relation-actions { width: 100%; }
    .request-card { align-items: flex-start; flex-direction: column; }
    .request-actions { width: 100%; justify-content: flex-start; }
  }
</style>
