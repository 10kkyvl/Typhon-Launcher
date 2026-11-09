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
    bottom: calc(100% + 0.66rem);
    left: 50%;
    transform: translateX(-50%) translateY(2px);
    padding: 0.55rem 0.88rem;
    max-width: 35.2rem;
    background: var(--surface-4);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-pop);
    font-size: var(--font-xs);
    color: var(--text-2);
    white-space: normal;
    width: max-content;
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
