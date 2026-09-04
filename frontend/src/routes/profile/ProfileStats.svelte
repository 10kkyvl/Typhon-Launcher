<script lang="ts">
  import { CheckCircle2, Clock, Gamepad2 } from '@lucide/svelte';
  import StatTile from '../../lib/components/StatTile.svelte';
  import type { ProfileStats } from '../../lib/services/profile';
  import { formatCount } from '../../lib/utils/format';
  import HiddenBadge from './HiddenBadge.svelte';

  let { stats, hidden }: { stats: ProfileStats; hidden: boolean } = $props();
</script>

<div class="stats-row">
  <div class="stat">
    <StatTile value={formatCount(stats.games)} label="Игр">
      {#snippet icon()}<Gamepad2 size="1.8rem" strokeWidth={1.8} />{/snippet}
    </StatTile>
  </div>
  <div class="stat">
    <StatTile value={formatCount(stats.hours)} label="Часов">
      {#snippet icon()}<Clock size="1.8rem" strokeWidth={1.8} />{/snippet}
    </StatTile>
  </div>
  <div class="stat">
    <StatTile value={formatCount(stats.completed)} label="Пройдено">
      {#snippet icon()}<CheckCircle2 size="1.8rem" strokeWidth={1.8} />{/snippet}
    </StatTile>
  </div>
  {#if hidden}
    <div class="hidden-flag">
      <HiddenBadge text="Статистика скрыта от других. Вы её видите." />
    </div>
  {/if}
</div>

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

  .hidden-flag {
    display: flex;
    align-items: center;
    margin-left: var(--space-3);
  }
</style>
