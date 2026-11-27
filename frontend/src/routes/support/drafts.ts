import { get, writable as store, type Readable } from 'svelte/store';
import { dismissToast, queuedToasts, toast, toasts, type ToastAction, type ToastKind } from '../../lib/stores/toasts';
import { currentUser } from '../../lib/stores/user';

/**
 * What the player has typed into the support forms and not sent, kept while the form is gone: "Sign in
 * again" unmounts the whole shell, and the text comes back when the form opens again after the
 * sign-in. Kept for the account it was typed under: a guest keeps nothing, and another account signing
 * in drops it. In memory only, never on disk: it can hold personal text.
 *
 * Per account: the request draft (what the new-request form writes), ONE spare request (another
 * unsent one, which the form offers with "Restore it"), and a reply per request. A text leaves what is
 * kept when its send starts, so a form opened again meanwhile cannot send it a second time; a send that
 * fails gives it back to the first free place and answers where it went.
 */

export interface RequestDraft {
  topic: string;
  game: { igdbId: number; title: string } | null;
  subject: string;
  body: string;
  attachLogs: boolean;
  /** The player set the logs switch: a topic picked later leaves it as it is. */
  logsTouched: boolean;
}

/** The form that wrote what is kept: only it forgets it (Cancel, its own send). */
export type Writer = object;

/** A kept request; its `id` goes with it from the draft to the spare and back, so offers are told apart. */
interface Slot {
  draft: RequestDraft;
  id: number;
}

interface Kept {
  owner: string;
  request: (Slot & { by: Writer }) | null;
  spare: Slot | null;
  replies: Map<number, string>;
}

let kept: Kept | null = null;
let stopWatching: (() => void) | null = null;
let slots = 0;
const changes = store(0);

/** Ticks whenever the kept request or the spare changes: an open form reads its offer again. */
export const requestsChanged: Readable<number> = { subscribe: changes.subscribe };

function changed() {
  changes.update((count) => count + 1);
}

function owner(): string | null {
  return get(currentUser)?.id || null;
}

/** The account signed in now. A send notes it when it starts: its late answer acts only for it. */
export function account(): string | null {
  return owner();
}

/** Support toasts that name a text or a request of the account they were raised under, by toast id. */
const raised = new Map<number, string>();

function idle() {
  if (kept || raised.size > 0) return;
  stopWatching?.();
  stopWatching = null;
}

function forget() {
  const requests = Boolean(kept?.request || kept?.spare);
  kept = null;
  idle();
  if (requests) changed();
}

// Watched only while something is kept or said: a sign-in of another account drops what is kept at
// once; signing out (the session gone) keeps it for the sign-in that follows. A toast of an account
// goes as soon as that account is no longer the one signed in, whatever holds it on screen.
function watchOwner() {
  if (stopWatching) return;
  const stop = currentUser.subscribe((user) => {
    for (const [id, from] of raised) {
      if (user?.id === from) continue;
      raised.delete(id);
      dismissToast(id);
    }
    if (kept && user && user.id !== kept.owner) forget();
    else idle();
  });
  if (kept || raised.size > 0) stopWatching = stop;
  else stop();
}

/**
 * Shows a support toast that names a text, a subject or a request of the account `from`: it is
 * dismissed once another account, or no one, is signed in.
 */
export function accountToast(from: string | null, message: string, kind: ToastKind, action?: ToastAction): void {
  const id = toast(message, kind, action);
  if (!from) return;
  const live = new Set([...get(toasts), ...queuedToasts()].map((item) => item.id));
  for (const gone of raised.keys()) if (!live.has(gone)) raised.delete(gone);
  raised.set(id, from);
  watchOwner();
}

// While something is kept its owner is watched, so a signed-in player is its owner: no one signed in
// (a guest, the session gone) reads nothing.
function mine(): Kept | null {
  return owner() ? kept : null;
}

function writable(): Kept | null {
  const id = owner();
  if (!id) return null;
  if (kept?.owner !== id) kept = { owner: id, request: null, spare: null, replies: new Map() };
  watchOwner();
  return kept;
}

