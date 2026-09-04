<script lang="ts">
  import type { ActivityView } from '../../lib/services/social';
  import { kindLabel } from '../../lib/social/feed';
  import { relativeDate } from '../../lib/utils/format';
  import GameRow from './GameRow.svelte';

  let { items }: { items: ActivityView[] } = $props();

  function meta(item: ActivityView): string {
    const label = kindLabel(item.kind);
    const when = relativeDate(item.createdAt);
    return label ? `${label} · ${when}` : when;
  }
</script>

<section class="group">
  <h3>Недавняя активность</h3>
  <div class="list">
    {#each items as item (item.id)}
      <GameRow game={item.game} meta={meta(item)} />
    {/each}
  </div>
</section>

<style>
  .group {
    margin-bottom: var(--space-10);
  }

  h3 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-3);
  }

  .list {
    display: flex;
    flex-direction: column;
  }

  .list :global(.row + .row) {
    border-top: 1px solid var(--border);
  }
</style>
