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
    bottom: calc(100% + 0.7rem);
    left: 50%;
    transform: translateX(-50%) translateY(2px);
    padding: 0.5rem 0.9rem;
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    border-radius: 0.7rem;
    box-shadow: var(--shadow-pop);
    font-size: 1.3rem;
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
