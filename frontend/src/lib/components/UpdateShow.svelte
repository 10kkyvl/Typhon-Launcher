<script lang="ts">
  import { untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { msg } from '../i18n';
  import { layer } from '../layers';
  import { focusQuietly } from '../layers/focus';
  import { runFrames } from '../motion/frames';
  import { motionOn, tokenMs } from '../motion/motion';
  import { curtainDrawn, handoffWaiting } from '../stores/handoff';
  import { quitWaitingSuccessor } from '../services/selfupdate';
  import { updateReason } from '../services/selfupdateMessages';
  import { settings } from '../stores/settings';
  import { beginOwedFinish, endUpdateShow, hostUpdateShow, logoArcsAway, updateShow, type UpdateShowState } from '../stores/updateShow';
  import Button from './Button.svelte';
  import LogoArcs from './LogoArcs.svelte';
  import UpdateFunnel from './UpdateFunnel.svelte';
  import UpdateRing from './UpdateRing.svelte';
  import { LOGO_ARCS } from './logoArcs';
  import { portal } from '../utils/portal';
  import { arcFlights, curtainClosedAt, fillAt, showFrame, showTime, stillCurtain, type ArcPose, type ShowFrame, type ShowGeometry, type ShowTiming } from './updateShowTimeline';

  /**
   * The launcher-update show, all the window shows while an update is applied: over a dim the logo's
   * five arcs lift off the sidebar logo and fly into a turning funnel that holds while the update
   * applies; the finish brings them back, the way home brings them back after a failure. What
   * happens when is stores/updateShow.ts's; every frame comes from the clock (updateShowTimeline.ts),
   * so a show mounted late draws its present frame without the flight before it.
   *
   * With motion off, or with no sidebar logo to fly from (Big Picture), it is a still ring that fills,
   * on the logo's place or above the words, and a finish or a way home has nothing to play.
   * While it says something it is a cover on the layer stack (lib/layers): the page and every dialog
   * under it take no press or key; at its end focus goes back where it was, else to the page's heading.
   */

  const show = $derived($updateShow);
  const words = $derived(show?.version ? show.version : '');
  const tint = $derived(Boolean($settings?.tintLogo));

  // A successor whose old launcher is slow to go: the page under the show is inert, so the one thing
  // the player can do is offered here and takes focus as it appears.
  const waitingForOld = $derived(Boolean(show?.handoff && show.held && $handoffWaiting));
  let closing = $state(false);
  // The show covers the page, the toasts included: a refusal is said here.
  let closeRefused = $state('');
  const focusClose: Attachment<HTMLElement> = (node) => {
    const button = node.querySelector('button');
    if (button) focusQuietly(button);
  };

  async function closeLauncher() {
    if (closing) return;
    closing = true;
    closeRefused = '';
    try {
      await quitWaitingSuccessor();
    } catch (err) {
      // Go quits on success, and this window goes with it: only a refusal comes back. One that says
      // the wait is over needs no words: the old launcher is gone and the finish follows.
      closing = false;
      if (!(err instanceof Error && err.message.includes('selfupdate.not_waiting'))) closeRefused = updateReason(err);
    }
  }

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
  const time = showTime();
  let now = $state(Date.now());

  // A phase never begins before a frame already drawn: after the wall clock stepped back, a way home
  // stamped by it would otherwise have begun long ago and end at once. Kept per phase, so the start
  // stays put while it plays.
  let phase: { of: UpdateShowState; since: number } | null = null;
  function phaseClock(show: UpdateShowState): UpdateShowState {
    if (phase?.of !== show) phase = { of: show, since: Math.max(show.since, time.last()) };
    return { ...show, since: phase.since };
  }

  // A funnel with no box (a window not laid out) has nowhere to fly to: the ring stands in. A
  // successor takes the show over before its shell, and the logo, are there: it draws the funnel the
  // old window holds (the arcs are inside it, out of sight) and looks for the logo at the finish.
  const mode = $derived($motionOn && (logo || show?.handoff) && (!funnel || funnel.width >= 4) ? 'flight' : 'ring');
  const geometry = $derived(funnel ? { logo: logo ?? { left: funnel.left + funnel.width / 2, top: funnel.top + funnel.height / 2, size: 0 }, funnel } : null);
  const flights = $derived(geometry ? arcFlights(geometry) : null);
  const laidOut = $derived(geometry !== null);

  function measureLogo(): ShowGeometry['logo'] | null {
    const origin = document.querySelector('[data-logo-origin]');
    const box = origin?.getBoundingClientRect();
    if (!box || box.width < 4) return null;
    return { left: box.left, top: box.top, size: box.width };
  }

  function measureFunnelBox(node: Element) {
    const box = node.getBoundingClientRect();
    const next = { left: box.left, top: box.top, width: box.width, height: box.height };
    const was = untrack(() => funnel);
    if (!was || Object.entries(next).some(([key, value]) => was[key as keyof typeof next] !== value)) funnel = next;
  }

  function remeasureLogo() {
    const next = measureLogo();
    const was = untrack(() => logo);
    if (!next || !was || next.left !== was.left || next.top !== was.top || next.size !== was.size) logo = next;
  }

  // A resize or a snap of the window mid-show moves the logo and the funnel: they are measured again,
  // so the copies fly from and to where they are now. A host without ResizeObserver measures once.
  function watchSize(nodes: (Element | null)[], measure: () => void): (() => void) | undefined {
    if (typeof ResizeObserver !== 'function') return;
    const observer = new ResizeObserver(measure);
    for (const node of nodes) if (node) observer.observe(node);
    return () => observer.disconnect();
  }

  const measureFunnel: Attachment<HTMLElement> = (node) => {
    measureFunnelBox(node);
    return watchSize([node], () => measureFunnelBox(node));
  };

  const sameAway = (a: readonly boolean[] | null, b: readonly boolean[]) => a !== null && a.every((value, i) => value === b[i]);

  $effect(() => {
    if (show) return;
    frame = null;
    funnel = null;
    phase = null;
    time.reset();
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
    const current = show;
    if (!current || mode !== 'flight' || !laidOut) return;
    // An owed finish starts with this, its first frame; the store's answer runs this effect again.
    if (current.owed) {
      beginOwedFinish(time.now());
      return;
    }
    const clock = phaseClock(current);
    // Each frame takes the places measured last: a resize moves the flight, it does not restart it.
    const draw = () => {
      const paths = untrack(() => flights);
      if (!paths) return true;
      const next = showFrame(clock, time.now(), timing, paths);
      frame = next;
      if (!sameAway(untrack(() => $logoArcsAway), next.away)) logoArcsAway.set(next.away);
      if (next.stage !== 'done') return true;
      endUpdateShow();
      return false;
    };
    if (!draw()) return;
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
    now = time.now();
    const timer = setInterval(() => (now = time.now()), 1000);
    // The curtain's step lands on its moment, not on the next second's tick.
    const closing = clock.handoff ? curtainClosedAt(clock.startedAt, timing, false) - now : -1;
    const step = closing > 0 ? setTimeout(() => (now = time.now()), closing) : undefined;
    return () => {
      clearInterval(timer);
      clearTimeout(step);
    };
  });

  // The shell going (Big Picture swapped in, the language switched): the logo gets its arcs back;
  // a show that is still running is drawn again by the next mount from the clock.
  $effect(() => () => logoArcsAway.set(null));

  // Mounted means the screen under it is up: a finish owed after that would dim a page in use.
  $effect(() => hostUpdateShow());

  // The show covers the window, so its own box changes with the window's: the funnel, centred in it,
  // moves without changing size.
  const measure: Attachment<HTMLElement> = (node) => {
    logo = measureLogo();
    return watchSize([node, document.querySelector('[data-logo-origin]')], () => {
      remeasureLogo();
      const box = node.querySelector('.funnel-box');
      if (box) measureFunnelBox(box);
    });
  };

  // Only the status ends the show, never Escape or Big Picture's back.
  const stay = () => {};
  const pageHeading = () => document.querySelector<HTMLElement>('main h1');

  const transform = (pose: ArcPose) => `translate(${pose.x}px, ${pose.y}px) rotate(${pose.rotate}deg) scale(${pose.sx}, ${pose.sy})`;
  const ringValue = $derived(show ? fillAt(now - show.startedAt) : 0);
  // A successor has no logo when it takes over: in a seamless restart both windows draw the ring in the middle.
  const ringOnLogo = $derived(Boolean(logo) && !show?.handoff);
  const curtain = $derived(mode === 'flight' ? (frame?.curtain ?? 0) : show ? stillCurtain(show, now, timing) : 0);
</script>

{#if show && words}
  <div
    class="show"
    role="dialog"
    aria-modal="true"
    aria-label={msg('ui.installingUpdateTitle')}
    data-testid="update-show"
    data-mode={mode}
    data-stage={mode === 'flight' ? (frame?.stage ?? 'fly') : 'ring'}
    {@attach layer({ cover: true, dismissible: () => false, dismiss: stay, returnFocus: pageHeading })}
    {@attach measure}
  >
    {@render picture(show)}
  </div>
{:else if show}
  <!-- The finish at a start (no words) is decoration: no layer, no focus, hidden from screen readers.
       Out of #app, so no dialog opened under it makes it inert: for its two seconds its dim takes
       every press, rather than let one through to what it half hides. -->
  <div
    class="show"
    aria-hidden="true"
    data-testid="update-show"
    data-mode={mode}
    data-stage={mode === 'flight' ? (frame?.stage ?? 'fly') : 'ring'}
    {@attach portal}
    {@attach measure}
  >
    {@render picture(show)}
  </div>
{/if}

{#snippet picture(show: UpdateShowState)}
  <div class="scrim" style:opacity={mode === 'flight' ? (frame?.scrim ?? 0) : 1}></div>
  <div class="curtain" style:opacity={curtain} style:background-color={show.background}></div>
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
        <p class="note">{msg(!show.handoff ? 'ui.launcherWillRestartShort' : show.held && $handoffWaiting ? 'ui.handoffWaitingNote' : 'ui.handoffNote')}</p>
      </div>
    {/if}
    {#if waitingForOld}
      <div class="wait-actions" {@attach focusClose}>
        <Button busy={closing} onclick={closeLauncher}>{msg('ui.handoffCloseLauncher')}</Button>
        <div role="alert">
          {#if closeRefused}<p class="wait-refused">{closeRefused}</p>{/if}
        </div>
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
{/snippet}

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

  /* A seamless restart's curtain: the page colour the successor's window opens in. A successor paints
     it in the colour handed over (UpdateShowState.background), which its own theme may not share. */
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

  .wait-actions {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .wait-refused {
    padding: var(--space-1) var(--space-3);
    color: var(--text);
    font-size: var(--font-sm);
    text-align: center;
    background: var(--surface-3);
    border-radius: var(--radius-sm);
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
