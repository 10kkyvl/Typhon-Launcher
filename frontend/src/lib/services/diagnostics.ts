import { Service as DiagnosticsService } from '../../../bindings/typhon/internal/diagnostics';
import { inWails } from './backend';

const MAX_MESSAGE = 2000;
const MAX_STACK = 8000;
const DEDUPE_WINDOW_MS = 30000;
const MAX_TRACKED_KEYS = 200;

// Paths collapse whole, not just their user segment. The Go layer sanitizes
// again and its patterns stop at '<', so leaving a readable tail behind a
// placeholder ("C:\Users\<user>\Typhon\state.json") survives the second pass
// intact and reports itself clean.
// The lookbehind keeps a single drive letter from matching the tail of a URL
// scheme: without it the "s:/" in "https://" reads as a drive path.
const WIN_PATH = /(?<![A-Za-z])[A-Za-z]:[\\/][^\s"'<>|)]*/g;
const UNIX_HOME_PATH = /\/home\/[^\s"'<>|)]*/g;
const MAC_USERS_PATH = /\/Users\/[^\s"'<>|)]*/g;
const MAGNET_URI = /magnet:\?[^\s"'()<>]*/gi;
const INFOHASH = /\b[0-9a-fA-F]{40}\b/g;
const BEARER_TOKEN = /Bearer\s+[A-Za-z0-9\-._~+/]+=*/g;
const URL_QUERY = /(https?:\/\/[^\s"'()<>?]+)\?[^\s"'()<>]*/g;

function sanitize(text: string): string {
  if (!text) return '';
  return text
    .replace(WIN_PATH, '<path>')
    .replace(UNIX_HOME_PATH, '<path>')
    .replace(MAC_USERS_PATH, '<path>')
    .replace(MAGNET_URI, '<magnet>')
    .replace(INFOHASH, '<hash>')
    .replace(BEARER_TOKEN, 'Bearer <token>')
    .replace(URL_QUERY, '$1');
}

const lastReportedAt = new Map<string, number>();

function shouldReport(key: string): boolean {
  const now = Date.now();
  const last = lastReportedAt.get(key);
  if (last !== undefined && now - last < DEDUPE_WINDOW_MS) return false;
  lastReportedAt.set(key, now);
  if (lastReportedAt.size > MAX_TRACKED_KEYS) {
    const oldest = lastReportedAt.keys().next().value;
    if (oldest !== undefined) lastReportedAt.delete(oldest);
  }
  return true;
}

async function send(component: string, operation: string, message: string, stack: string, fatal: boolean): Promise<void> {
  try {
    if (!inWails) return;
    const sanitizedMessage = sanitize(message).slice(0, MAX_MESSAGE);
    const sanitizedStack = sanitize(stack).slice(0, MAX_STACK);
    const key = `${component}|${operation}|${sanitizedMessage.slice(0, 200)}`;
    if (!shouldReport(key)) return;
    await DiagnosticsService.ReportClientError(component, operation, sanitizedMessage, sanitizedStack, fatal);
  } catch {
    // a failed diagnostics report is silent by design: it never becomes a user-visible error
  }
}

function handleError(event: ErrorEvent) {
  try {
    const error = event.error;
    const message = error instanceof Error ? error.message : String(event.message ?? 'unknown error');
    const stack = error instanceof Error && error.stack ? error.stack : '';
    void send('frontend', 'window.onerror', message, stack, true);
  } catch {
    // the capture handler must never throw back into the caller
  }
}

function handleRejection(event: PromiseRejectionEvent) {
  try {
    const reason = event.reason;
    const message = reason instanceof Error ? reason.message : String(reason);
    const stack = reason instanceof Error && reason.stack ? reason.stack : '';
    void send('frontend', 'window.unhandledrejection', message, stack, false);
  } catch {
    // the capture handler must never throw back into the caller
  }
}

let installed = false;

export function installDiagnostics(): void {
  if (installed) return;
  installed = true;
  if (!inWails) return;
  window.addEventListener('error', handleError);
  window.addEventListener('unhandledrejection', handleRejection);
}
