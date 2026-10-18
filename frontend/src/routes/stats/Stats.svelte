<script lang="ts">
  import { onMount } from 'svelte';
  import { ChartColumn } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import StatTile from '../../lib/components/StatTile.svelte';
  import { coverOf } from '../../lib/profile/view';
  import { getStats, onPlaylogRecorded, STATS_PERIODS, type Stats, type StatsPeriod } from '../../lib/services/playlog';
  import { barHeight, bestSeconds, chartBuckets, labelEvery, type ChartBucket } from '../../lib/stats/buckets';
  import { gameTitle, summarize } from '../../lib/stats/summary';
  import { libraryGames } from '../../lib/stores/library';
  import { gameArt, loadArt } from '../../lib/stores/metadata';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { playtime, shortDate, weekday } from '../../lib/utils/format';
  import { errorCode, hasMessage, locale, msg } from '../../lib/i18n';

  const periodLabels: Record<StatsPeriod, () => string> = {
    week: () => msg('stats.periodWeek'),
    month: () => msg('stats.periodMonth'),
    year: () => msg('stats.periodYear'),
  };

  let period = $state<StatsPeriod>('week');
  let data = $state<Stats | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let requestId = 0;

  const summary = $derived(data ? summarize(data) : null);
  const buckets = $derived(data ? chartBuckets(data.period, data.days ?? []) : []);
  const best = $derived(bestSeconds(buckets));
  const step = $derived(labelEvery(data?.period ?? period));
  const monthShort = $derived(new Intl.DateTimeFormat($locale, { month: 'short' }));
  const monthLong = $derived(new Intl.DateTimeFormat($locale, { month: 'long', year: 'numeric' }));
  const libraryById = $derived(new Map($libraryGames.map((game) => [game.id, game])));

  function failureText(err: unknown): string {
    const code = errorCode(err);
    if (code && hasMessage(code)) return msg('stats.loadErrorReason', { reason: msg(code) });
    return msg('stats.loadError');
  }

  async function load(target: StatsPeriod) {
    const id = ++requestId;
    loading = true;
    try {
      const next = await getStats(target);
      if (id !== requestId) return;
      data = next;
      failed = false;
    } catch (err) {
      if (id !== requestId) return;
      failed = true;
      toast(failureText(err), 'danger');
    } finally {
      if (id === requestId) loading = false;
    }
  }

  function pick(next: StatsPeriod) {
    if (next === period) return;
    period = next;
    data = null;
    failed = false;
    void load(next);
  }

  function barLabel(bucket: ChartBucket, index: number): string {
    if (bucket.unit === 'month') return monthShort.format(bucket.at);
    if (step === 1) return weekday(bucket.at);
    return index % step === 0 ? `${bucket.at.getDate()}` : '';
  }

  function barTitle(bucket: ChartBucket): string {
    const value = bucket.seconds > 0 ? playtime(bucket.seconds) : msg('stats.chartIdle');
    const when = bucket.unit === 'month' ? monthLong.format(bucket.at) : shortDate(bucket.at);
    return `${when} · ${value}`;
  }

  function titleOf(gameId: string, title: string): string {
    return gameTitle(title, libraryById.get(gameId)?.title, msg('stats.deletedGame'));
  }

  $effect(() => {
    const ids = (summary?.top ?? [])
      .map((item) => libraryById.get(item.gameId)?.canonicalGameId)
      .filter((id): id is string => Boolean(id));
    if (ids.length > 0) void loadArt(ids);
  });

  onMount(() => {
    void load(period);
    return onPlaylogRecorded(() => void load(period));
  });
</script>

