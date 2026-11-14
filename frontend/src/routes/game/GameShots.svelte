<script lang="ts">
  import Artwork from '../../lib/components/Artwork.svelte';
  import { msg } from '../../lib/i18n';
  import type { MediaAsset } from '../../lib/services/metadata';

  // The game's gallery pictures as a grid; the page owns the viewer they open.
  let { shots, onopen }: { shots: readonly MediaAsset[]; onopen: (index: number) => void } = $props();

  const shotCols = $derived(
    shots.length >= 5 ? 4 : shots.length === 4 ? 2 : Math.max(1, shots.length),
  );
</script>

<div
  class="shots"
  class:featured={shots.length >= 5}
  class:one={shots.length === 1}
  style:--cols={shotCols}
>
  {#each shots as shot, index (shot.id)}
    <button class="shot" onclick={() => onopen(index)} aria-label={msg('games.detailOpenScreenshotAt', { index: index + 1, count: shots.length })}>
      <Artwork src={shot.url ?? ''} alt="" ratio="16 / 9" radius="var(--radius-sm)" />
    </button>
  {/each}
</div>

<style>
  .shots {
    display: grid;
    grid-template-columns: repeat(var(--cols, 3), minmax(0, 1fr));
    gap: var(--space-3);
  }

  .shots.one {
    max-width: 72rem;
  }

  .shot {
    display: block;
    width: 100%;
    aspect-ratio: 16 / 9;
    border-radius: var(--radius-sm);
    overflow: hidden;
    transition:
      transform var(--dur) var(--ease),
      opacity var(--dur) var(--ease);
  }

  .shot:hover {
    transform: translateY(-2px);
    opacity: 0.92;
  }

  .shots.featured .shot:first-child {
    grid-column: span 2;
    grid-row: span 2;
  }

  @media (max-width: 1400px) {
    .shots {
      grid-template-columns: repeat(auto-fill, minmax(28rem, 1fr));
    }

    .shots.featured {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 1200px) {
    .shots,
    .shots.featured {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .shots.featured .shot:first-child {
      grid-row: span 1;
    }
  }
</style>
