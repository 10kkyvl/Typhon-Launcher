<script lang="ts">
  import AppShell from './lib/components/AppShell.svelte';
  import MoveGameModal from './lib/components/MoveGameModal.svelte';
  import ReleaseNotesModal from './lib/components/ReleaseNotesModal.svelte';
  import TelemetryConsentScreen from './lib/components/TelemetryConsentScreen.svelte';
  import UpdateOverlay from './lib/components/UpdateOverlay.svelte';
  import { initDiscovery } from './lib/stores/discovery';
  import { initDownloads } from './lib/stores/downloads';
  import { initHistory } from './lib/stores/history';
  import { initInstalls } from './lib/stores/install';
  import { initLan } from './lib/stores/lan';
  import { route } from './lib/stores/router';
  import { initLibrary } from './lib/stores/library';
  import { initMetadata } from './lib/stores/metadata';
  import { initMoves } from './lib/stores/relocate';
  import { initSelfUpdate } from './lib/stores/selfupdate';
  import { initSettings, settings } from './lib/stores/settings';
  import { initSocial } from './lib/stores/social';
  import { initSources } from './lib/stores/sources';
  import { showTelemetryConsent } from './lib/stores/telemetryConsent';
  import { initUpdates } from './lib/stores/updates';
  import { authState, initAuth } from './lib/stores/user';
  import { refreshStorage } from './lib/stores/storage';
  import AuthScreen from './routes/auth/AuthScreen.svelte';
  import Catalog from './routes/catalog/Catalog.svelte';
  import Downloads from './routes/downloads/Downloads.svelte';
  import GameDetails from './routes/game/GameDetails.svelte';
  import Friends from './routes/friends/Friends.svelte';
  import History from './routes/history/History.svelte';
  import Installed from './routes/installed/Installed.svelte';
  import Lan from './routes/lan/Lan.svelte';
  import Library from './routes/library/Library.svelte';
  import Profile from './routes/profile/Profile.svelte';
  import Settings from './routes/settings/Settings.svelte';
  import Sources from './routes/sources/Sources.svelte';
  import UserProfile from './routes/user/UserProfile.svelte';

  initDownloads();
  initInstalls();
  initSettings().then(refreshStorage);
  initLibrary();
  initMoves();
  initSources();
  initUpdates();
  initSelfUpdate();
  initMetadata();
  initDiscovery();
  initAuth();
  initHistory();
  initLan();
  initSocial();

  let lastGamesPath: string | undefined;
  settings.subscribe((value) => {
    if (!value) return;
    if (lastGamesPath !== undefined && lastGamesPath !== value.gamesPath) {
      refreshStorage();
    }
    lastGamesPath = value.gamesPath;
  });
</script>

{#if $authState === 'bootstrapping'}
  <div class="boot">
    <img class="boot-mark" src="/typhon.png" alt="" draggable="false" />
  </div>
{:else if $showTelemetryConsent}
  <TelemetryConsentScreen />
{:else if $authState === 'authenticated' || $authState === 'guest' || $authState === 'offline'}
  <AppShell>
    {#if $route.name === 'library'}
      <Library />
    {:else if $route.name === 'catalog'}
      <Catalog />
    {:else if $route.name === 'game'}
      <GameDetails id={$route.params.id} />
    {:else if $route.name === 'downloads'}
      <Downloads />
    {:else if $route.name === 'sources'}
      <Sources />
    {:else if $route.name === 'installed'}
      <Installed />
    {:else if $route.name === 'history'}
      <History />
    {:else if $route.name === 'lan'}
      <Lan />
    {:else if $route.name === 'friends'}
      <Friends />
    {:else if $route.name === 'user'}
      <UserProfile username={$route.params.username} />
    {:else if $route.name === 'profile'}
      <Profile />
    {:else if $route.name === 'settings'}
      <Settings tab={$route.params.tab} />
    {/if}
  </AppShell>
  <UpdateOverlay />
  <ReleaseNotesModal />
  <MoveGameModal />
{:else}
  <AuthScreen />
{/if}

<style>
  .boot {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    background: var(--bg);
  }

  .boot-mark {
    width: 5.6rem;
    height: 5.6rem;
    animation: pulse 1.4s var(--ease) infinite;
  }

  @keyframes pulse {
    0%,
    100% {
      opacity: 0.35;
    }
    50% {
      opacity: 1;
    }
  }
</style>
