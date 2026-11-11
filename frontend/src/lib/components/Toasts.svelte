<script lang="ts">
  import { tick } from 'svelte';
  import { CircleAlert, CircleCheck, Info, TriangleAlert, X } from '@lucide/svelte';
  import { msg } from '../i18n';
  import { modalOpen } from '../layers';
  import { dismissToast, holdToasts, pauseToast, resumeToast, toasts, type Toast, type ToastKind } from '../stores/toasts';
  import Button from './Button.svelte';
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

  const urgent = $derived($toasts.filter((item) => URGENT.includes(item.kind)));
  const polite = $derived($toasts.filter((item) => !URGENT.includes(item.kind)));

  $effect(() => holdToasts($modalOpen));

  const watch = createToastWatch<HTMLElement>();
  let stack = $state<HTMLElement>();

  function read(id: number, change: { hover?: boolean; focus?: boolean }) {
    if (watch.read(id, change)) pauseToast(id);
    else resumeToast(id);
  }

  // A toast can leave the list without being closed from inside: its time is up, or code removes it
  // (the "opening the link" toast). If focus was inside it, focus goes back where it came from
  // instead of dropping to the page's top; either way nothing is kept about a toast that is gone.
  let listed: number[] = [];
  $effect.pre(() => {
    const present = $toasts.map((item) => item.id);
    const gone = listed.filter((id) => !present.includes(id));
    listed = present;
    // Before the cards are taken out of the page: afterwards nothing says which one held focus.
    const held = gone.find((id) => stack?.querySelector(`[data-toast="${id}"]`)?.contains(document.activeElement));
    const home = held === undefined ? undefined : watch.origin(held);
    watch.prune(present);
    if (!home) return;
    void tick().then(() => {
      const lost = !document.activeElement?.isConnected || document.activeElement === document.body;
      if (lost && home.isConnected) home.focus({ preventScroll: true });
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
    const back = card?.contains(document.activeElement) ? watch.origin(item.id) : undefined;
    watch.forget(item.id);
    dismissToast(item.id);
    if (back?.isConnected) back.focus({ preventScroll: true });
  }

  function act(item: Toast, event: MouseEvent) {
    close(item, (event.currentTarget as HTMLElement).closest<HTMLElement>('.toast'));
    item.action?.run();
  }

  function onkeydown(item: Toast, event: KeyboardEvent) {
    if (event.key !== 'Escape' || event.defaultPrevented) return;
    // The key is the toast's: listeners on the window (a theme being tried, Big Picture's "back") must
    // not act on it as well.
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
    onpointerenter={() => read(item.id, { hover: true })}
    onpointerleave={() => read(item.id, { hover: false })}
    onfocusin={(event) => onfocusin(item, event)}
    onfocusout={(event) => onfocusout(item, event)}
    onkeydown={(event) => onkeydown(item, event)}
  >
    <Icon size="1.6rem" strokeWidth={1.8} aria-hidden="true" />
    <!-- One atomic piece inside the lane: when the count changes, the message is said again with it,
         and the buttons beside it are not. -->
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
    <!-- No hint on the X: a hint is a layer, and it would take the first Escape meant for the toast. -->
    <IconButton
      size="sm"
      tooltip={false}
      label={msg('ui.toastDismiss')}
      onclick={(event) => close(item, (event.currentTarget as HTMLElement).closest<HTMLElement>('.toast'))}
    >
      <X size="1.4rem" strokeWidth={1.8} />
    </IconButton>
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

  .message {
    flex: 1;
    min-width: 0;
    margin: 0;
    padding: var(--space-2) 0;
    line-height: 1.4;
    overflow-wrap: anywhere;
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
