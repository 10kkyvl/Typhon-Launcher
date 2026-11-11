<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    padding = 'var(--space-5) var(--space-6)',
    title,
    action,
    surface = 'card',
    children,
  }: {
    padding?: string;
    title?: string;
    action?: Snippet;
    surface?: 'panel' | 'card';
    children?: Snippet;
  } = $props();
</script>

<div class="card {surface}" style:padding>
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
