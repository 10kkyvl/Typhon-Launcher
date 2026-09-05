<script lang="ts">
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
    <div class="fallback" aria-label={alt || undefined}>
      <span>{initials || '?'}</span>
    </div>
  {:else}
    <img {src} {alt} loading="lazy" draggable="false" onerror={() => (failed = true)} />
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
