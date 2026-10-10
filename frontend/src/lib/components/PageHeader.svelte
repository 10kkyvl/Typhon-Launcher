<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    title,
    subtitle,
    count,
    actions,
  }: {
    title: string;
    subtitle?: string;
    count?: string;
    actions?: Snippet;
  } = $props();
</script>

<header class="page-header">
  <div class="text">
    <div class="heading">
      <h1>{title}</h1>
      {#if count}
        <span class="count">{count}</span>
      {/if}
    </div>
    {#if subtitle}
      <p>{subtitle}</p>
    {/if}
  </div>
  {#if actions}
    <div class="actions">
      {@render actions()}
    </div>
  {/if}
</header>

<style>
  .page-header {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-6);
    padding-top: var(--space-3);
    margin-bottom: var(--space-6);
  }

  .text {
    min-width: 0;
  }

  .heading {
    display: flex;
    align-items: baseline;
    gap: var(--space-3);
    min-width: 0;
  }

  .count {
    font-size: var(--font-md);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  h1 {
    font-size: var(--font-title);
    font-weight: 600;
    line-height: 1.1;
    letter-spacing: var(--tracking-title);
  }

  p {
    margin-top: 0.6rem;
    max-width: var(--prose-max);
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
    padding-bottom: 0.3rem;
  }
</style>
