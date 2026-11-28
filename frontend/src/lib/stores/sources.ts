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
// once, not by the event as well: an error event for an id one of them did not list is held, and
// said when the last such add ends without having said it itself.
interface Adding {
  before: Set<string>;
  // New ids an event showed past their first refresh while the add was out.
  settled: Set<string>;
}
const adding = new Set<Adding>();
const heldErrors = new Map<string, string>();
const holdsError = (id: string) => [...adding].some((item) => !item.before.has(id));
const locationOf = (item: Source) => (item.type === 'file' ? (item.path ?? '') : item.url);

// A failure an add said whose error event had not come by its answer: that event, when it comes, is
// swallowed once. Go sends a refresh as `updating`, then the settled source, then the error, so an
// `updating` after a settled state starts another refresh and ends the wait; a different message
// is another failure and is said.
const saidByAdd = new Map<string, { message: string; settled: boolean }>();

function noteStatus(item: Source) {
  const settled = item.status !== 'updating';
  if (settled) for (const run of adding) if (!run.before.has(item.id)) run.settled.add(item.id);
  const said = saidByAdd.get(item.id);
  if (!said) return;
  if (!item.name || (said.settled && !settled)) saidByAdd.delete(item.id);
  else if (settled) said.settled = true;
}

function swallowed(sourceId: string, message: string) {
  const said = saidByAdd.get(sourceId);
  if (!said) return false;
  saidByAdd.delete(sourceId);
  return said.message.includes(message);
}

function settleAdd(run: Adding, said?: { id: string; failure: unknown }) {
  adding.delete(run);
  if (said && !heldErrors.delete(said.id)) {
    saidByAdd.set(said.id, { message: errorMessage(said.failure), settled: run.settled.has(said.id) });
  }
  for (const [id, text] of heldErrors) {
    if (holdsError(id)) continue;
    heldErrors.delete(id);
    toast(text, 'danger');
  }
}

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
    noteStatus(item);
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
    if (swallowed(sourceId, message)) return;
    const text = msg('state.sourcesErrorToast', { name, message: sourceErrorText(new Error(message)) });
    if (holdsError(sourceId)) heldErrors.set(sourceId, text);
    else toast(text, 'danger');
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

/**
 * Why a source cannot be removed now, '' when it can: the one rule of the row menu and the details
 * dialog. Go removes a refreshing source, and the refresh's caller then hears "source not found".
 */
export function removeHeldReason(source: Source | undefined): string {
  return source?.status === 'updating' ? msg('modals.sourceDetailsRemoveHeldUpdating') : '';
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
  const run: Adding = { before: new Set(get(sources).map((s) => s.id)), settled: new Set() };
  adding.add(run);
  let said: { id: string; failure: unknown } | undefined;
  try {
    let source: Source;
    try {
      source = await send();
    } catch (err) {
      const saved = await savedBy(kind, place, whole, run.before, err);
      if (!saved) throw err;
      said = { id: saved.id, failure: err };
      return { source: saved, failure: err };
    }
    // Go answered with the source: a list that cannot be read now is the list's trouble, and the
    // source is shown from its events or, when none came yet, from the answer.
    if (!(await readList(listSources)) && !get(sources).some((s) => s.id === source.id)) upsert(source);
    return { source };
  } finally {
    settleAdd(run, said);
  }
}

async function savedBy(kind: SourceType, place: string | undefined, whole: boolean, before: Set<string>, err: unknown) {
  if (errorCode(err) === 'sources.source_exists') return undefined;
  // A list that cannot be read leaves the one the events built: a source they brought is still Go's
  // word that it was saved; without one the add's own reason stands.
  await readList(listSources);
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
