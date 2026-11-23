<script lang="ts">
  import { untrack } from 'svelte';
  import { t } from '../i18n';
  import { focusQuietly } from '../layers/focus';
  import { bytesSize } from '../utils/format';
  import { nextIndex } from '../utils/roving';
  import { METER_WIDTH, meter, sizeWords, type MeterPart, type MeterSegment, type MeterSize } from './diskMeter';
  import Reading from './Reading.svelte';
  import TextLink from './TextLink.svelte';
  import Tooltip from './Tooltip.svelte';

  /**
   * How a disk or a folder is filled: one bar of named segments over a total, and a legend in words.
   * The numbers go in as data (`segments`, `total`, `free`); diskMeter.ts holds the truth rules (an
   * unknown total draws nothing, an overflow is drawn in proportion and says no free space, a size not measured
   * is said, not drawn as 0).
   *
   * The bar is one image whose text alternative carries the legend's numbers. With `onpick` its
   * segments are buttons instead (one Tab stop, the arrows move, a hint names each); a segment under
   * one pixel still shows as a hairline. Neighbouring segments are kept apart by a gap of the track,
   * which every tone stands out from (theme-contrast.test.ts), so adjacency never rests on colour.
   * Segments too small to be drawn at their size at the narrowest width the form is laid out for
   * (diskMeter.ts::METER_WIDTH) are folded into one "N more" part, said with their count and size
   * (`foldLabel` words it, "12 more games"; the catalogue's "N more parts" by default); the fold never
   * takes every segment. On the bar the fold is a button only with `onpickfolded`, which gets the keys
   * folded into it. In the full form's legend, with `onpick`, the fold's entry is a disclosure that
   * lists the folded segments by name and size, each a pick; without `onpick` it is text. A segment the
   * bar does not draw (zero or unknown size) is picked from its name in the legend. The track clips as
   * a last guard for a place narrower than that.
   *
   * `compact`: the bar and one line (free of total), for a settings row. It has no legend, so it offers
   * only its drawn parts and, with `onpickfolded`, the fold: a folded or undrawn segment cannot be
   * picked from a compact meter.
   */
  let {
    segments,
    total,
    free,
    label,
    compact = false,
    foldLabel,
    onpick,
    onpickfolded,
  }: {
    segments: MeterSegment[];
    /** Bytes in all; zero, missing or not a number reads as unknown. */
    total: number | null | undefined;
    /** Free bytes as measured; null when they could not be. Left out, derived from the segments. */
    free?: number | null;
    /** What is measured ("Disk C:", a folder): the start of the text alternative. */
    label: string;
    compact?: boolean;
    /** The fold's name for its count of segments, from the caller's catalogue ("12 more games"). */
    foldLabel?: (count: number) => string;
    /** Makes each drawn segment a button that reports its key. */
    onpick?: (key: string) => void;
    /** With `onpick`, makes the "N more" part a button too, reporting the keys folded into it. */
    onpickfolded?: (keys: string[]) => void;
  } = $props();

  const model = $derived(meter(segments, total, free, compact ? METER_WIDTH.compact : METER_WIDTH.full));
  const uid = $props.id();
  // The fold's entry in the legend, opened to list the segments folded into it.
  let open = $state(false);

  const words = $derived({
    atLeast: (size: string) => $t('common.meterAtLeast', { size }),
    atMost: (size: string) => $t('common.meterAtMost', { size }),
    unknown: $t('common.meterSizeUnknown'),
  });
  const said = (size: MeterSize) => sizeWords(size, bytesSize, words);
  function nameOf(part: MeterPart): string {
    if (part.kind === 'other') return $t('common.meterOther');
    if (part.kind === 'more') {
      const count = part.folded?.length ?? 0;
      return foldLabel ? foldLabel(count) : $t('common.meterMore', { count });
    }
    return part.label;
  }
  const partText = (part: MeterPart) => $t('common.meterPart', { name: nameOf(part), size: said(part.size) });

  // The free space and the total in one clause where both are known; an overflow says so instead.
  const tail = $derived.by(() => {
    if (model.overflow && model.total !== null) return [$t('common.meterOverflow'), $t('common.meterTotal', { total: bytesSize(model.total) })];
    if (model.total === null) {
      const free = model.free ? [$t('common.meterPart', { name: $t('common.meterFree'), size: said(model.free) })] : [];
      return [...free, $t('common.meterTotalUnknown')];
    }
    if (model.free) return [$t('common.meterFreeOf', { free: said(model.free), total: bytesSize(model.total) })];
    return [$t('common.meterTotal', { total: bytesSize(model.total) })];
  });

  // A list format joins Russian items with bare spaces, and a comma is the Russian decimal mark: the
  // parts are kept apart by the catalogue's own separator.
  const joined = (items: string[]) => items.join($t('common.meterSeparator'));
  const alt = $derived($t('common.meterAlt', { label, details: joined([...model.parts.map(partText), ...tail]) }));

  // The drawn parts a press reports: the buttons, in the order the arrows move.
  const pickable = $derived(model.drawn.filter((part) => part.kind === 'segment' || (part.kind === 'more' && onpickfolded)));

  function pick(part: MeterPart) {
    if (part.kind === 'more') onpickfolded?.((part.folded ?? []).map((member) => member.key));
    else onpick?.(part.key);
  }

  // The one Tab stop, kept by the part's id; when that part leaves, the one now in its place.
  let stop = $state<{ id: string; index: number }>({ id: '', index: 0 });
  let bar = $state<HTMLElement>();
  const tabStop = $derived.by(() => {
    const at = pickable.findIndex((part) => part.id === stop.id);
    return at >= 0 ? at : Math.min(stop.index, Math.max(0, pickable.length - 1));
  });

  const buttons = () => [...(bar?.querySelectorAll<HTMLButtonElement>('button.part') ?? [])];

  function onkeydown(event: KeyboardEvent, index: number) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const next = nextIndex(index, pickable.length, event.key, { orientation: 'horizontal', loop: false });
    if (next === null) return;
    event.preventDefault();
    stop = { id: pickable[next].id, index: next };
    buttons()[next]?.focus();
  }

  // A focused part that leaves hands focus to the part now in its place, else to its neighbour, else to
  // the group. Read before the bar changes: which part held focus, and where. Not state: nothing is
  // drawn from it.
  let held: number | null = null;
  $effect.pre(() => {
    void pickable;
    const active = document.activeElement;
    held = active instanceof HTMLButtonElement ? untrack(buttons).indexOf(active) : -1;
    if (held < 0) held = null;
  });
  $effect(() => {
    void pickable;
    const was = held;
    held = null;
    if (was === null || !bar) return;
    const active = document.activeElement;
    if (active && active !== document.body && active.isConnected) return;
    const now = buttons();
    const at = Math.min(was, now.length - 1);
    if (at >= 0) stop = { id: pickable[at].id, index: at };
    focusQuietly(now[at] ?? bar);
  });
