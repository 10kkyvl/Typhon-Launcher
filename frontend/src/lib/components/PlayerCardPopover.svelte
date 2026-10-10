<script lang="ts">
  import { onDestroy } from 'svelte';
  import PlayerCard from './PlayerCard.svelte';
  import { createHoverIntent, placePopover } from './playerCardPopover';
  import type { PresenceView, UserCard } from '../services/social';

  let { anchor, user, presence }: { anchor?: HTMLElement | null; user: UserCard; presence?: PresenceView | null } = $props();

  const id = `player-card-${Math.random().toString(36).slice(2, 10)}`;
  let open = $state(false);
  let place = $state<{ left: number; top: number } | null>(null);
  let popover: HTMLDivElement | undefined = $state();

  const intent = createHoverIntent((next) => {
    open = next;
    if (!next) place = null;
  });
  onDestroy(() => intent.destroy());

  function portal(node: HTMLDivElement) {
    document.body.appendChild(node);
    return { destroy: () => node.remove() };
  }

  $effect(() => {
    const el = anchor;
    if (!el) return;
    const enter = (event: PointerEvent) => { if (event.pointerType !== 'touch') intent.hover(); };
    const leave = () => intent.unhover();
    const focusin = (event: FocusEvent) => {
      if (event.target instanceof HTMLElement && event.target.matches(':focus-visible')) intent.focus();
    };
    const focusout = (event: FocusEvent) => {
      if (event.relatedTarget instanceof Node && el.contains(event.relatedTarget)) return;
      intent.blur();
    };
    el.addEventListener('pointerenter', enter);
    el.addEventListener('pointerleave', leave);
    el.addEventListener('focusin', focusin);
    el.addEventListener('focusout', focusout);
    return () => {
      el.removeEventListener('pointerenter', enter);
      el.removeEventListener('pointerleave', leave);
      el.removeEventListener('focusin', focusin);
      el.removeEventListener('focusout', focusout);
      intent.unhover();
      intent.blur();
    };
  });

  $effect(() => {
    if (!open) return;
    const close = () => intent.dismiss();
    const key = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.stopPropagation();
      intent.escape();
    };
    window.addEventListener('keydown', key, true);
    window.addEventListener('scroll', close, true);
    window.addEventListener('blur', close);
    return () => {
      window.removeEventListener('keydown', key, true);
      window.removeEventListener('scroll', close, true);
      window.removeEventListener('blur', close);
    };
  });

  $effect(() => {
    const el = anchor;
    if (!open || !el || !popover) return;
    const a = el.getBoundingClientRect();
    place = placePopover(
      { left: a.left, top: a.top, width: a.width, height: a.height },
      { width: popover.offsetWidth, height: popover.offsetHeight },
      { width: window.innerWidth, height: window.innerHeight },
    );
  });

  $effect(() => {
    const el = anchor;
    if (!el || !open) return;
    el.setAttribute('aria-describedby', id);
    return () => el.removeAttribute('aria-describedby');
  });
</script>

{#if open}
  <div {id} class="popover" role="tooltip" use:portal bind:this={popover}
    style:left={place ? `${place.left}px` : '0'} style:top={place ? `${place.top}px` : '0'} style:visibility={place ? 'visible' : 'hidden'}>
    <PlayerCard {user} {presence} />
  </div>
{/if}

<style>
  .popover {
    position: fixed;
    z-index: 80;
    pointer-events: none;
    box-shadow: var(--shadow-pop);
    border-radius: var(--radius-lg);
    animation: pop-in var(--dur) var(--ease);
  }
</style>
