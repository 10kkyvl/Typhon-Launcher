<script lang="ts">
  import type { Attachment } from 'svelte/attachments';
  import { on } from 'svelte/events';
  import Reading from './Reading.svelte';

  /**
   * One value on a track of discrete steps (WAI-ARIA slider): the positions 0…`max`, which the caller
   * maps to its values (utils/speedLimit.ts for the speed limit, "no limit" being the last position).
   * Arrows move one step, Page Up/Down `page` steps, Home and End to the ends; a press on the track
   * takes the step under the pointer and a drag follows it, with the pointer captured.
   *
   * It never writes the value itself: the thumb is drawn at `position`, what the caller holds. A key
   * commits through `onchange`; a drag reports each new step through `oninput` and commits once on
   * release through `onchange` (when it ends off the value it began on), then `oninput(null)`: the
   * drag is over (a cancelled drag sends only that). A caller that shows the drag hands the reported
   * step back as `position` until `null`, and a refused commit leaves the thumb on the caller's value.
   * `position` may sit between steps (a value stored off the steps): it is drawn there and moves to a
   * step only when the player moves it.
   *
   * `label` names it (visible above the track unless `hideLabel`), or `labelledby` gives the id of a
   * visible label beside it (a settings row's), and the slider draws no label of its own; `valueText`
   * is the value in words, shown above the track and read by assistive tech. `startText` and `endText` mark the ends of the
   * scale under the track. `disabled` and `busy` (a commit under way) take no input and keep focus.
   * Sizes: `--slider-track` (track height), `--slider-thumb` (thumb side).
   */
  // One name: its own label, or the id of the label beside it.
  type Named = { label: string; labelledby?: undefined } | { labelledby: string; label?: undefined };

  let {
    position,
    max,
    label,
    labelledby,
    hideLabel = false,
    valueText,
    startText,
    endText,
    page = 3,
    describedby,
    disabled = false,
    busy = false,
    oninput,
    onchange,
    testid,
  }: {
    /** Where the caller's value sits, 0…max; between two steps for a value off the steps. */
    position: number;
    /** The last position; the track has max + 1 steps. */
    max: number;
    hideLabel?: boolean;
    /** The value in words ("20 MB/s", "No limit"): the readout and `aria-valuetext`. */
    valueText: string;
    startText?: string;
    endText?: string;
    /** Steps that Page Up and Page Down move. */
    page?: number;
    /** Id of the text that says what the slider sets. */
    describedby?: string;
    disabled?: boolean;
    busy?: boolean;
    /** The step under a dragging pointer, each time it changes; `null` when the drag ends. */
    oninput?: (position: number | null) => void;
    /** A committed step: a key, or a drag released on a step other than the value it began on. */
    onchange: (position: number) => void;
    /** On the slider element itself. */
    testid?: string;
  } & Named = $props();

  const uid = $props.id();

  let track: HTMLElement | undefined = $state();
  let thumb: HTMLElement | undefined = $state();
  // The drag under way: its pointer, the caller's value when it began and the last step reported. Not
  // state: nothing is drawn from it. A caller showing the drag hands each step back as `position`, so
  // the release is measured from where the drag began, not from `position`.
  let drag: { id: number; from: number; at: number } | null = null;

  const blocked = $derived(disabled || busy);
  const shown = $derived(Math.min(max, Math.max(0, position)));
  const at = $derived(max > 0 ? shown / max : 0);

  const clamp = (step: number) => Math.min(max, Math.max(0, step));

  function stepAt(clientX: number): number | null {
    const box = track?.getBoundingClientRect();
    if (!box || box.width <= 0) return null;
    return clamp(Math.round(((clientX - box.left) / box.width) * max));
  }

  function key(event: KeyboardEvent) {
    let next: number;
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowUp':
        next = Math.floor(shown) + 1;
        break;
      case 'ArrowLeft':
      case 'ArrowDown':
        next = Math.ceil(shown) - 1;
        break;
      case 'PageUp':
        next = Math.floor(shown) + page;
        break;
      case 'PageDown':
        next = Math.ceil(shown) - page;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = max;
        break;
      default:
        return;
    }
    event.preventDefault();
    if (blocked || drag) return;
    next = clamp(next);
    if (next !== position) onchange(next);
  }

  function down(event: PointerEvent) {
    if (blocked || drag || event.button !== 0) return;
    const next = stepAt(event.clientX);
    if (next === null) return;
    event.preventDefault();
    thumb?.focus({ preventScroll: true });
    (event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId);
    drag = { id: event.pointerId, from: position, at: next };
    oninput?.(next);
  }

  function move(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.id) return;
    const next = stepAt(event.clientX);
    if (next === null || next === drag.at) return;
    drag.at = next;
    oninput?.(next);
  }

  function up(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.id) return;
    const { from, at: done } = drag;
    drag = null;
    const bar = event.currentTarget as HTMLElement;
    if (bar.hasPointerCapture?.(event.pointerId)) bar.releasePointerCapture(event.pointerId);
    if (done !== from) onchange(done);
    oninput?.(null);
  }

  function cancel(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.id) return;
    drag = null;
    oninput?.(null);
  }

  // The track area is the pointer's target, not a control of its own: the keyboard reaches the same
  // value through the slider element inside it.
  const pointer: Attachment<HTMLElement> = (node) => {
    const offs = [
      on(node, 'pointerdown', down),
      on(node, 'pointermove', move),
      on(node, 'pointerup', up),
      on(node, 'pointercancel', cancel),
      on(node, 'lostpointercapture', cancel),
    ];
    return () => {
      for (const off of offs) off();
      // Torn down under a held pointer (a popover closed by Escape or a scroll): no release will come,
      // so the drag ends here as a cancel, and a caller showing it returns to its own value.
      if (drag) {
        drag = null;
        oninput?.(null);
      }
    };
  };
