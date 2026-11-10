<script lang="ts">
  import '../../styles/zoom-slider.css';
  import { onDestroy } from 'svelte';
  import { Minus, Plus } from '@lucide/svelte';
  import Button from './Button.svelte';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import { minZoom, maxZoom } from '../utils/crop';
  import { bannerScale, clampBanner, zoomBanner, bannerCrop, type BannerView } from '../utils/bannerCrop';
  let { open = $bindable(false), src, onsave }: { open?: boolean; src: string; onsave: (src: string) => void } = $props();
  let stage: HTMLDivElement;
  let image: HTMLImageElement;
  let viewport = $state(0);
  let natural = $state({ width: 0, height: 0 });
  let zoom = $state(1);
  let offset = $state({ x: 0, y: 0 });
  let failed = $state(false), saving = $state(false), error = $state(''), dragging = $state(false);
  let alive = true, previousViewport = 0;
  let dragFrom = { x: 0, y: 0 };
  onDestroy(() => { alive = false; });
  const view = $derived<BannerView>({ viewport, ...natural, zoom, offsetX: offset.x, offsetY: offset.y });
  const scale = $derived(bannerScale(view));
  const ready = $derived(scale > 0 && !failed);
  $effect(() => { src; natural = { width: 0, height: 0 }; zoom = 1; offset = { x: 0, y: 0 }; failed = false; error = ''; });
  $effect(() => {
    const width = viewport;
    if (previousViewport > 0 && width > 0 && previousViewport !== width) {
      const ratio = width / previousViewport;
      offset = { x: offset.x * ratio, y: offset.y * ratio };
    }
    previousViewport = width;
  });
  function apply(v: BannerView) { zoom = v.zoom; offset = { x: v.offsetX, y: v.offsetY }; }
  function loaded() {
    viewport = stage.clientWidth;
    natural = { width: image.naturalWidth, height: image.naturalHeight };
    const base = bannerScale({ ...view, ...natural, zoom: 1, viewport });
    zoom = 1; offset = { x: (viewport - natural.width * base) / 2, y: (viewport / 4 - natural.height * base) / 2 };
  }
  function changeZoom(next: number, x?: number, y?: number) { if (ready && !saving) apply(zoomBanner(view, next, x, y)); }
  function down(e: PointerEvent) {
    if (!ready || saving || e.button !== 0) return;
    dragging = true; dragFrom = { x: e.clientX - offset.x, y: e.clientY - offset.y };
    stage.setPointerCapture(e.pointerId); stage.focus();
  }
  function move(e: PointerEvent) { if (dragging) apply(clampBanner({ ...view, offsetX: e.clientX - dragFrom.x, offsetY: e.clientY - dragFrom.y })); }
  function up(e: PointerEvent) { dragging = false; if (stage.hasPointerCapture(e.pointerId)) stage.releasePointerCapture(e.pointerId); }
  function wheel(e: WheelEvent) { e.preventDefault(); const r = stage.getBoundingClientRect(); changeZoom(zoom * (e.deltaY < 0 ? 1.12 : 1 / 1.12), e.clientX - r.left, e.clientY - r.top); }
  function key(e: KeyboardEvent) {
    if (!ready || saving) return;
    const directions: Record<string, [number, number]> = { ArrowLeft: [16, 0], ArrowRight: [-16, 0], ArrowUp: [0, 16], ArrowDown: [0, -16] };
    const delta = directions[e.key];
    if (delta) { e.preventDefault(); apply(clampBanner({ ...view, offsetX: offset.x + delta[0], offsetY: offset.y + delta[1] })); }
  }
  async function save() {
    if (!ready || saving) return;
    saving = true; error = '';
    try {
      const rect = bannerCrop(view), canvas = document.createElement('canvas');
      canvas.width = rect.outputWidth; canvas.height = rect.outputHeight;
      const ctx = canvas.getContext('2d');
      if (!ctx) throw new Error('canvas');
      ctx.drawImage(image, rect.x, rect.y, rect.width, rect.height, 0, 0, canvas.width, canvas.height);
      const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob((b) => b ? resolve(b) : reject(new Error('encode')), 'image/webp', .92));
      if (blob.size > 8 * 1024 * 1024) throw new Error('size');
      const encoded = await new Promise<string>((resolve, reject) => { const r = new FileReader(); r.onload = () => resolve(String(r.result)); r.onerror = reject; r.readAsDataURL(blob); });
      if (alive && open) { onsave(encoded); open = false; }
    } catch { if (alive) error = msg('profile.coverDecode'); }
    finally { if (alive) saving = false; }
  }
