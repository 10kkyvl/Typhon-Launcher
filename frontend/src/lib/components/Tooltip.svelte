<script lang="ts">
  import { onDestroy, untrack, type Snippet } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import { layer } from '../layers';
  import type { Placement } from '../utils/position';
  import { hintBubble, hintTrigger, usable } from './hint';
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

  const intent = createHoverIntent((next) => {
    if (next && !usable(control())) intent.dismiss();
    else open = next;
  });
  onDestroy(() => intent.destroy());

  const describedIds = (element: Element) => (element.getAttribute('aria-describedby') ?? '').split(/\s+/).filter(Boolean);

  function undescribe(element: Element) {
    const rest = describedIds(element).filter((id) => id !== descriptionId);
    if (rest.length) element.setAttribute('aria-describedby', rest.join(' '));
    else element.removeAttribute('aria-describedby');
  }

  // The hint joins whatever description the control has of its own. The page may rewrite that
  // attribute (a field gaining an error message) or create the control again: the id goes back on.
  $effect(() => {
    const root = host;
    if (!root || !describe || !text) return;
    let described: HTMLElement | null = null;
    const apply = () => {
      const element = control();
      if (described && described !== element) undescribe(described);
      described = element;
      const ids = element ? describedIds(element) : [];
      if (element && !ids.includes(descriptionId)) element.setAttribute('aria-describedby', [...ids, descriptionId].join(' '));
    };
    apply();
    const watcher = new MutationObserver(apply);
    watcher.observe(root, { subtree: true, childList: true, attributes: true, attributeFilter: ['aria-describedby'] });
    return () => {
      watcher.disconnect();
      if (described) undescribe(described);
    };
  });

  // Events of the control reach the host as they bubble, so a control created again inside it still counts.
  const listen: Attachment<HTMLElement> = (node) => hintTrigger(node, { intent, control });

  // Untracked: a re-render of the control must not set the shown hint up again (and miss its own removal).
  const bubble: Attachment<HTMLElement> = (node) =>
    untrack(() => hintBubble(node, { intent, control, placement, watch: host ?? null }));
</script>

<span class="tooltip-host" bind:this={host} {@attach listen}>
  {@render children()}
  {#if describe && text}<span id={descriptionId} hidden>{text}</span>{/if}
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
    z-index: var(--z-hint);
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
