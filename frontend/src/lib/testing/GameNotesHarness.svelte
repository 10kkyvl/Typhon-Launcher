<script lang="ts">
  import GameNotes from '../../routes/game/GameNotes.svelte';

  // The notes block as the game page mounts it: once per game, created again when the page moves to
  // another game, gone when the page has no game. For GameNotes.dom.test.ts.
  let { game }: { game: { id: string; notes: string } | null } = $props();
</script>

{#if game}
  {#key game.id}
    <GameNotes gameId={game.id} stored={game.notes} />
  {/key}
{/if}
