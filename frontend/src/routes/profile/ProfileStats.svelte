<script lang="ts">
  import type { ProfileStats } from '../../lib/services/profile';
  import { formatCount } from '../../lib/utils/format';
  import HiddenBadge from './HiddenBadge.svelte';

  let { stats, hidden }: { stats: ProfileStats; hidden: boolean } = $props();

  const tiles = $derived([
    { label: 'Игры', value: formatCount(stats.games) },
    { label: 'Часов', value: formatCount(stats.hours) },
    { label: 'Пройдено', value: formatCount(stats.completed) },
    { label: 'Играю сейчас', value: formatCount(stats.playing) },
  ]);
</script>

<section class="group">
  <div class="group-head">
    <h3>Статистика</h3>
    {#if hidden}<HiddenBadge />{/if}
  </div>
  <div class="tiles">
    {#each tiles as tile (tile.label)}
      <div class="tile">
        <span class="value">{tile.value}</span>
        <span class="label">{tile.label}</span>
      </div>
    {/each}
  </div>
</section>

<style>
  .group {
    margin-bottom: var(--space-10);
  }

  .group-head {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-3);
  }

  h3 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(15rem, 1fr));
    gap: var(--space-4);
  }

  .tile {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .value {
    font-size: var(--font-title);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
