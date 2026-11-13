<script lang="ts">
  import { CircleAlert, CircleCheck, Gamepad2, Info, TriangleAlert, type LucideIcon } from '@lucide/svelte';
  import { tick, untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { on } from 'svelte/events';
  import { get } from 'svelte/store';
  import { msg } from '../i18n';
  import { modalOpen, topModal } from '../layers';
  import { SpringValue } from '../motion/spring';
  import {
    claimLiveIsland,
    liveIsland,
    liveIslandCurrent,
    liveIslandMessages,
    liveIslandUrgent,
    type LiveIslandAction,
    type LiveIslandIcon,
    type LiveIslandMessage,
  } from '../stores/liveIsland';
  import { initLiveIslandFeed } from '../stores/liveIslandFeed';
  import Avatar from './Avatar.svelte';
  import Button from './Button.svelte';
  import CountBadge from './CountBadge.svelte';
  import { giveFocusBack } from './focusBack';
  import { createHoverIntent } from './hoverIntent';

  /**
   * The live island: one pill at the top centre, in the title bar's band, that says what just
   * happened. Mounted by AppShell while motion is on; with motion off the toast stack says it instead.
   *
   * It is hidden, compact (the message's mark and line) or open (a card with the whole message, up to
   * two actions and dismissal), one shape whose width and height follow a spring. It opens on a press,
   * on Enter or Space (focus then goes into the card), or when the pointer rests on it; Escape with
   * focus inside (focus then returns to the pill), a press outside or the pointer leaving (when it
   * opened by resting) close it. While a dialog is open, Escape is the dialog's. While it is
   * open, hovered or holds focus, the messages' clocks stand still.
   *
   * It sits after the page in the Tab order, where the toast stack is: a message appearing never puts
   * itself ahead of the window's controls and never takes focus. A message that goes while focus is in
   * the island gives focus back where it was before it came in (focusBack.ts, the toast stack's rule).
   * Its own two live regions say each message once, when it comes: conflicts and errors at once, the
   * rest politely.
   */

  const ICONS: Record<LiveIslandIcon, LucideIcon> = {
    info: Info,
    success: CircleCheck,
    warning: TriangleAlert,
    danger: CircleAlert,
    game: Gamepad2,
  };

  let shell = $state<HTMLElement>();
  let head = $state<HTMLButtonElement>();
  let card = $state<HTMLElement>();
  let top = $state<HTMLButtonElement>();

  let pinned = $state(false);
  let hoverOpen = $state(false);
  let pointerOver = $state(false);
  let focusInside = $state(false);
  let origin: HTMLElement | undefined;
  // Kept while the island fades out, so the words do not vanish before the shape does.
  let primary = $state<LiveIslandMessage>();
  let width = $state(0);
  let height = $state(0);

  const uid = $props.id();
  const collapseId = `${uid}-collapse`;

  const live = $derived($liveIslandCurrent);
  const shown = $derived(live !== undefined);
  const expanded = $derived(shown && (pinned || hoverOpen));
  const phase = $derived(!shown ? 'hidden' : expanded ? 'expanded' : 'compact');
  // What waits behind the message on screen.
  const waiting = $derived(shown ? $liveIslandMessages.length - 1 : 0);
  const tone = $derived(primary?.tone ?? 'accent');
  const urgent = $derived($liveIslandMessages.filter((m) => liveIslandUrgent(m)));
  const polite = $derived($liveIslandMessages.filter((m) => !liveIslandUrgent(m)));
  // Escape inside the open island is its own, unless a dialog is open: then it is the dialog's.
  const ownEscape = $derived(expanded && !$topModal);

  // As an attribute, so `[inert]` finds it wherever it is asked (jsdom has no `inert` property to reflect).
  const inert =
    (on: boolean): Attachment<HTMLElement> =>
    (node) => {
      node.toggleAttribute('inert', on);
    };

  const intent = createHoverIntent((open) => (hoverOpen = open));
  const w = new SpringValue(0, (v) => (width = v), 'bouncy');
  const h = new SpringValue(0, (v) => (height = v), 'bouncy');
  let measured = false;

  $effect(() => {
    const release = claimLiveIsland();
    const stopFeed = initLiveIslandFeed();
    return () => {
      stopFeed();
      release();
      intent.destroy();
      w.stop();
      h.stop();
      liveIsland.hold(false);
      liveIsland.dialog(false);
    };
  });

  $effect(() => liveIsland.hold(expanded || pointerOver || focusInside));
  $effect(() => liveIsland.dialog($modalOpen));

  function said(m: LiveIslandMessage): string {
    const words = [m.text, m.title, m.detail].filter(Boolean).join('. ');
    return m.count ? `${words}, ${msg('ui.toastRepeated', { count: m.count })}` : words;
  }

  // Focus that fell with the message: gone, on <body>, in a card that closed or in an island that hid.
  function fallen(): boolean {
    const active = document.activeElement;
    if (!active || active === document.body || !active.isConnected) return true;
    if (!live) return shell?.contains(active) ?? false;
    return !expanded && (card?.contains(active) ?? false);
  }

  // Focus on a control of the island that can be used now: not in a part made inert (a hidden island,
  // a closed card).
  function focusedInside(): boolean {
    const active = document.activeElement;
    return Boolean(active && shell?.contains(active) && !active.closest('[inert]'));
  }

  // Another message, or none: a new one comes up compact, and focus that was inside goes back. Where
  // focus came from belongs to the message that goes. Whether focus is still inside is asked of the
  // document, not learnt from a focusout: a webview may send none when a part turns inert or hidden,
  // and a focus taken for inside would hold every later message's clock.
  $effect.pre(() => {
    const next = live;
    untrack(() => {
      const before = primary?.id;
      if (next) primary = next;
      if (before === next?.id) return;
      const held = shell?.contains(document.activeElement) ?? false;
      const from = origin;
      origin = undefined;
      pinned = false;
      intent.dismiss();
      if (!next) measured = false;
      if (!held) return;
      void tick().then(() => {
        if (fallen()) giveFocusBack(from);
        focusInside = focusedInside();
      });
    });
  });

  function target(): { w: number; h: number } | null {
    if (!head || !card) return null;
    const box = expanded ? card : head;
    return box.offsetWidth ? { w: box.offsetWidth, h: box.offsetHeight } : null;
  }

  function retarget() {
    const next = target();
    if (!next) return;
    const instant = !measured;
    measured = true;
    w.set(next.w, { instant });
    h.set(next.h, { instant });
  }

  $effect(() => {
    void expanded;
    void primary;
    void tick().then(retarget);
  });

  $effect(() => {
    if (!card || !head || typeof ResizeObserver !== 'function') return;
    const sizes = new ResizeObserver(() => retarget());
    sizes.observe(card);
    sizes.observe(head);
    return () => sizes.disconnect();
  });

  function open() {
    const fromHead = document.activeElement === head;
    pinned = true;
    // The pill is inert while the card is open: focus it held moves to the card's first control.
    if (fromHead) void tick().then(() => top?.focus());
  }

  function collapse(returnFocus: boolean) {
    pinned = false;
    if (hoverOpen) intent.escape();
    if (returnFocus) void tick().then(() => head?.focus());
  }

  function close(m: LiveIslandMessage) {
    if (m.close) m.close();
    else liveIsland.hide(m.id);
  }

  function closeAll() {
    for (const m of get(liveIslandMessages)) close(m);
  }

  // Focus held by the pressed button goes back where it came from before the action runs: a dialog the
  // action opens takes the focused element as the place to return to, and the button goes with the
  // message. A press that gave the button no focus (WebKit) moves nothing.
  const ran = new WeakSet<LiveIslandAction>();
  function act(m: LiveIslandMessage, action: LiveIslandAction) {
    if (ran.has(action)) return;
    ran.add(action);
    if (focusedInside()) {
      const from = origin;
      origin = undefined;
      giveFocusBack(from);
    }
    close(m);
    action.run();
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.key !== 'Escape' || !expanded || event.defaultPrevented || $topModal) return;
    event.preventDefault();
    collapse(true);
  }

  function onfocusin(event: FocusEvent) {
    const from = event.relatedTarget;
    if (from instanceof HTMLElement && !shell?.contains(from)) origin = from;
    focusInside = true;
  }

  function onfocusout(event: FocusEvent) {
    const to = event.relatedTarget;
    if (to instanceof Node && shell?.contains(to)) return;
    focusInside = false;
    // Focus has left the island, for a place of its own or for none (a press on a blank spot): there
    // is nothing to give back later.
    origin = undefined;
  }

  function onpointerenter(event: PointerEvent) {
    if (event.pointerType !== 'mouse') return;
    pointerOver = true;
    intent.hover();
  }

  function onpointerleave(event: PointerEvent) {
    if (event.pointerType !== 'mouse') return;
    pointerOver = false;
    intent.unhover();
  }

  // A press outside closes the open card. Not through lib/layers: a layer opened over a dialog becomes
  // the top layer and takes the dialog's Escape (the stack closes the top layer, or swallows Escape when
  // it may not close), and the stack has no layer that leaves Escape to the one under it. So one
  // listener, only while the card is open. Escape is the island's own only with focus inside it
  // (onkeydown); a card the pointer opened closes when the pointer leaves.
  $effect(() => {
    if (!expanded) return;
    return on(window, 'pointerdown', (event) => {
      if (event.target instanceof Node && !shell?.contains(event.target)) collapse(false);
    });
  });
