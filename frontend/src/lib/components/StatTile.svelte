<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * A figure of a screen: its value over its label. `mark` stands where the value would when there is
   * no value to show (a figure its owner hid); `sub` is a second line under the label (what the
   * figure is of: the game and the day of the longest session).
   */
  let {
    value,
    mark,
    label,
    sub,
    icon,
  }: ({ value: string | number; mark?: undefined } | { value?: undefined; mark: Snippet }) & {
    label: string;
    sub?: string;
    icon?: Snippet;
  } = $props();
</script>

<div class="tile">
  {#if icon}
    <span class="icon">{@render icon()}</span>
  {/if}
  {#if mark}
    <span class="mark">{@render mark()}</span>
  {:else}
    <span class="value">{value}</span>
  {/if}
  <span class="label">{label}</span>
  {#if sub}
    <span class="sub">{sub}</span>
  {/if}
</div>

<style>
  .tile {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.8rem;
    min-width: 0;
  }

  .icon {
    display: inline-flex;
    color: var(--text-3);
  }

  .value {
    font-size: var(--font-title);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    line-height: 1.1;
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }

  .mark {
    display: inline-flex;
  }

  .label {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .sub {
    max-width: 100%;
    font-size: var(--font-xs);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
