<script lang="ts">
  import GameQuickMenu from '../components/GameQuickMenu.svelte';
  import PageHeader from '../components/PageHeader.svelte';
  import { openGameMenu } from '../stores/gameMenu';
  import { libraryGames } from '../stores/library';

  // A page of game cards, each with its context menu, as on the library page; optionally a plain
  // note after them in the list (nothing to focus). For GameQuickMenu.dom.test.ts.
  let { note = false }: { note?: boolean } = $props();
</script>

<PageHeader title="Library" />
<ul>
  {#each $libraryGames as game (game.id)}
    <li class="card" oncontextmenu={(event) => openGameMenu(event, game.id)}>
      <a href={`#${game.id}`}>{game.title}</a>
    </li>
  {/each}
  {#if note}<li>That is the whole library.</li>{/if}
</ul>
<GameQuickMenu />
