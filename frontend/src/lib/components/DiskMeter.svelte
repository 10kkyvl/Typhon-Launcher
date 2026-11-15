<script lang="ts">
  import { locale, t } from '../i18n';
  import { bytesSize } from '../utils/format';
  import { nextIndex } from '../utils/roving';
  import { meter, sizeWords, type MeterPart, type MeterSegment, type MeterSize } from './diskMeter';
  import Reading from './Reading.svelte';
  import Tooltip from './Tooltip.svelte';

  /**
   * How a disk or a folder is filled: one bar of named segments over a total, and a legend in words.
   * The numbers go in as data (`segments`, `total`, `free`); diskMeter.ts holds the truth rules (an
   * unknown total draws nothing, an overflow is clamped and never shown as free, a size not measured
   * is said, not drawn as 0).
   *
   * The bar is one image whose text alternative carries the legend's numbers. With `onpick` its
   * segments are buttons instead (one Tab stop, the arrows move, a hint names each); a segment under
   * one pixel still shows as a hairline. Neighbouring segments are kept apart by a gap of the track,
   * which every tone stands out from (theme-contrast.test.ts), so adjacency never rests on colour.
   *
   * `compact`: the bar and one line (free of total), for a settings row.
   */
  let {
    segments,
    total,
    free,
    label,
    compact = false,
    onpick,
  }: {
    segments: MeterSegment[];
    /** Bytes in all; zero, missing or not a number reads as unknown. */
    total: number | null | undefined;
    /** Free bytes as measured; null when they could not be. Left out, derived from the segments. */
    free?: number | null;
    /** What is measured ("Disk C:", a folder): the start of the text alternative. */
    label: string;
    compact?: boolean;
    /** Makes each drawn segment a button that reports its key. */
    onpick?: (key: string) => void;
  } = $props();

  const model = $derived(meter(segments, total, free));

  const words = $derived({ atLeast: (size: string) => $t('common.meterAtLeast', { size }), unknown: $t('common.meterSizeUnknown') });
  const said = (size: MeterSize) => sizeWords(size, bytesSize, words);
  const partText = (part: MeterPart) => $t('common.meterPart', { name: part.label, size: said(part.size) });

  // The free space and the total in one clause where both are known.
  const tail = $derived.by(() => {
    if (model.total === null) {
      const free = model.free ? [$t('common.meterPart', { name: $t('common.meterFree'), size: said(model.free) })] : [];
      return [...free, $t('common.meterTotalUnknown')];
    }
    if (model.free) return [$t('common.meterFreeOf', { free: said(model.free), total: bytesSize(model.total) })];
    return [$t('common.meterTotal', { total: bytesSize(model.total) })];
  });

  const list = $derived(new Intl.ListFormat($locale, { type: 'unit', style: 'long' }));
  const alt = $derived($t('common.meterAlt', { label, details: list.format([...model.parts.map(partText), ...tail]) }));

  let stop = $state(0);
  let bar = $state<HTMLElement>();
  const tabStop = $derived(Math.min(stop, Math.max(0, model.drawn.length - 1)));

  function onkeydown(event: KeyboardEvent, index: number) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const next = nextIndex(index, model.drawn.length, event.key, { orientation: 'horizontal', loop: false });
    if (next === null) return;
    event.preventDefault();
    stop = next;
    bar?.querySelectorAll<HTMLButtonElement>('.part')[next]?.focus();
  }
</script>

<div class="meter" class:compact>
  {#if onpick && model.drawn.length > 0}
    <div class="bar" role="group" aria-label={alt} bind:this={bar}>
      {#each model.drawn as part, index (part.key)}
        <Tooltip text={partText(part)} describe={false}>
          <button
            type="button"
            class="part"
            data-tone={part.tone}
            style:flex-basis="{part.share * 100}%"
            aria-label={partText(part)}
            tabindex={index === tabStop ? 0 : -1}
            onfocus={() => (stop = index)}
            onclick={() => onpick(part.key)}
            onkeydown={(event) => onkeydown(event, index)}
          ></button>
        </Tooltip>
      {/each}
    </div>
  {:else}
    <div class="bar" role="img" aria-label={alt} data-state={model.total === null ? 'unknown' : undefined}>
      {#each model.drawn as part (part.key)}
        <span class="part" data-tone={part.tone} style:flex-basis="{part.share * 100}%"></span>
      {/each}
    </div>
  {/if}

  {#if compact}
    <p class="line"><Reading text={tail.join(', ')} /></p>
  {:else}
    <ul class="legend">
      {#each model.parts as part (part.key)}
        <li>
          <span class="swatch" data-tone={part.tone} class:hollow={part.size.kind === 'unknown'} aria-hidden="true"></span>
          <span class="name">{part.label}</span>
          <span class="size"><Reading text={said(part.size)} /></span>
        </li>
      {/each}
      {#if model.free && model.total !== null}
        <li>
          <span class="swatch free" aria-hidden="true"></span>
          <span class="name">{$t('common.meterFree')}</span>
          <span class="size"><Reading text={said(model.free)} /></span>
        </li>
      {/if}
      <li class="total"><Reading text={model.total === null ? tail.join(', ') : $t('common.meterTotal', { total: bytesSize(model.total) })} /></li>
    </ul>
  {/if}
</div>

<style>
  .meter {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .meter.compact {
    gap: var(--space-1);
  }

  /* The track. Its colour shows in the gap between segments and in the free space after them. */
  .bar {
    display: flex;
    gap: 0.2rem;
    height: 1.4rem;
    padding: 0.2rem;
    border-radius: var(--radius-xs);
    background: var(--hover-strong);
  }

  .compact .bar {
    height: 1rem;
  }

  .part {
    flex-grow: 0;
    flex-shrink: 1;
    min-width: 0.2rem;
    height: 100%;
    padding: 0;
    border: 0;
    border-radius: var(--cut);
    transition: flex-basis var(--dur-slow) var(--ease);
  }

  button.part {
    cursor: pointer;
  }

  /* Hovered, a segment takes a share of the text colour: further from the track in every theme. */
  button.part:hover {
    background-image: linear-gradient(color-mix(in srgb, var(--text) 20%, transparent), color-mix(in srgb, var(--text) 20%, transparent));
  }

  button.part:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 2px;
  }

  [data-tone='game'] {
    background: var(--accent-bright);
  }

  [data-tone='game-2'] {
    background: var(--accent-text);
  }

  [data-tone='game-3'] {
    background: color-mix(in srgb, var(--accent-bright) 50%, var(--text-3));
  }

  [data-tone='data'] {
    background: var(--success);
  }

  [data-tone='reclaim'] {
    background: var(--warning);
  }

  [data-tone='other'] {
    background: var(--text-3);
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-6);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--font-sm);
  }

  .legend li {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .swatch {
    flex: none;
    width: 1rem;
    height: 1rem;
    border-radius: var(--cut);
  }

  .swatch.free {
    background: var(--hover-strong);
    box-shadow: inset 0 0 0 1px var(--border-strong);
  }

  .swatch.hollow {
    background: none;
    box-shadow: inset 0 0 0 1px var(--text-3);
  }

  .name {
    color: var(--text-2);
  }

  .size {
    color: var(--text);
  }

  .legend .total {
    color: var(--text-3);
  }

  .line {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
