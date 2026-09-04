<script lang="ts">
  import type { StatsView } from '../../lib/services/social';
  import { formatCount } from '../../lib/utils/format';
  import HiddenBadge from '../profile/HiddenBadge.svelte';

  let { stats }: { stats: StatsView | null } = $props();

  const hint = 'Владелец профиля скрыл эти данные';

  const tiles = $derived(
    stats
      ? [
          { label: 'Игры', value: formatCount(stats.games), hidden: false },
          { label: 'Часов', value: formatCount(stats.hours ?? 0), hidden: stats.hours == null },
          { label: 'Пройдено', value: formatCount(stats.completed), hidden: false },
        ]
      : [],
  );
</script>

<section class="group">
  <div class="group-head">
    <h3>Статистика</h3>
    {#if !stats}<HiddenBadge text={hint} />{/if}
  </div>
  {#if !stats}
    <p class="muted">Статистика скрыта</p>
  {:else}
    <div class="tiles">
      {#each tiles as tile (tile.label)}
        <div class="tile">
          <span class="value" class:hidden={tile.hidden}>
            {#if tile.hidden}
              <HiddenBadge text={hint} />
            {:else}
              {tile.value}
            {/if}
          </span>
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

  .value.hidden {
    display: inline-flex;
    align-items: center;
    min-height: calc(var(--font-title) * 1.1);
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
