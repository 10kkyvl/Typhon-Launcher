<script lang="ts">
  import { untrack } from 'svelte';
  import { SvelteMap, SvelteSet } from 'svelte/reactivity';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import { installErrorText } from '../../lib/install/installErrors';
  import { focusQuietly } from '../../lib/layers/focus';
  import type { Download } from '../../lib/services/downloads';
  import { downloadOffers, waitingForSchedule } from '../../lib/stores/downloadOffers';
  import { queue, requestDownloadAction } from '../../lib/stores/downloads';
  import { playThrottle } from '../../lib/stores/playThrottle';
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import { msg } from '../../lib/i18n';
  import DownloadsSection from './DownloadsSection.svelte';
  import QueueRow from './QueueRow.svelte';
  import { downloadsSectionIds } from './sections';
  import { cancelDownloadPrompt } from '../../lib/confirm/prompts';
  import { cancelAsks } from '../../lib/downloads/cancel';
  import { heldBack } from '../../lib/downloads/schedule';
  import { askMove, keyTarget, pointerTarget, roomFor, settleMove, shownOrder, type MoveAsk, type RowBox } from './queueReorder';
  import type { LeaveTicket } from '../../lib/components/downloadPanel';
  import { byData, createWatchers, handOffInView, pressed, type HandOff } from './rowFocus';

  /**
   * The downloads Go holds as queued, in Go's order: a row's place here is the `position` MoveTo takes.
   * A download moves by its grip (arrow keys, Home/End, or a pointer drag), "To top" or the row menu;
   * each move is one `moveTo`. While a move is in flight the list shows the asked order; Go's
   * `download:reordered` (or a refusal) then puts Go's order back on screen, so no order lives only here.
   *
   * `onannounce` speaks through the screen's one polite live region: a move once Go confirms it, a
   * refused move with its reason, a cancel.
   */
  let {
    onannounce = () => {},
    onopen,
  }: {
    onannounce?: (text: string) => void;
    /** Opens a download's details; without it the row menu offers none. */
    onopen?: (id: string) => void;
  } = $props();

  const uid = $props.id();
  const helpId = `${uid}-help`;
  let list = $state<HTMLElement>();

  const goIds = $derived($queue.map((d) => d.id));
  const goKey = $derived(goIds.join('\n'));
  const byId = $derived(new Map($queue.map((d) => [d.id, d])));

  /** Where focus goes once the page shows a move: decided at the press (`focusHandOff`), applied after. */
  interface Follow {
    handOff: HandOff;
    target: () => HTMLElement | null;
  }

  /**
   * A download being moved: `move` while a `moveTo` is in flight, `place` the last place asked, `order`
   * Go's order when the last call went out. Once Go answered, the entry stays until Go's order shows the
   * download there or changes at all.
   */
  interface Moving {
    move: MoveAsk | undefined;
    place: number;
    order: string;
    title: string;
    follow: Follow;
  }

  const moving = new SvelteMap<string, Moving>();

  const shownIds = $derived(shownOrder(goIds, new Map(Array.from(moving, ([id, m]) => [id, m.place]))));
  const shownKey = $derived(shownIds.join('\n'));
  const rows = $derived(shownIds.flatMap((id) => byId.get(id) ?? []));

  const gripOf = (id: string) => byData(list, 'grip', id);
  const menuOf = (id: string) => byData(list, 'menu', id);

  const follow = (target: () => HTMLElement | null): Follow => ({ handOff: focusHandOff(), target });
  const refocus = (f: Follow) => void handOffInView(f.handOff, f.target);

  function ask(id: string, to: number, how: Follow) {
    const current = moving.get(id);
    const step = askMove(current?.move, to);
    moving.set(id, {
      move: step.state,
      place: to,
      order: step.send === null && current ? current.order : goKey,
      title: byId.get(id)?.name ?? current?.title ?? '',
      follow: how,
    });
    if (step.send !== null) void send(id, step.send);
  }

  async function send(id: string, position: number) {
    const outcome = await requestDownloadAction(id, 'moveTo', { position, toast: false });
    const current = moving.get(id);
    if (!current?.move) return;
    const step = settleMove(current.move, outcome.ok);
    if (!outcome.ok) {
      moving.delete(id);
      onannounce(msg('downloads.queue.moveFailed', { title: current.title, reason: installErrorText(outcome.error) }));
      refocus(current.follow);
      return;
    }
    if (step.send !== null) {
      moving.set(id, { ...current, move: step.state, place: step.send, order: goKey });
      void send(id, step.send);
      return;
    }
    moving.set(id, { ...current, move: undefined });
    confirmMoves();
  }

  // A move Go answered is confirmed when Go's order shows the download at the asked place, or when Go's
  // order changed since the call went out (its event arrived): the place said is then Go's, not ours.
  function confirmMoves() {
    for (const [id, m] of moving) {
      if (m.move) continue;
      const at = goIds.indexOf(id);
      if (at !== m.place && goKey === m.order) continue;
      moving.delete(id);
      if (at >= 0) onannounce(msg('downloads.queue.moved', { title: m.title, position: at + 1, total: goIds.length }));
      refocus(m.follow);
    }
  }

  $effect(() => {
    void goKey;
    untrack(confirmMoves);
  });

  // A row the list moves is a node moved in the page, which can drop its focus: put it back by id.
  $effect(() => {
    void shownKey;
    untrack(() => {
      for (const m of moving.values()) refocus(m.follow);
    });
  });

  function gripKey(event: KeyboardEvent, id: string) {
    if (drag) {
      if (event.key === 'Escape') {
        event.preventDefault();
        letGo();
      }
      return;
    }
    const index = shownIds.indexOf(id);
    const to = keyTarget(event.key, index, shownIds.length);
    if (to === null) return;
    event.preventDefault();
    if (to !== index) ask(id, to, follow(() => gripOf(id)));
  }

  interface Drag {
    id: string;
    from: number;
    to: number;
    dy: number;
    startY: number;
    pointerId: number;
    boxes: RowBox[];
    /** The held row's height and the gap after it: how far the others move to make room. */
    step: number;
    /** The list as the drag found it; any change cancels the drag. */
    order: string;
    grip: HTMLElement;
  }

  let drag = $state<Drag | null>(null);

  function grab(event: PointerEvent, id: string) {
    if (event.button !== 0 || drag || !list) return;
    const from = shownIds.indexOf(id);
    const rowEls = Array.from(list.querySelectorAll<HTMLElement>(':scope > li'));
    if (from < 0 || rowEls.length < 2) return;
    const boxes = rowEls.map((row) => {
      const box = row.getBoundingClientRect();
      return { top: box.top, height: box.height };
    });
    const gap = Math.max(0, boxes[1].top - (boxes[0].top + boxes[0].height));
    const grip = event.currentTarget as HTMLElement;
    grip.setPointerCapture(event.pointerId);
    // WebKit does not focus a pressed button: the grip needs focus for Escape to reach it.
    focusQuietly(grip);
    drag = { id, from, to: from, dy: 0, startY: event.clientY, pointerId: event.pointerId, boxes, step: boxes[from].height + gap, order: shownKey, grip };
  }

  function track(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.pointerId) return;
    event.preventDefault();
    const dy = event.clientY - drag.startY;
    drag = { ...drag, dy, to: pointerTarget(drag.boxes, drag.from, dy) };
  }

  function drop(event: PointerEvent) {
    if (!drag || event.pointerId !== drag.pointerId) return;
    const { id, from, to } = drag;
    letGo();
    if (to !== from) ask(id, to, follow(() => gripOf(id)));
  }

  function letGo() {
    const held = drag;
    drag = null;
    if (held?.grip.hasPointerCapture?.(held.pointerId)) held.grip.releasePointerCapture(held.pointerId);
  }

  // A download that starts, leaves or arrives under the pointer changes what each place means.
  $effect(() => {
    const key = shownKey;
    untrack(() => {
      if (drag && drag.order !== key) letGo();
    });
  });

  function shiftOf(id: string, index: number): string | undefined {
    if (!drag) return undefined;
    if (id === drag.id) return `translateY(${drag.dy}px)`;
    return `translateY(${roomFor(index, drag.from, drag.to, drag.step)}px)`;
  }

  const watchers = createWatchers();

  $effect(() => {
    void goKey;
    void $downloadOffers;
    untrack(() => watchers.check());
  });

  const busyNow = new SvelteSet<string>();
  const busyCancel = new SvelteSet<string>();

  /**
   * The download a Cancel asks about, by id: the dialog says what the download holds now, not at the
   * press, and goes when the download leaves the queue.
   */
  let asking = $state<{ id: string; ticket: LeaveTicket; done: boolean } | null>(null);
  const askingDownload = $derived(asking ? byId.get(asking.id) : undefined);

  $effect(() => {
    if (asking && !askingDownload) untrack(closeCancel);
  });

  const gone = (id: string) => () => !goIds.includes(id);

  function cancelled(download: Download, ticket: LeaveTicket) {
    onannounce(msg('downloads.cancel.done', { name: download.name }));
    watchers.watch(gone(download.id), () => void ticket.handOn());
  }

  function askCancel(download: Download) {
    const ticket = pressed(download.id);
    if (cancelAsks(download)) asking = { id: download.id, ticket, done: false };
    else void cancelNow(download, ticket);
  }

  async function cancelNow(download: Download, ticket: LeaveTicket) {
    busyCancel.add(download.id);
    const outcome = await requestDownloadAction(download.id, 'cancel');
    busyCancel.delete(download.id);
    if (outcome.ok) cancelled(download, ticket);
    else ticket.drop();
  }

  async function confirmCancel() {
    const current = asking;
    const download = askingDownload;
    if (!current || !download) return;
    const outcome = await requestDownloadAction(current.id, 'cancel', { toast: false });
    if (!outcome.ok) throw outcome.error;
    current.done = true;
    cancelled(download, current.ticket);
  }

  function closeCancel() {
    if (asking && !asking.done) asking.ticket.drop();
    asking = null;
  }

  async function downloadNow(download: Download) {
    const ticket = pressed(download.id, { follow: 'first' });
    busyNow.add(download.id);
    const outcome = await requestDownloadAction(download.id, 'downloadNow');
    busyNow.delete(download.id);
    if (!outcome.ok) {
      ticket.drop();
      return;
    }
    // The button goes once Go lifts the schedule for the download, and the row may go as it starts.
    watchers.watch(() => !($downloadOffers.get(download.id) ?? []).includes('downloadNow'), () => void ticket.handOn());
  }

  async function startNow(download: Download) {
    const ticket = pressed(download.id, { prefer: '[data-menu]' });
    const outcome = await requestDownloadAction(download.id, 'forceStart');
    if (!outcome.ok) {
      ticket.drop();
      return;
    }
    watchers.watch(gone(download.id), () => void ticket.handOn());
  }

  function pick(action: 'details' | 'start' | 'up' | 'down', download: Download) {
    if (action === 'details') onopen?.(download.id);
    else if (action === 'start') void startNow(download);
    else {
      const at = shownIds.indexOf(download.id);
      ask(download.id, action === 'up' ? at - 1 : at + 1, follow(() => menuOf(download.id)));
    }
  }