</script>

<div class="meter" class:compact>
  {#if onpick}
    <div class="bar" role="group" aria-label={alt} tabindex="-1" data-state={model.total === null ? 'unknown' : undefined} bind:this={bar}>
      {#each model.drawn as part (part.id)}
        {@const index = pickable.indexOf(part)}
        {#if index >= 0}
          <Tooltip text={partText(part)} describe={false}>
            <button
              type="button"
              class="part"
              data-tone={part.tone}
              style:flex-basis="{part.share * 100}%"
              aria-label={partText(part)}
              tabindex={index === tabStop ? 0 : -1}
              onfocus={() => (stop = { id: part.id, index })}
              onclick={() => pick(part)}
              onkeydown={(event) => onkeydown(event, index)}
            ></button>
          </Tooltip>
        {:else}
          <span class="part" data-tone={part.tone} style:flex-basis="{part.share * 100}%"></span>
        {/if}
      {/each}
    </div>
  {:else}
    <div class="bar" role="img" aria-label={alt} data-state={model.total === null ? 'unknown' : undefined}>
      {#each model.drawn as part (part.id)}
        <span class="part" data-tone={part.tone} style:flex-basis="{part.share * 100}%"></span>
      {/each}
    </div>
  {/if}

  {#if compact}
    <p class="line"><Reading text={joined(tail)} /></p>
  {:else}
    <ul class="legend">
      {#each model.parts as part (part.id)}
        <li>
          <span class="swatch" data-tone={part.tone} class:hollow={part.size.kind === 'unknown'} aria-hidden="true"></span>
          {#if onpick && part.kind === 'segment' && part.share === 0}
            <span class="name"><TextLink tone="muted" onclick={() => onpick(part.key)}>{nameOf(part)}</TextLink></span>
          {:else if onpick && part.kind === 'more'}
            <span class="name">
              <TextLink tone="muted" id="{uid}-fold" aria-expanded={open} aria-controls="{uid}-members" onclick={() => (open = !open)}>{nameOf(part)}</TextLink>
            </span>
          {:else}
            <span class="name">{nameOf(part)}</span>
          {/if}
          <span class="size"><Reading text={said(part.size)} /></span>
          {#if onpick && part.kind === 'more' && open}
            <ul class="members" id="{uid}-members" aria-labelledby="{uid}-fold">
              {#each part.folded ?? [] as member (member.id)}
                <li>
                  <span class="name"><TextLink tone="muted" onclick={() => onpick(member.key)}>{member.label}</TextLink></span>
                  <span class="size"><Reading text={said(member.size)} /></span>
                </li>
              {/each}
            </ul>
          {/if}
        </li>
      {/each}
      {#if model.free && model.total !== null}
        <li>
          <span class="swatch free" aria-hidden="true"></span>
          <span class="name">{$t('common.meterFree')}</span>
          <span class="size"><Reading text={said(model.free)} /></span>
        </li>
      {/if}
      <li class="total"><Reading text={model.total === null || model.overflow ? joined(tail) : $t('common.meterTotal', { total: bytesSize(model.total) })} /></li>
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
  /* The gap, the padding and the parts' min-width are diskMeter.ts::METER_GAP, METER_PAD and
     METER_HAIRLINE: the fold rule counts with them. */
  .bar {
    display: flex;
    gap: 0.2rem;
    height: 1.4rem;
    padding: 0.2rem;
    overflow: hidden;
    border-radius: var(--radius-xs);
    background: var(--hover-strong);
  }

  .compact .bar {
    height: 1rem;
  }

  /* An unknown total is not an empty drive, which is the filled track: nothing is filled, the place of
     the bar is a broken outline, as the hollow swatch of a size not measured. */
  .bar[data-state='unknown'] {
    background: none;
    outline: 1px dashed var(--text-3);
    outline-offset: -1px;
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

  /* The group takes focus only when the focused part left and none took its place. */
  .bar:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 2px;
  }

  /* Right on the part: the track clips, and its padding holds the ring. */
  button.part:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 0;
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

  [data-tone='more'] {
    background: color-mix(in srgb, var(--accent-text) 45%, var(--text-3));
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

  /* The fold's segments, opened under its entry on a line of their own, set in by the swatch. */
  .legend li:has(> .members) {
    flex-wrap: wrap;
    flex-basis: 100%;
  }

  .members {
    display: flex;
    flex-basis: 100%;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-6);
    margin: 0;
    padding: 0 0 0 calc(1rem + var(--space-2));
    list-style: none;
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
