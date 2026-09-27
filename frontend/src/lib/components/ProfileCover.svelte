<script lang="ts">
  import type { ProfileAppearance } from '../services/account';
  import { appearanceOf, themeOf } from '../profile/appearance';
  let { appearance }: { appearance?: ProfileAppearance } = $props();
  const a = $derived(appearanceOf(appearance));
  const theme = $derived(themeOf(a));
  let failed = $state(false);
  $effect(() => { a.coverUrl; failed = false; });
</script>
<div class="cover" class:has-image={a.coverUrl && !failed} style:background={theme.banner}>
  {#if a.coverUrl && !failed}
    <img src={a.coverUrl} alt="" style:object-position={`50% ${a.coverPosition}%`} onerror={() => (failed = true)} />
  {/if}
  <div class="dim" style:opacity={a.coverDim / 100}></div>
  <div class="fade"></div>
</div>
<style>
  .cover { position: relative; height: 16rem; margin: 0 -2rem -4.8rem; border-radius: var(--radius-lg) var(--radius-lg) 0 0; overflow: hidden; }
  .cover.has-image { height: auto; aspect-ratio: 4 / 1; }
  img, .dim, .fade { position: absolute; inset: 0; width: 100%; height: 100%; }
  img { object-fit: cover; }
  .dim { background: #000; }
  .fade { background: linear-gradient(transparent 35%, var(--profile-bg)); }
  @media (max-width: 800px) { .cover { margin-left: -1.2rem; margin-right: -1.2rem; } }
</style>
