<script lang="ts">
  import type { Snippet } from 'svelte';

  /**
   * The root of every main-window route: the page's content at the page inset (`--page-x`) and width
   * (`--page-max`), painting nothing, so the window backdrop shows around the page's own blocks.
   * `bleed` is the one way out of the inset: it renders above the content across the whole scroller.
   * A bleed child may rely on two things: the bleed area is an inline-size container as wide as the
   * scroller, and `--page-inset` is the distance from its edge to the page content's edge, in container
   * units, so it holds as padding, as an inset of an absolutely placed child or in vertical use alike.
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
    container-type: inline-size;
    --page-inset: max(var(--page-x), calc((100cqi - var(--page-max)) / 2));
  }

  .content {
    max-width: calc(var(--page-max) + 2 * var(--page-x));
    margin-inline: auto;
    padding: 0 var(--page-x) var(--space-10);
  }
</style>
