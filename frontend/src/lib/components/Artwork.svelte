<script lang="ts">
  import { revealImage } from '../utils/revealImage';

  /**
   * A cover or a wide picture. `alt` is its text alternative, for a picture that alone names what
   * holds it; next to the visible title it is '' and `label` gives the title only to draw its first
   * letters when there is no picture. Those letters are decoration, hidden from assistive
   * technology: without a picture the artwork is still an image named `alt`, or nothing when `alt`
   * is empty.
   */
  let {
    src,
    alt = '',
    label = '',
    ratio,
    radius = '0',
  }: {
    src: string;
    alt?: string;
    label?: string;
    ratio?: string;
    radius?: string;
  } = $props();

  let failed = $state(false);

  $effect(() => {
    src;
    failed = false;
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
  {#if failed || !src}
    <div class="fallback" role={alt ? 'img' : undefined} aria-label={alt || undefined}>
      <span aria-hidden="true">{initials || '?'}</span>
    </div>
  {:else}
    {#key src}
      <img {src} {alt} class="media-reveal" use:revealImage loading="lazy" decoding="async" draggable="false" onerror={() => (failed = true)} />
    {/key}
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
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    background:
      linear-gradient(160deg, rgba(255, 255, 255, 0.05), transparent 55%),
      var(--surface-3);
    color: var(--text-3);
    font-size: 2rem;
    font-weight: 600;
    letter-spacing: 0.02em;
    container-type: inline-size;
  }

  .fallback span {
    font-size: clamp(1.2rem, 22cqw, 3.2rem);
  }
</style>
