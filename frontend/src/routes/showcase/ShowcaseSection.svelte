<script lang="ts">
  import type { Snippet } from 'svelte';

  /** One group of the showcase: a heading the page's outline can jump to, then its rows. */
  let { title, note, children }: { title: string; note?: string; children: Snippet } = $props();

  const uid = $props.id();
</script>

<section class="section" aria-labelledby="{uid}-title">
  <h2 id="{uid}-title">{title}</h2>
  {#if note}<p class="note">{note}</p>{/if}
  <div class="rows">
    {@render children()}
  </div>
</section>

<style>
  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-6);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--island-fill);
  }

  h2 {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    color: var(--text);
  }

  .note {
    max-width: var(--prose-max);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .rows {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
</style>
