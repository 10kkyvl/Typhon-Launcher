<script lang="ts">
  import '../../styles/zoom-slider.css';
  import { Minus, Plus } from '@lucide/svelte';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import {
    centerOffset,
    clampOffset,
    clampZoom,
    coverScale,
    cropRect,
    maxZoom,
    minZoom,
    zoomAround,
    type CropRect,
    type CropView,
  } from '../utils/crop';

  let {
    open = $bindable(false),
    src,
    saving = false,
    error = '',
    onsave,
  }: {
    open?: boolean;
    src: string;
    saving?: boolean;
    error?: string;
    onsave: (encoded: string, crop: CropRect) => void;
  } = $props();

  let image = $state<HTMLImageElement | undefined>(undefined);
  let stage = $state<HTMLElement | undefined>(undefined);
  let viewport = $state(0);
  let natural = $state({ width: 0, height: 0 });
  let zoom = $state(minZoom);
  let offset = $state({ x: 0, y: 0 });
  let dragging = $state(false);
  let dragFrom = { x: 0, y: 0 };
  let failed = $state(false);

  const scale = $derived(coverScale(viewport, natural.width, natural.height) * zoom);
  const displayed = $derived({ width: natural.width * scale, height: natural.height * scale });
  const ready = $derived(!failed && scale > 0);

  $effect(() => {
    src;
    natural = { width: 0, height: 0 };
    zoom = minZoom;
    offset = { x: 0, y: 0 };
    failed = false;
  });

  $effect(() => {
    if (!ready) return;
    const x = clampOffset(offset.x, displayed.width, viewport);
    const y = clampOffset(offset.y, displayed.height, viewport);
    if (x !== offset.x || y !== offset.y) offset = { x, y };
  });

  function view(): CropView {
    return { viewport, width: natural.width, height: natural.height, zoom, offsetX: offset.x, offsetY: offset.y };
  }

  function onLoad() {
    if (!image) return;
    if (viewport <= 0) viewport = stage?.clientWidth ?? 0;
    natural = { width: image.naturalWidth, height: image.naturalHeight };
    zoom = minZoom;
    const base = coverScale(viewport, natural.width, natural.height);
    offset = {
      x: centerOffset(natural.width * base, viewport),
      y: centerOffset(natural.height * base, viewport),
    };
  }

  function applyZoom(next: number, anchorX = viewport / 2, anchorY = viewport / 2) {
    if (!ready) return;
    const zoomed = zoomAround(view(), next, anchorX, anchorY);
    zoom = zoomed.zoom;
    offset = { x: zoomed.offsetX, y: zoomed.offsetY };
  }

  function onPointerDown(e: PointerEvent) {
    if (!ready || saving) return;
    dragging = true;
    dragFrom = { x: e.clientX - offset.x, y: e.clientY - offset.y };
    (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  }

  function onPointerMove(e: PointerEvent) {
    if (!dragging) return;
    offset = {
      x: clampOffset(e.clientX - dragFrom.x, displayed.width, viewport),
      y: clampOffset(e.clientY - dragFrom.y, displayed.height, viewport),
    };
  }

  function onPointerUp(e: PointerEvent) {
    if (!dragging) return;
    dragging = false;
    (e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
  }

  function onWheel(e: WheelEvent) {
    if (!ready || saving) return;
    e.preventDefault();
    const rect = stage?.getBoundingClientRect();
    const step = e.deltaY < 0 ? 1.12 : 1 / 1.12;
    applyZoom(zoom * step, rect ? e.clientX - rect.left : viewport / 2, rect ? e.clientY - rect.top : viewport / 2);
  }

  function save() {
    if (!image || !ready || saving) return;
    const rect = cropRect(view());
    const payload = src.split(',')[1] ?? '';
    if (rect.size <= 0 || !payload) return;
    onsave(payload, rect);
  }
</script>

<Modal bind:open title={msg('modals.avatarCropTitle')} width="42rem">
  <div class="crop">
    <div
      class="stage"
      class:dragging
      bind:this={stage}
      bind:clientWidth={viewport}
      role="presentation"
      onpointerdown={onPointerDown}
      onpointermove={onPointerMove}
      onpointerup={onPointerUp}
      onpointercancel={onPointerUp}
      onwheel={onWheel}
    >
      <img
        bind:this={image}
        {src}
        alt=""
        draggable="false"
        onload={onLoad}
        onerror={() => (failed = true)}
        style:width="{displayed.width}px"
        style:height="{displayed.height}px"
        style:left="{offset.x}px"
        style:top="{offset.y}px"
        style:visibility={ready ? 'visible' : 'hidden'}
      />
      <span class="mask"></span>
    </div>

    <div class="zoom">
      <IconButton size="sm" label={msg('modals.avatarCropZoomOut')} disabled={!ready || zoom <= minZoom} onclick={() => applyZoom(zoom - 0.25)}>
        <Minus size="1.6rem" strokeWidth={1.8} />
      </IconButton>
      <input
        class="slider zoom-slider"
        style:--zoom-progress={`${((zoom - minZoom) / (maxZoom - minZoom)) * 100}%`}
        aria-valuetext={`${Math.round(zoom * 100)}%`}
        type="range"
        min={minZoom}
        max={maxZoom}
        step="0.01"
        value={zoom}
        disabled={!ready}
        aria-label={msg('modals.avatarCropZoomLevel')}
        oninput={(e) => applyZoom(clampZoom(Number(e.currentTarget.value)))}
      />
      <IconButton size="sm" label={msg('modals.avatarCropZoomIn')} disabled={!ready || zoom >= maxZoom} onclick={() => applyZoom(zoom + 0.25)}>
        <Plus size="1.6rem" strokeWidth={1.8} />
      </IconButton>
      <span class="zoom-value">{Math.round(zoom * 100)}%</span>
    </div>

    {#if failed}
      <span class="error">{msg('modals.avatarCropLoadFailed')}</span>
    {:else}
      <span class="hint">{msg('modals.avatarCropHint')}</span>
    {/if}
  </div>

  {#snippet footer()}
    {#if error}<span class="error foot-error">{error}</span>{/if}
    <Button variant="ghost" disabled={saving} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={!ready || saving} onclick={save}>
      {saving ? msg('modals.avatarCropSaving') : msg('common.save')}
    </Button>
  {/snippet}
</Modal>

<style>
  .crop {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    align-items: center;
  }

  .stage {
    position: relative;
    width: 100%;
    aspect-ratio: 1;
    overflow: hidden;
    border-radius: var(--radius-lg);
    background: var(--surface-3);
    cursor: grab;
    touch-action: none;
    user-select: none;
  }

  .stage.dragging {
    cursor: grabbing;
  }

  .stage img {
    position: absolute;
    max-width: none;
    pointer-events: none;
  }

  .mask {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    box-shadow: 0 0 0 100rem rgba(4, 6, 10, 0.62);
    border: 1px solid rgba(255, 255, 255, 0.55);
    pointer-events: none;
  }

  .zoom {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.4rem 0.8rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
  }

  .slider {
    flex: 1;
    min-width: 0;
    cursor: pointer;
  }

  .slider:disabled {
    cursor: default;
    opacity: 0.5;
  }

  .zoom-value { min-width: 4.2rem; color: var(--text-2); font-size: var(--font-xs); font-variant-numeric: tabular-nums; text-align: center; }

  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
    text-align: center;
  }

  .error {
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .foot-error {
    margin-right: auto;
  }
</style>
