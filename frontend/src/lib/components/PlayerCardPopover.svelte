<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import type { Attachment } from 'svelte/attachments';
  import PlayerCard from './PlayerCard.svelte';
  import { layer } from '../layers';
  import { hintBubble, hintTrigger, usable } from './hint';
  import { createHoverIntent } from './hoverIntent';
  import type { PresenceView, UserCard } from '../services/social';

  /**
   * The hover card of a friend row: a description of the row's name, shown by the same rules as a
   * tooltip (hint.ts, hoverIntent.ts) and as a non-modal layer on the stack. It has no controls.
   */
  let {
    anchor,
    user,
    presence,
    beside = false,
  }: {
    anchor?: HTMLElement | null;
    user: UserCard;
    presence?: PresenceView | null;
    /** The row keeps its links and actions under the name (a grid tile): the card opens beside it. */
    beside?: boolean;
  } = $props();

  const uid = $props.id();
  const id = `${uid}-card`;
  let open = $state(false);

  const control = (): HTMLElement | null => anchor ?? null;

  const intent = createHoverIntent((next) => {
    if (next && !usable(control())) intent.dismiss();
    else open = next;
  });
  onDestroy(() => intent.destroy());

  $effect(() => {
    const el = anchor;
    if (!el) return;
    const stop = untrack(() => hintTrigger(el, { intent, control: () => el }));
    return () => {
      stop();
      intent.dismiss();
    };
  });

  $effect(() => {
    const el = anchor;
    if (!el || !open) return;
    el.setAttribute('aria-describedby', id);
    return () => el.removeAttribute('aria-describedby');
  });

  // Away from the row's own links and actions: over them the card would cover them, and the pointer on
  // its way to them would keep it open. A row has them beside the name, a grid tile under it.
  const card: Attachment<HTMLElement> = (node) =>
    untrack(() =>
      hintBubble(node, {
        intent,
        control,
        placement: beside ? 'right-start' : 'bottom-start',
        fallback: beside ? 'bottom-start' : 'top-start',
        offset: 8,
        watch: anchor?.parentElement ?? null,
      }),
    );
</script>

{#if open}
  <div
    {id}
    class="popover"
    role="tooltip"
    {@attach layer({ modal: false, outsidePress: false, owner: control, dismiss: () => intent.escape() })}
    {@attach card}
  >
    <PlayerCard {user} {presence} />
  </div>
{/if}

<style>
  .popover {
    position: fixed;
    z-index: var(--z-hint);
    box-shadow: var(--shadow-pop);
    border-radius: var(--radius-lg);
    animation: popup-in var(--dur) var(--ease);
  }
</style>
