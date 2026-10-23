<script lang="ts">
  import Card from '../Card.svelte';
  import CollectionBlock from './CollectionBlock.svelte';
  import { msg } from '../../i18n';
  import type { BlockGame } from '../../profile/layoutView';

  let { year, total, games }: { year: number; total: number; games: BlockGame[] } = $props();

  const title = $derived(msg('profile.completedTitle', { year: String(year), total: String(total) }));
</script>

{#if games.length === 0}
  <Card {title}>
    <p class="empty">{msg('profile.completedEmpty')}</p>
  </Card>
{:else}
  <CollectionBlock {title} {games} dated />
{/if}

<style>
  .empty {
    padding: var(--space-4) 0;
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
