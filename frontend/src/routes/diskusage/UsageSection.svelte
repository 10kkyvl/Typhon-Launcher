<script lang="ts">
  import type { Snippet } from 'svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import SectionHeader from '../../lib/components/SectionHeader.svelte';

  // One section of the page: its heading with the count of its rows, its total and tools, its hint,
  // then its rows as a list or a line saying it has none. The heading's id names the section; the
  // summary's meter and the focus rule find the section by it.
  let {
    id,
    title,
    count,
    hint,
    total,
    tools,
    empty = false,
    emptyText = '',
    children,
  }: {
    id: string;
    title: string;
    count: number;
    hint: string;
    total: string;
    /** Controls beside the total. */
    tools?: Snippet;
    empty?: boolean;
    emptyText?: string;
    /** The rows: UsageRow items. */
    children: Snippet;
  } = $props();
</script>

<section aria-labelledby={id} data-usage-section={id}>
  <SectionHeader {id} {title} {count}>
    {#snippet actions()}
      <span class="section-total">{total}</span>
      {@render tools?.()}
    {/snippet}
  </SectionHeader>
  <p class="hint">{hint}</p>
  {#if empty}
    <EmptyState size="inline" title={emptyText} />
  {:else}
    <ul class="list">
      {@render children()}
    </ul>
  {/if}
</section>

<style>
  section {
    margin-top: var(--space-8);
  }

  .hint {
    margin: 0 0 var(--space-3);
    font-size: var(--font-sm);
    color: var(--text-3);
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
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
