<script lang="ts">
  import { t } from '../i18n';
  import type { SpeedSeries } from '../downloads/speedHistory';
  import { speedBytes } from '../utils/format';
  import Reading from './Reading.svelte';

  /**
   * A download's speed over the last seconds, drawn like a barograph trace: newest on the right, a
   * hairline grid of three lines labelled with their speed, seconds ago along the bottom, one thin line
   * in the game's colour (the bright accent outside a game) and a dot on its last point. A second
   * without data (`null`) is a gap in the line, not a zero; fewer than two points in a run draw no line.
   * The scale follows the highest speed drawn and the limit; a dashed line marks the limit when one is
   * set. A still picture: nothing animates and nothing ticks, it changes when `series` does.
   *
   * The drawing, its scale and its axis are hidden from assistive tech; the caption it renders ("Speed,
   * last 40 s", "peak 41.7 MB/s") is the figure's name and carries its numbers. It is not a live region,
   * so nothing is announced per tick. Height: `--graph-h`; the room for the gridline labels:
   * `--graph-gutter`.
   */
  let {
    series,
    window: span = 40,
    limit = 0,
    format = speedBytes,
    testid,
  }: {
    /** Bytes/s per second, oldest first, the current second last (lib/downloads/speedHistory.ts). */
    series: SpeedSeries;
    /** How many seconds are drawn: the last `window` samples, one per second, spread over the width. */
    window?: number;
    /** The speed limit in bytes/s; 0 for none, which draws no limit line. */
    limit?: number;
    /** Says a speed in bytes/s: the gridline labels and the peak. */
    format?: (bytesPerSecond: number) => string;
    testid?: string;
  } = $props();

  // The drawing's own coordinates; the SVG is stretched over the frame.
  const W = 240;
  const H = 72;
  const PAD = 3;
  const PLOT = H - PAD * 2;
  // Gridlines at these fractions of the scale, top first.
  const GRID = [0.75, 0.5, 0.25];
  // Room over the highest point, so the line never runs along the top edge.
  const HEADROOM = 1.12;
  // Steps of the time axis in seconds; the first that leaves at most three marks is used.
  const AXIS_STEPS = [5, 10, 15, 30, 60, 120, 300, 600, 1800, 3600];

  const uid = $props.id();
  const seconds = $derived(Math.max(2, Math.floor(span)));
  const recent = $derived(series.samples.slice(-seconds));
  const values = $derived(recent.filter((v): v is number => v !== null && Number.isFinite(v)));
  const peak = $derived(values.length ? Math.max(...values) : null);
  const top = $derived(Math.max(1, limit, peak ?? 0) * HEADROOM);

  const x = (i: number) => W - (recent.length - 1 - i) * (W / (seconds - 1));
  const y = (v: number) => H - PAD - (Math.max(0, v) / top) * PLOT;
  const at = (n: number) => Number(n.toFixed(2));

  const path = $derived.by(() => {
    const runs: string[] = [];
    let run: string[] = [];
    const close = () => {
      if (run.length > 1) runs.push(`M${run.join(' L')}`);
      run = [];
    };
    recent.forEach((v, i) => {
      if (v === null || !Number.isFinite(v)) close();
      else run.push(`${at(x(i))},${at(y(v))}`);
    });
    close();
    return runs.join(' ');
  });

  const last = $derived.by(() => {
    for (let i = recent.length - 1; i >= 0; i--) {
      const v = recent[i];
      if (v !== null && Number.isFinite(v)) return { x: x(i), y: y(v) };
    }
    return null;
  });

  const marks = $derived.by(() => {
    const step = AXIS_STEPS.find((s) => Math.floor((seconds - 1) / s) <= 3) ?? AXIS_STEPS[AXIS_STEPS.length - 1];
    const out: number[] = [];
    for (let s = step * Math.floor((seconds - 1) / step); s > 0; s -= step) if (s < seconds - 1) out.push(s);
    return out;
  });
  const agoX = (s: number) => W - s * (W / (seconds - 1));
  const gridY = (g: number) => H - PAD - g * PLOT;