/**
 * What is kept for `from` while no one is signed in: a text on its way when the session ended comes
 * back to it there, for the sign-in that follows. Another account's sign-in drops it (the watcher).
 */
function keptWhileOut(from: string): Kept | null {
  if (kept && kept.owner !== from) return null;
  kept ??= { owner: from, request: null, spare: null, replies: new Map() };
  watchOwner();
  return kept;
}

function settle() {
  if (kept && !kept.request && !kept.spare && kept.replies.size === 0) forget();
}

function typed(draft: RequestDraft): boolean {
  return Boolean(draft.subject.trim() || draft.body.trim() || draft.game);
}

function copy(draft: RequestDraft): RequestDraft {
  return { ...draft, game: draft.game ? { ...draft.game } : null };
}

function slot(draft: RequestDraft): Slot {
  slots += 1;
  return { draft: copy(draft), id: slots };
}

/**
 * `by` is to write the draft: a draft another form wrote moves to the spare. With the spare taken it
 * has no place left: it is dropped and answered, for the form to say so and offer to copy it.
 */
function displace(into: Kept, by: Writer): RequestDraft | null {
  const other = into.request;
  if (!other || other.by === by) return null;
  into.request = null;
  if (!into.spare) {
    into.spare = { draft: other.draft, id: other.id };
    return null;
  }
  return copy(other.draft);
}

/**
 * Keeps the new request as the player has it now in `by`; a form with nothing typed keeps nothing, and
 * forgets what it wrote itself. A draft another form wrote is not lost to it: it becomes the spare. An
 * edit never touches the spare. Answers a draft that found no place left (both were taken), if any.
 */
export function keepRequest(draft: RequestDraft, by: Writer): RequestDraft | null {
  if (!typed(draft)) {
    forgetRequest(by);
    return null;
  }
  const into = writable();
  if (!into) return null;
  const dropped = displace(into, by);
  into.request = into.request ? { ...into.request, draft: copy(draft) } : { ...slot(draft), by };
  changed();
  return dropped;
}

/** The new request the signed-in player left unsent, if any. */
export function keptRequest(): RequestDraft | null {
  const request = mine()?.request;
  return request ? copy(request.draft) : null;
}

/** The other unsent request the signed-in player has, the one a form offers, if any. */
export function spareRequest(): RequestDraft | null {
  const spare = mine()?.spare;
  return spare ? copy(spare.draft) : null;
}

/**
 * A plain way in (the list's "New request") opens with what is kept: `by` becomes the writer of the
 * draft, or, with no draft, of the spare, which is the draft from then on.
 */
export function openPlain(by: Writer): RequestDraft | null {
  const from = mine();
  if (!from) return null;
  if (from.request) {
    from.request.by = by;
    return copy(from.request.draft);
  }
  if (!from.spare) return null;
  from.request = { ...from.spare, by };
  from.spare = null;
  changed();
  return copy(from.request.draft);
}

/**
 * A way in with a prefill opens with the prefill: the draft kept then is offered as the spare when the
 * spare is free (else it stays the draft, offered once the spare is restored).
 */
export function openPrefilled(by: Writer): void {
  const from = mine();
  if (!from?.request || from.request.by === by || from.spare) return;
  from.spare = { draft: from.request.draft, id: from.request.id };
  from.request = null;
  changed();
}

/**
 * What the form `by` offers with "Restore it": the spare, or else a draft another form wrote (one that
 * came back while `by` sent its own). Its `id` tells one offer from the next.
 */
export function offeredRequest(by: Writer): { id: number; draft: RequestDraft } | null {
  const from = mine();
  const offer = from?.spare ?? (from?.request && from.request.by !== by ? from.request : null);
  return offer ? { id: offer.id, draft: copy(offer.draft) } : null;
}

/**
 * "Restore it" in the form `by`, which shows `shown`: the offer becomes the draft, and what the form
 * held typed becomes the spare in its place, the swap the notice promises. Answers the restored request
 * and a text that found no place left, if any.
 */