</script>
<Modal bind:open title={msg('profile.cropTitle')} width="72rem">
  <div class="crop">
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (Two-dimensional crop canvas supports focus and arrow-key panning.) -->
    <div class="stage" class:dragging bind:this={stage} bind:clientWidth={viewport} role="group" aria-label={msg('profile.cropTitle')} tabindex="0" onkeydown={key} onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up} onwheel={wheel}>
      <img bind:this={image} {src} alt="" draggable="false" onload={loaded} onerror={() => (failed = true)} style:width={`${natural.width * scale}px`} style:height={`${natural.height * scale}px`} style:left={`${offset.x}px`} style:top={`${offset.y}px`} style:visibility={ready ? 'visible' : 'hidden'} />
      <span class="frame"></span>
    </div>
    <div class="zoom">
      <IconButton size="sm" label={msg('modals.avatarCropZoomOut')} disabled={!ready || saving || zoom <= minZoom} onclick={() => changeZoom(zoom - .25)}><Minus size="1.6rem" /></IconButton>
      <input class="zoom-slider" type="range" min={minZoom} max={maxZoom} step=".01" value={zoom} style:--zoom-progress={`${(zoom - minZoom) / (maxZoom - minZoom) * 100}%`} aria-label={msg('modals.avatarCropZoomLevel')} aria-valuetext={`${Math.round(zoom * 100)}%`} disabled={!ready || saving} oninput={(e) => changeZoom(Number(e.currentTarget.value))} />
      <IconButton size="sm" label={msg('modals.avatarCropZoomIn')} disabled={!ready || saving || zoom >= maxZoom} onclick={() => changeZoom(zoom + .25)}><Plus size="1.6rem" /></IconButton>
      <span>{Math.round(zoom * 100)}%</span>
    </div>
    <p class:error={failed}>{failed ? msg('profile.coverDecode') : msg('profile.cropHint')}</p>
  </div>
  {#snippet footer()}
    {#if error}<p class="error" role="alert">{error}</p>{/if}
    <Button variant="ghost" disabled={saving} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={!ready || saving} onclick={save}>{saving ? msg('social.saving') : msg('profile.applyCrop')}</Button>
  {/snippet}
</Modal>
<style>
  .crop { display: flex; flex-direction: column; gap: var(--space-4); }
  .stage { position: relative; width: 100%; aspect-ratio: 4 / 1; overflow: hidden; border-radius: var(--radius-md); background: var(--surface-3); touch-action: none; user-select: none; cursor: grab; }
  .stage.dragging { cursor: grabbing; }
  .stage:focus-visible { outline: 2px solid var(--accent-ring); outline-offset: 3px; }
  img { position: absolute; max-width: none; pointer-events: none; }
  .frame { position: absolute; inset: 0; border: 1px solid rgba(255,255,255,.6); border-radius: inherit; pointer-events: none; }
  .zoom { display: flex; align-items: center; gap: var(--space-3); padding: .4rem .8rem; border: 1px solid var(--border); border-radius: var(--radius-lg); background: var(--surface-2); }
  .zoom-slider { flex: 1; min-width: 0; } .zoom > span { min-width: 4.2rem; font-variant-numeric: tabular-nums; text-align: center; }
  p, .zoom > span { font-size: var(--font-xs); color: var(--text-2); } .error { color: var(--danger); }
</style>
