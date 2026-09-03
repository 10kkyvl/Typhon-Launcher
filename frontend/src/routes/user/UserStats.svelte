<script lang="ts">
  import type { StatsView } from '../../lib/services/social';
  import { formatCount } from '../../lib/utils/format';

  let { stats }: { stats: StatsView | null } = $props();

  const tiles = $derived(
    stats
      ? [
          { label: 'Игры', value: formatCount(stats.games) },
          { label: 'Часов', value: stats.hours == null ? 'скрыто' : formatCount(stats.hours) },
          { label: 'Пройдено', value: formatCount(stats.completed) },
        ]
      : [],
  );
</script>

<section class="group">
  <h3>Статистика</h3>
  {#if tiles.length === 0}
    <p class="muted">Статистика скрыта</p>
  {:else}
    <div class="tiles">
      {#each tiles as tile (tile.label)}
        <div class="tile">
          <span class="value">{tile.value}</span>
          <span class="label">{tile.label}</span>
        </div>
      {/each}
    </div>
  {/if}
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

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
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
