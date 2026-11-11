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
      <!-- Focusable by script only: focus lands here when the item it was on is gone (a removed game). -->
      <h1 tabindex="-1">{title}</h1>
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
    flex-wrap: wrap;
    justify-content: space-between;
    gap: var(--space-3) var(--space-6);
    padding-top: var(--space-3);
    margin-bottom: var(--space-6);
  }

  /* The row is bottom-aligned for the buttons; the title stays at the top whatever stands beside it. */
  .text {
    flex: 1 1 24rem;
    align-self: flex-start;
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
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-2);
    max-width: 100%;
    padding-bottom: 0.3rem;
  }
</style>
