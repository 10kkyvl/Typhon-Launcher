<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { ArrowLeft, Clock3, Gamepad2, Pencil, Trophy, UserRound } from '@lucide/svelte';
  import Avatar from '../../components/Avatar.svelte';
  import Artwork from '../../components/Artwork.svelte';
  import { accountErrorText } from '../../services/accountMessages';
  import type { LibraryGame } from '../../services/library';
  import type { RequestText } from '../contracts';
  import { focusControl } from '../navigation';
  import { recentLocalGames } from '../profile';
  import { profileSnapshot, initProfile } from '../../stores/profile';
  import { libraryGames } from '../../stores/library';
  import { authState, currentUser, saveProfile } from '../../stores/user';
  import { t } from '../../i18n';
  import { playtime } from '../../utils/format';

  let { onback, ongame, onaccount, requestText }: {
    onback: () => void;
    ongame: (id: string) => void;
    onaccount: () => void;
    requestText: RequestText;
  } = $props();

  let page: HTMLElement;
  let editBusy = $state(false);
  let editMessage = $state('');
  let editFailed = $state(false);
  let expanded = $state(false);

  const isGuest = $derived($authState === 'guest');
  const isOffline = $derived($authState === 'offline');
  const isAuthenticated = $derived($authState === 'authenticated');
  const recentGames = $derived(recentLocalGames($libraryGames));
  const visibleGames = $derived(expanded ? recentGames : recentGames.slice(0, 8));
  // Profile.Snapshot reads the local play history in every auth state, including guest and offline.
  const stats = $derived($profileSnapshot.stats);
  const displayName = $derived($currentUser?.displayName || $currentUser?.username || $t('bp.profile.guestTitle'));
  const showContent = $derived($authState !== 'bootstrapping');

  async function focusInitial() {
    await tick();
    const target = page?.querySelector<HTMLButtonElement>('[data-bp-default]') ?? page?.querySelector<HTMLButtonElement>('[data-bp-focus]');
    if (target) focusControl(target);
  }

  async function editDisplayName() {
    if (editBusy || !isAuthenticated || !$currentUser) return;
    const owner = $currentUser.id;
    let proposed: string | null;
    try {
      proposed = await requestText({
        title: $t('bp.profile.namePrompt'),
        initialValue: $currentUser.displayName,
        maxLength: 32,
      });
    } catch {
      editMessage = $t('bp.profile.nameSaveFailed');
      editFailed = true;
      return;
    }
    const displayName = proposed?.trim();
    if (!displayName || $currentUser?.id !== owner || displayName === $currentUser.displayName) return;

    editBusy = true;
    editMessage = '';
    editFailed = false;
    try {
      await saveProfile({ displayName });
      if ($currentUser?.id === owner) editMessage = $t('bp.profile.nameSaved');
    } catch (err) {
      if ($currentUser?.id === owner) {
        editMessage = accountErrorText(err, $t('bp.profile.nameSaveFailed'));
        editFailed = true;
      }
    } finally {
      editBusy = false;
    }
  }

  onMount(() => {
    initProfile();
    void focusInitial();
  });
</script>

