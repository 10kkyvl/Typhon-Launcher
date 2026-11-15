<script lang="ts">
  import type { Snippet } from 'svelte';
  import { locale, t } from '../i18n';
  import { motionOn } from '../motion/motion';
  import { nextIndex } from '../utils/roving';
  import { barShare, chartMax, chartValue, isEmpty, ticks, type ChartBucket, type ChartScale } from './barChart';
  import Reading from './Reading.svelte';
  import Tooltip from './Tooltip.svelte';

  /**
   * Vertical bars over buckets (days, weeks, months). The values are the caller's unit and `format`
   * writes them, so hours, hours a day and sessions share the chart. A zero bucket keeps its slot;
   * with nothing above zero the caller's `empty` is shown instead. A value that is not a number, is
   * below zero or infinite is no value (barChart.ts::chartValue) for the bar, its label, the table and
   * the radio alike; the words always say the value given, a bar over the scale is drawn to the top.
   *
   * Plain: the drawing is hidden from assistive tech and a table with the same numbers stands in for
   * it; each bar's value is a label over it.
   *
   * With `selectable` the bars are one choice of a radio group (one Tab stop, the arrows, Home and
   * End move the choice and the focus, stopping at the ends); `bind:selected` holds the chosen key,
   * which may be none (null or a key no bucket has) without the group turning into a table. Each radio is named by its bucket and
   * value and shows them as a hint, so the radios are the list of numbers and no table repeats them.
   * `values="selected"` labels the chosen bar only, for a chart of many bars.
   *
   * `scale` gives the axis (barChart.ts::secondsScale for a time): its ticks are drawn and the bars
   * are measured against its maximum; without it the tallest bar fills the height. The ticks are
   * written by `tickFormat`, else by `format` with the zero tick a bare 0 (a format may word zero as
   * an absence, "no play", which an axis must not say); a `tickFormat` writes the zero tick too. Bars grow from
   * zero only while motion is on. `--bar-chart-height` (or `height`) sets the height of the plot.
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
  // The one Tab stop: the chosen bar; while the choice is no bucket, the current one, else the last.
  const stop = $derived.by(() => {
    if (chosen >= 0) return chosen;
    const current = buckets.findIndex((bucket) => bucket.current);
    return current >= 0 ? current : buckets.length - 1;
  });

  const said = (bucket: ChartBucket) => format(chartValue(bucket));
  const named = (bucket: ChartBucket) => $t('common.chartBucket', { label: bucket.label, value: said(bucket) });
  const labelled = (bucket: ChartBucket) => chartValue(bucket) > 0 && (values === 'all' || bucket.key === selected);
  const zero = $derived(new Intl.NumberFormat($locale).format(0));
  const tickText = (mark: number) => (tickFormat ? tickFormat(mark) : mark === 0 ? zero : format(mark));

  let group = $state<HTMLElement>();

  function onkeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const radios = [...(group?.querySelectorAll<HTMLButtonElement>('[role="radio"]') ?? [])];
    const next = nextIndex(radios.indexOf(document.activeElement as HTMLButtonElement), buckets.length, event.key, { orientation: 'horizontal', loop: false });
    if (next === null) return;
    event.preventDefault();
    selected = buckets[next].key;
    radios[next]?.focus();
  }
</script>

{#snippet column(bucket: ChartBucket)}
  {@const share = barShare(chartValue(bucket), max)}
  <span class="well" aria-hidden="true">
    {#if share > 0}<span class="bar" style:height="{share * 100}%"></span>{/if}
    {#if labelled(bucket)}<span class="value" style:bottom="{share * 100}%"><Reading text={said(bucket)} /></span>{/if}
  </span>
  <span class="tick" aria-hidden="true">{#if tick}{@render tick(bucket)}{:else}{bucket.tick ?? bucket.label}{/if}</span>
{/snippet}

{#if isEmpty(buckets) && empty}
  {@render empty()}
{:else}
  <div class="bar-chart" class:grow={$motionOn} class:scaled={!!scale} style:--bar-chart-height={height} style:--count={buckets.length} data-testid={testid}>
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
        <div class="cols" role="radiogroup" aria-label={label} bind:this={group}>
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

<style>
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

  /* A bar that can be chosen: quiet until it is, as the week strip of the prototype. */
  button.col {
    padding-bottom: var(--space-1);
    cursor: pointer;
  }

  button.col .bar {
    background: var(--text-3);
  }

  button.col:hover {
    background: var(--hover);
    color: var(--text);
  }

  button.col:hover .bar {
    background: var(--text-2);
  }

  button.col[aria-checked='true'] {
    background: var(--hover-strong);
    color: var(--text);
  }

  button.col[aria-checked='true'] .bar {
    background: var(--accent-bright);
  }

  button.col:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 1px;
  }
</style>
