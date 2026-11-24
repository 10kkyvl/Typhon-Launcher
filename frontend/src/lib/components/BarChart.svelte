<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import { locale, t } from '../i18n';
  import { focusQuietly } from '../layers/focus';
  import { motionOn } from '../motion/motion';
  import { nextIndex } from '../utils/roving';
  import { barShare, chartMax, chartValue, isEmpty, ticks, type ChartBucket, type ChartScale } from './barChart';
  import Reading from './Reading.svelte';
  import Tooltip from './Tooltip.svelte';

  /**
   * Vertical bars over buckets (days, weeks, months). The values are the caller's unit and `format`
   * writes them, so hours, hours a day and sessions share the chart. A zero bucket keeps its slot;
   * with nothing above zero the caller's `empty` is shown instead. The words say the real value even
   * when the bar is clamped to the scale; a NaN, negative or infinite value is said as `format(0)`,
   * and the bar, its label, the table and the radio all take it as no value (barChart.ts::chartValue).
   *
   * Plain: the drawing is hidden from assistive tech and a table with the same numbers stands in for
   * it; each bar's value is a label over it.
   *
   * With `selectable` the bars are one choice of a radio group (one Tab stop, the arrows, Home and
   * End move the choice and the focus, stopping at the ends); `bind:selected` holds the chosen key,
   * which may be none (null or a key no bucket has) without the group turning into a table. The Tab
   * stop is kept by key; a focused bar whose bucket leaves hands focus to the bar in its place, and
   * to the chart's own box when the bars leave for the empty state, and from that box to the bar with
   * the Tab stop when they come back. Each radio is named by its bucket
   * and value and shows them as a hint, so the radios are the list of numbers and no table repeats
   * them.
   * `values="selected"` labels the chosen bar only, for a chart of many bars.
   *
   * `scale` gives the axis (barChart.ts::secondsScale for a time): its ticks are drawn and the bars
   * are measured against its maximum; without it the tallest bar fills the height. The ticks are
   * written by `tickFormat`, else by `format` with the zero tick a bare 0 (a format may word zero as
   * an absence, "no play", which an axis must not say); a `tickFormat` writes the zero tick too. Bars
   * grow from zero only while motion is on. `--bar-chart-height` (or `height`) sets the height of the
   * plot.
   */
  let {
    buckets,
    label,
    format,
    selectable = false,
    selected = $bindable(),
    scale,
    values = 'all',
    columns,
    height,
    tick,
    tickFormat,
    empty,
    testid,
  }: {
    buckets: ChartBucket[];
    /** What the chart shows: the table's caption, or the radio group's name. */
    label: string;
    /** A value in words ("2 h 10 min", "3 sessions"), for the labels, the axis and the table. */
    format: (value: number) => string;
    /** The bars are a radio group, chosen or not yet. */
    selectable?: boolean;
    /** With `selectable`: the key of the chosen bucket, null for none. */
    selected?: string | null;
    scale?: ChartScale;
    values?: 'all' | 'selected';
    /** The heads of the stand-in table's two columns; "Period" and "Value" by default. */
    columns?: [string, string];
    height?: string;
    /** Draws what stands under a bar instead of the bucket's `tick` or `label`. */
    tick?: Snippet<[ChartBucket]>;
    /** Writes the axis ticks, zero included; by default `format`, with the zero tick a bare 0. */
    tickFormat?: (value: number) => string;
    empty?: Snippet;
    testid?: string;
  } = $props();

  const max = $derived(chartMax(buckets, scale));
  const marks = $derived(scale ? ticks(scale) : []);
  const chosen = $derived(buckets.findIndex((bucket) => bucket.key === selected));
  // The key of the bar focus was last on (a press, an arrow, a hand-off).
  let focusedKey = $state<string | null>(null);
  // The one Tab stop, by key: the chosen bar; while the choice is no bucket, the bar focus was last on,
  // else the current one, else the last.
  const stop = $derived.by(() => {
    if (chosen >= 0) return chosen;
    const held = buckets.findIndex((bucket) => bucket.key === focusedKey);
    if (held >= 0) return held;
    const current = buckets.findIndex((bucket) => bucket.current);
    return current >= 0 ? current : buckets.length - 1;
  });

  const said = (bucket: ChartBucket) => format(chartValue(bucket));
  const named = (bucket: ChartBucket) => $t('common.chartBucket', { label: bucket.label, value: said(bucket) });
  const labelled = (bucket: ChartBucket) => chartValue(bucket) > 0 && (values === 'all' || bucket.key === selected);
  const zero = $derived(new Intl.NumberFormat($locale).format(0));
  const tickText = (mark: number) => (tickFormat ? tickFormat(mark) : mark === 0 ? zero : format(mark));

  let group = $state<HTMLElement>();
  let root = $state<HTMLElement>();
  const blank = $derived(isEmpty(buckets) && !!empty);
  const radios = () => [...(group?.querySelectorAll<HTMLButtonElement>('[role="radio"]') ?? [])];

  function onkeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const all = radios();
    const next = nextIndex(all.indexOf(document.activeElement as HTMLButtonElement), buckets.length, event.key, { orientation: 'horizontal', loop: false });
    if (next === null) return;
    event.preventDefault();
    selected = buckets[next].key;
    all[next]?.focus();
  }

  // A focused bar that leaves (its bucket gone from the data) hands focus to the bar now in its place,
  // else to its neighbour, else to the group, else (the group gone with the bars, the empty state in
  // their place) to the chart itself. Read before the bars change: which one held focus. Not state:
  // nothing is drawn from it.
  let held: number | null = null;
  $effect.pre(() => {
    void buckets;
    const active = document.activeElement;
    const at = active instanceof HTMLButtonElement ? untrack(radios).indexOf(active) : -1;
    held = at >= 0 ? at : null;
  });
  $effect(() => {
    void buckets;
    const was = held;
    held = null;
    if (was === null || !root) return;
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected) return;
    const now = radios();
    const at = Math.min(was, now.length - 1);
    if (at >= 0) focusedKey = untrack(() => buckets[at].key);
    focusQuietly(now[at] ?? (group?.isConnected ? group : root));
  });

  // The chart's box loses its role and name when the bars come back from the empty state: focus left
  // on it goes on to the bar that holds the Tab stop. Not state: nothing is drawn from it.
  let wasBlank = false;
  $effect(() => {
    const before = wasBlank;
    wasBlank = blank;
    untrack(() => {
      if (before && !blank && document.activeElement === root) {
        const target = radios().find((radio) => radio.tabIndex === 0);
        if (target) focusQuietly(target);
      }
    });
  });
