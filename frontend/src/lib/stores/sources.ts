import { get, writable } from 'svelte/store';
import { listenFirst } from './listenFirst';
import { inWails } from '../services/backend';
import {
  addSource as addSourceRequest,
  addSourceFile as addSourceFileRequest,
  listSources,
  refreshAllSources,
  refreshSource,
  removeSource as removeSourceRequest,
  setSourceEnabled,
  type Source,
  type SourceType,
} from '../services/sources';
import { sourceErrorText } from '../sources/sourceErrors';
import { errorCode } from '../i18n/errors';
import { errorMessage } from '../utils/errors';
import { msg } from '../i18n';
import { toast } from './toasts';

export { errorMessage, sourceErrorText };

export const sources = writable<Source[]>([]);
/** Until a whole list arrives, `sources` holds only what events said: not the list of sources. */
export const sourcesLoaded = writable(false);
/** Why the first list could not be read; null once a list has arrived. */
export const sourcesLoadError = writable<unknown>(null);
export const refreshingAll = writable(false);

interface SourceErrorEvent {
  sourceId: string;
  name: string;
  message: string;
  scheduled: boolean;
}

interface ReleaseBatchEvent {
  sourceId: string;
  count: number;
}

const announcedLimit = 20;
const announced: { seq: number; message: string }[] = [];
let announcedSeq = 0;

function announce(message: string) {
  announced.push({ seq: ++announcedSeq, message });
  if (announced.length > announcedLimit) announced.shift();
}

export function errorMark() {
  return announcedSeq;
}

export function toastUnannounced(err: unknown, since: number) {
  const raw = errorMessage(err);
  if (announced.some((item) => item.seq > since && item.message !== '' && raw.includes(item.message))) return;
  toast(sourceErrorText(err), 'danger');
}

// The adds still waiting for Go's answer, each with the ids listed at its press. The source an add
// saves is one whose id is not among them: Go saves the location in its own spelling, so no string
// the frontend derives from the typed one names it. The error of a first refresh is said by the add,
// once, not by the event as well: an error event for an id one of them did not list is held.
const adding = new Set<Set<string>>();
const holdsError = (id: string) => [...adding].some((before) => !before.has(id));
const locationOf = (item: Source) => (item.type === 'file' ? (item.path ?? '') : item.url);

function upsert(item: Source) {
  sources.update((list) => {
    const index = list.findIndex((s) => s.id === item.id);
    if (index < 0) return [...list, item];
    const next = [...list];
    next[index] = item;
    return next;
  });
}

function received(list: Source[]) {
  sources.set(list);
  sourcesLoaded.set(true);
  sourcesLoadError.set(null);
}

/** Reads the whole list again after an action; a failure is the action's to report. */
async function reload() {
  received(await listSources());
}

// Every source event Go sent: a list asked for before one of them may predate it.
let heard = 0;

/** Reads the list; answers whether a list is there. A failure before any list is kept as state. */
async function readList(read: () => Promise<Source[]>): Promise<boolean> {
  try {
    received(await read());
  } catch (err) {
    if (!get(sourcesLoaded)) sourcesLoadError.set(err);
    return false;
  }
  return true;
}

/**
 * Reads the list again, as the page's Retry after a failed first read does; answers whether a list is
 * there. An event that arrives meanwhile gets the list asked once more, as the first read does.
 */
export function reloadSources(): Promise<boolean> {
  return readList(async () => {
    const before = heard;
    const list = await listSources();
    return heard === before ? list : listSources();
  });
}

/**
 * Subscribes first, then reads the list; never rejects: a failed first read is `sourcesLoadError`.
 * Events before a list arrived still go into `sources` (a download row names its source from there);
 * the whole list replaces them when it comes.
 */
