<script lang="ts" module>
  /** What a bar's colour says: running, done, waiting on the user, failed, or paused. */
  export type ProgressTone = 'accent' | 'success' | 'warning' | 'danger' | 'muted';
</script>

<script lang="ts">
  /**
   * A progress bar (WAI-ARIA progressbar). `label` or `labelledby` says what it measures; with
   * `indeterminate` the amount is unknown and it claims no value. The state shows as a `tone`, never
   * a colour from the caller. `height` is in tenths of a rem; Big Picture passes its pinned pixel
   * sizes through it.
   */
  let {
    value,
    indeterminate = false,
    max = 100,
    height = 4,
    tone = 'accent',
    label,
    labelledby,
    valueText,
    stretch = false,
  }: {
    value: number;
    indeterminate?: boolean;
    max?: number;
    height?: number;
    tone?: ProgressTone;
    label?: string;
    labelledby?: string;
    /** The value in words, when a percentage of the range is not how the page says it. */
    valueText?: string;
    /** Takes the free room of the row it sits in. */
    stretch?: boolean;
  } = $props();

  const now = $derived(max > 0 && Number.isFinite(value) ? Math.min(max, Math.max(0, value)) : 0);
  const pct = $derived(max > 0 ? (now / max) * 100 : 0);
  // A count stays exact; a fraction is cut to the whole percent, the way `progressPercent` shows it.
  const exposed = $derived(Number.isInteger(now) ? now : (Math.floor(pct) * max) / 100);
</script>

<div
  class="track"
  class:stretch
  data-tone={tone}
  role="progressbar"
  aria-label={label}
  aria-labelledby={label ? undefined : labelledby}
  aria-valuemin={0}
  aria-valuemax={max}
  aria-valuenow={indeterminate ? undefined : exposed}
  aria-valuetext={indeterminate ? undefined : valueText}
  aria-busy={indeterminate ? 'true' : undefined}
  style:height="{height / 10}rem"
>
  <div class="fill" class:indeterminate style:--pct={indeterminate ? undefined : `${pct}%`}></div>
</div>

<style>
  .track {
    --bar: var(--accent-bright);
    width: 100%;
    background: var(--hover-strong);
    border-radius: var(--radius-bar);
    overflow: hidden;
  }

  .track.stretch {
    flex: 1;
  }

  .track[data-tone='success'] {
    --bar: var(--success);
  }

  .track[data-tone='warning'] {
    --bar: var(--warning);
  }

  .track[data-tone='danger'] {
    --bar: var(--danger);
  }

  .track[data-tone='muted'] {
    --bar: var(--text-3);
  }

  /* Full width, shifted by the value: a change moves the fill and lays nothing out again. */
  .fill {
    width: 100%;
    height: 100%;
    border-radius: var(--radius-bar);
    background: var(--bar);
    transform: translateX(calc(var(--pct, 0%) - 100%));
    transition: transform var(--progress-step) linear;
  }

  .fill.indeterminate {
    width: 30%;
    transform: none;
    transition: none;
    animation: sweep var(--progress-sweep) ease-in-out infinite;
  }

  @keyframes sweep {
    from {
      transform: translateX(-100%);
    }
    to {
      transform: translateX(340%);
    }
  }

  /* With animations off a short still bar would read as "30 % done": the unknown amount is stripes. */
  :global(.no-anim) .fill.indeterminate {
    width: 100%;
    animation: none;
    background: repeating-linear-gradient(-45deg, var(--bar) 0 0.4rem, transparent 0.4rem 0.8rem);
  }

  @media (prefers-reduced-motion: reduce) {
    .fill.indeterminate {
      width: 100%;
      animation: none;
      background: repeating-linear-gradient(-45deg, var(--bar) 0 0.4rem, transparent 0.4rem 0.8rem);
    }
  }
</style>
