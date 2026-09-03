<script lang="ts">
  import Card from '../../lib/components/Card.svelte';
  import type { ActivityDay } from '../../lib/services/profile';
  import { dayLabel } from '../../lib/profile/view';
  import { navigate } from '../../lib/stores/router';
  import { playtime } from '../../lib/utils/format';
  import HiddenBadge from './HiddenBadge.svelte';

  let { days, hidden }: { days: ActivityDay[]; hidden: boolean } = $props();
</script>

{#if days.length > 0}
  <Card>
    <div class="block-head">
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
  </Card>
{/if}

<style>
  .block-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
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
    font-size: var(--font-xs);
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-3);
    margin-bottom: 0.4rem;
  }

  ul {
    list-style: none;
  }

  .entry {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    width: 100%;
    padding: 0.6rem 0;
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .entry:hover .title {
    color: var(--accent-text);
  }

  .title {
    font-size: var(--font-md);
    font-weight: 500;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    transition: color var(--dur) var(--ease);
  }

  .played {
    font-size: var(--font-sm);
    color: var(--text-2);
    white-space: nowrap;
  }
</style>
