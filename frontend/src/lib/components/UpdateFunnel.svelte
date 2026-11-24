<script lang="ts">
  import { FUNNEL_FLAT, FUNNEL_LAYERS, FUNNEL_VIEW } from './updateShowTimeline';

  /**
   * The funnel of the update show: five squashed rings, widest on top. Each ring shows by `layers`,
   * lights up by `lit` (bottom first, as the update fills) and stands at its `turn` in degrees. It
   * holds no motion of its own: the show draws it frame by frame from the clock.
   */
  let {
    layers = FUNNEL_LAYERS.map(() => 1),
    lit = FUNNEL_LAYERS.map(() => 0),
    turn = FUNNEL_LAYERS.map(() => 0),
  }: { layers?: readonly number[]; lit?: readonly number[]; turn?: readonly number[] } = $props();
</script>

<svg
  class="funnel"
  viewBox="{-FUNNEL_VIEW.w / 2} {-FUNNEL_VIEW.h / 2} {FUNNEL_VIEW.w} {FUNNEL_VIEW.h}"
  aria-hidden="true"
  focusable="false"
  data-funnel
>
  {#each FUNNEL_LAYERS as layer, i (i)}
    <g data-layer={i} transform="translate(0 {layer.y})" opacity={layers[i] ?? 0}>
      <g transform="scale(1 {FUNNEL_FLAT})">
        <circle class="track" r={layer.rx} />
        <g opacity={lit[i] ?? 0}>
          <circle class="dash" r={layer.rx} pathLength="100" transform="rotate({turn[i] ?? 0})" />
        </g>
      </g>
    </g>
  {/each}
</svg>

<style>
  .funnel {
    display: block;
    width: 100%;
    height: auto;
    overflow: visible;
    color: var(--accent-text);
  }

  circle {
    fill: none;
    stroke: currentColor;
    stroke-linecap: round;
  }

  .track {
    stroke-width: 5;
    opacity: 0.2;
  }

  .dash {
    stroke-width: 9;
    stroke-dasharray: 38 7 20 7 12 6;
  }
</style>
