<script lang="ts">
  let {
    src,
    alt = '',
    ratio,
    radius = '0',
  }: {
    src: string;
    alt?: string;
    ratio?: string;
    radius?: string;
  } = $props();

  let failed = $state(false);
</script>

<div class="artwork" style:aspect-ratio={ratio} style:border-radius={radius}>
  {#if failed}
    <div class="fallback">
      <span>{alt.slice(0, 1) || '?'}</span>
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
    background: linear-gradient(135deg, #1a2330, #12181f);
    color: var(--text-3);
    font-size: 28px;
    font-weight: 600;
  }
</style>