</script>

{#if rows.length > 0}
<DownloadsSection id={downloadsSectionIds.queue} title={msg('transfers.downloadsQueueHeading')} count={rows.length}>
  <ol class="queue" bind:this={list}>
    {#each rows as download, index (download.id)}
      {@const waiting = $waitingForSchedule.has(download.id)}
      <QueueRow
        {download}
        {index}
        total={rows.length}
        offers={$downloadOffers.get(download.id) ?? []}
        {waiting}
        startNow={!heldBack(download, $playThrottle)}
        details={Boolean(onopen)}
        {helpId}
        lifted={drag?.id === download.id}
        transform={shiftOf(download.id, index)}
        busyNow={busyNow.has(download.id)}
        busyCancel={busyCancel.has(download.id)}
        ongripkey={(event) => gripKey(event, download.id)}
        ongrab={(event) => grab(event, download.id)}
        ontrack={track}
        ondrop={drop}
        onletgo={letGo}
        ontop={() => ask(download.id, 0, follow(() => gripOf(download.id)))}
        onnow={() => downloadNow(download)}
        oncancel={() => askCancel(download)}
        onpick={(action) => pick(action, download)}
      />
    {/each}
  </ol>
  <p id={helpId} class="sr-only">{msg('downloads.queue.help')}</p>
</DownloadsSection>
{/if}

{#if asking && askingDownload}
  {@const ticket = asking.ticket}
  <ConfirmModal
    prompt={cancelDownloadPrompt(askingDownload)}
    onconfirm={confirmCancel}
    errorText={installErrorText}
    onclose={closeCancel}
    returnFocus={() => ticket.target()}
  />
{/if}

<style>
  .queue {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
  }
</style>
