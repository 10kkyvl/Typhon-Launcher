import { get } from 'svelte/store';
import { currentUser } from '../../lib/stores/user';

/**
 * What the player has typed into the support forms and not sent, kept while the form is gone: "Sign in
 * again" unmounts the whole shell, and the text comes back when the form opens again after the
 * sign-in. Kept for the account it was typed under: a guest keeps nothing, and another account signing
 * in drops it. In memory only, never on disk: it can hold personal text.
 *
 * A text leaves what is kept when its send starts, so a form opened again meanwhile cannot send it a
 * second time; a send that fails gives it back, unless something newer is kept by then.
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

interface Kept {
  owner: string;
  request: { draft: RequestDraft; by: Writer } | null;
  replies: Map<number, string>;
}

let kept: Kept | null = null;
let stopWatching: (() => void) | null = null;

function owner(): string | null {
  return get(currentUser)?.id || null;
}

/** The account signed in now. A send notes it when it starts: its late answer acts only for it. */
export function account(): string | null {
  return owner();
}

function forget() {
  kept = null;
  stopWatching?.();
  stopWatching = null;
}

// Watched only while something is kept: a sign-in of another account drops it at once. Signing out
// (the session gone) keeps it for the sign-in that follows.
function watchOwner() {
  if (stopWatching) return;
  const stop = currentUser.subscribe((user) => {
    if (kept && user && user.id !== kept.owner) forget();
  });
  if (kept) stopWatching = stop;
  else stop();
}

// While something is kept its owner is watched, so a signed-in player is its owner: no one signed in
// (a guest, the session gone) reads nothing.
function mine(): Kept | null {
  return owner() ? kept : null;
}

function writable(): Kept | null {
  const id = owner();
  if (!id) return null;
  if (kept?.owner !== id) kept = { owner: id, request: null, replies: new Map() };
  watchOwner();
  return kept;
}

function settle() {
  if (kept && !kept.request && kept.replies.size === 0) forget();
}

function typed(draft: RequestDraft): boolean {
  return Boolean(draft.subject.trim() || draft.body.trim() || draft.game);
}

function copy(draft: RequestDraft): RequestDraft {
  return { ...draft, game: draft.game ? { ...draft.game } : null };
}

/** Keeps the new request as the player has it now in `by`; a form with nothing typed keeps nothing. */
export function keepRequest(draft: RequestDraft, by: Writer): void {
  if (!typed(draft)) {
    const from = mine();
    if (from) from.request = null;
    settle();
    return;
  }
  const into = writable();
  if (!into) return;
  into.request = { draft: copy(draft), by };
}

/** The new request the signed-in player left unsent, if any. */
export function keptRequest(): RequestDraft | null {
  const request = mine()?.request;
  return request ? copy(request.draft) : null;
}

/** Forgets the kept request if `by` wrote it: a draft of another form stays. */
export function forgetRequest(by: Writer): void {
  const from = mine();
  if (!from || from.request?.by !== by) return;
  from.request = null;
  settle();
}

/** Listeners of a request given back by a failed send: a form open and untouched takes it. */
const requestBack = new Set<(draft: RequestDraft) => void>();

export function onRequestBack(listener: (draft: RequestDraft) => void): () => void {
  requestBack.add(listener);
  return () => requestBack.delete(listener);
}

/**
 * A send of `draft` from `by`, started under the account `from`, failed: it is kept again, unless
 * something newer is kept by now or another account (or no one) is signed in. Answers whether it was.
 */
export function returnRequest(draft: RequestDraft, by: Writer, from: string | null): boolean {
  // The text is lost with its account's session rather than given to whoever signed in since.
  if (!from || owner() !== from) return false;
  if (mine()?.request) return false;
  const into = writable();
  if (!into) return false;
  into.request = { draft: copy(draft), by };
  for (const listener of [...requestBack]) listener(copy(draft));
  return true;
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

const replyBack = new Set<(ticket: number, text: string) => void>();

export function onReplyBack(listener: (ticket: number, text: string) => void): () => void {
  replyBack.add(listener);
  return () => replyBack.delete(listener);
}

/**
 * A send of a reply, started under the account `from`, failed: it is kept again, unless a newer reply is
 * kept in that request by now or another account (or no one) is signed in.
 */
export function returnReply(ticket: number, text: string, from: string | null): boolean {
  // The text is lost with its account's session rather than given to whoever signed in since.
  if (!from || owner() !== from) return false;
  if (keptReply(ticket)) return false;
  const into = writable();
  if (!into) return false;
  into.replies.set(ticket, text);
  for (const listener of [...replyBack]) listener(ticket, text);
  return true;
}
