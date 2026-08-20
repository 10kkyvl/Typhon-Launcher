<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    text,
    children,
  }: {
    text: string;
    children?: Snippet;
  } = $props();
</script>

<span class="tooltip-host">
  {@render children?.()}
  <span class="tip" role="tooltip">{text}</span>
</span>

<style>
  .tooltip-host {
    position: relative;
    display: inline-flex;
  }

  .tip {
    position: absolute;
    z-index: 60;
    bottom: calc(100% + 7px);
    left: 50%;
    transform: translateX(-50%) translateY(2px);
    padding: 5px 9px;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: 7px;
    box-shadow: var(--shadow-pop);
    font-size: 12px;
    color: var(--text-2);
    white-space: nowrap;
    opacity: 0;
    pointer-events: none;
    transition:
      opacity var(--dur-fast) var(--ease),
      transform var(--dur-fast) var(--ease);
  }

  .tooltip-host:hover .tip,
  .tooltip-host:focus-within .tip {
    opacity: 1;
    transform: translateX(-50%) translateY(0);
    transition-delay: 350ms;
  }
</style>
