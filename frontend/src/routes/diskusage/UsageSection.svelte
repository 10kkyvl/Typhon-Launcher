<script lang="ts">
  import type { Snippet } from 'svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';

  // One section of the page: its heading, hint and total, then its rows or a line saying it has none.
  let {
    title,
    hint,
    total,
    tools,
    empty = false,
    emptyText = '',
    children,
  }: {
    title: string;
    hint: string;
    total: string;
    /** Controls beside the total; they and the total then share one group. */
    tools?: Snippet;
    empty?: boolean;
    emptyText?: string;
    children: Snippet;
  } = $props();
</script>

<section>
  <div class="section-head">
    <div>
      <h2>{title}</h2>
      <p class="muted">{hint}</p>
    </div>
    {#if tools}
      <div class="section-tools">
        <span class="section-total">{total}</span>
        {@render tools()}
      </div>
    {:else}
      <span class="section-total">{total}</span>
    {/if}
  </div>
  {#if empty}
    <EmptyState size="inline" title={emptyText} />
  {:else}
    <div class="list">
      {@render children()}
    </div>
  {/if}
</section>

<style>
  section {
    margin-top: var(--space-8);
  }

  h2 {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  .muted {
    margin-top: 0.3rem;
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .section-head {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-3);
  }

  .section-tools {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .section-total {
    font-size: var(--font-md);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
</style>