<div class="bp-page profile-page" bind:this={page}>
  <header class="bp-header">
    <div class="title-copy">
      <span class="eyebrow">TYPHON · BIG PICTURE</span>
      <h1>{$t('bp.profile.title')}</h1>
    </div>
    <div class="bp-actions">
      <button class="bp-button back-button" data-bp-focus="profile-back" data-bp-default onclick={onback}>
        <ArrowLeft size="1.1em" />{$t('bp.social.back')}
      </button>
    </div>
  </header>

  {#if $authState === 'bootstrapping'}
    <div class="bp-empty"><UserRound size="2.5rem" /><p>{$t('bp.profile.loading')}</p></div>
  {:else if showContent}
    <section class="identity bp-card">
      <Avatar size="lg" name={displayName} src={$currentUser?.avatarUrl} />
      <div class="identity-copy">
        <span class="eyebrow">{#if isGuest || !isAuthenticated}{$t('bp.profile.localLabel')}{:else}@{$currentUser?.username}{/if}</span>
        <h2>{displayName}</h2>
        {#if isGuest}
          <p>{$t('bp.profile.guestDescription')}</p>
        {:else if isOffline}
          <p>{$t('bp.profile.offlineHint')}</p>
        {:else if !isAuthenticated}
          <p>{$t('bp.profile.guestDescription')}</p>
        {/if}
        {#if editMessage}<p class:danger={editFailed} class="edit-message" role={editFailed ? 'alert' : 'status'}>{editMessage}</p>{/if}
      </div>
      <div class="identity-actions">
        {#if isAuthenticated}
          <button class="bp-button" data-bp-focus="profile-edit-name" disabled={editBusy} onclick={editDisplayName}>
            <Pencil size="1.05em" />{editBusy ? $t('bp.settings.saving') : $t('bp.profile.editName')}
          </button>
        {:else if isGuest || !isOffline}
          <button class="bp-button bp-primary" data-bp-focus="profile-account" onclick={onaccount}>{$t('bp.profile.account')}</button>
        {/if}
      </div>
    </section>

    <section class="stats-section">
      <h2>{$t('bp.profile.stats')}</h2>
      <div class="stat-grid">
        <article class="bp-card stat-card"><Gamepad2 size="1.45em" /><strong>{stats.games}</strong><span>{$t('bp.profile.gamesCount')}</span></article>
        <article class="bp-card stat-card"><Clock3 size="1.45em" /><strong>{stats.hours}</strong><span>{$t('bp.profile.hoursCount')}</span></article>
        <article class="bp-card stat-card"><Trophy size="1.45em" /><strong>{stats.completed}</strong><span>{$t('bp.profile.completedCount')}</span></article>
      </div>
    </section>

    <section class="games-section">
      <div class="section-title">
        <div><h2>{$t('bp.profile.games')}</h2><p>{$t('bp.profile.localLabel')}</p></div>
        {#if recentGames.length > 8}
          <button class="bp-button" data-bp-focus="profile-expand-games" aria-expanded={expanded} onclick={() => (expanded = !expanded)}>
            {expanded ? $t('bp.profile.showLess') : $t('bp.profile.showMore')}
          </button>
        {/if}
      </div>
      {#if visibleGames.length > 0}
        <div class="game-grid">
          {#each visibleGames as game (game.id)}
            <button class="game-card bp-card" data-bp-focus={`profile-game-${encodeURIComponent(game.id)}`} onclick={() => ongame(game.id)} aria-label={`${game.title} · ${playtime(game.playtimeSeconds)}`}>
              <span class="cover"><Artwork src={game.cover} alt="" label={game.title} ratio="3 / 4" radius="inherit" /></span>
              <span class="game-name">{game.title}</span>
              <span class="game-meta">{#if game.playtimeSeconds > 0}{playtime(game.playtimeSeconds)}{:else}{game.status || $t('bp.profile.localLabel')}{/if}</span>
            </button>
          {/each}
        </div>
      {:else}
        <div class="bp-empty empty-games"><Gamepad2 size="2.5rem" /><h3>{$t('bp.profile.noGamesTitle')}</h3><p>{$t('bp.profile.noGamesDescription')}</p></div>
      {/if}
    </section>
  {/if}
</div>

<style>
  .profile-page { display: flex; flex-direction: column; gap: clamp(1.2rem, 2.1vh, 2rem); padding-bottom: 2rem; }
  .title-copy { min-width: 0; }
  .eyebrow { display: block; margin-bottom: .35rem; color: var(--text-3); font-size: .72em; font-weight: 700; letter-spacing: .16em; }
  .bp-header h1 { margin: 0; font-size: clamp(2rem, 4vw, 4.6rem); line-height: 1.08; }
  .back-button { min-height: 3.4rem; padding-inline: 1.2rem; }
  .identity { display: flex; align-items: center; gap: clamp(1rem, 2.2vw, 2.5rem); padding: clamp(1.2rem, 2vw, 2.25rem); }
  .identity :global(.avatar.lg) { width: clamp(5rem, 8vw, 8.5rem); height: clamp(5rem, 8vw, 8.5rem); }
  .identity-copy { min-width: 0; flex: 1; }
  .identity-copy h2 { margin: 0; font-size: clamp(1.55rem, 2.6vw, 3rem); line-height: 1.12; overflow-wrap: anywhere; }
  .identity-copy p { margin: .45rem 0 0; color: var(--text-2); font-size: .95em; line-height: 1.45; }
  .identity-copy p.edit-message { color: var(--success); }
  .identity-copy p.edit-message.danger { color: var(--danger); }
  .identity-actions { flex: none; }
  .identity-actions .bp-button { min-height: 3.25rem; padding: .7rem 1.1rem; }
  .stats-section h2, .section-title h2 { margin: 0; font-size: clamp(1.25rem, 1.8vw, 2rem); }
  .stat-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: clamp(.65rem, 1.2vw, 1.2rem); margin-top: .75rem; }
  .stat-card { min-height: 7.5rem; display: grid; grid-template-columns: auto 1fr; grid-template-rows: auto auto; align-content: center; align-items: center; column-gap: .7rem; row-gap: .08rem; padding: clamp(1rem, 1.6vw, 1.7rem); }
  .stat-card :global(svg) { grid-row: span 2; color: var(--accent); }
  .stat-card strong { font-size: clamp(1.6rem, 2.4vw, 2.8rem); line-height: 1; font-variant-numeric: tabular-nums; }
  .stat-card span { color: var(--text-3); font-size: .86em; }
  .section-title { display: flex; align-items: flex-end; justify-content: space-between; gap: 1rem; margin-bottom: .8rem; }
  .section-title p { margin: .25rem 0 0; color: var(--text-3); font-size: .85em; }
  .section-title .bp-button { min-height: 2.9rem; padding: .6rem 1rem; }
  .game-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 11rem), 1fr)); gap: clamp(.7rem, 1.2vw, 1.2rem); }
  .game-card { display: flex; flex-direction: column; gap: .5rem; width: 100%; padding: .65rem; border-radius: var(--radius-lg); color: var(--text); text-align: left; overflow: hidden; }
  .cover { display: block; width: 100%; aspect-ratio: 3 / 4; overflow: hidden; border-radius: var(--radius-md); }
  .game-name { width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 650; }
  .game-meta { color: var(--text-3); font-size: .8em; }
  .empty-games { min-height: 11rem; }
  .empty-games h3 { margin: .5rem 0 0; }
  .empty-games p { margin: .3rem 0 0; max-width: 38rem; }
  @media (max-width: 760px) {
    .identity { align-items: flex-start; flex-wrap: wrap; }
    .identity-copy { flex-basis: calc(100% - 7rem); }
    .identity-actions { width: 100%; }
    .identity-actions .bp-button { width: 100%; }
    .stat-grid { grid-template-columns: 1fr; }
    .stat-card { min-height: 5.8rem; }
  }
</style>
