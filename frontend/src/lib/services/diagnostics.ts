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
//
// A path may contain spaces, and stopping at the first one is worse than
// useless here: "D:\Games\My Game\a.exe" would leave "My Game\a.exe" behind
// the placeholder, and the Go pass cannot recover it either, because this
// pass has already eaten the drive letter its own rules key on. So the
// patterns run to the end of the line and trimPath gives back the tail.
//
// The lookbehind keeps a single drive letter from matching the tail of a URL
// scheme: without it the "s:/" in "https://" reads as a drive path.
const WIN_PATH = /(?<![A-Za-z])[A-Za-z]:[\\/][^"'<>|)\r\n]*/g;
const UNC_PATH = /\\\\[^"'<>|)\r\n]*/g;
const UNIX_PATH = /(^|[\s"'(\[=,])(\/[^\s"'<>|)\r\n/]+(?:\/[^"'<>|)\r\n]*)?)/g;
const LOC_SUFFIX = /:\d+(?::\d+)?(?: \+0x[0-9a-f]+)?$/i;
const MAGNET_URI = /magnet:\?[^\s"'()<>]*/gi;
const INFOHASH = /\b[0-9a-fA-F]{40}\b/g;
const BEARER_TOKEN = /Bearer\s+[A-Za-z0-9\-._~+/]+=*/g;
const URL_QUERY = /(https?:\/\/[^\s"'()<>?]+)\?[^\s"'()<>]*/g;
const MAC_ADDRESS = /\b[0-9a-f]{2}(?:[:-][0-9a-f]{2}){5}\b/gi;
const IPV4 = /\b(?:\d{1,3}\.){3}\d{1,3}\b/g;
const IPV6 = /(?:\b[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,6})?::(?:[0-9a-f]{1,4}(?::[0-9a-f]{1,4}){0,6})?/gi;
const GENERIC_HOST = /\b(?:desktop|laptop)-[a-z0-9]{4,}\b/gi;

// trimPath mirrors the Go side: the greedy match is replaced by the
// placeholder and the tail it swallowed is handed back, so an error keeps its
// "op path: message" shape and a stack frame keeps its line number. skip
// steps over the prefix that carries its own colon.
function trimPath(match: string, skip: number): string {
  let body = match;
  let tail = '';
  const sep = body.slice(skip).indexOf(': ');
  if (sep >= 0) {
    const cut = skip + sep;
    tail = body.slice(cut);
    body = body.slice(0, cut);
  }
  const loc = body.match(LOC_SUFFIX);
  if (loc) tail = loc[0] + tail;
  return `<path>${tail}`;
}

function sanitize(text: string): string {
  if (!text) return '';
  return text
    .replace(WIN_PATH, (m) => trimPath(m, 2))
    .replace(UNC_PATH, (m) => trimPath(m, 2))
    .replace(UNIX_PATH, (_m, lead: string, path: string) => lead + trimPath(path, 1))
    .replace(MAGNET_URI, '<magnet>')
    .replace(INFOHASH, '<hash>')
    .replace(BEARER_TOKEN, 'Bearer <token>')
    .replace(URL_QUERY, '$1')
    .replace(MAC_ADDRESS, '<mac>')
    .replace(IPV6, '<ip>')
    .replace(IPV4, '<ip>')
    .replace(GENERIC_HOST, '<host>');
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
