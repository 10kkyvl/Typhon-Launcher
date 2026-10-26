<script lang="ts">
  import { untrack } from 'svelte';
  import type { PresenceDot } from '../social/presence';
  import type { AvatarFrame, FrameImage } from '../services/account';
  import { MAX_FRAME_BLEED, frameBleed, frameScale, frameShift, isCustomFrame, presetFrame } from '../profile/frameLook';

  let {
    src,
    name,
    size = 'md',
    status,
    frame = 'none',
    frameColor,
    frameImage,
  }: {
    src?: string;
    name: string;
    size?: 'sm' | 'md' | 'lg';
    status?: PresenceDot;
    frame?: AvatarFrame;
    frameColor?: string;
    frameImage?: FrameImage;
  } = $props();

  let failed = $state(false);
  let frameFailed = $state(false);
  let ratio = $state(1);

  $effect(() => {
    src;
    failed = false;
  });

  $effect(() => {
    frameImage?.url;
    frameFailed = false;
    ratio = 1;
  });

  const preset = $derived(presetFrame(frame));
  const layer = $derived(isCustomFrame(frame) && frameImage && !frameFailed ? frameImage : null);

  function onFrameLoad(event: Event) {
    const img = event.currentTarget as HTMLImageElement;
    if (img.naturalWidth > 0 && img.naturalHeight > 0) ratio = img.naturalHeight / img.naturalWidth;
  }

  const initial = $derived(name.trim().slice(0, 1).toUpperCase() || '?');

  let lastStatus = untrack(() => status);
  let changed = $state(false);

  $effect(() => {
    if (status === lastStatus) return;
    const known = lastStatus !== undefined && status !== undefined;
    lastStatus = status;
    if (known) changed = true;
  });
</script>

