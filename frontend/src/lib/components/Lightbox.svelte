<script lang="ts">
  import '../../styles/zoom-slider.css';
  import { ChevronLeft, ChevronRight, X, Minus, Plus, RotateCcw } from '@lucide/svelte';
  import { fade } from 'svelte/transition';
  import { stepIndex } from '../game/view';
  import { msg } from '../i18n';
  import IconButton from './IconButton.svelte';

  export interface LightboxItem {
    id: string;
    url: string;
  }

  let {
    open = $bindable(false),
    index = $bindable(0),
    items,
    label = msg('ui.imageViewer'),
  }: {
    open?: boolean;
    index?: number;
    items: LightboxItem[];
    label?: string;
  } = $props();

  let failed = $state<Record<string, boolean>>({});

  const current = $derived(items[index]);
  const broken = $derived(current ? Boolean(failed[current.id]) : false);

  let zoom = $state(1);
  let x = $state(0);
  let y = $state(0);
  let viewport = $state<HTMLDivElement>();
  let picture = $state<HTMLImageElement>();
  let drag = $state<{ id: number; x: number; y: number; startX: number; startY: number; moved: boolean } | null>(null);

  let reducedMotion = $state(false);
  $effect(() => {
    const preference = window.matchMedia('(prefers-reduced-motion: reduce)');
    const update = () => { reducedMotion = preference.matches; };
    update();
    preference.addEventListener('change', update);
    return () => preference.removeEventListener('change', update);
  });

  // Warm adjacent images so their fade does not finish before the image arrives.
  $effect(() => {
    if (!open || items.length < 2) return;
    for (const delta of [-1, 1]) {
      const image = new Image();
      image.src = items[stepIndex(index, items.length, delta)].url;
    }
  });

  function reset() { zoom = 1; x = 0; y = 0; drag = null; }
  $effect(() => { open; current?.url; reset(); });

  function clampPosition() {
    if (!viewport || !picture) return;
    const maxX = Math.max(0, (picture.clientWidth * zoom - viewport.clientWidth) / 2);
    const maxY = Math.max(0, (picture.clientHeight * zoom - viewport.clientHeight) / 2);
    x = Math.max(-maxX, Math.min(maxX, x));
    y = Math.max(-maxY, Math.min(maxY, y));
  }
  function setZoom(value: number) {
    zoom = Math.max(1, Math.min(4, value));
    clampPosition();
  }
  function pointerDown(event: PointerEvent) {
    if (event.button !== 0) return;
    event.currentTarget instanceof HTMLElement && event.currentTarget.setPointerCapture(event.pointerId);
    drag = { id: event.pointerId, x: event.clientX, y: event.clientY, startX: x, startY: y, moved: false };
  }
  function pointerMove(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.id) return;
    const dx = event.clientX - drag.x, dy = event.clientY - drag.y;
    if (Math.hypot(dx, dy) > 4) drag.moved = true;
    if (zoom > 1) { x = drag.startX + dx; y = drag.startY + dy; clampPosition(); }
  }
  function pointerUp(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.id) return;
    const clicked = !drag.moved;
    drag = null;
    if (clicked) setZoom(zoom === 1 ? 2 : 1);
  }

  function step(delta: number) {
    index = stepIndex(index, items.length, delta);
  }

  function onKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      open = false;
      return;
    }
    if (event.target instanceof HTMLInputElement) return;
    if (event.key === '+' || event.key === '=') { event.preventDefault(); setZoom(zoom + 0.25); }
    if (event.key === '-') { event.preventDefault(); setZoom(zoom - 0.25); }
    if (event.key === '0') reset();
    if (items.length < 2) return;
    if (event.key === 'ArrowRight') {
      event.preventDefault();
      step(1);
    } else if (event.key === 'ArrowLeft') {
      event.preventDefault();
      step(-1);
    }
  }
</script>

<svelte:window onkeydown={open ? onKeydown : undefined} onresize={clampPosition} />

