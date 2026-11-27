<script lang="ts">
  import { onDestroy } from 'svelte';
  import { Minus, Plus } from '@lucide/svelte';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Slider from './Slider.svelte';
  import { msg } from '../i18n';
  import { focusHandOff } from '../utils/focusHandOff';
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

  /**
   * Crops the picked picture to the avatar's circle: drag or arrow keys move it, the wheel, the zoom
   * slider and its two buttons scale it. Save hands the picture and the crop to `onsave` once: while
   * the answer it returns is pending, or while the caller says `saving`, nothing in the dialog acts and
   * it cannot be dismissed. The caller says a failed upload as `error`, by its code; focus then goes to
   * those words. Go crops and encodes the picture: nothing is encoded here.
   */
  let {
    open = $bindable(false),
    src,
    saving = false,
    error = '',
    onsave,
    returnFocus,
  }: {
    open?: boolean;
    src: string;
    saving?: boolean;
    error?: string;
    onsave: (encoded: string, crop: CropRect) => unknown;
    /** Where focus goes on close when the control that opened the dialog is gone. */
    returnFocus?: () => HTMLElement | null | undefined;
  } = $props();
  const uid = $props.id();
  const hintId = `${uid}-hint`;
  const keysId = `${uid}-keys`;
  const errorId = `${uid}-error`;

  // Zoom positions on the kit's slider: 1× to 4× in steps of 5 %.
  const ZOOM_STEP = 0.05;
  const zoomSteps = Math.round((maxZoom - minZoom) / ZOOM_STEP);
  // How far an arrow key moves the picture, in pixels of the crop area.
  const NUDGE = 16;

  let image = $state<HTMLImageElement | undefined>(undefined);
  let stage = $state<HTMLElement | undefined>(undefined);
  let viewport = $state(0);
  let natural = $state({ width: 0, height: 0 });
  let zoom = $state(minZoom);
  let offset = $state({ x: 0, y: 0 });
  let dragging = $state(false);
  let dragFrom = { x: 0, y: 0 };
  let failed = $state(false);
  // Save was pressed and the answer `onsave` returned has not come.
  let sending = $state(false);
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  const scale = $derived(coverScale(viewport, natural.width, natural.height) * zoom);
  const displayed = $derived({ width: natural.width * scale, height: natural.height * scale });
  const ready = $derived(!failed && scale > 0);
  const busy = $derived(saving || sending);
  const percent = $derived(`${Math.round(zoom * 100)}%`);

  const running = $derived(busy ? msg('modals.actionRunning') : '');
  const notReady = $derived(failed ? msg('modals.avatarCropLoadFailed') : msg('common.loading'));
  const zoomOutReason = $derived(running || (!ready ? notReady : zoom <= minZoom ? msg('modals.avatarCropZoomAtMin') : ''));
  const zoomInReason = $derived(running || (!ready ? notReady : zoom >= maxZoom ? msg('modals.avatarCropZoomAtMax') : ''));

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
    if (!ready || busy) return;
    const zoomed = zoomAround(view(), next, anchorX, anchorY);
    zoom = zoomed.zoom;
    offset = { x: zoomed.offsetX, y: zoomed.offsetY };
  }

  function onPointerDown(e: PointerEvent) {
    if (!ready || busy) return;
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
    if (!ready || busy) return;
    e.preventDefault();
    const rect = stage?.getBoundingClientRect();
    const step = e.deltaY < 0 ? 1.12 : 1 / 1.12;
    applyZoom(zoom * step, rect ? e.clientX - rect.left : viewport / 2, rect ? e.clientY - rect.top : viewport / 2);
  }

  // The arrow moves the picture the way it points, as a drag would.
  const NUDGES: Record<string, [number, number]> = {
    ArrowLeft: [-NUDGE, 0],
    ArrowRight: [NUDGE, 0],
    ArrowUp: [0, -NUDGE],
    ArrowDown: [0, NUDGE],
  };

  function onKey(e: KeyboardEvent) {
    const nudge = NUDGES[e.key];
    if (!nudge) return;
    e.preventDefault();
    if (!ready || busy) return;
    offset = {
      x: clampOffset(offset.x + nudge[0], displayed.width, viewport),
      y: clampOffset(offset.y + nudge[1], displayed.height, viewport),
    };
  }

  async function save() {
    if (!image || !ready || busy) return;
    const rect = cropRect(view());
    const payload = src.split(',')[1] ?? '';
    if (rect.size <= 0 || !payload) return;
    // The focus Save holds at the press goes to the caller's words for a refusal.
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    sending = true;
    try {
      await onsave(payload, rect);
    } finally {
      if (alive) sending = false;
    }
    if (alive && open && error) await hand(() => document.getElementById(errorId));
  }
