<script lang="ts">
  import type { Snippet } from 'svelte';

  let {
    padding = 'var(--space-5) var(--space-6)',
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
    border-radius: var(--radius-lg);
  }
</style>
