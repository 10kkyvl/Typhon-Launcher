<script lang="ts">
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { layer } from '../layers';
  import { anchorTo, scrolledUnder, type Placement } from '../utils/position';
  import { createHoverIntent } from './hoverIntent';

  /**
   * A hint for one control (WAI-ARIA tooltip). Wraps exactly one focusable control (a button, link,
   * field or `[tabindex]`): a hint on something that cannot take focus never reaches a keyboard or
   * screen-reader user, so give such text a control or show it as text instead.
   *
   * The hint shows when the pointer rests on the control and at once on keyboard focus; it stays while
   * the pointer moves onto it; Escape, a press on the control, blur, a scroll of the page under it, the
   * window losing focus, the control going away or turning disabled and a dialog opening hide it. It is
   * a non-modal layer on the stack (see lib/layers): it never takes focus and Escape closes it before
   * anything under it.
   *
   * `describe`: the text adds information and is the control's description (`aria-describedby`).
   * Off when it only repeats the control's name (an icon button's label): it is then shown and not
   * announced a second time.
   */
  let {
    text,
    placement = 'top',
    describe = true,
    children,
  }: {
    text: string;
    placement?: Placement;
    describe?: boolean;
    children: Snippet;
  } = $props();

  const uid = $props.id();
  const descriptionId = `${uid}-description`;

  // Anything that can take focus; a disabled control still counts, so that its hint can go away.
  const CONTROL = 'button, a[href], input, select, textarea, summary, [tabindex], [contenteditable]:not([contenteditable="false"])';

  let host = $state<HTMLElement>();
  let open = $state(false);

  const control = (): HTMLElement | null => host?.querySelector<HTMLElement>(CONTROL) ?? null;
  const usable = (): boolean => {
    const element = control();
    return element !== null && !element.matches(':disabled');
  };
  const inControl = (target: EventTarget | null): boolean => target instanceof Node && Boolean(control()?.contains(target));

  const intent = createHoverIntent((next) => {
    if (next && !usable()) intent.dismiss();
    else open = next;
  });
  onDestroy(() => intent.destroy());

  $effect(() => {
    const element = describe && host ? control() : null;
    if (!element) return;
    const own = element.getAttribute('aria-describedby');
    element.setAttribute('aria-describedby', own ? `${own} ${descriptionId}` : descriptionId);
    return () => {
      const rest = (element.getAttribute('aria-describedby') ?? '').split(/\s+/).filter((id) => id && id !== descriptionId);
      if (rest.length) element.setAttribute('aria-describedby', rest.join(' '));
      else element.removeAttribute('aria-describedby');
    };
  });

  function onPointerover(event: PointerEvent) {
    if (event.pointerType === 'touch' || !inControl(event.target) || !usable()) return;
    intent.hover();
  }

  function onPointerout(event: PointerEvent) {
    if (!inControl(event.target) || inControl(event.relatedTarget)) return;
    intent.unhover();
  }

  function onFocusin(event: FocusEvent) {
    const target = event.target;
    if (target instanceof HTMLElement && inControl(target) && target.matches(':focus-visible')) intent.focus();
  }

  function onFocusout(event: FocusEvent) {
    if (inControl(event.relatedTarget)) return;
    intent.blur();
  }

  function onPointerdown(event: PointerEvent) {
    if (inControl(event.target)) intent.escape();
  }

  // Events of the control reach the host as they bubble, so a control re-created inside it still counts.
  const listen: Attachment<HTMLElement> = (node) => {
    node.addEventListener('pointerover', onPointerover);
    node.addEventListener('pointerout', onPointerout);
    node.addEventListener('focusin', onFocusin);
    node.addEventListener('focusout', onFocusout);
    node.addEventListener('pointerdown', onPointerdown);
    return () => {
      node.removeEventListener('pointerover', onPointerover);
      node.removeEventListener('pointerout', onPointerout);
      node.removeEventListener('focusin', onFocusin);
      node.removeEventListener('focusout', onFocusout);
      node.removeEventListener('pointerdown', onPointerdown);
    };
  };

  // While shown: follows the control, keeps itself under the pointer, and goes away with the control.
  // Untracked: a re-render of the control must not set the hint up again (and miss its own removal).
  const bubble: Attachment<HTMLElement> = (node) => untrack(() => follow(node));

  function follow(node: HTMLElement): () => void {
    const stopAnchor = anchorTo(node, () => control(), { placement, offset: 6, onhide: () => intent.dismiss() });
    const enter = () => intent.hover();
    const leave = () => intent.unhover();
    // A press on the hint must not take focus off the control it describes.
    const keepFocus = (event: MouseEvent) => event.preventDefault();
    const onScroll = (event: Event) => {
      if (scrolledUnder(event.target, control())) intent.dismiss();
    };
    const onBlur = () => intent.dismiss();
    const watcher = new MutationObserver(() => {
      if (!usable()) intent.dismiss();
    });
    if (host) watcher.observe(host, { subtree: true, childList: true, attributes: true, attributeFilter: ['disabled'] });
    node.addEventListener('pointerenter', enter);
    node.addEventListener('pointerleave', leave);
    node.addEventListener('mousedown', keepFocus);
    window.addEventListener('scroll', onScroll, true);
    window.addEventListener('blur', onBlur);
    return () => {
      stopAnchor();
      watcher.disconnect();
      node.removeEventListener('pointerenter', enter);
      node.removeEventListener('pointerleave', leave);
      node.removeEventListener('mousedown', keepFocus);
      window.removeEventListener('scroll', onScroll, true);
      window.removeEventListener('blur', onBlur);
    };
  }
</script>

<span class="tooltip-host" bind:this={host} {@attach listen}>
  {@render children()}
  {#if describe}<span id={descriptionId} hidden>{text}</span>{/if}
</span>

{#if open && text}
  <div
    class="tooltip"
    data-part="tooltip"
    aria-hidden="true"
    {@attach layer({ modal: false, outsidePress: false, owner: () => control(), dismiss: () => intent.escape() })}
    {@attach bubble}
  >
    {text}
  </div>
{/if}

<style>
  /* No box of its own: the control sits in its parent's layout as if unwrapped. */
  .tooltip-host {
    display: contents;
  }

  .tooltip {
    position: fixed;
    z-index: var(--z-layer);
    max-width: 32rem;
    width: max-content;
    padding: 0.5rem 0.8rem;
    background: var(--surface-4);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    box-shadow: var(--shadow-pop);
    font-size: var(--font-xs);
    line-height: 1.4;
    color: var(--text-2);
    white-space: normal;
    overflow-wrap: anywhere;
    animation: popup-in var(--dur-fast) var(--ease);
  }
</style>
