<script lang="ts">
  import type { Snippet } from 'svelte';
  import Artwork from './Artwork.svelte';

  let {
    src,
    alt = '',
    ratio = '3.8 / 1',
    minHeight,
    children,
  }: {
    src: string;
    alt?: string;
    ratio?: string;
    minHeight?: string;
    children?: Snippet;
  } = $props();
</script>

<section class="hero" style:aspect-ratio={ratio} style:min-height={minHeight}>
  <div class="art">
    <Artwork {src} {alt} />
  </div>
  <div class="overlay"></div>
  <div class="content">
    {@render children?.()}
  </div>
</section>

<style>
  .hero {
    position: relative;
    border-radius: var(--radius-xl);
    overflow: hidden;
    border: 1px solid var(--border);
  }

  .art {
    position: absolute;
    inset: 0;
  }

  .overlay {
    position: absolute;
    inset: 0;
    background: linear-gradient(90deg, rgba(5, 8, 12, 0.9), rgba(5, 8, 12, 0.45) 45%, rgba(5, 8, 12, 0.05));
  }

  .content {
    position: relative;
    z-index: 1;
    height: 100%;
    display: flex;
    flex-direction: column;
    justify-content: center;
    padding: var(--space-8) var(--space-10);
    max-width: 60%;
  }

  @media (max-width: 1280px) {
    .content {
      max-width: 75%;
      padding: var(--space-6) var(--space-8);
    }
  }
</style>
