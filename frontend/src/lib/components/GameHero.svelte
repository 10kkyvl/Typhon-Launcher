<script lang="ts">
  import type { Snippet } from 'svelte';
  import Artwork from './Artwork.svelte';

  let {
    src,
    alt = '',
    ratio = '3.8 / 1',
    minHeight,
    maxHeight,
    children,
  }: {
    src: string;
    alt?: string;
    ratio?: string;
    minHeight?: string;
    maxHeight?: string;
    children?: Snippet;
  } = $props();
</script>

<section class="hero" style:aspect-ratio={ratio} style:min-height={minHeight} style:max-height={maxHeight}>
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
    width: 100%;
    border-radius: var(--cut) var(--radius-xl) var(--radius-xl) var(--radius-xl);
    overflow: hidden;
    background: var(--surface);
  }

  .art {
    position: absolute;
    inset: 0;
  }

  .overlay {
    position: absolute;
    inset: 0;
    background:
      linear-gradient(180deg, rgba(11, 15, 20, 0) 55%, rgba(11, 15, 20, 0.92) 100%),
      linear-gradient(90deg, rgba(11, 15, 20, 0.88) 0%, rgba(11, 15, 20, 0.5) 38%, rgba(11, 15, 20, 0) 70%);
  }

  .content {
    position: relative;
    z-index: 1;
    height: 100%;
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    padding: var(--space-8) var(--space-8) var(--space-8);
    max-width: 62%;
  }

  @media (max-width: 1280px) {
    .content {
      max-width: 78%;
      padding: var(--space-6);
    }
  }
</style>
