<script lang="ts">
  import { onMount } from 'svelte';
  import Card from '../../lib/components/Card.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { type ProfileSettings, DEFAULT_PROFILE } from '../../lib/services/account';
  import { initProfile, profileSnapshot } from '../../lib/stores/profile';
  import { libraryGames } from '../../lib/stores/library';
  import { gameArt, loadArt } from '../../lib/stores/metadata';
  import { authState, currentUser } from '../../lib/stores/user';
  import { coverOf } from '../../lib/profile/view';
  import { playtime, relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import ProfileActivity from './ProfileActivity.svelte';
  import ProfileHeader from './ProfileHeader.svelte';
  import ProfilePlaying from './ProfilePlaying.svelte';
  import ProfileSettingsModal from './ProfileSettingsModal.svelte';
  import ProfileShowcase from './ProfileShowcase.svelte';

  import ProfileCanvas from '../../lib/components/ProfileCanvas.svelte';
  import ProfileCover from '../../lib/components/ProfileCover.svelte';
  import ProfileAppearancePanel from './ProfileAppearancePanel.svelte';
  import { getProfilePreview, type ProfileSnapshot } from '../../lib/services/profile';
  import { appearanceOf } from '../../lib/profile/appearance';

  let appearanceOpen = $state(false);
  let preview = $state<ProfileSettings | null>(null);
  let allShowcases = $state<ProfileSnapshot | null>(null);
  let settingsOpen = $state(false);

  const isGuest = $derived($authState === 'guest');
  const settings = $derived((isGuest ? null : $currentUser?.profile) ?? DEFAULT_PROFILE);
  const shown = $derived(preview ?? settings);
  const appearance = $derived(appearanceOf(shown.appearance));
  const blocks = $derived(shown.showcase.map((kind) => (allShowcases ?? $profileSnapshot).showcase.find((b) => b.kind === kind) ?? { kind, games: [] }));
  const bio = $derived(!isGuest ? ($currentUser?.bio ?? '') : '');

  function lastPlayedOf(id: string): string | null {
    return $libraryGames.find((game) => game.id === id)?.lastPlayed ?? null;
  }

  function openSettings() {
    appearanceOpen = false;
    preview = null;
    settingsOpen = true;
  }

  $effect(() => {
    const snapshot = $profileSnapshot;
    const ids = [
      ...snapshot.playing.map((entry) => entry.game.canonicalGameId),
      ...snapshot.running.map((game) => game.canonicalGameId),
      ...(allShowcases ?? snapshot).showcase.flatMap((block) => block.games.map((game) => game.canonicalGameId)),
    ].filter((id): id is string => Boolean(id));
    if (ids.length > 0) loadArt(ids);
  });

  $effect(() => {
    let active = true;
    $currentUser?.id;
    if (appearanceOpen) {
      getProfilePreview().then((snapshot) => { if (active) allShowcases = snapshot; }).catch((err) => console.error('profile preview failed', err));
    } else { allShowcases = null; }
    return () => { active = false; };
  });

  onMount(() => {
    initProfile();
  });
</script>

<PageHeader title={msg('social.profileLabel')} />

<div class="workspace" class:customizing={appearanceOpen}>
<div class="profile">
<ProfileCanvas {appearance}>
  <ProfileCover {appearance} />
  <ProfileHeader
    running={$profileSnapshot.running}
    stats={$profileSnapshot.stats}
    showOnline={shown.showOnline}
    showPlaying={shown.showPlaying}
    showStats={shown.showStats}
    onappearance={() => { appearanceOpen = true; settingsOpen = false; }}
    onsettings={openSettings}
  />

  {#if !isGuest}
    <div class="columns">
      <div class="main">
        <ProfileShowcase {blocks} showEmpty={appearanceOpen} onmanage={() => (appearanceOpen = true)} />
        {#if $profileSnapshot.playing.length > 0}
          <Card title={msg('social.recentlyPlayedTitle')}>
            <div class="recent-row">
              {#each $profileSnapshot.playing as entry (entry.game.id)}
                <div class="recent-item">
                  <GameCard id={entry.game.id} title={entry.game.title} cover={coverOf(entry.game, $gameArt)} variant="capsule">
                    {#snippet footer()}
                      <span class="recent-meta">
                        <span class="dot"></span>
                        {playtime(entry.game.playtimeSeconds)} · {relativeDate(lastPlayedOf(entry.game.id))}
                      </span>
                    {/snippet}
                  </GameCard>
                </div>
              {/each}
            </div>
          </Card>
        {/if}

        <div class="pair">
          <div class="pair-left">
            <ProfileActivity days={$profileSnapshot.activity} hidden={!shown.showActivity} />
          </div>
        </div>
      </div>
      <div class="side">
        <ProfilePlaying running={$profileSnapshot.running} hidden={!shown.showPlaying} />
        {#if bio}
          <Card title={msg('social.aboutTitle')}>
            <p class="bio">{bio}</p>
          </Card>
        {/if}
      </div>
    </div>
  {/if}
</ProfileCanvas>
</div>
{#if !isGuest && appearanceOpen}
  {#key $currentUser?.id}
    <ProfileAppearancePanel {settings} onpreview={(value) => (preview = value)} onclose={() => { appearanceOpen = false; preview = null; }} />
  {/key}
{/if}
</div>

{#if !isGuest && settingsOpen}
  <ProfileSettingsModal bind:open={settingsOpen} {settings} />
{/if}

<style>
  .workspace { display: flex; gap: 1.6rem; align-items: flex-start; }
  .profile {
    flex: 1; min-width: 0;
    display: flex;
    flex-direction: column;
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
    grid-template-columns: 1fr;
    gap: var(--space-6);
    align-items: start;
  }

  .pair-left {
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

  .recent-row {
    display: flex;
    gap: var(--space-4);
    overflow-x: auto;
    padding-bottom: var(--space-2);
  }

  .recent-item {
    flex: 0 0 auto;
    width: 22rem;
  }

  .recent-meta {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .recent-meta .dot {
    width: 0.7rem;
    height: 0.7rem;
    border-radius: 50%;
    background: var(--success);
    flex-shrink: 0;
  }

  @media (min-width: 1250px) {
    .columns {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 28rem;
      gap: 0 var(--space-6);
      align-items: start;
    }
  }

  .customizing .columns { display: flex; align-items: stretch; }
  @media (max-width: 1000px) {
    .workspace.customizing { flex-direction: column-reverse; }
    .profile { width: 100%; }
  }
  @media (max-width: 1200px) {
    .pair {
      grid-template-columns: 1fr;
    }
  }
</style>
