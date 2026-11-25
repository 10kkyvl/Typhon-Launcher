import { msg } from '../../lib/i18n';
import type { ConfirmPrompt } from '../../lib/confirm/prompts';
import { diskusageErrorText, isScanCancelled, PartialDeleteError } from '../../lib/diskusage/errors';
import { gameTitle, leftoverKindKey } from '../../lib/diskusage/usage';
import {
  cachedUsage,
  cancelScan,
  deleteDownloadData,
  deleteLeftover,
  deleteScreenshots,
  deleteSnapshots,
  scanUsage,
  type DownloadUsage,
  type GroupDelete,
  type GroupUsage,
  type Leftover,
  type Report,
} from '../../lib/services/diskusage';
import { toast } from '../../lib/stores/toasts';
import { bytesSize } from '../../lib/utils/format';
import { placeOf, returnTo, type RowPlace } from './rowFocus';

export type DiskUsagePage = ReturnType<typeof createDiskUsagePage>;

/** The one confirmation in view: what it asks, what it runs, where focus goes when its row has gone. */
export interface PendingDelete {
  /** Read anew with every report for a group, whose delete Go answers from its latest scan. */
  readonly prompt: ConfirmPrompt;
  /** The prompt's words follow the report (a group's question). */
  follows: boolean;
  run: () => Promise<void>;
  returnFocus: () => HTMLElement | null;
}

type GroupKind = 'screenshots' | 'snapshots';
interface GroupRef {
  kind: GroupKind;
  gameId: string;
}

/** A group's size in the words of a question: exact, at least, or not measured. */
function groupSize(group: GroupUsage): string {
  if (group.sizeUnknown && group.sizeBytes <= 0) return msg('diskusage.sizeUnknown');
  const size = bytesSize(group.sizeBytes);
  return group.sizeUnknown ? msg('diskusage.atLeast', { size }) : size;
}

const groupsOf = (from: Report, kind: GroupKind) => (kind === 'screenshots' ? from.screenshots : from.snapshots);
const rowOf = (from: Report | null, group: GroupRef) => (from ? groupsOf(from, group.kind).find((g) => g.gameId === group.gameId) : undefined);

/**
 * What the disk usage page reads and deletes: the report (the cached one first, then a scan; one scan
 * at a time, a measure asked during it runs once after it), the scan's state and error, and the one confirmation every
 * delete goes through, which rescans when it is done. Created once by the page; the page starts
 * `load()` on mount, `refresh()` when a move ends, and `leave()` when it unmounts.
 */
