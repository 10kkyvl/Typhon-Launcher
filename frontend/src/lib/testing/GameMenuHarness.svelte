<script lang="ts">
  import { Ellipsis } from '@lucide/svelte';
  import GameCard from '../components/GameCard.svelte';
  import GameQuickMenu from '../components/GameQuickMenu.svelte';
  import PageHeader from '../components/PageHeader.svelte';
  import { msg } from '../i18n';
  import { gameContextMenu, gameMenu, gameMenuButton } from '../stores/gameMenu';
  import { libraryGames } from '../stores/library';

  // A page of games, each with its context menu: rows as on the library page, or (`cards`) game cards
  // with a "…" menu button as on the catalog; a control and an icon button before them; optionally a
  // plain note after the rows (nothing to focus). For GameQuickMenu.dom.test.ts and ContextMenu.dom.test.ts.
  let { note = false, cards = false }: { note?: boolean; cards?: boolean } = $props();
</script>

<div>
  <button type="button">Outside</button>
  <button type="button" aria-label="Help">
    <svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0 0h10v10H0z" /></svg>
  </button>
</div>
<PageHeader title="Library" />
{#if cards}
  <div class="grid">
    {#each $libraryGames as game (game.id)}
      <GameCard
        id={game.id}
        title={game.title}
        actions={[{ id: 'more', label: msg('games.installedMenuFor', { title: game.title }), icon: Ellipsis, menu: gameMenuButton(game.id, `more-${game.id}`, $gameMenu) }]}
      />
    {/each}
  </div>
{:else}
  <ul>
    {#each $libraryGames as game (game.id)}
      <li class="card" {@attach gameContextMenu(game.id)}>
        <a href={`#${game.id}`}>{game.title}</a>
      </li>
    {/each}
    {#if note}<li>That is the whole library.</li>{/if}
  </ul>
{/if}
<GameQuickMenu />
