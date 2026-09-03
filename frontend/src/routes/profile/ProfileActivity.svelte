<script lang="ts">
  import type { ActivityDay } from '../../lib/services/profile';
  import { dayLabel } from '../../lib/profile/view';
  import { navigate } from '../../lib/stores/router';
  import { playtime } from '../../lib/utils/format';
  import HiddenBadge from './HiddenBadge.svelte';

  let { days, hidden }: { days: ActivityDay[]; hidden: boolean } = $props();
</script>

{#if days.length > 0}
  <section class="group">
    <div class="group-head">
      <h3>Недавняя активность</h3>
      {#if hidden}<HiddenBadge />{/if}
    </div>
    <div class="days">
      {#each days as day (day.date)}
        <section class="day">
          <h4>{dayLabel(day.date)}</h4>
          <ul>
            {#each day.entries as entry (entry.game.id)}
              <li>
                <button class="entry" onclick={() => navigate('game', { id: entry.game.id })}>
                  <span class="title">{entry.game.title}</span>
                  <span class="played">Сыграно {playtime(entry.seconds)}</span>
                </button>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    </div>
  </section>
{/if}

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

  .days {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  h4 {
    font-size: 1.2rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
    margin-bottom: var(--space-2);
  }

  ul {
    list-style: none;
  }

  .entry {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    width: 100%;
    padding: 0.8rem;
    background: none;
    border: 0;
    border-radius: var(--radius-md);
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition: background var(--dur) var(--ease);
  }

  .entry:hover {
    background: var(--hover);
  }

  .title {
    font-size: var(--font-md);
    font-weight: 500;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .played {
    font-size: var(--font-sm);
    color: var(--text-3);
    white-space: nowrap;
  }
</style>