export function createDiskUsagePage() {
  let report = $state<Report | null>(null);
  let scanning = $state(false);
  let scanError = $state('');
  let pending = $state<PendingDelete | null>(null);
  // The group the open question is about, when it is one: a report without that group closes it.
  let pendingGroup: GroupRef | null = null;
  // A confirmation's run is under way: its own rescan does not close it; it closes when it is done.
  let running = false;
  let scanSerial = 0;
  let left = false;
  // The scan in flight, and the one more asked while it runs: Go joins a running walk, whose game list
  // was read when it began, so its answer may predate the change that asked. Any number of asks during
  // a scan make one follow-up.
  let inFlight: Promise<boolean> | null = null;
  let followUp: Promise<boolean> | null = null;

  /**
   * Measures again; answers whether a report came, from a scan that began after the call. The last
   * failure stays said until the scan answers, so a retry in flight keeps its words. A scan stopped by
   * `leave` (or by CancelScan) is not a failure.
   */
  function refresh(): Promise<boolean> {
    if (left) return Promise.resolve(false);
    if (!inFlight) return start();
    followUp ??= inFlight.then(() => {
      followUp = null;
      return start();
    });
    return followUp;
  }

  // After a delete Go stops the walk that was running and the waiting scan measures the disk as the
  // delete left it: that answer is the one to wait for, and a second walk after it would only hold
  // the confirmation open.
  const remeasured = () => inFlight ?? refresh();

  function start(): Promise<boolean> {
    if (left) return Promise.resolve(false);
    const flight = measure();
    inFlight = flight;
    void flight.then(() => {
      if (inFlight === flight) inFlight = null;
    });
    return flight;
  }

  async function measure(): Promise<boolean> {
    const serial = ++scanSerial;
    scanning = true;
    try {
      const next = await scanUsage();
      if (serial !== scanSerial) return false;
      report = next;
      scanError = '';
      if (pendingGroup && !running && !rowOf(next, pendingGroup)) closePending();
      return true;
    } catch (err) {
      if (serial === scanSerial) scanError = isScanCancelled(err) ? '' : diskusageErrorText(err, msg('diskusage.scanFailed'));
      return false;
    } finally {
      // With a follow-up asked, the page stays measuring until it answers.
      if (serial === scanSerial && !followUp) scanning = false;
    }
  }

  async function load() {
    try {
      const cached = await cachedUsage();
      if (cached && !report && !left) report = cached;
    } catch (err) {
      scanError = diskusageErrorText(err, msg('diskusage.scanFailed'));
    }
    await refresh();
  }

  /** The page is left: its scan in flight is stopped, and no answer lands any more. */
  function leave() {
    left = true;
    scanSerial += 1;
    if (!scanning) return;
    scanning = false;
    cancelScan().catch((err: unknown) => console.warn('cancel the disk usage scan', err));
  }

  /**
   * Opens the confirmation of one delete. Its run re-reads the disk after Go answered, worked or not,
   * so the rows show what is left; a refusal or a group delete that left items rejects, and the
   * confirmation keeps that failure in view with Retry. `row` is the row's key: where it stood is read
   * now, for focus once the row has gone.
   */
  function ask(prompt: () => ConfirmPrompt, action: () => Promise<GroupDelete | void>, done: string, row: string, group: GroupRef | null = null) {
    const place: RowPlace = placeOf(row, 'delete');
    pendingGroup = group;
    pending = {
      get prompt() {
        return prompt();
      },
      returnFocus: () => returnTo(place),
      follows: group !== null,
      run: async () => {
        // A Retry after the group's rows went: Go has nothing of it to delete.
        if (group && !rowOf(report, group)) return;
        let result: GroupDelete | void;
        running = true;
        try {
          try {
            result = await action();
          } catch (err) {
            await remeasured();
            throw err;
          }
          await remeasured();
        } finally {
          running = false;
        }
        if (result && result.failed > 0) throw new PartialDeleteError(result);
        toast(done, 'success');
      },
    };
  }

  function closePending() {
    pending = null;
    pendingGroup = null;
  }

  function askDownload(download: DownloadUsage) {
    ask(
      () => ({
        title: msg('diskusage.downloadConfirmTitle'),
        text: msg('diskusage.downloadConfirmText', { name: download.name }),
        confirm: msg('diskusage.downloadDelete'),
        busy: msg('diskusage.deleting'),
      }),
      () => deleteDownloadData(download.id),
      msg('diskusage.downloadDeleted'),
      `downloads:${download.id}`,
    );
  }

  function askLeftover(leftover: Leftover) {
    ask(
      () => ({
        title: msg('diskusage.leftoverConfirmTitle'),
        text: msg('diskusage.leftoverConfirmText', {
          kind: msg(leftoverKindKey(leftover.kind)),
          path: leftover.path,
        }),
        confirm: msg('diskusage.leftoverDelete'),
        busy: msg('diskusage.deleting'),
      }),
      () => deleteLeftover(leftover.path),
      msg('diskusage.leftoverDeleted'),
      `leftovers:${leftover.path}`,
    );
  }

  /**
   * A group delete sends only the game's id and Go deletes what its latest scan counted, so the question
   * is put in the words of the group's row as the latest report shows it, read again with each report;
   * the row it was put from stands in once the group has gone.
   */
  function askGroup(kind: GroupKind, group: GroupUsage, words: (row: GroupUsage) => ConfirmPrompt, action: () => Promise<GroupDelete>, done: string) {
    const ref: GroupRef = { kind, gameId: group.gameId };
    ask(() => words(rowOf(report, ref) ?? group), action, done, `${kind}:${group.gameId}`, ref);
  }

  function askScreenshots(group: GroupUsage) {
    askGroup(
      'screenshots',
      group,
      (row) => ({
        title: msg('diskusage.screenshotsConfirmTitle'),
        text: msg('diskusage.screenshotsConfirmSized', {
          title: gameTitle(row),
          count: msg('diskusage.screenshotsCount', { count: row.count }),
          size: groupSize(row),
        }),
        confirm: msg('diskusage.deleteAll'),
        busy: msg('diskusage.deleting'),
      }),
      () => deleteScreenshots(group.gameId),
      msg('diskusage.screenshotsDeleted'),
    );
  }

  function askSnapshots(group: GroupUsage) {
    askGroup(
      'snapshots',
      group,
      (row) => ({
        title: msg('diskusage.snapshotsConfirmTitle'),
        text: msg('diskusage.snapshotsConfirmSized', {
          title: gameTitle(row),
          count: msg('diskusage.snapshotsCount', { count: row.count }),
          size: groupSize(row),
        }),
        note: msg('diskusage.snapshotsConfirmNote'),
        confirm: msg('diskusage.snapshotsConfirmAction'),
        busy: msg('diskusage.deleting'),
      }),
      () => deleteSnapshots(group.gameId),
      msg('diskusage.snapshotsDeleted'),
    );
  }

  return {
    get report() { return report; },
    get scanning() { return scanning; },
    get scanError() { return scanError; },
    get pending() { return pending; },
    closePending,
    load,
    refresh,
    leave,
    askDownload,
    askLeftover,
    askScreenshots,
    askSnapshots,
  };
}
