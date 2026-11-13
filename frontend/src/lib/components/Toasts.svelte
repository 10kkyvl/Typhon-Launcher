<script lang="ts">
  import { tick } from 'svelte';
  import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from '@lucide/svelte';
  import { msg } from '../i18n';
  import { modalOpen } from '../layers';
  import { dismissToast, holdToasts, pauseToast, resumeToast, toasts, type Toast, type ToastKind } from '../stores/toasts';
  import Button from './Button.svelte';
  import { giveFocusBack } from './focusBack';
  import IconButton from './IconButton.svelte';
  import { createToastWatch } from './toastWatch';

  /**
   * The stack of toasts, mounted by a shell in a part the layer stack leaves live (`data-layer-keep`).
   * A named region holding two live regions that exist before any toast does, so the first toast is
   * announced too: errors and warnings go to the `alert` one (said at once), the rest to the `status`
   * one (said when the reader is free). What a new toast adds to its lane is its message, the label of
   * its action (an offer worth hearing) and a close button that has a name but no text. Timing, the
   * queue and repeats are the store's (stores/toasts.ts); this component tells it when a toast is being
   * read and whether a dialog is open.
   */

  const icons = { info: Info, success: CircleCheck, warning: TriangleAlert, danger: CircleAlert };
  const URGENT: readonly ToastKind[] = ['danger', 'warning'];

  // `elsewhere`: the toasts are shown by the live island (the main window with motion on); the stack
  // draws none of them and still tells the store whether a dialog is open.
  let { elsewhere = false }: { elsewhere?: boolean } = $props();

  const list = $derived(elsewhere ? [] : $toasts);
  const urgent = $derived(list.filter((item) => URGENT.includes(item.kind)));
  const polite = $derived(list.filter((item) => !URGENT.includes(item.kind)));

  $effect(() => holdToasts($modalOpen));

  const watch = createToastWatch<HTMLElement>();
  let stack = $state<HTMLElement>();

  function read(id: number, change: { hover?: boolean; focus?: boolean }) {
    if (watch.read(id, change)) pauseToast(id);
    else resumeToast(id);
  }

  // The watch follows the list, before the page does: a toast can leave it without being closed from
  // inside (its time is up, or code removes it: the "opening the link" toast).
  $effect.pre(() => {
    const present = list.map((item) => item.id);
    // Asked while the cards are still in the page: afterwards nothing says which one held focus.
    const left = watch.listed(present, (id) => stack?.querySelector(`[data-toast="${id}"]`)?.contains(document.activeElement) ?? false);
    if (!left) return;
    void tick().then(() => {
      const lost = !document.activeElement?.isConnected || document.activeElement === document.body;
      if (lost) giveFocusBack(left.origin);
    });
  });

  function onfocusin(item: Toast, event: FocusEvent) {
    const card = event.currentTarget as HTMLElement;
    const from = event.relatedTarget;
    if (from instanceof HTMLElement && !card.contains(from)) watch.entered(item.id, from);
    read(item.id, { focus: true });
  }

  function onfocusout(item: Toast, event: FocusEvent) {
    const card = event.currentTarget as HTMLElement;
    if (event.relatedTarget instanceof Node && card.contains(event.relatedTarget)) return;
    read(item.id, { focus: false });
  }

  // Closing a toast that holds focus would drop focus to the page's top.
  function close(item: Toast, card: HTMLElement | null) {
    const held = card?.contains(document.activeElement) ?? false;
    const origin = watch.origin(item.id);
    dismissToast(item.id);
    if (held) giveFocusBack(origin);
  }

  function act(item: Toast, event: MouseEvent) {
    close(item, (event.currentTarget as HTMLElement).closest<HTMLElement>('.toast'));
    item.action?.run();
  }

  function onkeydown(item: Toast, event: KeyboardEvent) {
    if (event.key !== 'Escape' || event.defaultPrevented) return;
    // The key is the toast's. The layer stack leaves it alone (`data-layer-own-escape`: a dialog under
    // the toast stays open, the hint on the X goes with its button), and listeners on the window (a
    // theme being tried, Big Picture's "back") must not act on it as well.
    event.preventDefault();
    event.stopPropagation();
    close(item, event.currentTarget as HTMLElement);
  }
</script>

