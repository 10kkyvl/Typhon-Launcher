<script lang="ts">
  import GeneratedArt from '../art/GeneratedArt.svelte';
  import type { ArtShape } from '../art/scene';
  import { revealImage } from '../utils/revealImage';

  /**
   * A cover or a wide picture. `alt` is its text alternative, for a picture that alone names what
   * holds it; next to the visible title it is '' and `label` gives the title only to draw its first
   * letters when there is no picture. Those letters are decoration, hidden from assistive
   * technology: without a picture the artwork is still an image named `alt`, or nothing when `alt`
   * is empty.
   *
   * What shows, the first that exists and loads: `src`, then `fallback` (the game's own cover under a
   * metadata cover that may fail), then, given a `seed`, the game's generated art (lib/art) with the
   * letters on top. The seed names the game, the same one on every screen (its canonical id): without
   * one (a search hit, a screenshot) the letters, or "?", stand on a plain tile.
   */
  let {
    src,
    fallback = '',
    alt = '',
    label = '',
    seed,
    ratio,
    radius = '0',
  }: {
    src: string;
    fallback?: string;
    alt?: string;
    label?: string;
    seed?: string;
    ratio?: string;
    radius?: string;
  } = $props();

  let failed = $state<Record<string, true>>({});

  $effect(() => {
    void src;
    void fallback;
    failed = {};
  });

  const shown = $derived([src, fallback].find((url) => url && !(url in failed)) ?? '');

  const shape = $derived.by((): ArtShape => {
    const [w, h] = (ratio ?? '').split('/').map((part) => Number(part.trim()));
    return w > 0 && h > 0 && w > h ? 'wide' : 'cover';
  });

  const initials = $derived(
    (label || alt)
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((w) => w[0]?.toUpperCase() ?? '')
      .join(''),
  );
</script>

<div class="artwork" style:aspect-ratio={ratio} style:border-radius={radius}>
  {#if shown}
    {#key shown}
      <img
        src={shown}
        {alt}
        class="media-reveal"
        use:revealImage
        loading="lazy"
        decoding="async"
        draggable="false"
        onerror={() => (failed = { ...failed, [shown]: true })}
      />
    {/key}
  {:else if seed}
    <div class="fallback" role={alt ? 'img' : undefined} aria-label={alt || undefined}>
      <span class="art"><GeneratedArt {seed} {shape} /></span>
      {#if initials}<span class="letters" aria-hidden="true">{initials}</span>{/if}
    </div>
  {:else}
    <div class="tile" role={alt ? 'img' : undefined} aria-label={alt || undefined}>
      <span aria-hidden="true">{initials || '?'}</span>
    </div>
  {/if}
</div>

<style>
  .artwork {
    position: relative;
    overflow: hidden;
    background: var(--surface-3);
    width: 100%;
    height: 100%;
  }

  .artwork:has(img:not([data-ready]))::after {
    content: '';
    position: absolute;
    inset: 0;
    background: var(--surface-2);
    animation: loading-breathe 1.6s ease-in-out 3;
    pointer-events: none;
  }

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .fallback {
    position: relative;
    display: grid;
    width: 100%;
    height: 100%;
    container-type: inline-size;
  }

  .fallback > span {
    grid-area: 1 / 1;
  }

  .letters {
    place-self: center;
    color: var(--art-text);
    font-size: clamp(1.2rem, 22cqw, 3.2rem);
    font-weight: 600;
    letter-spacing: 0.02em;
    text-shadow: var(--art-text-shadow);
  }

  .tile {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    background:
      linear-gradient(160deg, var(--artwork-sheen), transparent 55%),
      var(--surface-3);
    color: var(--text-3);
    font-size: 2rem;
    font-weight: 600;
    letter-spacing: 0.02em;
    container-type: inline-size;
  }

  .tile span {
    font-size: clamp(1.2rem, 22cqw, 3.2rem);
  }
</style>