<Card surface="panel">
  <PageHeader title={msg('stats.title')} subtitle={msg('stats.subtitle')}>
    {#snippet actions()}
      <div class="periods" role="group" aria-label={msg('stats.periodLabel')}>
        {#each STATS_PERIODS as value (value)}
          <Chip selected={period === value} onclick={() => pick(value)}>{periodLabels[value]()}</Chip>
        {/each}
      </div>
    {/snippet}
  </PageHeader>

  {#if failed && !data}
    <EmptyState title={msg('stats.loadError')}>
      {#snippet actions()}
        <Button onclick={() => load(period)}>{msg('stats.retry')}</Button>
      {/snippet}
    </EmptyState>
  {:else if !data && loading}
    <p class="muted" role="status">{msg('stats.loading')}</p>
  {:else if summary && data}
    {#if failed}
      <p class="muted danger" role="alert">{msg('stats.loadError')}</p>
    {/if}
    {#if summary.empty}
      <EmptyState title={msg('stats.title')} description={msg('stats.empty')}>
        {#snippet icon()}<ChartColumn size="3.2rem" strokeWidth={1.6} />{/snippet}
      </EmptyState>
    {:else}
      <div class="body" class:stale={loading}>
        <div class="tiles">
          <StatTile value={playtime(summary.totalSeconds)} label={msg('stats.totalLabel')} />
          <StatTile value={summary.sessions} label={msg('stats.sessionsLabel')} />
          <div class="longest">
            <span class="longest-value">{summary.longest ? playtime(summary.longest.seconds) : msg('stats.longestNone')}</span>
            <span class="longest-label">{msg('stats.longestLabel')}</span>
            {#if summary.longest}
              <span class="longest-sub">
                {msg('stats.longestSub', {
                  game: titleOf(summary.longest.gameId, summary.longest.title),
                  date: summary.longest.at ? shortDate(summary.longest.at) : '',
                })}
              </span>
            {/if}
          </div>
        </div>

        <section class="section">
          <h2>{data.period === 'year' ? msg('stats.chartTitleYear') : msg('stats.chartTitle')}</h2>
          <div class="chart" style:--count={buckets.length}>
            {#each buckets as bucket, index (bucket.key)}
              <span class="col" title={barTitle(bucket)}>
                <span class="track">
                  <span class="fill" style:height="{barHeight(bucket.seconds, best)}%"></span>
                </span>
                <span class="tick">{barLabel(bucket, index)}</span>
              </span>
            {/each}
          </div>
        </section>

        {#if summary.top.length > 0}
          <section class="section">
            <h2>{msg('stats.topTitle')}</h2>
            <div class="top">
              {#each summary.top as item (item.gameId)}
                {@const game = libraryById.get(item.gameId)}
                <button class="row" type="button" disabled={!game} onclick={() => navigate('game', { id: item.gameId })}>
                  <span class="cover">
                    {#if game}
                      <Artwork src={coverOf({ ...game, status: game.status ?? '' }, $gameArt)} alt="" ratio="3 / 4" radius="var(--radius-sm)" />
                    {/if}
                  </span>
                  <span class="text">
                    <span class="title">{titleOf(item.gameId, item.title)}</span>
                    <span class="sub">
                      {playtime(item.seconds)} · {msg('stats.topSessions', { count: item.sessions })}
                    </span>
                    <span class="share"><span class="share-fill" style:width="{Math.round(item.share * 100)}%"></span></span>
                  </span>
                </button>
              {/each}
            </div>
          </section>
        {/if}
      </div>
    {/if}
  {/if}
</Card>

<style>
  .periods {
    display: flex;
    gap: var(--space-2);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-8);
    max-width: 110rem;
    transition: opacity var(--dur) var(--ease);
  }

  .body.stale {
    opacity: 0.6;
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-6);
  }

  .longest {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.8rem;
    min-width: 0;
  }

  .longest-value {
    font-size: var(--font-title);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
    font-variant-numeric: tabular-nums;
  }

  .longest-label {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .longest-sub {
    max-width: 100%;
    font-size: 1.2rem;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .section h2 {
    margin-bottom: var(--space-4);
    font-size: var(--font-md);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .chart {
    display: grid;
    grid-template-columns: repeat(var(--count, 7), minmax(0, 1fr));
    gap: var(--space-1);
  }

  .col {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.6rem;
    min-width: 0;
  }

  .track {
    display: flex;
    align-items: flex-end;
    width: 100%;
    height: 12rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    overflow: hidden;
  }

  .fill {
    display: block;
    width: 100%;
    border-radius: var(--radius-sm);
    background: var(--accent);
    transition: height var(--dur-panel) var(--ease);
  }

  .tick {
    height: 1.6rem;
    font-size: 1.2rem;
    color: var(--text-3);
    white-space: nowrap;
  }

  .top {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.6rem var(--space-3);
    border-radius: var(--radius-md);
    text-align: left;
    transition: background var(--dur) var(--ease);
  }

  .row:not(:disabled):hover {
    background: var(--hover);
  }

  .cover {
    display: block;
    flex-shrink: 0;
    width: 4rem;
    min-height: 5.3rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    overflow: hidden;
  }

  .text {
    display: flex;
    flex-direction: column;
    flex: 1;
    gap: 0.4rem;
    min-width: 0;
  }

  .title {
    font-size: var(--font-sm);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sub {
    font-size: 1.2rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .share {
    display: block;
    height: 0.4rem;
    border-radius: var(--radius-sm);
    background: var(--surface-3);
    overflow: hidden;
  }

  .share-fill {
    display: block;
    height: 100%;
    background: var(--accent);
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .danger {
    color: var(--danger);
  }

  @media (max-width: 720px) {
    .tiles {
      grid-template-columns: 1fr;
    }
  }
</style>