{#snippet card(item: Toast)}
  {@const Icon = icons[item.kind]}
  <!-- svelte-ignore a11y_no_static_element_interactions: the listeners only watch the toast being read and take Escape from its own buttons -->
  <div
    class="toast {item.kind}"
    data-part="toast"
    data-toast={item.id}
    data-layer-own-escape
    onpointerenter={() => read(item.id, { hover: true })}
    onpointerleave={() => read(item.id, { hover: false })}
    onfocusin={(event) => onfocusin(item, event)}
    onfocusout={(event) => onfocusout(item, event)}
    onkeydown={(event) => onkeydown(item, event)}
  >
    <Icon size="1.6rem" strokeWidth={1.8} aria-hidden="true" />
    <!-- One atomic piece inside the lane: when the count changes, the message is said again with it,
         and the buttons beside it are not. -->
    <!-- The message and the action wrap as one group: an action that would leave the message less
         than a readable measure goes to a row of its own under it. The close button stays outside,
         at the top right. -->
    <div class="content">
      <p class="message" aria-atomic="true">
        {item.message}
        {#if item.count}
          <span class="times" aria-hidden="true">×{item.count}</span>
          <span class="sr-only">{msg('ui.toastRepeated', { count: item.count })}</span>
        {/if}
      </p>
      {#if item.action}
        <Button size="sm" onclick={(event) => act(item, event)}>{item.action.label}</Button>
      {/if}
    </div>
    <span class="close">
      <IconButton
        size="sm"
        label={msg('ui.toastDismiss')}
        onclick={(event) => close(item, (event.currentTarget as HTMLElement).closest<HTMLElement>('.toast'))}
      >
        <X size="1.4rem" strokeWidth={1.8} />
      </IconButton>
    </span>
  </div>
{/snippet}

<section class="toasts" aria-label={msg('ui.toastsLabel')} bind:this={stack}>
  <!-- Not atomic (an alert and a status are by default): a second toast is said alone, without the
       ones already in its lane. -->
  <div class="lane" role="alert" aria-atomic="false">
    {#each urgent as item (item.id)}
      {@render card(item)}
    {/each}
  </div>
  <div class="lane" role="status" aria-atomic="false">
    {#each polite as item (item.id)}
      {@render card(item)}
    {/each}
  </div>
</section>

<style>
  .toasts,
  .lane {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
  }

  .lane {
    gap: var(--space-2);
  }

  .lane:has(.toast) + .lane:has(.toast) {
    margin-top: var(--space-2);
  }

  .toast {
    /* The least width a message is read in: about 24 characters at --font-sm, three or four words a
       line. toastSpot.ts narrows the stack by the same number (TOAST_MEASURE_REM). */
    --toast-measure: 16rem;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    max-width: min(38rem, calc(100vw - var(--space-12)));
    padding: var(--space-1) var(--space-1) var(--space-1) var(--space-3);
    background: var(--surface-4);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-pop);
    font-size: var(--font-sm);
    color: var(--text);
    animation: toast-in var(--dur-panel) var(--ease);
  }

  .content {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: flex-end;
    column-gap: var(--space-2);
  }

  .message {
    flex: 1;
    min-width: 0;
    margin: 0;
    padding: var(--space-2) 0;
    line-height: 1.4;
    overflow-wrap: anywhere;
  }

  /* Beside an action the message asks for the measure: when the two do not fit in one row, the
     action wraps under it and the message takes the whole row. */
  .message:not(:only-child) {
    min-width: var(--toast-measure);
  }

  /* As tall as the first line of the message, so the button is level with it however many lines
     and rows the toast has. */
  .close {
    align-self: flex-start;
    display: flex;
    align-items: center;
    min-height: calc(1.4em + 2 * var(--space-2));
  }

  .times {
    margin-left: var(--space-1);
    color: var(--text-2);
    font-variant-numeric: tabular-nums;
  }

  .toast > :global(svg) {
    flex-shrink: 0;
    color: var(--accent-text);
  }

  .toast.success > :global(svg) {
    color: var(--success);
  }

  .toast.warning > :global(svg) {
    color: var(--warning);
  }

  .toast.danger > :global(svg) {
    color: var(--danger);
  }

  @keyframes toast-in {
    from {
      opacity: 0;
      transform: translateY(0.6rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