</script>

<Modal dismissible={!busy} busy={busy} bind:open title={msg('modals.avatarCropTitle')} width="42rem" {returnFocus}>
  <div class="crop">
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (The crop area takes focus so the arrow keys move the picture.) -->
    <div
      class="stage"
      class:dragging
      bind:this={stage}
      bind:clientWidth={viewport}
      role="group"
      aria-label={msg('modals.avatarCropArea')}
      aria-describedby={failed ? keysId : `${hintId} ${keysId}`}
      tabindex="0"
      onkeydown={onKey}
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
      <IconButton size="sm" label={msg('modals.avatarCropZoomOut')} disabled={!!zoomOutReason} reason={zoomOutReason} onclick={() => applyZoom(zoom - 0.25)}>
        <Minus size="1.6rem" strokeWidth={1.8} />
      </IconButton>
      <div class="slider">
        <Slider
          label={msg('modals.avatarCropZoomLevel')}
          hideLabel
          valueText={percent}
          position={(zoom - minZoom) / ZOOM_STEP}
          max={zoomSteps}
          page={5}
          disabled={!ready}
          {busy}
          fill
          oninput={(step) => {
            if (step !== null) applyZoom(clampZoom(minZoom + step * ZOOM_STEP));
          }}
          onchange={(step) => applyZoom(clampZoom(minZoom + step * ZOOM_STEP))}
        />
      </div>
      <IconButton size="sm" label={msg('modals.avatarCropZoomIn')} disabled={!!zoomInReason} reason={zoomInReason} onclick={() => applyZoom(zoom + 0.25)}>
        <Plus size="1.6rem" strokeWidth={1.8} />
      </IconButton>
      <span class="zoom-value" aria-hidden="true">{percent}</span>
    </div>

    {#if failed}
      <div class="failed"><Notice tone="danger" compact>{msg('modals.avatarCropLoadFailed')}</Notice></div>
    {:else}
      <span class="hint" id={hintId}>{msg('modals.avatarCropHint')}</span>
    {/if}
    <span class="sr-only" id={keysId}>{msg('modals.avatarCropKeysHint')}</span>
  </div>

  {#snippet footer()}
    {#if error}<div class="foot-error"><Notice tone="danger" compact id={errorId} tabindex={-1}>{error}</Notice></div>{/if}
    <Button variant="ghost" disabled={busy} reason={running} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={!ready && !busy} reason={!ready && !busy ? notReady : ''} {busy} onclick={save}>
      {busy ? msg('modals.avatarCropSaving') : msg('common.save')}
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

  .stage:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: 3px;
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
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface-2);
  }

  .slider {
    flex: 1;
    min-width: 0;
  }

  .zoom-value { min-width: 4.2rem; color: var(--text-2); font-size: var(--font-xs); font-variant-numeric: tabular-nums; text-align: center; }

  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
    text-align: center;
  }

  .failed {
    align-self: stretch;
  }

  .foot-error {
    flex: 1;
    min-width: 0;
    margin-right: auto;
  }
</style>
