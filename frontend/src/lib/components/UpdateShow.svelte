<script lang="ts">
  import { untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { msg } from '../i18n';
  import { focusQuietly } from '../layers/focus';
  import { runFrames } from '../motion/frames';
  import { motionOn, tokenMs } from '../motion/motion';
  import { curtainDrawn } from '../stores/handoff';
  import { settings } from '../stores/settings';
  import { endUpdateShow, logoArcsAway, updateShow } from '../stores/updateShow';
  import LogoArcs from './LogoArcs.svelte';
  import UpdateFunnel from './UpdateFunnel.svelte';
  import UpdateRing from './UpdateRing.svelte';
  import { LOGO_ARCS } from './logoArcs';
  import { arcFlights, curtainClosedAt, fillAt, showFrame, stillCurtain, type ArcPose, type ShowFrame, type ShowGeometry, type ShowTiming } from './updateShowTimeline';

  /**
   * The launcher-update show, in place of the old "installing" panel: over a dim the logo's five arcs
   * lift off the sidebar logo and fly into a turning funnel that holds while the update applies; the
   * finish brings them back, the way home brings them back after a failure. What happens when is
   * stores/updateShow.ts's; every frame comes from the clock (updateShowTimeline.ts), so a show
   * mounted late draws its present frame without the flight before it.
   *
   * With motion off, or with no sidebar logo to fly from (Big Picture), it is a still ring that fills,
   * on the logo's place or above the words, and a finish or a way home has nothing to play.
   * While it is up it covers the window and holds focus (the page under it takes no press or key);
   * at its end focus goes back where it was, else to the page's heading.
   */

  const show = $derived($updateShow);
  const words = $derived(show?.version ? show.version : '');
  const tint = $derived(Boolean($settings?.tintLogo));

  const timing: ShowTiming = {
    fly: tokenMs('--motion-show', 1200),
    step: tokenMs('--motion-stagger-arc', 120),
    slow: tokenMs('--dur-slow', 450),
    base: tokenMs('--dur-panel', 200),
    spin: tokenMs('--motion-funnel', 1400),
  };

  let logo = $state<ShowGeometry['logo'] | null>(null);
  let funnel = $state<ShowGeometry['funnel'] | null>(null);
  let frame = $state<ShowFrame | null>(null);
  let now = $state(Date.now());

  // A funnel with no box (a window not laid out) has nowhere to fly to: the ring stands in. A
  // successor takes the show over before its shell, and the logo, are there: it draws the funnel the
  // old window holds (the arcs are inside it, out of sight) and looks for the logo at the finish.
  const mode = $derived($motionOn && (logo || show?.handoff) && (!funnel || funnel.width >= 4) ? 'flight' : 'ring');
  const geometry = $derived(funnel ? { logo: logo ?? { left: funnel.left + funnel.width / 2, top: funnel.top + funnel.height / 2, size: 0 }, funnel } : null);

  function measureLogo(): ShowGeometry['logo'] | null {
    const origin = document.querySelector('[data-logo-origin]');
    const box = origin?.getBoundingClientRect();
    if (!box || box.width < 4) return null;
    return { left: box.left, top: box.top, size: box.width };
  }

  const measureFunnel: Attachment<HTMLElement> = (node) => {
    const box = node.getBoundingClientRect();
    funnel = { left: box.left, top: box.top, width: box.width, height: box.height };
  };

  const sameAway = (a: readonly boolean[] | null, b: readonly boolean[]) => a !== null && a.every((value, i) => value === b[i]);

  $effect(() => {
    if (show) return;
    frame = null;
    funnel = null;
  });

  $effect(() => {
    if (show && show.phase !== 'run' && !untrack(() => logo)) logo = measureLogo();
  });

  // A successor lets Go show its window once the closed curtain is on screen (stores/handoff.ts).
  $effect(() => {
    if (show?.held && curtain >= 1) curtainDrawn();
  });

  // The flight: one frame now, then one per animation frame, until the show's last.
  $effect(() => {
    const clock = show;
    if (!clock || mode !== 'flight' || !geometry) return;
    const flights = arcFlights(geometry);
    const draw = (at: number) => {
      const next = showFrame(clock, at, timing, flights);
      frame = next;
      if (!sameAway(untrack(() => $logoArcsAway), next.away)) logoArcsAway.set(next.away);
      if (next.stage !== 'done') return true;
      endUpdateShow();
      return false;
    };
    if (!draw(Date.now())) return;
    const stop = runFrames(draw);
    return () => stop?.();
  });

  // Motion off or no logo: the logo stays whole and the ring fills; nothing is left to play at the end.
  $effect(() => {
    const clock = show;
    if (!clock || mode !== 'ring') return;
    frame = null;
    logoArcsAway.set(null);
    if (clock.phase !== 'run') {
      endUpdateShow();
      return;
    }
    now = Date.now();
    const timer = setInterval(() => (now = Date.now()), 1000);
    // The curtain's step lands on its moment, not on the next second's tick.
    const closing = clock.handoff ? curtainClosedAt(clock.startedAt, timing, false) - now : -1;
    const step = closing > 0 ? setTimeout(() => (now = Date.now()), closing) : undefined;
    return () => {
      clearInterval(timer);
      clearTimeout(step);
    };
  });

  // The shell going (Big Picture swapped in, the language switched): the logo gets its arcs back;
  // a show that is still running is drawn again by the next mount from the clock.
  $effect(() => () => logoArcsAway.set(null));

  // Only a show that says something holds focus: the finish at a start (no words) is decoration.
  const cover: Attachment<HTMLElement> = (node) => {
    logo = measureLogo();
    if (!untrack(() => words)) return;
    const held = document.activeElement;
    const before = held instanceof HTMLElement && held !== document.body ? held : null;
    focusQuietly(node);
    return () => {
      queueMicrotask(() => {
        const active = document.activeElement;
        if (active && active !== document.body && active.isConnected) return;
        const back = before?.isConnected && !before.closest('[inert]') ? before : document.querySelector<HTMLElement>('main h1');
        if (back) focusQuietly(back);
      });
    };
  };

  function holdKeys(event: KeyboardEvent) {
    if (event.key === 'Tab') event.preventDefault();
  }

  const transform = (pose: ArcPose) => `translate(${pose.x}px, ${pose.y}px) rotate(${pose.rotate}deg) scale(${pose.sx}, ${pose.sy})`;
  const flights = $derived(geometry ? arcFlights(geometry) : null);
  const ringValue = $derived(show ? fillAt(now - show.startedAt) : 0);
  // A successor has no logo when it takes over: in a seamless restart both windows draw the ring in the middle.
  const ringOnLogo = $derived(Boolean(logo) && !show?.handoff);
  const curtain = $derived(mode === 'flight' ? (frame?.curtain ?? 0) : show ? stillCurtain(show, now, timing) : 0);
</script>

{#if show}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex: it holds focus so the page under it takes no key -->
  <div
    class="show"
    role={words ? 'dialog' : undefined}
    aria-modal={words ? 'true' : undefined}
    aria-label={words ? msg('ui.installingUpdateTitle') : undefined}
    aria-hidden={words ? undefined : 'true'}
    tabindex={words ? -1 : undefined}
    data-testid="update-show"
    data-mode={mode}
    data-stage={mode === 'flight' ? (frame?.stage ?? 'fly') : 'ring'}
    onkeydown={holdKeys}
    {@attach cover}
  >
    <div class="scrim" style:opacity={mode === 'flight' ? (frame?.scrim ?? 0) : 1}></div>
    <div class="curtain" style:opacity={curtain}></div>
    {#if mode === 'flight'}
      <div class="expand" style:transform="scale({frame?.ring.scale ?? 0})" style:opacity={frame?.ring.opacity ?? 0}></div>
    {/if}
    <div class="stage">
      {#if mode === 'flight'}
        <div class="funnel-box" {@attach measureFunnel}>
          <UpdateFunnel layers={frame?.layers} lit={frame?.lit} turn={frame?.turn} />
        </div>
      {:else if !ringOnLogo}
        <div class="ring-box"><UpdateRing value={ringValue} /></div>
      {/if}
      {#if words}
        <div class="words" role="status" style:opacity={mode === 'flight' ? (frame?.words ?? 0) : 1}>
          <p class="title">{msg('ui.installingVersion', { version: words })}</p>
          <p class="note">{msg(show.handoff ? 'ui.handoffNote' : 'ui.launcherWillRestartShort')}</p>
        </div>
      {/if}
    </div>
    {#if mode === 'ring' && ringOnLogo && logo}
      <div class="ringmark" style:left="{logo.left}px" style:top="{logo.top}px" style:width="{logo.size}px" style:height="{logo.size}px">
        <UpdateRing value={ringValue} />
      </div>
    {/if}
    {#if mode === 'flight' && logo && flights && frame}
      {#each LOGO_ARCS as arc, i (arc.id)}
        <div
          class="clone"
          data-arc-clone={arc.id}
          style:left="{logo.left}px"
          style:top="{logo.top}px"
          style:width="{logo.size}px"
          style:height="{logo.size}px"
          style:transform-origin="{flights[i].ox}px {flights[i].oy}px"
          style:transform={transform(frame.arcs[i])}
          style:opacity={frame.arcs[i].opacity}
        >
          <LogoArcs only={arc.id} {tint} />
        </div>
      {/each}
    {/if}
  </div>
{/if}

<style>
  .show {
    position: fixed;
    inset: 0;
    z-index: var(--z-update);
    outline: none;
  }

  .scrim {
    position: absolute;
    inset: 0;
    background: var(--scrim-update);
  }

  /* A seamless restart's curtain: the page colour the successor's window opens in. */
  .curtain {
    position: absolute;
    inset: 0;
    background: var(--bg);
  }

  .stage {
    position: absolute;
    left: 50%;
    top: 50%;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-3);
    width: min(36rem, 80vw);
    transform: translate(-50%, -56%);
  }

  .funnel-box {
    width: 100%;
  }

  .ring-box {
    width: var(--spinner-lg);
    height: var(--spinner-lg);
  }

  /* The words sit over the live, dimmed page: a plate of their own keeps them readable. */
  .words {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-4);
    text-align: center;
    color: var(--text);
    background: var(--surface-3);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-pop);
  }

  .words p {
    margin: 0;
  }

  .title {
    font-size: var(--font-md);
    font-weight: 600;
  }

  .note {
    color: var(--text-2);
    font-size: var(--font-sm);
  }

  .expand {
    position: absolute;
    left: 50%;
    top: 50%;
    width: 44rem;
    height: 44rem;
    margin: -22rem 0 0 -22rem;
    border: 2px solid var(--accent-text);
    border-radius: var(--radius-round);
  }

  .clone {
    position: absolute;
    will-change: transform, opacity;
  }

  .ringmark {
    position: absolute;
    padding: 0.2rem;
    background: var(--surface);
    border-radius: var(--radius-sm);
  }
</style>
