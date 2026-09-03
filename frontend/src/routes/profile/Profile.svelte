<script lang="ts">
  import { onMount } from 'svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import { DEFAULT_PROFILE } from '../../lib/services/account';
  import { initProfile, profileSnapshot, refreshProfile } from '../../lib/stores/profile';
  import { authState, currentUser } from '../../lib/stores/user';
  import ProfileActivity from './ProfileActivity.svelte';
  import ProfileHeader from './ProfileHeader.svelte';
  import ProfilePlaying from './ProfilePlaying.svelte';
  import ProfileSettingsModal from './ProfileSettingsModal.svelte';
  import ProfileShowcase from './ProfileShowcase.svelte';
  import ProfileStats from './ProfileStats.svelte';

  let settingsOpen = $state(false);

  const isGuest = $derived($authState === 'guest');
  const settings = $derived(!isGuest && $currentUser ? $currentUser.profile : DEFAULT_PROFILE);

  onMount(() => {
    initProfile();
    void refreshProfile();
  });
</script>

<PageHeader title="Профиль" />

<div class="profile">
  <ProfileHeader running={$profileSnapshot.running} showOnline={settings.showOnline} onsettings={() => (settingsOpen = true)} />
  <ProfileStats stats={$profileSnapshot.stats} hidden={!settings.showStats} />
  <ProfilePlaying entries={$profileSnapshot.playing} hidden={!settings.showPlaying} />
  <ProfileActivity days={$profileSnapshot.activity} hidden={!settings.showActivity} />
  <ProfileShowcase blocks={$profileSnapshot.showcase} />
</div>

{#if !isGuest}
  <ProfileSettingsModal bind:open={settingsOpen} {settings} />
{/if}

<style>
  .profile {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-width: 96rem;
  }
</style>
