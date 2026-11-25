import { get } from 'svelte/store';
import { currentUser } from '../../lib/stores/user';

/**
 * What the player has typed into the support forms and not sent, kept while the form is gone: "Sign in
 * again" unmounts the whole shell, and the text comes back when the form opens again after the
 * sign-in. Kept for the account it was typed under: a guest keeps nothing, and another account signing
 * in drops it. In memory only, never on disk: it can hold personal text.
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

interface Kept {
  owner: string;
  request: RequestDraft | null;
  replies: Map<number, string>;
}

let kept: Kept | null = null;
let stopWatching: (() => void) | null = null;

function owner(): string | null {
  return get(currentUser)?.id || null;
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

/** Keeps the new request as the player has it now; a form with nothing typed keeps nothing. */
export function keepRequest(draft: RequestDraft): void {
  if (!typed(draft)) {
    forgetRequest();
    return;
  }
  const into = writable();
  if (!into) return;
  into.request = { ...draft, game: draft.game ? { ...draft.game } : null };
}

/** The new request the signed-in player left unsent, if any. */
export function keptRequest(): RequestDraft | null {
  const request = mine()?.request;
  return request ? { ...request, game: request.game ? { ...request.game } : null } : null;
}

export function forgetRequest(): void {
  const from = mine();
  if (!from) return;
  from.request = null;
  settle();
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
