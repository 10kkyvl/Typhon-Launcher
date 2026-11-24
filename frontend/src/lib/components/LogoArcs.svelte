<script lang="ts">
  import { LOGO_ARCS, LOGO_VIEWBOX } from './logoArcs';

  /**
   * The logo in vector: its five arcs (`data-arc`), on its tile when `tile`. Drawn in the logo's own
   * colours, or in the accent when `tint` (the appearance setting that tints the logo). `only` draws
   * one arc (a flying copy in the update show); `away` hides the arcs that are flying.
   */
  let {
    tile = false,
    tint = false,
    only,
    away = null,
  }: { tile?: boolean; tint?: boolean; only?: number; away?: readonly boolean[] | null } = $props();

  const id = $props.id();
  const arcs = $derived(only === undefined ? LOGO_ARCS : LOGO_ARCS.filter((arc) => arc.id === only));
  // Arc 1 carries the sheen of the picture's highlight.
  const sheen = $derived(arcs.some((arc) => arc.id === 1));
  const hidden = (arc: number) => (away?.[arc] ? 0 : undefined);
</script>

<svg class="logo-arcs" class:tint viewBox="0 0 {LOGO_VIEWBOX} {LOGO_VIEWBOX}" aria-hidden="true" focusable="false" data-logo-mark>
  <defs>
    {#if tile}
      <linearGradient id="{id}-tile" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0" class="tile-top" />
        <stop offset="1" class="tile-bottom" />
      </linearGradient>
      <radialGradient id="{id}-glow" gradientUnits="userSpaceOnUse" cx="140" cy="120" r="110">
        <stop offset="0" class="glow" stop-opacity="0.55" />
        <stop offset="1" class="glow" stop-opacity="0" />
      </radialGradient>
    {/if}
    <linearGradient id="{id}-arc" gradientUnits="userSpaceOnUse" x1="200" y1="40" x2="60" y2="220">
      <stop offset="0" class="light" />
      <stop offset="0.55" class="mid" />
      <stop offset="1" class="deep" />
    </linearGradient>
    <radialGradient id="{id}-sheen" gradientUnits="userSpaceOnUse" cx="152" cy="108" r="62">
      <stop offset="0" class="light" stop-opacity="0.9" />
      <stop offset="1" class="light" stop-opacity="0" />
    </radialGradient>
  </defs>
  {#if tile}
    <rect width={LOGO_VIEWBOX} height={LOGO_VIEWBOX} rx="50" fill="url(#{id}-tile)" />
    <rect width={LOGO_VIEWBOX} height={LOGO_VIEWBOX} rx="50" fill="url(#{id}-glow)" />
  {/if}
  {#each arcs as arc (arc.id)}
    <path data-arc={arc.id} d={arc.d} fill="url(#{id}-arc)" fill-opacity={arc.opacity} style:opacity={hidden(arc.id)} />
  {/each}
  {#if sheen}
    <path d={LOGO_ARCS[1].d} fill="url(#{id}-sheen)" fill-opacity="0.6" style:opacity={hidden(1)} />
  {/if}
</svg>

<style>
  .logo-arcs {
    display: block;
    width: 100%;
    height: 100%;
    overflow: visible;
  }

  .tile-top {
    stop-color: var(--logo-tile-top);
  }

  .tile-bottom {
    stop-color: var(--logo-tile-bottom);
  }

  .glow {
    stop-color: var(--logo-glow);
  }

  .light {
    stop-color: var(--logo-arc-light);
  }

  .mid {
    stop-color: var(--logo-arc-mid);
  }

  .deep {
    stop-color: var(--logo-arc-deep);
  }

  .tint .light {
    stop-color: var(--accent-text);
  }

  .tint .mid,
  .tint .deep {
    stop-color: var(--accent);
  }
</style>
