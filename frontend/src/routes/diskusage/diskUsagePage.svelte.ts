import { msg } from '../../lib/i18n';
import type { ConfirmPrompt } from '../../lib/confirm/prompts';
import { leftoverKindKey } from '../../lib/diskusage/usage';
import { installErrorText } from '../../lib/install/installErrors';
import {
  cachedUsage,
  deleteDownloadData,
  deleteLeftover,
  deleteScreenshots,
  deleteSnapshots,
  scanUsage,
  type DownloadUsage,
  type GroupUsage,
  type Leftover,
  type Report,
} from '../../lib/services/diskusage';
import { toast } from '../../lib/stores/toasts';

export type DiskUsagePage = ReturnType<typeof createDiskUsagePage>;

function failureText(err: unknown) {
  return installErrorText(err, msg('diskusage.deleteFailed'));
}

/**
 * What the disk usage page reads and deletes: the report (the cached one first, then a scan; an answer
 * overtaken by a newer scan is dropped), the scan's state and error, and the one confirmation every
 * delete goes through, which rescans when it is done. Created once by the page; the page starts
 * `load()` on mount and `refresh()` when a move ends.
 */
export function createDiskUsagePage() {
  let report = $state<Report | null>(null);
  let scanning = $state(false);
  let scanError = $state('');
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);
  let scanSerial = 0;

  async function refresh() {
    const serial = ++scanSerial;
    scanning = true;
    scanError = '';
    try {
      const next = await scanUsage();
      if (serial === scanSerial) report = next;
    } catch (err) {
      if (serial === scanSerial) scanError = installErrorText(err, msg('diskusage.scanFailed'));
    } finally {
      if (serial === scanSerial) scanning = false;
    }
  }

  async function load() {
    try {
      const cached = await cachedUsage();
      if (cached && !report) report = cached;
    } catch (err) {
      scanError = installErrorText(err, msg('diskusage.scanFailed'));
    }
    await refresh();
  }

  function ask(prompt: ConfirmPrompt, action: () => Promise<void>, done: string) {
    pending = {
      prompt,
      run: async () => {
        try {
          await action();
          toast(done, 'success');
        } catch (err) {
          toast(failureText(err), 'danger');
        }
        await refresh();
      },
    };
  }

  function askDownload(download: DownloadUsage) {
    ask(
      {
        title: msg('diskusage.downloadConfirmTitle'),
        text: msg('diskusage.downloadConfirmText', { name: download.name }),
        confirm: msg('diskusage.downloadDelete'),
        busy: msg('diskusage.deleting'),
      },
      () => deleteDownloadData(download.id),
      msg('diskusage.downloadDeleted'),
    );
  }

  function askLeftover(leftover: Leftover) {
    ask(
      {
        title: msg('diskusage.leftoverConfirmTitle'),
        text: msg('diskusage.leftoverConfirmText', {
          kind: msg(leftoverKindKey(leftover.kind)),
          path: leftover.path,
        }),
        confirm: msg('diskusage.leftoverDelete'),
        busy: msg('diskusage.deleting'),
      },
      () => deleteLeftover(leftover.path),
      msg('diskusage.leftoverDeleted'),
    );
  }

  function askScreenshots(group: GroupUsage) {
    ask(
      {
        title: msg('diskusage.screenshotsConfirmTitle'),
        text: msg('diskusage.screenshotsConfirmText', { title: group.title, count: group.count }),
        confirm: msg('diskusage.deleteAll'),
        busy: msg('diskusage.deleting'),
      },
      () => deleteScreenshots(group.gameId),
      msg('diskusage.screenshotsDeleted'),
    );
  }

  function askSnapshots(group: GroupUsage) {
    ask(
      {
        title: msg('diskusage.snapshotsConfirmTitle'),
        text: msg('diskusage.snapshotsConfirmText', { title: group.title, count: group.count }),
        note: msg('diskusage.snapshotsConfirmNote'),
        confirm: msg('diskusage.snapshotsConfirmAction'),
        busy: msg('diskusage.deleting'),
      },
      () => deleteSnapshots(group.gameId),
      msg('diskusage.snapshotsDeleted'),
    );
  }

  return {
    get report() { return report; },
    get scanning() { return scanning; },
    get scanError() { return scanError; },
    get pending() { return pending; },
    closePending() { pending = null; },
    load,
    refresh,
    askDownload,
    askLeftover,
    askScreenshots,
    askSnapshots,
  };
}
