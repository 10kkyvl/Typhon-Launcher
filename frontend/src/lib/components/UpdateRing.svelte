<script lang="ts">
  /**
   * The update show with motion off: a still ring that fills by `value` (0..1). Decorative: the show
   * says in words what is going on.
   */
  let { value = 0 }: { value?: number } = $props();

  const RADIUS = 40;
  const LENGTH = 2 * Math.PI * RADIUS;
  const v = $derived(Math.min(1, Math.max(0, Number.isFinite(value) ? value : 0)));
</script>

<svg class="ring" viewBox="0 0 100 100" aria-hidden="true" focusable="false" data-ring-value={v.toFixed(2)}>
  <circle class="track" cx="50" cy="50" r={RADIUS} />
  <circle class="fill" cx="50" cy="50" r={RADIUS} stroke-dasharray={LENGTH} stroke-dashoffset={LENGTH * (1 - v)} transform="rotate(-90 50 50)" />
</svg>

<style>
  .ring {
    display: block;
    width: 100%;
    height: 100%;
    color: var(--accent-text);
  }

  circle {
    fill: none;
    stroke: currentColor;
    stroke-width: 8;
    stroke-linecap: round;
  }

  .track {
    opacity: 0.22;
  }
</style>
