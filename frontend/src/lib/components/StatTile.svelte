<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    value,
    label,
    icon,
  }: {
    value: string | number;
    label: string;
    icon?: Snippet;
  } = $props();
</script>

<div class="tile">
  {#if icon}
    <span class="icon">{@render icon()}</span>
  {/if}
  <span class="value">{value}</span>
  <span class="label">{label}</span>
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

  .label {
    font-size: var(--font-sm);
    color: var(--text-3);
  }
</style>