export async function initSources(): Promise<void> {
  const first = listenFirst();
  if (!inWails) {
    await readList(listSources);
    return;
  }
  const on: typeof first.on = (name, handler) =>
    first.on(name, (event) => {
      heard += 1;
      handler(event);
    });

  on('source:updated', (event) => {
    const item = event.data as Source;
    if (!item.name) {
      sources.update((list) => list.filter((s) => s.id !== item.id));
      return;
    }
    upsert(item);
  });
  on('source:error', (event) => {
    const { sourceId, name, message, scheduled } = event.data as SourceErrorEvent;
    if (scheduled) return;
    announce(message);
    if (holdsError(sourceId)) return;
    toast(msg('state.sourcesErrorToast', { name, message: sourceErrorText(new Error(message)) }), 'danger');
  });
  on('source:degraded', (event) => {
    const status = event.data as { degraded: boolean; message: string } | null;
    if (status?.degraded) announce(status.message);
  });
  on('release:added', (event) => {
    const { count } = event.data as ReleaseBatchEvent;
    if (count > 0) toast(msg('state.sourcesNewEntries', { count }));
  });
  await readList(() => first.load(listSources));
}

export async function refresh(id: string) {
  const mark = errorMark();
  try {
    const summary = await refreshSource(id);
    await reload();
    toast(
      msg('state.sourcesRefreshResult', {
        entries: summary.entries,
        added: summary.added,
        review: summary.review,
      }),
      'success',
    );
  } catch (err) {
    toastUnannounced(err, mark);
  }
}

export async function refreshAll() {
  refreshingAll.set(true);
  try {
    await refreshAllSources();
    await reload();
  } catch (err) {
    toast(sourceErrorText(err), 'danger');
  } finally {
    refreshingAll.set(false);
  }
}

export async function toggle(id: string, enabled: boolean) {
  try {
    await setSourceEnabled(id, enabled);
    await reload();
  } catch (err) {
    toast(sourceErrorText(err), 'danger');
  }
}

/** Rejects when the backend refused: both callers ask through a confirmation, which shows the reason. */
export async function remove(id: string) {
  await removeSourceRequest(id);
  sources.update((list) => list.filter((s) => s.id !== id));
}

/** What an add did: the source Go saved and, when its first refresh failed, why. */
export interface AddResult {
  source: Source;
  failure?: unknown;
}

/**
 * Go's add saves the source and then answers its first refresh's error: an error does not say that
 * nothing was saved. The list read again tells: a source of that kind whose id was not listed at the
 * press is this add's, saved, with the failure; otherwise the add was refused and rejects. `place` is
 * Go's own spelling of the location, from the check's preview: it tells this add's source from one
 * that arrived meanwhile, and is compared as it is, never re-derived.
 */
async function addAt(kind: SourceType, place: string | undefined, send: () => Promise<Source>): Promise<AddResult> {
  const whole = get(sourcesLoaded);
  const before = new Set(get(sources).map((s) => s.id));
  adding.add(before);
  try {
    let source: Source;
    try {
      source = await send();
    } catch (err) {
      const saved = await savedBy(kind, place, whole, before, err);
      if (!saved) throw err;
      return { source: saved, failure: err };
    }
    await reload();
    return { source };
  } finally {
    adding.delete(before);
  }
}

async function savedBy(kind: SourceType, place: string | undefined, whole: boolean, before: Set<string>, err: unknown) {
  if (errorCode(err) === 'sources.source_exists') return undefined;
  try {
    await reload();
  } catch {
    // The list could not be read: nothing shows the source was saved, so the add's own reason stands.
    return undefined;
  }
  const fresh = get(sources).filter((s) => s.type === kind && !before.has(s.id));
  // Without Go's spelling, a lone new source is this add's only when the press saw the whole list.
  const mine = place !== undefined ? fresh.filter((s) => locationOf(s) === place) : whole ? fresh : [];
  return mine.length === 1 ? mine[0] : undefined;
}

/** `place`: the URL as Go spelt it in the check's preview. */
export function add(url: string, place?: string): Promise<AddResult> {
  return addAt('url', place, () => addSourceRequest(url));
}

/** `place`: the path as Go spelt it in the check's preview. */
export function addFile(path: string, place?: string): Promise<AddResult> {
  return addAt('file', place, () => addSourceFileRequest(path));
}
