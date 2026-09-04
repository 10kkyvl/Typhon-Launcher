<script lang="ts">
  import { CheckCircle2, Clock, Gamepad2 } from '@lucide/svelte';
  import StatTile from '../../lib/components/StatTile.svelte';
  import type { StatsView } from '../../lib/services/social';
  import { formatCount } from '../../lib/utils/format';
  import HiddenBadge from '../profile/HiddenBadge.svelte';

  let { stats }: { stats: StatsView | null } = $props();

  const hint = 'Владелец профиля скрыл эти данные';
</script>

{#if stats}
  <div class="stats-row">
    <div class="stat">
      <StatTile value={formatCount(stats.games)} label="Игр">
        {#snippet icon()}<Gamepad2 size="1.8rem" strokeWidth={1.8} />{/snippet}
      </StatTile>
    </div>
    <div class="stat">
      {#if stats.hours == null}
        <div class="tile">
          <span class="icon"><Clock size="1.8rem" strokeWidth={1.8} /></span>
          <HiddenBadge text={hint} />
          <span class="label">Часов</span>
        </div>
      {:else}
        <StatTile value={formatCount(stats.hours)} label="Часов">
          {#snippet icon()}<Clock size="1.8rem" strokeWidth={1.8} />{/snippet}
        </StatTile>
      {/if}
    </div>
    <div class="stat">
      <StatTile value={formatCount(stats.completed)} label="Пройдено">
        {#snippet icon()}<CheckCircle2 size="1.8rem" strokeWidth={1.8} />{/snippet}
      </StatTile>
    </div>
  </div>
{:else}
  <div class="stats-hidden">
    <HiddenBadge text={hint} />
    <span class="label">Статистика скрыта</span>
  </div>
{/if}

<style>
  .stats-row {
    display: flex;
    align-items: flex-start;
  }

  .stat {
    padding: 0 var(--space-6);
    border-left: 1px solid var(--border);
  }

  .stat:first-child {
    padding-left: 0;
    border-left: 0;
  }

  .tile {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.8rem;
    min-width: 0;
  }

  .tile .icon {
    display: inline-flex;
    color: var(--text-3);
  }

  .tile .label {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .stats-hidden {
    display: flex;
    align-items: center;
    gap: 0.8rem;
  }

  .stats-hidden .label {
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
