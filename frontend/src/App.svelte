<script lang="ts">
  import AppShell from './lib/components/AppShell.svelte';
  import { initDownloads } from './lib/stores/downloads';
  import { initInstalls } from './lib/stores/install';
  import { route } from './lib/stores/router';
  import { initLibrary } from './lib/stores/library';
  import { initSettings, settings } from './lib/stores/settings';
  import { initSources } from './lib/stores/sources';
  import { refreshStorage } from './lib/stores/storage';
  import Achievements from './routes/achievements/Achievements.svelte';
  import Collections from './routes/collections/Collections.svelte';
  import Downloads from './routes/downloads/Downloads.svelte';
  import GameDetails from './routes/game/GameDetails.svelte';
  import Installed from './routes/installed/Installed.svelte';
  import Library from './routes/library/Library.svelte';
  import Settings from './routes/settings/Settings.svelte';
  import Sources from './routes/sources/Sources.svelte';

  initDownloads();
  initInstalls();
  initSettings().then(refreshStorage);
  initLibrary();
  initSources();

  let lastGamesPath: string | undefined;
  settings.subscribe((value) => {
    if (!value) return;
    if (lastGamesPath !== undefined && lastGamesPath !== value.gamesPath) {
      refreshStorage();
    }
    lastGamesPath = value.gamesPath;
  });
</script>

<AppShell>
  {#if $route.name === 'library'}
    <Library />
  {:else if $route.name === 'game'}
    <GameDetails id={$route.params.id} />
  {:else if $route.name === 'downloads'}
    <Downloads />
  {:else if $route.name === 'sources'}
    <Sources />
  {:else if $route.name === 'installed'}
    <Installed />
  {:else if $route.name === 'collections'}
    <Collections />
  {:else if $route.name === 'achievements'}
    <Achievements />
  {:else if $route.name === 'settings'}
    <Settings />
  {/if}
</AppShell>
