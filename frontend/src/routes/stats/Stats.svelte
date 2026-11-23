<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import { onMount } from 'svelte';
  import { ChartColumn } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import BarChart from '../../lib/components/BarChart.svelte';
  import { secondsScale, type ChartBucket as Bar } from '../../lib/components/barChart';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import SectionHeader from '../../lib/components/SectionHeader.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatTile from '../../lib/components/StatTile.svelte';
  import { coverOf } from '../../lib/profile/view';
  import { getStats, onPlaylogRecorded, STATS_PERIODS, type Stats, type StatsPeriod } from '../../lib/services/playlog';
  import { bestSeconds, chartBuckets, currentKey, labelEvery, type ChartBucket } from '../../lib/stats/buckets';
  import { gameTitle, summarize } from '../../lib/stats/summary';
  import { libraryGames } from '../../lib/stores/library';
  import { gameArt, loadArt } from '../../lib/stores/metadata';
  import { playtime, shortDate, weekday } from '../../lib/utils/format';
  import { errorCode, hasMessage, locale, msg } from '../../lib/i18n';

  const periodLabels: Record<StatsPeriod, () => string> = {
    week: () => msg('stats.periodWeek'),
    month: () => msg('stats.periodMonth'),
    year: () => msg('stats.periodYear'),
  };

  const uid = $props.id();

  const periodOptions = STATS_PERIODS.map((value) => ({ id: value, label: periodLabels[value]() }));

  let period = $state<StatsPeriod>('week');
  let data = $state<Stats | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let failure = $state('');
  let requestId = 0;
  // The chart's chosen bar, by its key: none at first and after a change of period.
  let chosenBar = $state<string | null>(null);

  const summary = $derived(data ? summarize(data) : null);
  const buckets = $derived(data ? chartBuckets(data.period, data.days ?? []) : []);
  const best = $derived(bestSeconds(buckets));
  const step = $derived(labelEvery(data?.period ?? period));
  const monthShort = $derived(new Intl.DateTimeFormat($locale, { month: 'short' }));
  const monthLong = $derived(new Intl.DateTimeFormat($locale, { month: 'long', year: 'numeric' }));
  const current = $derived(data ? currentKey(data.period, data.to) : '');
  const bars = $derived<Bar[]>(
    buckets.map((bucket, index) => ({
      key: bucket.key,
      label: bucket.unit === 'month' ? monthLong.format(bucket.at) : shortDate(bucket.at),
      tick: barTick(bucket, index),
      value: bucket.seconds,
      current: bucket.key === current,
    })),
  );
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
      failure = failureText(err);
    } finally {
      if (id === requestId) loading = false;
    }
  }

  let retrying = $state(false);
  let topList = $state<HTMLElement>();

  // The notice's retry: it worked when the statistics came. On success the focus goes to the first
  // most-played game, else to the page's heading.
  async function retry(): Promise<boolean> {
    await load(period);
    return !failed;
  }

  function pick(next: StatsPeriod) {
    if (next === period) return;
    period = next;
    chosenBar = null;
    data = null;
    failed = false;
    void load(next);
  }

  function barTick(bucket: ChartBucket, index: number): string {
    if (bucket.unit === 'month') return monthShort.format(bucket.at);
    if (step === 1) return weekday(bucket.at);
    return index % step === 0 ? `${bucket.at.getDate()}` : '';
  }

  // A day or a month without play is said as such in the labels and the table; the axis keeps its bare 0.
  const playedWords = (seconds: number) => (seconds > 0 ? playtime(seconds) : msg('stats.chartIdle'));

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

<Page>
  <PageHeader title={msg('stats.title')} subtitle={msg('stats.subtitle')}>
    {#snippet actions()}
      <SegmentedControl
        options={periodOptions}
        label={msg('stats.periodLabel')}
        variant="pill"
        bind:value={() => period, (next) => pick(next as StatsPeriod)}
      />
    {/snippet}
  </PageHeader>

  {#if (failed || retrying) && !data}
    <Notice tone="danger" bind:retrying retry={{ label: msg('stats.retry'), run: retry, within: () => topList }}>
      {failure || msg('stats.loadError')}
    </Notice>
  {:else if !data && loading}
    <Spinner label={msg('stats.loading')} />
  {:else if summary && data}
    {#if failed}
      <div class="load-error"><Notice tone="danger">{failure || msg('stats.loadError')}</Notice></div>
    {/if}
    {#if summary.empty}
      <EmptyState level={2} title={msg('stats.title')} description={msg('stats.empty')}>
        {#snippet icon()}<ChartColumn size="3.2rem" strokeWidth={1.6} />{/snippet}
      </EmptyState>
    {:else}
      <div class="body" class:stale={loading}>
        <div class="tiles">
          <StatTile value={playtime(summary.totalSeconds)} label={msg('stats.totalLabel')} />
          <StatTile value={summary.sessions} label={msg('stats.sessionsLabel')} />
          <StatTile
            label={msg('stats.longestLabel')}
            value={summary.longest ? playtime(summary.longest.seconds) : msg('stats.longestNone')}
            sub={summary.longest
              ? msg('stats.longestSub', {
                  game: titleOf(summary.longest.gameId, summary.longest.title),
                  date: summary.longest.at ? shortDate(summary.longest.at) : '',
                })
              : undefined}
          />
        </div>

        <section aria-labelledby="{uid}-chart">
          <SectionHeader id="{uid}-chart" title={data.period === 'year' ? msg('stats.chartTitleYear') : msg('stats.chartTitle')} />
          <!-- Each bar is a choice named by its date and value, shown in a hint on hover and focus. Thirty
               days leave no room for a value over each bar: a month labels the chosen one only. -->
          <BarChart
            label={data.period === 'year' ? msg('stats.chartTitleYear') : msg('stats.chartTitle')}
            buckets={bars}
            format={playedWords}
            scale={secondsScale(best)}
            selectable
            bind:selected={chosenBar}
            values={data.period === 'month' ? 'selected' : 'all'}
          />
        </section>

        {#if summary.top.length > 0}
          <section aria-labelledby="{uid}-top">
            <SectionHeader id="{uid}-top" title={msg('stats.topTitle')} count={summary.top.length} />
            <!-- A game gone from the library has no page: its title is plain text and its cover blank. -->
            <ol class="top" bind:this={topList}>
              {#each summary.top as item (item.gameId)}
                {@const game = libraryById.get(item.gameId)}
                {@const title = titleOf(item.gameId, item.title)}
                <li class="row">
                  <span class="cover">
                    {#if game}
                      <GameLink id={game.id} name={title} repeat>
                        <Artwork src={coverOf({ ...game, status: game.status ?? '' }, $gameArt)} alt="" label={game.title} ratio="3 / 4" radius="var(--radius-sm)" />
                      </GameLink>
                    {/if}
                  </span>
                  <span class="text">
                    <span class="title"><GameLink id={game ? game.id : null} name={title} truncate /></span>
                    <span class="sub">{msg('stats.topLine', { time: playtime(item.seconds), count: item.sessions })}</span>
                    <ProgressBar value={Math.round(item.share * 100)} label={title} valueText={msg('stats.topShare', { share: Math.round(item.share * 100) })} />
                  </span>
                </li>
              {/each}
            </ol>
          </section>
        {/if}
      </div>
    {/if}
  {/if}
</Page>

<style>
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

  .top {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: 0;
    list-style: none;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-1) var(--space-3);
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
    gap: var(--space-1);
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
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .load-error {
    margin-bottom: var(--space-4);
  }

  @media (max-width: 720px) {
    .tiles {
      grid-template-columns: 1fr;
    }
  }
</style>
