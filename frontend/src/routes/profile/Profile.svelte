<script lang="ts">
  import { onMount } from 'svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { DEFAULT_PROFILE } from '../../lib/services/account';
  import { initProfile, profileSnapshot } from '../../lib/stores/profile';
  import { authState, currentUser } from '../../lib/stores/user';
  import ProfileActivity from './ProfileActivity.svelte';
  import ProfileHeader from './ProfileHeader.svelte';
  import ProfilePlaying from './ProfilePlaying.svelte';
  import ProfileSettingsModal from './ProfileSettingsModal.svelte';
  import ProfileShowcase from './ProfileShowcase.svelte';
  import ProfileStats from './ProfileStats.svelte';

  let settingsOpen = $state(false);

  const isGuest = $derived($authState === 'guest');
  const settings = $derived((isGuest ? null : $currentUser?.profile) ?? DEFAULT_PROFILE);

  onMount(() => {
    initProfile();
  });
</script>

<PageHeader title="Профиль" />

<div class="profile">
  <div class="main">
    <div class="area area-header">
      <ProfileHeader
        running={$profileSnapshot.running}
        showOnline={settings.showOnline}
        showPlaying={settings.showPlaying}
        onsettings={() => (settingsOpen = true)}
      />
    </div>
    <div class="area area-activity">
      <ProfileActivity days={$profileSnapshot.activity} hidden={!settings.showActivity} />
    </div>
    <div class="area area-showcase">
      <ProfileShowcase blocks={$profileSnapshot.showcase} />
    </div>
  </div>
  <div class="side">
    <div class="area area-stats">
      <ProfileStats stats={$profileSnapshot.stats} hidden={!settings.showStats} />
    </div>
    <div class="area area-playing">
      <ProfilePlaying entries={$profileSnapshot.playing} hidden={!settings.showPlaying} />
    </div>
  </div>
</div>

{#if !isGuest && settingsOpen}
  <ProfileSettingsModal bind:open={settingsOpen} {settings} />
{/if}

<style>
  .profile {
    display: flex;
    flex-direction: column;
  }

  .main,
  .side {
    display: contents;
  }

  .area {
    min-width: 0;
  }

  .area-header {
    order: 1;
  }

  .area-stats {
    order: 2;
  }

  .area-playing {
    order: 3;
  }

  .area-activity {
    order: 4;
  }

  .area-showcase {
    order: 5;
  }

  @media (min-width: 1600px) {
    .profile {
      display: grid;
      grid-template-columns: minmax(0, 1fr) 40rem;
      gap: 0 var(--space-12);
      align-items: start;
    }

    .main,
    .side {
      display: flex;
      flex-direction: column;
      min-width: 0;
    }
  }
</style>
