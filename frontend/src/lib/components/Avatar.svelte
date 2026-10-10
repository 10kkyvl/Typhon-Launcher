<script lang="ts">
  import { untrack } from 'svelte';
  import type { PresenceDot } from '../social/presence';
  import type { AvatarFrame } from '../services/account';

  let {
    src,
    name,
    size = 'md',
    status,
    frame = 'none',
    frameColor,
  }: {
    src?: string;
    name: string;
    size?: 'sm' | 'md' | 'lg';
    status?: PresenceDot;
    frame?: AvatarFrame;
    frameColor?: string;
  } = $props();

  let failed = $state(false);

  $effect(() => {
    src;
    failed = false;
  });

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

<span class="avatar {size}" class:framed={frame !== 'none'} style:--frame-color={frameColor}>
  {#if frame !== 'none'}
    <span class="frame frame-{frame}" aria-hidden="true"></span>
  {/if}
  {#if !src || failed}
    <span class="fallback">{initial}</span>
  {:else}
    <img {src} alt="" draggable="false" onerror={() => (failed = true)} />
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

  .avatar.sm {
    --frame-w: 0.2rem;
    width: 3.2rem;
    height: 3.2rem;
  }

  .avatar.md {
    --frame-w: 0.3rem;
    width: 4.8rem;
    height: 4.8rem;
  }

  .avatar.lg {
    --frame-w: 0.4rem;
    width: 9.6rem;
    height: 9.6rem;
  }

  img {
    width: 100%;
    height: 100%;
    border-radius: 50%;
    object-fit: cover;
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
