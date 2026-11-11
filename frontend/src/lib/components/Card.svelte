<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    padding = 'md',
    title,
    action,
    surface = 'card',
    children,
  }: {
    /** `md` is wider at the sides, `sm` tighter; `-even` is the same on all four. */
    padding?: 'md' | 'md-even' | 'sm' | 'sm-even';
    title?: string;
    action?: Snippet;
    surface?: 'panel' | 'card';
    children?: Snippet;
  } = $props();
</script>

<div class="card {surface} pad-{padding}">
  {#if title}
    <div class="head">
      <h3 class="title">{title}</h3>
      {#if action}
        <div class="action">{@render action()}</div>
      {/if}
    </div>
  {/if}
  {@render children?.()}
</div>

<style>
  .card {
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .card.panel {
    background: var(--surface);
  }

  .pad-md {
    padding: var(--space-5) var(--space-6);
  }

  .pad-md-even {
    padding: var(--space-5);
  }

  .pad-sm {
    padding: var(--space-4) var(--space-5);
  }

  .pad-sm-even {
    padding: var(--space-4);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .title {
    font-size: var(--font-lg);
    font-weight: 600;
  }
</style>
