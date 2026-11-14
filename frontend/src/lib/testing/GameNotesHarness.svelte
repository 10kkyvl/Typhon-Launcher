<script lang="ts">
  import GameNotes from '../../routes/game/GameNotes.svelte';

  // The notes block on a game page that moves to another game: created again per game (`keyed`, as
  // the page mounts it today) or kept and handed the next game in place; gone when the page has no
  // game. For GameNotes.dom.test.ts.
  let { game, keyed = true }: { game: { id: string; notes: string } | null; keyed?: boolean } = $props();
</script>

{#if game}
  {#if keyed}
    {#key game.id}
      <GameNotes gameId={game.id} stored={game.notes} />
    {/key}
  {:else}
    <GameNotes gameId={game.id} stored={game.notes} />
  {/if}
{/if}
