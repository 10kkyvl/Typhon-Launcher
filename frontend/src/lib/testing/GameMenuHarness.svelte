<script lang="ts">
  import GameQuickMenu from '../components/GameQuickMenu.svelte';
  import PageHeader from '../components/PageHeader.svelte';
  import { openGameMenu } from '../stores/gameMenu';
  import { libraryGames } from '../stores/library';

  // A page of game cards, each with its context menu, as on the library page. For GameQuickMenu.dom.test.ts.
</script>

<PageHeader title="Library" />
<ul>
  {#each $libraryGames as game (game.id)}
    <li class="card" oncontextmenu={(event) => openGameMenu(event, game.id)}>
      <a href={`#${game.id}`}>{game.title}</a>
    </li>
  {/each}
</ul>
<GameQuickMenu />
