<script lang="ts">
  import GameDetails from '../../routes/game/GameDetails.svelte';
  import AppShell from '../components/AppShell.svelte';
  import { route } from '../stores/router';

  // The game route as App.svelte renders it: the page inside the shell, its id from the route. For
  // GameDetails.data.dom.test.ts, which goes from one game to another the way the app does.
</script>

<AppShell>
  {#if $route.name === 'game'}
    <GameDetails id={$route.params.id} />
  {/if}
</AppShell>