<span class="avatar {size}" class:framed={preset !== 'none' || !!layer} class:art={!!layer} style:--frame-color={frameColor || undefined} style:--frame-bleed={layer ? MAX_FRAME_BLEED : undefined} style:--bleed-l={layer ? frameBleed(layer.hole, layer.holeX ?? 0.5) : undefined} style:--bleed-r={layer ? frameBleed(layer.hole, 1 - (layer.holeX ?? 0.5)) : undefined} style:--bleed-t={layer ? frameBleed(layer.hole, layer.holeY ?? 0.5, ratio) : undefined} style:--bleed-b={layer ? frameBleed(layer.hole, 1 - (layer.holeY ?? 0.5), ratio) : undefined}>
  {#if preset !== 'none'}
    <span class="frame frame-{preset}" aria-hidden="true"></span>
  {/if}
  {#if !src || failed}
    <span class="fallback">{initial}</span>
  {:else}
    <img {src} alt="" draggable="false" onerror={() => (failed = true)} />
  {/if}
  {#if layer}
    <span class="frame-clip" aria-hidden="true"><img class="frame-art" src={layer.url} alt="" aria-hidden="true" decoding="async" loading="lazy" draggable="false" style:width="calc(var(--size) * {frameScale(layer.hole) / 100})" style:transform="translate({frameShift(layer.holeX ?? 0.5)}, {frameShift(layer.holeY ?? 0.5)})" onload={onFrameLoad} onerror={() => (frameFailed = true)} /></span>
  {/if}
  {#if status}
    <span class="dot {status}" class:changed onanimationend={() => (changed = false)}></span>
  {/if}
</span>

<style>
  .avatar {
    position: relative;
    flex-shrink: 0;
    display: block;
  }

  .avatar.framed {
    isolation: isolate;
  }

  .avatar.art {
    margin: calc(var(--size) * var(--bleed-t, 0)) calc(var(--size) * var(--bleed-r, 0)) calc(var(--size) * var(--bleed-b, 0)) calc(var(--size) * var(--bleed-l, 0));
  }

  .frame-clip {
    position: absolute;
    inset: calc(var(--frame-bleed) * -100%);
    overflow: clip;
    pointer-events: none;
    z-index: 1;
  }

  .avatar.sm {
    --frame-w: 0.2rem;
    --size: 3.2rem;
    width: var(--size);
    height: var(--size);
  }

  .avatar.md {
    --frame-w: 0.3rem;
    --size: 4.8rem;
    width: var(--size);
    height: var(--size);
  }

  .avatar.lg {
    --frame-w: 0.4rem;
    --size: 9.6rem;
    width: var(--size);
    height: var(--size);
  }

  img {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    object-fit: cover;
  }

  img.frame-art {
    position: absolute;
    top: 50%;
    left: 50%;
    height: auto;
    max-width: none;
    border-radius: 0;
    object-fit: contain;
    transform: translate(-50%, -50%);
    pointer-events: none;
  }

  .fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    border-radius: 50%;
    background: var(--surface-3);
    color: var(--text-2);
    font-weight: 600;
  }

  .sm .fallback {
    font-size: var(--font-sm);
  }

  .md .fallback {
    font-size: var(--font-lg);
  }

  .lg .fallback {
    font-size: 3.6rem;
  }

  .dot {
    position: absolute;
    right: -1px;
    bottom: -1px;
    border-radius: 50%;
    border: 2px solid var(--avatar-ring, var(--bg));
  }

  .sm .dot {
    width: 0.9rem;
    height: 0.9rem;
  }

  .md .dot {
    width: 1.2rem;
    height: 1.2rem;
  }

  .lg .dot {
    width: 2rem;
    height: 2rem;
    border-width: 3px;
  }

  .framed .dot {
    z-index: 2;
  }

  .dot.changed {
    animation: pop-in var(--dur-slow) var(--ease-spring);
  }

  .dot.online {
    background: var(--success);
  }

  .dot.away {
    background: var(--warning);
  }

  .dot.busy {
    background: var(--danger);
  }

  .dot.offline {
    background: var(--text-3);
  }

  .frame {
    --fc: var(--frame-color, var(--accent));
    position: absolute;
    inset: calc(var(--frame-w) * -1.5);
    border-radius: 50%;
    pointer-events: none;
    z-index: 1;
  }

  .frame-ring {
    border: var(--frame-w) solid var(--fc);
  }

  .frame-glow {
    inset: 0;
    box-shadow:
      0 0 0 var(--frame-w) var(--fc),
      0 0 calc(var(--frame-w) * 5) var(--fc);
  }

  .frame-neon {
    border: var(--frame-w) solid var(--fc);
    box-shadow:
      0 0 calc(var(--frame-w) * 2) var(--fc),
      inset 0 0 calc(var(--frame-w) * 2) var(--fc);
    animation: frame-pulse 2.4s ease-in-out infinite;
  }

  .frame-orbit {
    border: calc(var(--frame-w) / 2) solid color-mix(in srgb, var(--fc) 45%, transparent);
    animation: frame-spin 5s linear infinite;
  }

  .frame-orbit::after {
    content: '';
    position: absolute;
    top: calc(var(--frame-w) * -1.25);
    left: 50%;
    width: calc(var(--frame-w) * 2);
    height: calc(var(--frame-w) * 2);
    margin-left: calc(var(--frame-w) * -1);
    border-radius: 50%;
    background: var(--fc);
    box-shadow: 0 0 calc(var(--frame-w) * 2) var(--fc);
  }

  .frame-pixel {
    inset: calc(var(--frame-w) * -1);
    z-index: -1;
    border-radius: 0;
    background: var(--fc);
    clip-path: polygon(
      0 30%, 10% 30%, 10% 15%, 15% 15%, 15% 10%, 30% 10%, 30% 0,
      70% 0, 70% 10%, 85% 10%, 85% 15%, 90% 15%, 90% 30%, 100% 30%,
      100% 70%, 90% 70%, 90% 85%, 85% 85%, 85% 90%, 70% 90%, 70% 100%,
      30% 100%, 30% 90%, 15% 90%, 15% 85%, 10% 85%, 10% 70%, 0 70%
    );
  }

  @keyframes frame-pulse {
    50% {
      opacity: 0.55;
    }
  }

  @keyframes frame-spin {
    to {
      transform: rotate(360deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .frame-neon,
    .frame-orbit {
      animation: none;
    }
  }
</style>
