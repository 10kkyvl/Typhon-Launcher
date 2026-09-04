<script lang="ts">
  import { onMount } from 'svelte';
  import Card from '../../lib/components/Card.svelte';
  import GameCard from '../../lib/components/GameCard.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { DEFAULT_PROFILE } from '../../lib/services/account';
  import { initProfile, profileSnapshot } from '../../lib/stores/profile';
  import { libraryGames } from '../../lib/stores/library';
  import { authState, currentUser } from '../../lib/stores/user';
  import { playtime, relativeDate } from '../../lib/utils/format';
  import ProfileActivity from './ProfileActivity.svelte';
  import ProfileHeader from './ProfileHeader.svelte';
  import ProfilePlaying from './ProfilePlaying.svelte';
  import ProfileSettingsModal from './ProfileSettingsModal.svelte';
  import ProfileShowcase from './ProfileShowcase.svelte';

  let settingsOpen = $state(false);

  const isGuest = $derived($authState === 'guest');
  const settings = $derived((isGuest ? null : $currentUser?.profile) ?? DEFAULT_PROFILE);
  const bio = $derived(!isGuest ? ($currentUser?.bio ?? '') : '');

  function lastPlayedOf(id: string): string | null {
    return $libraryGames.find((game) => game.id === id)?.lastPlayed ?? null;
  }

  function openSettings() {
    settingsOpen = true;
  }

  onMount(() => {
    initProfile();
  });
</script>

<PageHeader title="Профиль" />

<div class="profile">
  <ProfileHeader
    running={$profileSnapshot.running}
    stats={$profileSnapshot.stats}
    showOnline={settings.showOnline}
    showPlaying={settings.showPlaying}
    showStats={settings.showStats}
    onsettings={openSettings}
  />

  {#if !isGuest}
    <div class="columns">
      <div class="main">
        {#if $profileSnapshot.playing.length > 0}
          <Card title="Недавно играл">
            <div class="recent-row">
              {#each $profileSnapshot.playing as entry (entry.game.id)}
                <div class="recent-item">
                  <GameCard id={entry.game.id} title={entry.game.title} cover={entry.game.cover} variant="capsule">
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
            <ProfileActivity days={$profileSnapshot.activity} hidden={!settings.showActivity} />
          </div>
          <div class="pair-right">
            <ProfileShowcase blocks={$profileSnapshot.showcase} onmanage={openSettings} />
          </div>
        </div>
      </div>
      <div class="side">
        <ProfilePlaying running={$profileSnapshot.running} hidden={!settings.showPlaying} />
        {#if bio}
          <Card title="О себе">
            <p class="bio">{bio}</p>
          </Card>
        {/if}
      </div>
    </div>
  {/if}
</div>

{#if !isGuest && settingsOpen}
  <ProfileSettingsModal bind:open={settingsOpen} {settings} />
{/if}

<style>
  .profile {
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
    grid-template-columns: 1fr 1fr;
    gap: var(--space-6);
    align-items: start;
  }

  .pair-left,
  .pair-right {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
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