</script>

{#snippet column(bucket: ChartBucket)}
  {@const share = barShare(chartValue(bucket), max)}
  <span class="well" aria-hidden="true">
    {#if share > 0}<span class="bar" style:height="{share * 100}%"></span>{/if}
    {#if labelled(bucket)}<span class="value" style:bottom="{share * 100}%"><Reading text={said(bucket)} /></span>{/if}
  </span>
  <span class="tick" aria-hidden="true">{#if tick}{@render tick(bucket)}{:else}{bucket.tick ?? bucket.label}{/if}</span>
{/snippet}

<!-- The chart's own box takes focus only from a bar that left with its group. Named while it holds the
     empty state; around the bars the table or the radio group carries the name. -->
<div class="chart-box" tabindex="-1" role={blank ? 'group' : undefined} aria-label={blank ? label : undefined} data-testid={testid} bind:this={root}>
  {#if blank}
    {@render empty?.()}
  {:else}
    <div class="bar-chart" class:grow={$motionOn} class:scaled={!!scale} style:--bar-chart-height={height} style:--count={buckets.length}>
      {#if scale}
        <div class="axis" aria-hidden="true">
          {#each marks as mark (mark)}
            <span class="mark" style:bottom="{barShare(mark, max) * 100}%"><Reading text={tickText(mark)} /></span>
          {/each}
        </div>
      {/if}
      <div class="plot">
        <div class="lines" aria-hidden="true">
          {#each marks.length ? marks : [0] as mark (mark)}
            <span class="line" class:base={mark === 0} style:bottom="{barShare(mark, max) * 100}%"></span>
          {/each}
        </div>
        {#if selectable}
          <div class="cols" class:chosen={chosen >= 0} role="radiogroup" aria-label={label} tabindex="-1" bind:this={group}>
            {#each buckets as bucket, index (bucket.key)}
              <Tooltip text={named(bucket)} describe={false}>
                <button
                  type="button"
                  class="col"
                  class:current={bucket.current}
                  role="radio"
                  aria-checked={bucket.key === selected}
                  aria-label={named(bucket)}
                  tabindex={index === stop ? 0 : -1}
                  onfocus={() => (focusedKey = bucket.key)}
                  onclick={() => (selected = bucket.key)}
                  {onkeydown}
                >
                  {@render column(bucket)}
                </button>
              </Tooltip>
            {/each}
          </div>
        {:else}
          <div class="cols" aria-hidden="true">
            {#each buckets as bucket (bucket.key)}
              <span class="col" class:current={bucket.current}>{@render column(bucket)}</span>
            {/each}
          </div>
        {/if}
      </div>
      {#if !selectable}
        <table class="sr-only">
          <caption>{label}</caption>
          <thead>
            <tr><th scope="col">{columns?.[0] ?? $t('common.chartPeriod')}</th><th scope="col">{columns?.[1] ?? $t('common.chartValue')}</th></tr>
          </thead>
          <tbody>
            {#each buckets as bucket (bucket.key)}
              <tr><th scope="row">{bucket.label}</th><td>{said(bucket)}</td></tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </div>
  {/if}
</div>

<style>
  .chart-box {
    min-width: 0;
  }

  .chart-box:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 2px;
  }

  .bar-chart {
    --plot: var(--bar-chart-height, 18rem);
    /* Over the plot, for the label of a bar that fills the height. */
    --room: var(--space-5);
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    column-gap: var(--space-3);
    min-width: 0;
  }

  .bar-chart.scaled {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .axis {
    position: relative;
    height: var(--plot);
    min-width: 3.2rem;
    margin-top: var(--room);
  }

  .mark {
    position: absolute;
    right: 0;
    transform: translateY(50%);
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  .plot {
    position: relative;
    min-width: 0;
  }

  .lines {
    position: absolute;
    inset: var(--room) 0 auto;
    height: var(--plot);
    pointer-events: none;
  }

  .line {
    position: absolute;
    right: 0;
    left: 0;
    border-top: 1px solid var(--border);
  }

  .line.base {
    border-top-color: var(--border-strong);
  }

  .cols {
    position: relative;
    display: grid;
    grid-template-columns: repeat(var(--count), minmax(0, 1fr));
  }

  .col {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-2);
    min-width: 0;
    padding: 0;
    border: 0;
    border-radius: var(--radius-sm);
    background: none;
    font: inherit;
    color: var(--text-2);
  }

  .well {
    position: relative;
    display: block;
    height: var(--plot);
    margin-top: var(--room);
  }

  /* A slim bar in the middle of its slot; a value far under a pixel still shows as a hairline. */
  .bar {
    position: absolute;
    right: 30%;
    bottom: 0;
    left: 30%;
    min-height: 0.2rem;
    border-radius: var(--cut) var(--cut) 0 0;
    background: var(--accent-bright);
    transform-origin: 50% 100%;
  }

  .current .bar {
    background: var(--accent-text);
  }

  .grow .bar {
    transition: transform var(--dur-slow) var(--ease);
  }

  @starting-style {
    .grow .bar {
      transform: scaleY(0);
    }
  }

  .value {
    position: absolute;
    right: 0;
    left: 0;
    margin-bottom: var(--space-1);
    font-size: var(--font-xs);
    text-align: center;
    white-space: nowrap;
    color: var(--text-2);
  }

  .tick {
    overflow: hidden;
    font-size: var(--font-xs);
    line-height: 1.3;
    text-align: center;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .current .tick {
    font-weight: 600;
    color: var(--text);
  }

  /* Bars that can be chosen read as a plain chart until one is; then the rest go quiet around it. */
  button.col {
    padding-bottom: var(--space-1);
    cursor: pointer;
  }

  button.col:hover {
    background: var(--hover);
    color: var(--text);
  }

  .chosen button.col .bar {
    background: var(--text-3);
  }

  .chosen button.col:hover .bar {
    background: var(--text-2);
  }

  button.col[aria-checked='true'] {
    background: var(--hover-strong);
    color: var(--text);
  }

  .chosen button.col[aria-checked='true'] .bar {
    background: var(--accent-bright);
  }

  button.col:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 1px;
  }

  /* The group takes focus only when the focused bar left and none took its place. */
  .cols:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 2px;
  }
</style>
