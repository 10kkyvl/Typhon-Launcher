<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    padding = 'var(--space-6)',
    children,
  }: {
    padding?: string;
    children?: Snippet;
  } = $props();
</script>

<div class="card" style:padding>
  {@render children?.()}
</div>

<style>
  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-card);
  }
</style>
