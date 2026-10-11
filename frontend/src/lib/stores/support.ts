import { get, writable } from 'svelte/store';
import { msg } from '../i18n';
import { AccountError } from '../services/account';
import { inWails } from '../services/backend';
import { unreadTickets } from '../services/support';
import { supportErrorText } from '../services/supportMessages';
import { toast } from './toasts';
import { authState } from './user';

export const POLL_INTERVAL_MS = 60_000;
export const FOCUS_REFRESH_MS = 15_000;

export const unreadReplies = writable(0);

let timer: ReturnType<typeof setInterval> | null = null;
let session = 0;
let latest = 0;
let counted = false;
let failing = false;
let lastAttempt = Number.NEGATIVE_INFINITY;

function reset() {
  lastAttempt = Number.NEGATIVE_INFINITY;
  if (timer !== null) {
    clearInterval(timer);
    timer = null;
  }
  session += 1;
  counted = false;
  failing = false;
  unreadReplies.set(0);
}

export async function refreshUnread(): Promise<void> {
  if (get(authState) !== 'authenticated') return;
  const owner = session;
  const request = ++latest;
  lastAttempt = Date.now();
  try {
    const count = await unreadTickets();
    if (owner !== session || request !== latest) return;
    failing = false;
    const before = get(unreadReplies);
    unreadReplies.set(count);
    if (counted && count > before) toast(msg('tickets.toastReply'));
    counted = true;
  } catch (err) {
    if (owner !== session || request !== latest) return;
    if (err instanceof AccountError && err.code === 'unauthenticated') return;
    if (failing) return;
    failing = true;
    toast(supportErrorText(err, msg('tickets.pollFailed')), 'danger');
  }
}

function start() {
  if (timer !== null) return;
  void refreshUnread();
  timer = setInterval(() => void refreshUnread(), POLL_INTERVAL_MS);
}

export function initSupport(): () => void {
  if (!inWails) return () => {};

  const unsubscribe = authState.subscribe((state) => {
    if (state === 'authenticated') start();
    else reset();
  });
  const onFocus = () => {
    if (Date.now() - lastAttempt >= FOCUS_REFRESH_MS) void refreshUnread();
  };
  window.addEventListener('focus', onFocus);

  return () => {
    window.removeEventListener('focus', onFocus);
    unsubscribe();
    reset();
  };
}