{#if open && current}
  <div
    class="backdrop"
    role="presentation"
    onpointerdown={(e) => e.target === e.currentTarget && (open = false)}
  >
    <div class="frame" bind:this={viewport} role="dialog" aria-modal="true" aria-label={label}>
      {#if broken}
        <p class="broken">{msg('ui.imageUnavailable')}</p>
      {:else}
        <div class="image-control" role="button" tabindex="0" aria-label={msg('ui.imageZoomToggle')}
          onkeydown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setZoom(zoom === 1 ? 2 : 1); } }}
          onpointerdown={pointerDown} onpointermove={pointerMove} onpointerup={pointerUp}
          onpointercancel={() => (drag = null)} onlostpointercapture={() => (drag = null)}
          onwheel={(event) => { event.preventDefault(); setZoom(zoom - event.deltaY * 0.002); }}
          class:zoomed={zoom > 1} class:dragging={drag?.moved}
          style:transform={`translate(${x}px, ${y}px) scale(${zoom})`}>
          {#key current.url}
          <img transition:fade={{ duration: reducedMotion ? 0 : 180 }} bind:this={picture} src={current.url} alt="" draggable="false" onload={clampPosition}
            onerror={() => (failed = { ...failed, [current.id]: true })} />
          {/key}
        </div>
      {/if}
    </div>

    {#if items.length > 1}
      <div class="nav prev">
        <IconButton label={msg('ui.previous')} onclick={() => step(-1)}>
          <ChevronLeft size="2.4rem" strokeWidth={1.6} />
        </IconButton>
      </div>
      <div class="nav next">
        <IconButton label={msg('ui.next')} onclick={() => step(1)}>
          <ChevronRight size="2.4rem" strokeWidth={1.6} />
        </IconButton>
      </div>
      <span class="counter">{index + 1} / {items.length}</span>
    {/if}

    {#if !broken}
      <div class="zoom-controls">
        <IconButton label={msg('ui.imageZoomOut')} onclick={() => setZoom(zoom - 0.25)} disabled={zoom <= 1}><Minus size="1.8rem" /></IconButton>
        <input class="zoom-slider" style:--zoom-progress={`${((zoom - 1) / 3) * 100}%`} aria-valuetext={`${Math.round(zoom * 100)}%`} type="range" min="100" max="400" step="5" value={zoom * 100} aria-label={msg('ui.imageZoom')}
          oninput={(event) => setZoom(Number(event.currentTarget.value) / 100)} />
        <IconButton label={msg('ui.imageZoomIn')} onclick={() => setZoom(zoom + 0.25)} disabled={zoom >= 4}><Plus size="1.8rem" /></IconButton>
        <span class="zoom-value">{Math.round(zoom * 100)}%</span>
        <IconButton label={msg('ui.imageZoomReset')} onclick={reset}><RotateCcw size="1.7rem" /></IconButton>
      </div>
    {/if}
    <div class="close">
      <IconButton label={msg('common.close')} onclick={() => (open = false)}>
        <X size="2rem" strokeWidth={1.8} />
      </IconButton>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 110;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 6rem;
    background: rgba(4, 6, 10, 0.9);
    animation: fade var(--dur-panel) var(--ease);
  }

  .frame {
    width: 100%;
    height: 100%;
    overflow: hidden;
    display: flex;
    align-items: center;
    justify-content: center;
    pointer-events: none;
    border-radius: var(--radius-md);
  }

  .image-control { display: grid; place-items: center; pointer-events: auto; cursor: zoom-in; touch-action: none; line-height: 0; flex-shrink: 0; max-width: 100%; }
  .image-control.zoomed { cursor: grab; }
  .image-control.dragging { cursor: grabbing; }
  .image-control:focus-visible { outline: 2px solid var(--text-1); outline-offset: 3px; }
  .zoom-controls { position: absolute; bottom: 1.6rem; display: flex; align-items: center; gap: 0.6rem; padding: 0.4rem 0.8rem; border-radius: 1.2rem; background: #181b24; box-shadow: var(--shadow-modal); }
  .zoom-controls input { width: clamp(8rem, 18vw, 18rem); }
  .zoom-value { min-width: 4.5rem; text-align: center; font-size: var(--font-xs); font-variant-numeric: tabular-nums; }
  img {
    grid-area: 1 / 1;
    max-width: 100%;
    max-height: calc(100vh - 12rem);
    width: auto;
    height: auto;
    object-fit: contain;
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-modal);
    display: block;
    user-select: none;
  }

  .broken {
    padding: var(--space-8);
    color: var(--text-3);
    font-size: var(--font-sm);
  }

  .nav {
    position: absolute;
    top: 50%;
    transform: translateY(-50%);
  }

  .prev {
    left: 1.6rem;
  }

  .next {
    right: 1.6rem;
  }

  .close {
    position: absolute;
    top: 1.6rem;
    right: 1.6rem;
  }

  .counter {
    position: absolute;
    bottom: 2rem;
    left: 2rem;
    font-size: var(--font-xs);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  @keyframes fade {
    from {
      opacity: 0;
    }
  }

</style>
