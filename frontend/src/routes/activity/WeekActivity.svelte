<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import { coverOf } from '../../lib/profile/view';
  import type { WeekDay, WeekSummary } from '../../lib/profile/week';
  import { gameArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { playtime, shortDate, weekday } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  let { week }: { week: WeekSummary } = $props();

  function barHeight(day: WeekDay): string {
    if (day.seconds <= 0 || week.bestSeconds <= 0) return '0';
    return `${Math.max(8, Math.round((day.seconds / week.bestSeconds) * 100))}%`;
  }

  function dayTitle(day: WeekDay): string {
    const value = day.seconds > 0 ? playtime(day.seconds) : msg('transfers.activityWeekIdle');
    return `${shortDate(day.at)} · ${value}`;
  }
</script>

<Card title={msg('transfers.activityWeekTitle')}>
  {#if week.totalSeconds === 0}
    <p class="muted">{msg('transfers.activityWeekEmpty')}</p>
  {:else}
    <div class="total">
      <span class="value">{playtime(week.totalSeconds)}</span>
      <span class="window">{msg('transfers.activityWeekWindow')}</span>
    </div>

    <div class="chart">
      {#each week.days as day (day.date)}
        <span class="col" class:today={day.today} title={dayTitle(day)}>
          <span class="track">
            <span class="fill" style:height={barHeight(day)}></span>
          </span>
          <span class="day">{weekday(day.at)}</span>
        </span>
      {/each}
    </div>

    {#if week.games.length > 0}
      <div class="games">
        {#each week.games as item (item.game.id)}
          <button class="row" type="button" onclick={() => navigate('game', { id: item.game.id })}>
            <span class="cover">
              <Artwork src={coverOf(item.game, $gameArt)} alt={item.game.title} ratio="3 / 4" radius="var(--radius-sm)" />
            </span>
            <span class="text">
              <span class="title">{item.game.title}</span>
              <span class="sub">{playtime(item.seconds)}</span>
            </span>
          </button>
        {/each}
      </div>
    {/if}
  {/if}

  <div class="all">
    <Button onclick={() => navigate('profile')}>{msg('social.viewAllActivity')}</Button>
  </div>
</Card>

<style>
  .total {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .value {
    font-size: var(--font-title);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
    font-variant-numeric: tabular-nums;
  }

  .window {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .chart {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: var(--space-1);
    margin-top: var(--space-5);
  }

  .col {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.6rem;
    width: 100%;
  }

  .track {
    display: flex;
    align-items: flex-end;
    justify-content: center;
    width: 100%;
    height: 6.4rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    overflow: hidden;
  }

  .fill {
    display: block;
    width: 100%;
    min-height: 0;
    border-radius: var(--radius-sm);
    background: var(--accent);
    opacity: 0.45;
    transition: height var(--dur-panel) var(--ease);
  }

  .col.today .fill {
    opacity: 1;
  }

  .day {
    font-size: 1.2rem;
    color: var(--text-3);
  }

  .col.today .day {
    color: var(--text-2);
  }

  .games {
    display: flex;
    flex-direction: column;
    margin: var(--space-4) calc(var(--space-4) * -1) 0;
    padding-top: var(--space-4);
    border-top: 1px solid var(--border);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.6rem var(--space-4);
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .cover {
    display: block;
    flex-shrink: 0;
    width: 3.2rem;
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .title {
    font-size: var(--font-sm);
    font-weight: 500;
    line-height: 1.3;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub {
    font-size: 1.2rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .all {
    margin-top: var(--space-4);
  }

  .all :global(.btn) {
    width: 100%;
  }
</style>
