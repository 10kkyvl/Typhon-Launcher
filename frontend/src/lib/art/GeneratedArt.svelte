<script lang="ts">
  import { artScene, type ArtShape } from './scene';

  /**
   * The generated picture of a game (scene.ts), drawn to fill its box and cropped like a cover. A
   * decoration: hidden from assistive technology; whatever holds it names the game.
   */
  let { seed, shape = 'cover' }: { seed: string; shape?: ArtShape } = $props();

  const uid = $props.id();
  const scene = $derived(artScene(seed, shape));
</script>

<svg class="generated-art" viewBox="0 0 {scene.width} {scene.height}" preserveAspectRatio="xMidYMid slice" aria-hidden="true" focusable="false">
  <defs>
    <linearGradient id="{uid}-sky" x1="0" y1="0" x2="0" y2="1">
      {#each scene.sky as stop (stop.offset)}
        <stop offset={stop.offset} stop-color={stop.color} />
      {/each}
    </linearGradient>
    {#if scene.sun}
      <radialGradient id="{uid}-glow">
        <stop offset="0" stop-color={scene.sun.glow} />
        <stop offset="1" stop-color={scene.sun.glow} stop-opacity="0" />
      </radialGradient>
    {/if}
  </defs>
  <rect width={scene.width} height={scene.height} fill="url(#{uid}-sky)" />
  {#if scene.stars}
    <path d={scene.stars} fill="currentColor" opacity={scene.starOpacity} />
  {/if}
  {#if scene.sun}
    <circle cx={scene.sun.x} cy={scene.sun.y} r={scene.sun.glowR} fill="url(#{uid}-glow)" />
    <circle cx={scene.sun.x} cy={scene.sun.y} r={scene.sun.r} fill={scene.sun.color} />
  {/if}
  {#each scene.lands as land, index (index)}
    <path d={land.d} fill={land.fill} />
  {/each}
</svg>

<style>
  .generated-art {
    display: block;
    width: 100%;
    height: 100%;
    color: var(--art-text);
  }
</style>
