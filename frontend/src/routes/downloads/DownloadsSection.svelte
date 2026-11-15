<script lang="ts">
  import type { Snippet } from 'svelte';

  let { title, count, children }: { title: string; count: number; children: Snippet } = $props();
</script>

<section class="section">
  <h2>{title} <span class="count">{count}</span></h2>
  {@render children()}
</section>

<style>
  .section {
    margin-bottom: var(--space-8);
    max-width: 140rem;
  }

  .section h2 {
    display: flex;
    align-items: baseline;
    gap: 0.8rem;
    font-size: var(--font-lg);
    margin-bottom: var(--space-3);
  }

  .count {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }
</style>