export function restoreRequest(by: Writer, shown: RequestDraft | null): { draft: RequestDraft; dropped: RequestDraft | null } | null {
  const from = mine();
  if (!from) return null;
  const other = from.request && from.request.by !== by ? from.request : null;
  const offer = from.spare ?? other;
  if (!offer) return null;
  const out = shown && typed(shown) ? slot(shown) : null;
  let dropped: RequestDraft | null = null;
  if (from.spare) {
    if (out && other) dropped = copy(other.draft);
    from.spare = out ?? (other ? { draft: other.draft, id: other.id } : null);
  } else {
    from.spare = out;
  }
  from.request = { draft: offer.draft, id: offer.id, by };
  changed();
  return { draft: copy(offer.draft), dropped };
}

/** Forgets the kept request if `by` wrote it: a draft of another form and the spare stay. */
export function forgetRequest(by: Writer): void {
  const from = mine();
  if (!from || from.request?.by !== by) return;
  from.request = null;
  changed();
  settle();
}

/** An open new-request form, as a text that comes back sees it. */
export interface RequestForm {
  by: Writer;
  /** It holds nothing typed and no game, and sends nothing: a text can go into its fields. */
  free(): boolean;
  /** Its own request is on its way: its text has left the draft. */
  sending(): boolean;
  /** Its fields and its notice are drawn (not waiting for, or failing, the read of the limits). */
  shown(): boolean;
  /** Puts the text in its fields; the form writes the draft from then on. */
  take(draft: RequestDraft): void;
}

const forms = new Set<RequestForm>();

export function onRequestBack(form: RequestForm): () => void {
  forms.add(form);
  return () => forms.delete(form);
}

/**
 * Where a text that came back went, as its toast may say it: into the fields of a form on screen
 * (`form`); kept, with no form on screen showing it or naming it (`kept`); kept and named by the notice
 * of a form on screen (`offered`); nowhere, both places being taken (`none`).
 */
export type Placed = 'form' | 'kept' | 'offered' | 'none';

/**
 * A send of `draft` from `by`, started under the account `from`, failed after its form was left: the
 * text goes to the first free place. An open form that holds nothing takes it into its fields; else the
 * draft, when no form is open or the open one sends its own; else the spare; else the draft, when it is
 * empty, whoever asks. Answers where it went, or null when there is no one to tell: with no one signed
 * in it is kept for `from` all the same (the draft, else the spare), and under another account it is
 * not kept at all.
 */
export function returnRequest(draft: RequestDraft, by: Writer, from: string | null): Placed | null {
  if (!from) return null;
  if (!owner()) {
    const into = keptWhileOut(from);
    if (!into) return null;
    if (!into.request) into.request = { ...slot(draft), by };
    else if (!into.spare) into.spare = slot(draft);
    else return null;
    changed();
    return null;
  }
  // The text is lost with its account's session rather than given to whoever signed in since.
  if (owner() !== from) return null;
  const into = writable();
  if (!into) return null;
  const open = [...forms].filter((form) => form.by !== by);
  // A form takes the text only where the draft it then writes pushes nothing out.
  const free = open.find((form) => form.free() && (!into.request || into.request.by === form.by || !into.spare));
  if (free) {
    displace(into, free.by);
    into.request = { ...slot(draft), by: free.by };
    changed();
    free.take(copy(draft));
    return free.shown() ? 'form' : 'kept';
  }
  // Beside a form that may write the draft at its next key the spare is the safer place; an empty
  // draft is still a free place when the spare is taken.
  let placed: Slot;
  if (!into.request && (open.every((form) => form.sending()) || into.spare)) {
    into.request = { ...slot(draft), by };
    placed = into.request;
  } else if (!into.spare) {
    into.spare = slot(draft);
    placed = into.spare;
  } else {
    settle();
    return 'none';
  }
  changed();
  const named = open.some((form) => form.shown() && offeredRequest(form.by)?.id === placed.id);
  return named ? 'offered' : 'kept';
}