</script>

<div class="slider" class:disabled class:busy>
  <div class="head" class:sr-only={hideLabel}>
    {#if label !== undefined}<span class="label" id="{uid}-label">{label}</span>{/if}
    <span class="value" aria-hidden="true"><Reading text={valueText} /></span>
  </div>
  <div class="bar" {@attach pointer}>
    <div class="track" bind:this={track}>
      <div class="fill" style:--at={at}></div>
      <div
        bind:this={thumb}
        class="thumb"
        role="slider"
        tabindex="0"
        aria-labelledby={labelledby ?? `${uid}-label`}
        aria-describedby={describedby}
        aria-orientation="horizontal"
        aria-valuemin={0}
        aria-valuemax={max}
        aria-valuenow={shown}
        aria-valuetext={valueText}
        aria-disabled={disabled || undefined}
        aria-busy={busy || undefined}
        data-testid={testid}
        style:--at={at}
        onkeydown={key}
      ></div>
    </div>
  </div>
  {#if startText || endText}
    <div class="scale" aria-hidden="true">
      <span>{startText ?? ''}</span>
      <span>{endText ?? ''}</span>
    </div>
  {/if}
</div>

<style>
  .slider {
    --thumb: var(--slider-thumb, 1.6rem);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .slider.disabled {
    opacity: 0.5;
  }

  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .label {
    color: var(--text-2);
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .value {
    margin-left: auto;
    color: var(--text);
    font-size: var(--font-sm);
    font-weight: 600;
  }

  /* The pointer's target: as tall as a control, with half a thumb of room at each end so the thumb
     stays inside it at 0 and at max. */
  .bar {
    display: flex;
    align-items: center;
    min-height: var(--control-md);
    padding: 0 calc(var(--thumb) / 2);
    cursor: pointer;
    touch-action: none;
  }

  .disabled .bar {
    cursor: default;
  }

  .busy .bar {
    cursor: progress;
  }

  .track {
    position: relative;
    flex: 1;
    height: var(--slider-track, 0.4rem);
    border-radius: var(--radius-bar);
    background: var(--surface-4);
  }

  .fill {
    height: 100%;
    border-radius: var(--radius-bar);
    background: var(--accent-bright);
    transform: scaleX(var(--at));
    transform-origin: left center;
  }

  .thumb {
    position: absolute;
    top: 50%;
    left: calc(var(--at) * 100%);
    box-sizing: border-box;
    width: var(--thumb);
    height: var(--thumb);
    border: calc(var(--thumb) / 5) solid var(--text);
    border-radius: var(--radius-round);
    background: var(--accent-bright);
    transform: translate(-50%, -50%);
  }

  .bar:hover .thumb,
  .thumb:focus-visible {
    box-shadow: 0 0 0 var(--space-1) var(--accent-subtle);
  }

  .disabled .bar:hover .thumb {
    box-shadow: none;
  }

  .scale {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    color: var(--text-2);
    font-size: var(--font-xs);
    font-variant-numeric: tabular-nums;
  }

  @media (pointer: coarse) {
    .slider {
      --thumb: var(--slider-thumb, 2.4rem);
    }
  }
</style>
