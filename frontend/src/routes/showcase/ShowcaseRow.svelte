<script lang="ts">
  import type { Snippet } from 'svelte';

  /** A caption over the samples of one kind, laid out in a wrapping row (or a column with `stack`). */
  let { caption, stack = false, children }: { caption: string; stack?: boolean; children: Snippet } = $props();
</script>

<div class="row">
  <h3 class="caption">{caption}</h3>
  <div class="samples" class:stack>
    {@render children()}
  </div>
</div>

<style>
  .row {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .caption {
    font-size: var(--font-xs);
    font-weight: 600;
    letter-spacing: var(--tracking-title);
    text-transform: uppercase;
    color: var(--text-3);
  }

  .samples {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
  }

  .samples.stack {
    flex-direction: column;
    flex-wrap: nowrap;
    align-items: stretch;
  }
</style>
