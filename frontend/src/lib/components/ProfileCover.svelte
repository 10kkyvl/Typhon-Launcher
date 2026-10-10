<script lang="ts">
  import type { ProfileAppearance } from '../services/account';
  import { appearanceOf, AUTO_THEME, themeOf } from '../profile/appearance';

  const REACH = 12;

  let { appearance, autoArt }: { appearance?: ProfileAppearance; autoArt?: string } = $props();
  const a = $derived(appearanceOf(appearance));
  const theme = $derived(themeOf(a));
  const fromArt = $derived(!a.coverUrl && a.theme === AUTO_THEME && !!autoArt);
  const image = $derived(a.coverUrl || (fromArt ? autoArt ?? '' : ''));
  let failed = $state(false);
  let el: HTMLDivElement | undefined = $state();
  let shift = $state({ x: 0, y: 0 });
  $effect(() => { image; failed = false; });

  const moving = $derived(a.parallax && !!image && !failed);
  $effect(() => {
    if (!moving || !el) return;
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    const cover = el;
    let frame = 0;
    let pointerX = 0, pointerY = 0, scrolled = 0;
    const clamp = (n: number) => Math.max(-REACH, Math.min(REACH, n));
    const apply = () => {
      frame = 0;
      shift = { x: clamp(pointerX * 8), y: clamp(pointerY * 8 + scrolled) };
    };
    const schedule = () => { if (!frame) frame = requestAnimationFrame(apply); };
    const onPointer = (event: PointerEvent) => {
      pointerX = (event.clientX / window.innerWidth - 0.5) * 2;
      pointerY = (event.clientY / window.innerHeight - 0.5) * 2;
      schedule();
    };
    const onScroll = () => {
      scrolled = Math.max(-8, Math.min(8, -cover.getBoundingClientRect().top * 0.05));
      schedule();
    };
    window.addEventListener('pointermove', onPointer, { passive: true });
    window.addEventListener('scroll', onScroll, { passive: true, capture: true });
    onScroll();
    return () => {
      window.removeEventListener('pointermove', onPointer);
      window.removeEventListener('scroll', onScroll, { capture: true });
      if (frame) cancelAnimationFrame(frame);
      shift = { x: 0, y: 0 };
    };
  });
</script>
<div class="cover" bind:this={el} class:has-image={image && !failed} style:background={theme.banner}>
  {#if image && !failed}
    <div class="layer" class:moving style:transform={moving ? `translate3d(${shift.x}px, ${shift.y}px, 0)` : undefined}>
      <img class:art={fromArt} src={image} alt="" style:object-position={`50% ${a.coverPosition}%`} onerror={() => (failed = true)} />
    </div>
  {/if}
  <div class="dim" style:opacity={a.coverDim / 100}></div>
  <div class="fade"></div>
</div>
<style>
  .cover { position: relative; height: 11rem; margin: 0 -2rem -4.8rem; border-radius: var(--radius-lg) var(--radius-lg) 0 0; overflow: hidden; }
  .cover.has-image { height: auto; aspect-ratio: 4 / 1; }
  .layer, .dim, .fade { position: absolute; inset: 0; width: 100%; height: 100%; }
  .layer.moving { inset: -14px; width: auto; height: auto; transition: transform var(--dur-panel) var(--ease); will-change: transform; }
  img { width: 100%; height: 100%; object-fit: cover; }
  img.art { filter: blur(14px) saturate(1.15); transform: scale(1.15); }
  .dim { background: #000; }
  .fade { background: linear-gradient(transparent 35%, var(--profile-bg)); }
  @media (prefers-reduced-motion: reduce) { .layer.moving { transition: none; } }
  @media (max-width: 800px) { .cover { margin-left: -1.2rem; margin-right: -1.2rem; } }
</style>
