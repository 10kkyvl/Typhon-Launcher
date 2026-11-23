<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import BarChart from '../../lib/components/BarChart.svelte';
  import type { ChartBucket } from '../../lib/components/barChart';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import { coverOf } from '../../lib/profile/view';
  import type { WeekSummary } from '../../lib/profile/week';
  import { gameArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { playtime, shortDate, weekday } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';

  let { week }: { week: WeekSummary } = $props();

  const bars = $derived<ChartBucket[]>(
    week.days.map((day) => ({ key: day.date, label: shortDate(day.at), tick: weekday(day.at), value: day.seconds, current: day.today })),
  );

  const playedWords = (seconds: number) => (seconds > 0 ? playtime(seconds) : msg('transfers.activityWeekIdle'));
</script>

<Card title={msg('transfers.activityWeekTitle')}>
  {#if week.totalSeconds === 0}
    <EmptyState size="inline" title={msg('transfers.activityWeekEmpty')} />
  {:else}
    <div class="total">
      <span class="value">{playtime(week.totalSeconds)}</span>
      <span class="window">{msg('transfers.activityWeekWindow')}</span>
    </div>

    <!-- A narrow card: no axis and no value over the bars; the total above and the table say them. -->
    <div class="chart">
      <BarChart
        label={msg('transfers.activityWeekTitle')}
        buckets={bars}
        format={playedWords}
        values="selected"
        height="6.4rem"
        columns={[msg('stats.chartDay'), msg('stats.chartPlaytime')]}
      />
    </div>

    {#if week.games.length > 0}
      <!-- An archived game has no page: its title is plain text and its cover a picture only. -->
      <ul class="games">
        {#each week.games as item (item.game.id)}
          {@const id = item.game.archived ? null : item.game.id}
          <li class="row">
            <span class="cover">
              <GameLink {id} name={item.game.title} repeat>
                <Artwork src={coverOf(item.game, $gameArt)} alt="" label={item.game.title} ratio="3 / 4" radius="var(--radius-sm)" />
              </GameLink>
            </span>
            <span class="text">
              <span class="title"><GameLink {id} name={item.game.title} truncate /></span>
              <span class="sub">{playtime(item.seconds)}</span>
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  {/if}

  <div class="all">
    <Button block onclick={() => navigate('profile')}>{msg('social.viewAllActivity')}</Button>
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
    margin-top: var(--space-5);
  }

  .games {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: var(--space-4) 0 0;
    padding: var(--space-4) 0 0;
    border-top: 1px solid var(--border);
    list-style: none;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-1) 0;
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

  .all {
    margin-top: var(--space-4);
  }

</style>