/**
 * A send of `draft` from `by` failed with its form on screen: the form still shows it, so it is the
 * draft again. A text kept as the draft meanwhile (one that came back beside the send) moves to the
 * spare; an older spare then has no place left and is answered, to be named and offered for copying.
 * Nothing is kept under another account.
 */
export function returnShown(draft: RequestDraft, by: Writer, from: string | null): RequestDraft | null {
  if (!from || owner() !== from) return null;
  const into = writable();
  if (!into) return null;
  let dropped: RequestDraft | null = null;
  const other = into.request && into.request.by !== by ? into.request : null;
  if (other) {
    if (into.spare) dropped = copy(into.spare.draft);
    into.spare = { draft: other.draft, id: other.id };
  }
  into.request = { ...slot(draft), by };
  changed();
  return dropped;
}

/** Keeps the reply typed in a request; an empty one keeps nothing. */
export function keepReply(ticket: number, text: string): void {
  if (!text.trim()) {
    forgetReply(ticket);
    return;
  }
  writable()?.replies.set(ticket, text);
}

/** The reply the signed-in player left unsent in this request, or an empty text. */
export function keptReply(ticket: number): string {
  return mine()?.replies.get(ticket) ?? '';
}

export function forgetReply(ticket: number): void {
  const from = mine();
  if (!from) return;
  from.replies.delete(ticket);
  settle();
}

/** An open reply box, as a reply that comes back sees it. */
export interface ReplyBox {
  readonly ticket: number;
  /** It holds nothing but white space. */
  empty(): boolean;
  /** A reply of its own is on its way: the box shows it, and the kept slot is free. */
  sending(): boolean;
  /** The box is drawn (its request and the limits are read). */
  shown(): boolean;
  /** Puts the text in the box. */
  take(text: string): void;
}

const boxes = new Set<ReplyBox>();

export function onReplyBack(box: ReplyBox): () => void {
  boxes.add(box);
  return () => boxes.delete(box);
}

/**
 * Where a reply that came back went, as its toast may say it: into the box on screen (`box`); kept for
 * its request, no box on screen showing it (`kept`); kept for the box once the reply on its way from it
 * is done (`after`); nowhere, the request keeping another reply (`none`).
 */
export type ReplyPlaced = 'box' | 'kept' | 'after' | 'none';

/**
 * A send of a reply, started under the account `from`, failed after its view was left: it is kept
 * again unless another reply is kept in that request by now. Answers where it went, or null when
 * there is no one to tell: with no one signed in it is kept for `from` all the same, and under another
 * account it is not kept at all.
 */
export function returnReply(ticket: number, text: string, from: string | null): ReplyPlaced | null {
  if (!from) return null;
  if (!owner()) {
    const into = keptWhileOut(from);
    if (into && !into.replies.has(ticket)) into.replies.set(ticket, text);
    return null;
  }
  // The text is lost with its account's session rather than given to whoever signed in since.
  if (owner() !== from) return null;
  if (keptReply(ticket)) return 'none';
  const box = [...boxes].find((open) => open.ticket === ticket);
  // A box that shows text nothing keeps would overwrite the slot at its next key.
  if (box && !box.empty() && !box.sending()) return 'none';
  const into = writable();
  if (!into) return null;
  into.replies.set(ticket, text);
  if (!box) return 'kept';
  if (box.sending()) return 'after';
  box.take(text);
  return box.shown() ? 'box' : 'kept';
}

/**
 * A send of a reply failed with its box on screen: the box keeps it, after a reply of this request that
 * came back while it was on its way (the earlier first, a blank line between: neither was sent), and
 * what the box then holds is what is kept. Answers that text, or null under another account.
 */
export function returnShownReply(ticket: number, text: string, from: string | null): string | null {
  if (!from || owner() !== from) return null;
  const into = writable();
  if (!into) return null;
  const before = into.replies.get(ticket);
  const box = before ? `${before.trimEnd()}\n\n${text}` : text;
  into.replies.set(ticket, box);
  return box;
}
