import { onMount } from 'svelte';
import { Clipboard } from '@wailsio/runtime';
import { cloudErrorText } from '../../lib/cloudsaves/messages';
import { codeText, msg } from '../../lib/i18n';
import { forgetSyncPrompt, sendLogsPrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
import { forgetRemote, syncNow } from '../../lib/services/accountSync';
import { accountSyncReason } from '../../lib/services/accountSyncMessages';
import { inWails } from '../../lib/services/backend';
import { importExternalGames } from '../../lib/services/extlib';
import { importErrorText, summarizeImport } from '../../lib/library/extlibSummary';
import { cloudForget } from '../../lib/services/cloudsaves';
import { listLegalDocuments, type LegalMeta } from '../../lib/services/legal';
import { logsReason } from '../../lib/services/logsMessages';
import { onLogUploadStatus, sendLogs, type LogUploadStatus, type SendLogsResult } from '../../lib/services/logsUpload';
import { logsUploadErrorText } from '../../lib/services/logsUploadErrors';
import { onOverlayStatus, overlayStatus, type OverlayStatus } from '../../lib/services/overlay';
import { getSettings, openFolder, type ScheduleWindow, type Settings } from '../../lib/services/settings';
import {
  exportLogs,
  getAppInfo,
  getSystemInfo,
  getWineStatus,
  type AppInfo,
  type LogBundle,
  type SystemInfo,
  type WineStatus,
} from '../../lib/services/system';
import { refreshCloudStatus } from '../../lib/stores/cloudsaves';
import { settings, updateSettingsReporting } from '../../lib/stores/settings';
import { toast } from '../../lib/stores/toasts';

export type SettingsPage = ReturnType<typeof createSettingsPage>;

/**
 * The catalogued message of a failed overlay, screenshot or schedule save's code, else '': Go's own
 * words are never shown, each place says its own catalogued text instead.
 */
export function overlayReason(err: unknown): string {
  return codeText(err, '');
}

// The page's cloud work: the switch is held, saying which, so no dialog opens into it. It lives past
// the page: a deletion runs on after the player leaves Settings, and a page opened meanwhile holds the
// switch until it ends.
let cloudWork = $state<'' | 'saving' | 'deleting'>('');

/**
 * What the settings page keeps while its tabs come and go: the app, system and legal facts read on
 * opening, the logs export and send, the account sync actions, cloud saves, the overlay status, the
 * schedule's save error, the page's dialogs and its one confirmation. Called once while the page
 * initialises: its `onMount`s are the page's, registered in the page's order.
 */
export function createSettingsPage() {
  let appInfo = $state<AppInfo | null>(null);
  let appInfoFailed = $state(false);
  let systemInfo = $state<SystemInfo | null>(null);
  let systemFailed = $state(false);
  let wineStatus = $state<WineStatus | null>(null);

  let legalDocs = $state<LegalMeta[]>([]);
  // The list failed to load in the app, or this build has no legal documents to show (a browser).
  let legalState = $state<'' | 'failed' | 'unavailable'>('');
  let legalOpen = $state(false);
  let legalActiveId = $state<string | null>(null);
  let legalActiveTitle = $state('');
  let sourcesNoticeReviewOpen = $state(false);
  let sentDataOpen = $state(false);

  let logsBundle = $state<LogBundle | null>(null);
  let logsSaving = $state(false);
  let logsSending = $state(false);
  let logsStage = $state<LogUploadStatus['state'] | 'idle'>('idle');
  let logsProgress = $state<LogUploadStatus | null>(null);
  let logsSendResult = $state<SendLogsResult | null>(null);
  let logsSendFailure = $state('');

  let syncingNow = $state(false);
  let importing = $state(false);
  let importDetails = $state<string[]>([]);
  let forgettingRemote = $state(false);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => void | Promise<void> } | null>(null);

  onMount(() => {
    void refreshCloudStatus();
    const offLogs = onLogUploadStatus((status) => {
      logsStage = status.state;
      if (status.state === 'sending' && (status.totalBytes ?? 0) > 0) {
        logsProgress = status;
      } else if (status.state !== 'sending') {
        logsProgress = null;
      }
    });
    // Each read fails on its own: a failed one keeps its rows' unknown state, the others still run.
    void (async () => {
      try {
        appInfo = await getAppInfo();
      } catch (err) {
        appInfoFailed = true;
        console.warn('settings: app info', err);
      }
      try {
        systemInfo = await getSystemInfo();
      } catch {
        systemFailed = true;
      }
      try {
        wineStatus = await getWineStatus();
      } catch (err) {
        console.warn('settings: CrossOver status', err);
      }
      try {
        legalDocs = await listLegalDocuments();
      } catch {
        legalState = inWails ? 'failed' : 'unavailable';
      }
    })();
    return offLogs;
  });

  async function saveLogs() {
    if (!inWails) {
      toast(msg('settings.aboutLogsDesktopOnly'));
      return;
    }
    logsSaving = true;
    try {
      const bundle = await exportLogs();
      logsBundle = bundle;
      toast(msg('settings.aboutLogsSavedToast'), 'success');
      try {
        await openFolder(bundle.dir);
      } catch {
        toast(bundle.path);
      }
    } catch (err) {
      logsBundle = null;
      toast(logsReason(err), 'danger');
    } finally {
      logsSaving = false;
    }
  }

  function openSendLogsConfirm() {
    if (!inWails) {
      toast(msg('settings.aboutLogsDesktopOnly'));
      return;
    }
    if (logsSending) return;
    logsSendFailure = '';
    pending = { prompt: sendLogsPrompt(), run: confirmSendLogs };
  }

  function confirmSendLogs() {
    if (logsSending) return;
    logsSending = true;
    logsSendResult = null;
    logsStage = 'preparing';
    logsProgress = null;
    logsSendFailure = '';
    void sendLogs()
      .then((result) => {
        logsSendResult = result;
      })
      .catch((err) => {
        logsSendResult = null;
        logsStage = 'error';
        logsSendFailure = logsUploadErrorText(err);
      })
      .finally(() => {
        logsSending = false;
      });
  }

  async function copySendLogsId() {
    if (!logsSendResult) return;
    try {
      await Clipboard.SetText(logsSendResult.id);
      toast(msg('settings.aboutLogsSendIdCopiedToast'), 'info');
    } catch {
      toast(msg('settings.aboutLogsSendIdCopyFailedToast'), 'danger');
    }
  }

  async function runSyncNow() {
    if (syncingNow) return;
    syncingNow = true;
    try {
      await syncNow();
      toast(msg('settings.generalSyncDoneToast'), 'success');
    } catch (err) {
      toast(accountSyncReason(err, msg('settings.generalSyncFailedToast')), 'danger');
    } finally {
      syncingNow = false;
    }
  }

  async function runImportExternal() {
    if (importing) return;
    importing = true;
    importDetails = [];
    try {
      const report = summarizeImport(await importExternalGames());
      importDetails = report.details;
      toast(report.text, report.kind);
    } catch (err) {
      toast(importErrorText(err), 'danger');
    } finally {
      importing = false;
    }
  }

  function askForgetRemote() {
    if (forgettingRemote) return;
    pending = { prompt: forgetSyncPrompt(), run: runForgetRemote };
  }

  async function runForgetRemote() {
    if (forgettingRemote) return;
    forgettingRemote = true;
    try {
      await forgetRemote();
      settings.set(await getSettings());
      toast(msg('settings.generalSyncForgetSuccessToast'), 'success');
    } catch (err) {
      toast(accountSyncReason(err, msg('settings.generalSyncForgetFailedToast')), 'danger');
    } finally {
      forgettingRemote = false;
    }
  }

  let cloudConsentOpen = $state(false);
  let cloudDisableOpen = $state(false);
  let cloudForgetFailed = $state(false);

  // A failed cloud saves write is said by the dialog that stays open, not by a toast behind it; the
  // store has logged the error.
  const saidByDialog = () => {};

  function changeCloudSaves(on: boolean) {
    if (cloudWork) return;
    if (on) cloudConsentOpen = true;
    else cloudDisableOpen = true;
  }

  async function enableCloudSaves() {
    if (cloudWork) return;
    cloudWork = 'saving';
    try {
      if (await updateSettingsReporting({ cloudSaves: true }, saidByDialog)) {
        cloudConsentOpen = false;
        cloudForgetFailed = false;
        await refreshCloudStatus();
      }
    } finally {
      cloudWork = '';
    }
  }

  async function runCloudForget() {
    cloudForgetFailed = false;
    try {
      await cloudForget();
      toast(msg('cloudsaves.forgetDone'), 'success');
    } catch (err) {
      cloudForgetFailed = true;
      toast(cloudErrorText(err, msg('cloudsaves.forgetFailed')), 'danger');
    }
    await refreshCloudStatus();
  }

  async function disableCloudSaves(forget: boolean) {
    if (cloudWork) return;
    cloudWork = 'saving';
    try {
      if (!(await updateSettingsReporting({ cloudSaves: false }, saidByDialog))) return;
      cloudDisableOpen = false;
      if (forget) {
        cloudWork = 'deleting';
        await runCloudForget();
      } else {
        await refreshCloudStatus();
      }
    } finally {
      cloudWork = '';
    }
  }

  async function retryCloudForget() {
    if (cloudWork) return;
    cloudWork = 'deleting';
    try {
      await runCloudForget();
    } finally {
      cloudWork = '';
    }
  }

  function openLegalDoc(meta: LegalMeta) {
    legalActiveId = meta.id;
    legalActiveTitle = meta.title;
    legalOpen = true;
  }

  let librarySetupOpen = $state(false);
  let historyOpen = $state(false);

  let overlayInfo = $state<OverlayStatus | null>(null);
  let overlayInfoFailed = $state(false);
  let overlayError = $state('');

  async function loadOverlayStatus() {
    try {
      overlayInfo = await overlayStatus();
      overlayInfoFailed = false;
    } catch (err) {
      console.warn('overlay status', err);
      overlayInfoFailed = true;
    }
  }

  async function setOverlay(patch: Partial<Settings>) {
    overlayError = '';
    await updateSettingsReporting(patch, (err) => {
      const reason = overlayReason(err);
      overlayError = reason ? msg('settings.overlaySaveError', { reason }) : msg('settings.overlaySaveErrorPlain');
    });
    await loadOverlayStatus();
  }

  onMount(() => {
    const off = onOverlayStatus((status) => {
      overlayInfo = status;
      overlayInfoFailed = false;
    });
    void loadOverlayStatus();
    return off;
  });

  function openLibrarySetup() {
    if (!inWails) {
      toast(msg('settings.generalLibraryDesktopOnlyToast'));
      return;
    }
    librarySetupOpen = true;
  }

  let scheduleError = $state('');

  async function setSchedule(downloadSchedule: ScheduleWindow[]) {
    scheduleError = '';
    await updateSettingsReporting({ downloadSchedule }, (err) => {
      const reason = overlayReason(err);
      scheduleError = reason
        ? msg('settings.downloadsScheduleSaveError', { reason })
        : msg('settings.downloadsScheduleSaveErrorPlain');
    });
  }

  return {
    get appInfo() { return appInfo; },
    get appInfoFailed() { return appInfoFailed; },
    get systemInfo() { return systemInfo; },
    get systemFailed() { return systemFailed; },
    get wineStatus() { return wineStatus; },
    get legalDocs() { return legalDocs; },
    get legalState() { return legalState; },
    get legalOpen() { return legalOpen; },
    set legalOpen(open: boolean) { legalOpen = open; },
    get legalActiveId() { return legalActiveId; },
    get legalActiveTitle() { return legalActiveTitle; },
    get sourcesNoticeReviewOpen() { return sourcesNoticeReviewOpen; },
    set sourcesNoticeReviewOpen(open: boolean) { sourcesNoticeReviewOpen = open; },
    get sentDataOpen() { return sentDataOpen; },
    set sentDataOpen(open: boolean) { sentDataOpen = open; },
    get librarySetupOpen() { return librarySetupOpen; },
    set librarySetupOpen(open: boolean) { librarySetupOpen = open; },
    get historyOpen() { return historyOpen; },
    set historyOpen(open: boolean) { historyOpen = open; },
    get logsBundle() { return logsBundle; },
    get logsSaving() { return logsSaving; },
    get logsSending() { return logsSending; },
    get logsStage() { return logsStage; },
    get logsProgress() { return logsProgress; },
    get logsSendResult() { return logsSendResult; },
    get logsSendFailure() { return logsSendFailure; },
    get syncingNow() { return syncingNow; },
    get importing() { return importing; },
    get importDetails() { return importDetails; },
    get forgettingRemote() { return forgettingRemote; },
    get pending() { return pending; },
    get cloudConsentOpen() { return cloudConsentOpen; },
    set cloudConsentOpen(open: boolean) { cloudConsentOpen = open; },
    get cloudDisableOpen() { return cloudDisableOpen; },
    set cloudDisableOpen(open: boolean) { cloudDisableOpen = open; },
    get cloudBusy() { return cloudWork !== ''; },
    get cloudWork() { return cloudWork; },
    get cloudForgetFailed() { return cloudForgetFailed; },
    get overlayInfo() { return overlayInfo; },
    get overlayInfoFailed() { return overlayInfoFailed; },
    get overlayError() { return overlayError; },
    get scheduleError() { return scheduleError; },
    closePending() { pending = null; },
    saveLogs,
    openSendLogsConfirm,
    copySendLogsId,
    runSyncNow,
    runImportExternal,
    askForgetRemote,
    changeCloudSaves,
    enableCloudSaves,
    disableCloudSaves,
    retryCloudForget,
    openLegalDoc,
    setOverlay,
    openLibrarySetup,
    setSchedule,
  };
}
