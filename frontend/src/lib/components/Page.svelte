<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * The root of every main-window route: the page's content at the page inset (`--page-x`) and width
   * (`--page-max`), painting nothing, so the window backdrop shows around the page's own blocks.
   * `bleed` is the one way out of the inset: it renders above the content across the whole scroller,
   * and `--page-inset` lines its own content up with the page's.
   */
  let { bleed, children }: { bleed?: Snippet; children?: Snippet } = $props();
</script>

{#if bleed}
  <div class="bleed">{@render bleed()}</div>
{/if}
<div class="content">
  {@render children?.()}
</div>

<style>
  .bleed {
    --page-inset: max(var(--page-x), calc((100% - var(--page-max)) / 2));
  }

  .content {
    max-width: calc(var(--page-max) + 2 * var(--page-x));
    margin-inline: auto;
    padding: 0 var(--page-x) var(--space-10);
  }
</style>
