<script lang="ts">
  import { Gamepad2 } from '@lucide/svelte';
  import Card from '../../lib/components/Card.svelte';
  import type { ActivityDay } from '../../lib/services/profile';
  import { dayLabel } from '../../lib/profile/view';
  import { navigate } from '../../lib/stores/router';
  import { playtime } from '../../lib/utils/format';
  import HiddenBadge from './HiddenBadge.svelte';

  let { days, hidden }: { days: ActivityDay[]; hidden: boolean } = $props();

  const rows = $derived(
    days.flatMap((day) => day.entries.map((entry) => ({ date: day.date, entry }))),
  );
</script>

{#if rows.length > 0}
  <Card title="Недавняя активность">
    {#snippet action()}
      {#if hidden}<HiddenBadge />{/if}
    {/snippet}
    <ul class="list">
      {#each rows as row (`${row.date}:${row.entry.game.id}`)}
        <li>
          <button class="row" type="button" onclick={() => navigate('game', { id: row.entry.game.id })}>
            <span class="icon"><Gamepad2 size="1.7rem" strokeWidth={1.8} /></span>
            <span class="text">
              <span class="title">{row.entry.game.title}</span>
              <span class="sub">Сыграно {playtime(row.entry.seconds)}</span>
            </span>
            <span class="when">{dayLabel(row.date)}</span>
          </button>
        </li>
      {/each}
    </ul>
    <button class="all" type="button" onclick={() => navigate('history')}>Смотреть всю активность</button>
  </Card>
{/if}

<style>
  .list {
    list-style: none;
    display: flex;
    flex-direction: column;
  }

  .list li + li .row {
    border-top: 1px solid var(--border);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.8rem;
    margin: 0 calc(var(--space-3) * -1);
    background: none;
    border: 0;
    border-radius: var(--radius-md);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.6rem;
    height: 3.6rem;
    flex-shrink: 0;
    border-radius: var(--radius-md);
    background: var(--surface-3);
    color: var(--text-2);
  }

  .text {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .title {
    font-size: var(--font-sm);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .when {
    flex-shrink: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  .all {
    display: block;
    width: 100%;
    margin-top: var(--space-4);
    padding: 0.9rem;
    background: none;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    color: var(--text-2);
    font-size: var(--font-sm);
    font-weight: 500;
    text-align: center;
    cursor: pointer;
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .all:hover {
    background: var(--hover);
    color: var(--text);
  }
</style>
