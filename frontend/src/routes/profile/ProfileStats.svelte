<script lang="ts">
  import Card from '../../lib/components/Card.svelte';
  import type { ProfileStats } from '../../lib/services/profile';
  import { hoursLabel } from '../../lib/profile/view';
  import { formatCount } from '../../lib/utils/format';
  import HiddenBadge from './HiddenBadge.svelte';

  let { stats, hidden }: { stats: ProfileStats; hidden: boolean } = $props();

  const tiles = $derived([
    { label: 'Игры', value: formatCount(stats.games) },
    { label: 'Часов', value: hoursLabel(stats.hours * 3600) },
    { label: 'Пройдено', value: formatCount(stats.completed) },
    { label: 'Играю сейчас', value: formatCount(stats.playing) },
  ]);
</script>

<Card>
  <div class="stats-head">
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
</Card>

<style>
  .stats-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-4);
  }

  h3 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: var(--space-4);
  }

  .tile {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    padding: var(--space-3) var(--space-4);
    background: var(--surface-2);
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

  @media (max-width: 1100px) {
    .tiles {
      grid-template-columns: repeat(2, 1fr);
    }
  }
</style>