</script>

{#snippet mark(m: LiveIslandMessage, size: string)}
  {#if m.icon}
    {@const Icon = ICONS[m.icon]}
    <Icon {size} strokeWidth={1.8} aria-hidden="true" />
  {:else}
    <span class="dot"></span>
  {/if}
{/snippet}

<div class="live-island-host" data-layer-keep>
  <!-- Not atomic: a second message is said alone, without the ones already in its region. -->
  <div class="sr-only" role="alert" aria-atomic="false" data-testid="live-island-alert">
    {#each urgent as m (m.id)}<p>{said(m)}</p>{/each}
  </div>
  <div class="sr-only" role="status" aria-atomic="false" data-testid="live-island-status">
    {#each polite as m (m.id)}<p>{said(m)}</p>{/each}
  </div>

  <!-- svelte-ignore a11y_no_noninteractive_element_interactions: the listeners watch hover, focus and Escape for the buttons inside -->
  <div
    bind:this={shell}
    class="live-island {tone}"
    class:expanded
    class:visible={shown}
    data-part="live-island"
    data-phase={phase}
    data-kind={primary?.kind ?? 'info'}
    data-layer-own-escape={ownEscape ? '' : undefined}
    role="group"
    aria-label={msg('ui.toastsLabel')}
    {@attach inert(!shown)}
    style:width={width ? `${width}px` : undefined}
    style:height={height ? `${height}px` : undefined}
    style:--live-island-now-h={height ? `${height}px` : undefined}
    {onpointerenter}
    {onpointerleave}
    {onfocusin}
    {onfocusout}
    {onkeydown}
  >
    <div class="content">
      <button bind:this={head} type="button" class="head" aria-expanded={expanded} {@attach inert(expanded)} onclick={open}>
        {#if primary}
          <span class="mark">{@render mark(primary, '1.6rem')}</span>
          <span class="text">{primary.text}</span>
          {#if primary.count}
            <span class="times" aria-hidden="true">×{primary.count}</span>
            <span class="sr-only">{msg('ui.toastRepeated', { count: primary.count })}</span>
          {/if}
          <CountBadge count={waiting} label={msg('ui.liveIslandWaiting', { count: waiting })} />
        {/if}
      </button>
      <div bind:this={card} class="card" {@attach inert(!expanded)}>
        {#if primary}
          {@const m = primary}
          <!-- Named by what it says, as the pill is; that it collapses the card is its description. -->
          <span class="sr-only" id={collapseId}>{msg('ui.liveIslandCollapse')}</span>
          <button bind:this={top} type="button" class="top" aria-expanded="true" aria-describedby={collapseId} onclick={() => collapse(true)}>
            <span class="lead">
              {#if m.avatar !== undefined}
                <Avatar src={m.avatar} name={m.title ?? m.text} size="md" />
              {:else}
                {@render mark(m, '2.2rem')}
              {/if}
            </span>
            <span class="about">
              <span class="title">{m.title ?? m.text}</span>
              {#if m.title}<span class="sub">{m.text}</span>{/if}
              {#if m.detail}<span class="sub">{m.detail}</span>{/if}
            </span>
          </button>
          <div class="actions">
            {#each (m.actions ?? []).slice(0, 2) as action, i (i)}
              <Button size="sm" variant={i === 0 ? 'primary' : 'secondary'} onclick={() => act(m, action)}>{action.label}</Button>
            {/each}
            <Button size="sm" variant="ghost" onclick={() => close(m)}>{msg('ui.toastDismiss')}</Button>
            {#if waiting > 0}
              <Button size="sm" variant="ghost" onclick={closeAll}>{msg('ui.liveIslandDismissAll')}</Button>
            {/if}
          </div>
        {/if}
      </div>
    </div>
  </div>
</div>

<style>
  .live-island {
    --tone: var(--accent-text);
    --wails-draggable: no-drag;
    position: fixed;
    top: var(--live-island-top);
    left: 50%;
    z-index: var(--z-live-island);
    overflow: hidden;
    contain: layout paint;
    max-width: calc(100vw - 2 * var(--live-island-clear));
    background: var(--surface-3);
    border: 1px solid var(--border-strong);
    /* Round while compact, a rounded block once it has grown. */
    border-radius: min(var(--radius-xl), calc(var(--live-island-now-h, var(--live-island-h)) / 2));
    box-shadow: var(--shadow-pop);
    color: var(--text);
    font-size: var(--font-md);
    transform-origin: top center;
    opacity: 0;
    visibility: hidden;
    pointer-events: none;
    transform: translate(-50%, -0.8rem) scale(0.9);
    transition:
      opacity var(--dur-panel) var(--ease),
      transform var(--dur-panel) var(--ease),
      visibility 0s linear var(--dur-panel);
  }

  .live-island.visible {
    opacity: 1;
    visibility: visible;
    pointer-events: auto;
    transform: translate(-50%, 0);
    transition:
      opacity var(--dur-panel) var(--ease),
      transform var(--dur-slow) var(--ease-spring),
      visibility 0s;
  }

  .success {
    --tone: var(--success);
  }

  .warning {
    --tone: var(--warning);
  }

  .danger {
    --tone: var(--danger);
  }

  .neutral {
    --tone: var(--text-2);
  }

  .live-island.warning,
  .live-island.danger {
    border-color: var(--tone);
  }

  .content {
    position: relative;
    left: 50%;
    width: min(var(--live-island-w), calc(100vw - 2 * var(--live-island-clear)));
    transform: translateX(-50%);
  }

  .head {
    position: absolute;
    top: 0;
    left: 50%;
    display: flex;
    align-items: center;
    gap: var(--space-1);
    width: max-content;
    max-width: 100%;
    height: calc(var(--live-island-h) - 2px);
    padding: 0 var(--space-4) 0 var(--space-2);
    border-radius: inherit;
    color: inherit;
    white-space: nowrap;
    transform: translateX(-50%);
    transition: opacity var(--motion-fast) var(--ease-out);
  }

  .expanded .head {
    opacity: 0;
  }

  .head:focus-visible,
  .top:focus-visible {
    outline: 2px solid var(--accent-ring);
    outline-offset: -2px;
  }

  .mark {
    display: inline-grid;
    flex: none;
    place-items: center;
    width: calc(var(--live-island-h) - 2px);
    height: 100%;
    color: var(--tone);
  }

  .dot {
    width: 0.8rem;
    height: 0.8rem;
    border-radius: var(--radius-round);
    background: var(--tone);
  }

  .text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .times {
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .card {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4);
    opacity: 0;
    transition: opacity var(--motion-fast) var(--ease-out);
  }

  .expanded .card {
    opacity: 1;
  }

  .top {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    border-radius: var(--radius-sm);
    color: inherit;
    text-align: start;
  }

  .lead {
    display: grid;
    flex: none;
    place-items: center;
    width: var(--live-island-lead);
    height: var(--live-island-lead);
    overflow: hidden;
    border-radius: var(--radius-sm);
    color: var(--tone);
  }

  .about {
    display: flex;
    flex: 1;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .title {
    font-weight: 600;
    overflow-wrap: anywhere;
  }

  .sub {
    color: var(--text-2);
    font-size: var(--font-sm);
    overflow-wrap: anywhere;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