</script>

<figure class="graph" aria-labelledby="{uid}-caption" data-testid={testid}>
  <figcaption id="{uid}-caption">
    <span><Reading text={$t('downloads.speedGraph.caption', { seconds })} /></span>
    {#if peak !== null}<span class="peak"><Reading text={$t('downloads.speedGraph.peak', { speed: format(peak) })} /></span>{/if}
  </figcaption>
  <div class="frame">
    <div class="gutter" aria-hidden="true">
      {#each GRID as g (g)}
        <span class="ylabel" style:top="{(gridY(g) / H) * 100}%"><Reading text={format(g * top)} /></span>
      {/each}
    </div>
    <div class="plot">
      <svg viewBox="0 0 {W} {H}" preserveAspectRatio="none" aria-hidden="true">
        {#each GRID as g (g)}
          <line class="grid" x1="0" x2={W} y1={gridY(g)} y2={gridY(g)} />
        {/each}
        {#each marks as s (s)}
          <line class="tick" x1={agoX(s)} x2={agoX(s)} y1={H - PAD} y2={H} />
        {/each}
        {#if limit > 0}
          <line class="limit" x1="0" x2={W} y1={at(y(limit))} y2={at(y(limit))} />
        {/if}
        {#if path}<path class="line" d={path} />{/if}
      </svg>
      {#if last}
        <span class="dot" style:left="{(last.x / W) * 100}%" style:top="{(last.y / H) * 100}%" aria-hidden="true"></span>
      {/if}
    </div>
  </div>
  <div class="axis" aria-hidden="true">
    <div class="axis-track">
      {#each marks as s (s)}
        <span style:left="{(agoX(s) / W) * 100}%"><Reading text={$t('downloads.speedGraph.ago', { seconds: s })} /></span>
      {/each}
    </div>
  </div>
</figure>

<style>
  .graph {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
    margin: 0;
  }

  figcaption {
    display: flex;
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--space-1) var(--space-3);
    color: var(--text-2);
    font-size: var(--font-xs);
  }

  .peak {
    color: var(--text-3);
  }

  .frame {
    display: grid;
    grid-template-columns: var(--graph-gutter, 6.4rem) minmax(0, 1fr);
    height: var(--graph-h, 8.8rem);
  }

  .gutter {
    position: relative;
  }

  .ylabel {
    position: absolute;
    right: var(--space-2);
    transform: translateY(-50%);
    white-space: nowrap;
    color: var(--text-3);
    font-size: var(--font-xs);
    pointer-events: none;
  }

  .plot {
    position: relative;
    height: 100%;
    border-bottom: 1px solid var(--border-strong);
    overflow: hidden;
  }

  svg {
    display: block;
    width: 100%;
    height: 100%;
  }

  .grid {
    stroke: var(--border);
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }

  .tick {
    stroke: var(--border-strong);
    stroke-width: 1;
    vector-effect: non-scaling-stroke;
  }

  .limit {
    stroke: var(--warning);
    stroke-width: 1;
    stroke-dasharray: 4 4;
    vector-effect: non-scaling-stroke;
  }

  .line {
    fill: none;
    stroke: var(--game-mark, var(--accent-bright));
    stroke-width: 1.5;
    stroke-linejoin: round;
    stroke-linecap: round;
    vector-effect: non-scaling-stroke;
  }

  .dot {
    position: absolute;
    width: var(--graph-dot, 0.6rem);
    height: var(--graph-dot, 0.6rem);
    border-radius: var(--radius-round);
    background: var(--game-mark, var(--accent-bright));
    transform: translate(-50%, -50%);
  }

  .axis {
    padding-left: var(--graph-gutter, 6.4rem);
  }

  .axis-track {
    position: relative;
    height: 1.4em;
    color: var(--text-3);
    font-size: var(--font-xs);
  }

  .axis-track span {
    position: absolute;
    transform: translateX(-50%);
    white-space: nowrap;
  }
</style>
