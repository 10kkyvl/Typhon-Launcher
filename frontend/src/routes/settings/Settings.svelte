<script lang="ts">
  import Page from '../../lib/components/Page.svelte';
  import './settings-layout.css';
  import { Clipboard } from '@wailsio/runtime';
  import { Copy, Download, Eye, FolderOpen, HardDrive, LifeBuoy, ListChecks, RefreshCw, ScrollText, Send, Trash2 } from '@lucide/svelte';
  import { onMount, untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import CloudConsentModal from '../../lib/components/CloudConsentModal.svelte';
  import CloudDisableModal from '../../lib/components/CloudDisableModal.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import LegalDocumentModal from '../../lib/components/LegalDocumentModal.svelte';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import IntegerInput from '../../lib/components/IntegerInput.svelte';
  import RateLimitInput from '../../lib/components/RateLimitInput.svelte';
  import ScheduleEditor from '../../lib/components/ScheduleEditor.svelte';
  import Select from '../../lib/components/Select.svelte';
  import { cloudErrorText } from '../../lib/cloudsaves/messages';
  import { errorCode, hasMessage, msg } from '../../lib/i18n';
  import SentDataModal from '../../lib/components/SentDataModal.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import ReleaseNotesList from '../../lib/components/ReleaseNotesList.svelte';
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { tabPanel } from '../../lib/components/tabs';
  import Toggle from '../../lib/components/Toggle.svelte';
  import RollbackAction from '../../lib/components/RollbackAction.svelte';
  import StableOffer from '../../lib/components/StableOffer.svelte';
  import UpdateBanner from '../../lib/components/UpdateBanner.svelte';
  import DiscordJoinButton from '../../lib/components/DiscordJoinButton.svelte';
  import AppearanceTab from './AppearanceTab.svelte';
  import DiscordCard from './DiscordCard.svelte';
  import EmulatorsCard from './EmulatorsCard.svelte';
  import LanSettingsRow from './LanSettingsRow.svelte';
  import LibraryFoldersRow from './LibraryFoldersRow.svelte';
  import LibraryLocationRow from './LibraryLocationRow.svelte';
  import NetworkSettingsCard from './NetworkSettingsCard.svelte';
  import { forgetSyncPrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import { forgetRemote, syncNow } from '../../lib/services/accountSync';
  import { accountSyncReason } from '../../lib/services/accountSyncMessages';
  import { inWails } from '../../lib/services/backend';
  import { importExternalGames } from '../../lib/services/extlib';
  import { importErrorText, summarizeImport } from '../../lib/library/extlibSummary';
  import { cloudForget } from '../../lib/services/cloudsaves';
  import { sendLogsPrompt } from '../../lib/confirm/prompts';
  import { listLegalDocuments, type LegalMeta } from '../../lib/services/legal';
  import { logsReason } from '../../lib/services/logsMessages';
  import { onLogUploadStatus, sendLogs, type LogUploadStatus, type SendLogsResult } from '../../lib/services/logsUpload';
  import { logsUploadErrorText } from '../../lib/services/logsUploadErrors';
  import { SCREENSHOT_HOTKEYS } from '../../lib/services/screenshots';
  import { OVERLAY_HOTKEYS, onOverlayStatus, overlayStatus, type OverlayStatus } from '../../lib/services/overlay';
  import { getSettings, maxActiveDownloadOptions, openFolder, type GameMode, type ScheduleWindow, type Settings } from '../../lib/services/settings';
  import { defaultPlayDownloadLimit, defaultPlayUploadLimit } from '../../lib/services/playThrottle';
  import {
    exportLogs,
    elevationSupported,
    getAppInfo,
    getSystemInfo,
    getWineStatus,
    type AppInfo,
    type LogBundle,
    type SystemInfo,
    type WineStatus,
  } from '../../lib/services/system';
  import { releaseNotesHistory, requestCheck, rollbackInfo, selfUpdateChecking, selfUpdateStatus } from '../../lib/stores/selfupdate';
  import { rollbackShown } from '../../lib/services/selfupdate';
  import { rollbackReason } from '../../lib/services/selfupdateMessages';
  import { cloudStatus, refreshCloudStatus } from '../../lib/stores/cloudsaves';
  import { saveBackupsEnabled } from '../../lib/stores/savebackup';
  import { navigate } from '../../lib/stores/router';
  import { settings, updateSettings, updateSettingsReporting, updateSettingsResult } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { authState, currentUser } from '../../lib/stores/user';
  import { betaToggleShown, isBeta } from '../../lib/services/selfupdateChannel';
  import { bytesLabel, bytesSize, relativeDate } from '../../lib/utils/format';

  let { tab: initialTab }: { tab?: string } = $props();

  function tabOptions() {
    return [
      { id: 'general', label: msg('settings.generalTab') },
      { id: 'downloads', label: msg('settings.downloadsTab') },
      { id: 'appearance', label: msg('settings.appearanceTab') },
      { id: 'about', label: msg('settings.aboutTab') },
    ];
  }

  const tabs = $derived(tabOptions());

  let tab = $state('general');

  $effect(() => {
    const next = initialTab;
    untrack(() => {
      if (next && tabs.some((t) => t.id === next)) tab = next;
    });
  });

  const current = $derived($settings);
  const scaleValue = $derived(String(Math.round(($settings?.uiScale ?? 1) * 100)));

  let appInfo = $state<AppInfo | null>(null);
  let systemInfo = $state<SystemInfo | null>(null);
  let systemFailed = $state(false);
  let wineStatus = $state<WineStatus | null>(null);

  let legalDocs = $state<LegalMeta[]>([]);
  let legalError = $state('');
  let legalOpen = $state(false);
  let legalActiveId = $state<string | null>(null);
  let legalActiveTitle = $state('');
  let sourcesNoticeReviewOpen = $state(false);
  let sentDataOpen = $state(false);

  let logsBundle = $state<LogBundle | null>(null);
  let logsSaving = $state(false);
  let logsSending = $state(false);
  let logsStage = $state<LogUploadStatus['state'] | 'idle'>('idle');
  let logsProgress = $state<{ sentBytes: number; totalBytes: number } | null>(null);
  const logsProgressPercent = $derived(
    logsProgress ? Math.min(100, Math.round((logsProgress.sentBytes / logsProgress.totalBytes) * 100)) : 0,
  );
  let logsSendResult = $state<SendLogsResult | null>(null);
  let logsSendFailure = $state('');

  const accountReady = $derived($authState === 'authenticated');
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
        logsProgress = { sentBytes: status.sentBytes ?? 0, totalBytes: status.totalBytes ?? 0 };
      } else if (status.state !== 'sending') {
        logsProgress = null;
      }
    });
    void (async () => {
      appInfo = await getAppInfo();
      try {
        systemInfo = await getSystemInfo();
      } catch {
        systemFailed = true;
      }
      wineStatus = await getWineStatus();
      try {
        legalDocs = await listLegalDocuments();
      } catch {
        legalError = inWails ? msg('settings.aboutLegalLoadError') : msg('settings.aboutLegalUnavailable');
      }
    })();
    return offLogs;
  });

  function cpuLine(info: SystemInfo): string {
    const threads = msg('settings.aboutCpuThreads', { cores: info.cores });
    const cores = info.physicalCoresUnknown ? '' : `${msg('settings.aboutCpuCores', { cores: info.physicalCores })} · `;
    return `${info.cpuUnknown ? msg('settings.aboutUnknown') : info.cpu} · ${cores}${threads}`;
  }

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
  let cloudBusy = $state(false);
  let cloudForgetFailed = $state(false);

  const cloudUsage = $derived.by(() => {
    const status = $cloudStatus;
    if (!current?.cloudSaves || !status) return '';
    if (status.quotaBytes > 0) {
      return msg('cloudsaves.usage', { used: bytesSize(status.usedBytes), quota: bytesSize(status.quotaBytes) });
    }
    return msg('cloudsaves.usageUnknown', { used: bytesSize(status.usedBytes) });
  });

  function changeCloudSaves(on: boolean) {
    if (on) cloudConsentOpen = true;
    else cloudDisableOpen = true;
  }

  async function enableCloudSaves() {
    if (cloudBusy) return;
    cloudBusy = true;
    try {
      if (await updateSettingsResult({ cloudSaves: true })) {
        cloudConsentOpen = false;
        cloudForgetFailed = false;
        await refreshCloudStatus();
      }
    } finally {
      cloudBusy = false;
    }
  }

  async function runCloudForget() {
    cloudForgetFailed = false;
    try {
      await cloudForget();
    } catch (err) {
      cloudForgetFailed = true;
      toast(cloudErrorText(err, msg('cloudsaves.forgetFailed')), 'danger');
    }
    await refreshCloudStatus();
  }

  async function disableCloudSaves(forget: boolean) {
    if (cloudBusy) return;
    cloudBusy = true;
    try {
      if (!(await updateSettingsResult({ cloudSaves: false }))) return;
      cloudDisableOpen = false;
      if (forget) await runCloudForget();
      else await refreshCloudStatus();
    } finally {
      cloudBusy = false;
    }
  }

  async function retryCloudForget() {
    if (cloudBusy) return;
    cloudBusy = true;
    try {
      await runCloudForget();
    } finally {
      cloudBusy = false;
    }
  }

  function openLegalDoc(meta: LegalMeta) {
    legalActiveId = meta.id;
    legalActiveTitle = meta.title;
    legalOpen = true;
  }

  type PathKey = 'gamesPath' | 'downloadsPath' | 'screenshotsPath';

  function folderRowOptions(): { key: PathKey; label: string }[] {
    return [
      { key: 'gamesPath', label: msg('settings.generalLibraryFolderGames') },
      { key: 'downloadsPath', label: msg('settings.generalLibraryFolderDownloads') },
      { key: 'screenshotsPath', label: msg('settings.generalLibraryFolderScreenshots') },
    ];
  }

  const folderRows = $derived(folderRowOptions());

  let librarySetupOpen = $state(false);
  let historyOpen = $state(false);

  function set(patch: Partial<Settings>) {
    updateSettings(patch);
  }

  // Ids of the row labels that name the fields next to them.
  const uid = $props.id();
  const labelId = (name: string) => `${uid}-${name}-label`;
  const subId = (name: string) => `${uid}-${name}-sub`;

  const portable = $derived(appInfo?.portable ?? false);
  const betaShown = $derived(betaToggleShown($authState, $currentUser));
  const onBeta = $derived(isBeta($selfUpdateStatus.channel) || isBeta(appInfo?.channel));

  async function setBetaChannel(value: boolean) {
    const saved = await updateSettingsReporting({ updateBetaChannel: value }, () => toast(msg('state.settingsSaveFailed'), 'danger'));
    if (saved) await requestCheck();
  }

  async function setLaunchOnStartup(value: boolean) {
    await updateSettingsReporting({ launchOnStartup: value }, (err) => {
      const code = errorCode(err);
      toast(code && hasMessage(code) ? msg(code) : msg('state.settingsSaveFailed'), 'danger');
    });
  }

  const gameModeMac = $derived(appInfo?.platform === 'darwin');
  const trayEnabled = $derived(current?.minimizeToTray ?? true);

  function setGameMode(change: Partial<GameMode>) {
    const base = current?.gameMode ?? { highPerformance: false, hideLauncher: false, raisePriority: false };
    updateSettings({ gameMode: { ...base, ...change } });
  }

  let overlayInfo = $state<OverlayStatus | null>(null);
  let overlayInfoFailed = $state(false);
  let overlayError = $state('');

  const overlayHotkeyOptions = OVERLAY_HOTKEYS.map((key) => ({ id: key, label: key }));
  const overlayHotkey = $derived.by(() => {
    const id = current?.overlayHotkey ?? OVERLAY_HOTKEYS[0];
    return overlayHotkeyOptions.some((o) => o.id === id) ? id : OVERLAY_HOTKEYS[0];
  });
  const overlayStatusFailed = $derived(overlayInfoFailed || (!!overlayInfo?.supported && !!overlayInfo.error));
  const overlayStatusText = $derived.by(() => {
    if (overlayInfoFailed) return msg('settings.overlayStatusUnknown');
    if (!overlayInfo) return '';
    if (!overlayInfo.supported) return msg('settings.overlayStatusUnsupported');
    if (overlayInfo.error) return overlayReason(overlayInfo.error);
    if (overlayInfo.enabled) return msg('settings.overlayStatusOn', { hotkey: overlayInfo.hotkey });
    return msg('settings.overlayStatusOff');
  });

  async function loadOverlayStatus() {
    try {
      overlayInfo = await overlayStatus();
      overlayInfoFailed = false;
    } catch (err) {
      console.warn('overlay status', err);
      overlayInfoFailed = true;
    }
  }

  function overlayReason(err: unknown): string {
    const code = errorCode(err);
    if (code && hasMessage(code)) return msg(code);
    return err instanceof Error ? err.message : String(err);
  }

  async function setOverlay(patch: Partial<Settings>) {
    overlayError = '';
    await updateSettingsReporting(patch, (err) => {
      const reason = overlayReason(err);
      overlayError = reason ? msg('settings.overlaySaveError', { reason }) : msg('settings.overlaySaveErrorPlain');
    });
    await loadOverlayStatus();
  }

  const screenshotHotkeyOptions = SCREENSHOT_HOTKEYS.map((key) => ({ id: key, label: key }));
  const screenshotHotkey = $derived.by(() => {
    const id = current?.screenshotHotkey ?? SCREENSHOT_HOTKEYS[0];
    return screenshotHotkeyOptions.some((o) => o.id === id) ? id : SCREENSHOT_HOTKEYS[0];
  });

  async function setScreenshots(patch: Partial<Settings>) {
    await updateSettingsReporting(patch, (err) => {
      const reason = overlayReason(err);
      toast(
        reason ? msg('settings.screenshotsSaveError', { reason }) : msg('settings.screenshotsSaveErrorPlain'),
        'danger',
      );
    });
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

  async function openPath(path: string | undefined) {
    if (!path) return;
    try {
      await openFolder(path);
    } catch {
      toast(msg('settings.generalLibraryFolderUnavailableToast'), 'danger');
    }
  }

  const downloadLimitPresets = [10, 25, 50];
  const uploadLimitPresets = [1, 5, 10];
  const playDownloadPresets = [1, 2, 5, 10];
  const playUploadPresets = [0.5, 1, 2, 5];

  const playModeOptions = $derived([
    { id: 'off', label: msg('settings.downloadsPlayOff') },
    { id: 'pause', label: msg('settings.downloadsPlayPause') },
    { id: 'limit', label: msg('settings.downloadsPlayLimit') },
  ]);

  const playModeValue = $derived(
    current?.playDownloadMode === 'pause' || current?.playDownloadMode === 'limit' ? current.playDownloadMode : 'off',
  );

  function pickPlayMode(id: string) {
    if (id !== 'limit') {
      set({ playDownloadMode: id });
      return;
    }
    set({
      playDownloadMode: id,
      playDownloadRateLimit: current?.playDownloadRateLimit || defaultPlayDownloadLimit,
      playUploadRateLimit: current?.playUploadRateLimit || defaultPlayUploadLimit,
    });
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

  const maxActiveValue = $derived.by(() => {
    const id = String(current?.maxActiveDownloads ?? 2);
    return maxActiveDownloadOptions.some((o) => o.id === id) ? id : '2';
  });

  function cleanupPolicyOptionsList() {
    return [
      { id: 'keep', label: msg('settings.downloadsCleanupKeep') },
      { id: 'ask', label: msg('settings.downloadsCleanupAsk') },
      { id: 'delete', label: msg('settings.downloadsCleanupDelete') },
    ];
  }

  const cleanupPolicyOptions = $derived(cleanupPolicyOptionsList());

  const cleanupPolicy = $derived.by(() => {
    const id = current?.installCleanupPolicy ?? 'delete';
    return cleanupPolicyOptions.some((o) => o.id === id) ? id : 'delete';
  });

  function sourceRefreshOptionsList() {
    return [
      { id: 'manual', label: msg('settings.downloadsSourceRefreshManual') },
      { id: '1h', label: msg('settings.downloadsSourceRefreshHourly') },
      { id: '6h', label: msg('settings.downloadsSourceRefresh6h') },
      { id: '12h', label: msg('settings.downloadsSourceRefresh12h') },
      { id: '24h', label: msg('settings.downloadsSourceRefreshDaily') },
    ];
  }

  const sourceRefreshOptions = $derived(sourceRefreshOptionsList());

  const sourceRefreshInterval = $derived.by(() => {
    const id = current?.sourceRefreshInterval ?? '6h';
    return sourceRefreshOptions.some((o) => o.id === id) ? id : '6h';
  });

  function keepPreviousOptionsList() {
    return [
      { id: 'off', label: msg('settings.downloadsKeepPreviousOff') },
      { id: 'first_launch', label: msg('settings.downloadsKeepPreviousFirstLaunch') },
      { id: '24h', label: msg('settings.downloadsKeepPrevious24h') },
    ];
  }

  const keepPreviousOptions = $derived(keepPreviousOptionsList());

  const keepPreviousVersion = $derived.by(() => {
    const id = current?.keepPreviousVersion ?? 'first_launch';
    return keepPreviousOptions.some((o) => o.id === id) ? id : 'first_launch';
  });

</script>

<Page>
  <PageHeader title={msg('settings.pageTitle')} />

  <div class="tabs-wrap">
    <Tabs {tabs} idPrefix={uid} label={msg('settings.pageTitle')} divider={false} bind:value={tab} />
  </div>

  <div class="settings-page">
  {#key tab}
  <div class="tab-body" {...tabPanel(uid, tab)}>
  {#if tab === 'general'}
    <div class="settings-grid">
      <div class="settings-column">
        <Card title={msg('settings.generalStartupCardTitle')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-launch-on-startup')}>{msg('settings.generalLaunchOnStartupLabel')}</span>
                <span class="row-sub" id={subId('general-launch-on-startup')}>{portable ? msg('settings.generalLaunchOnStartupPortable') : msg('settings.generalLaunchOnStartupSub')}</span>
              </div>
              <Toggle
                checked={!portable && (current?.launchOnStartup ?? false)}
                disabled={portable}
                labelledby={labelId('general-launch-on-startup')}
                describedby={subId('general-launch-on-startup')}
                onchange={(v) => void setLaunchOnStartup(v)}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-minimize-to-tray')}>{msg('settings.generalMinimizeToTrayLabel')}</span>
                <span class="row-sub" id={subId('general-minimize-to-tray')}>{msg('settings.generalMinimizeToTraySub')}</span>
              </div>
              <Toggle
                checked={current?.minimizeToTray ?? true}
                labelledby={labelId('general-minimize-to-tray')}
                describedby={subId('general-minimize-to-tray')}
                onchange={(v) => set({ minimizeToTray: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-discord-rpc')}>{msg('settings.generalDiscordRpcLabel')}</span>
                <span class="row-sub" id={subId('general-discord-rpc')}>{msg('settings.generalDiscordRpcSub')}</span>
              </div>
              <Toggle
                checked={current?.discordRichPresence ?? false}
                labelledby={labelId('general-discord-rpc')}
                describedby={subId('general-discord-rpc')}
                onchange={(v) => set({ discordRichPresence: v })}
              />
            </div>
          </div>
        </Card>

        <Card title={msg('settings.gameModeCardTitle')}>
          <div class="rows">
            <span class="row-sub">{msg('settings.gameModeDuring')}</span>
            {#if !gameModeMac}
              <div class="row">
                <div class="row-text">
                  <span class="row-label" id={labelId('game-mode-power')}>{msg('settings.gameModePowerLabel')}</span>
                  <span class="row-sub" id={subId('game-mode-power')}>{msg('settings.gameModePowerSub')}</span>
                </div>
                <Toggle
                  checked={current?.gameMode?.highPerformance ?? false}
                  labelledby={labelId('game-mode-power')}
                  describedby={subId('game-mode-power')}
                  onchange={(v) => setGameMode({ highPerformance: v })}
                />
              </div>
            {/if}
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('game-mode-hide')}>{msg('settings.gameModeHideLabel')}</span>
                <span class="row-sub" id={subId('game-mode-hide')}>{msg('settings.gameModeHideSub')}</span>
                {#if !trayEnabled}
                  <span class="row-sub">{msg('settings.gameModeHideNeedsTray')}</span>
                {/if}
              </div>
              <Toggle
                checked={trayEnabled && (current?.gameMode?.hideLauncher ?? false)}
                disabled={!trayEnabled}
                labelledby={labelId('game-mode-hide')}
                describedby={subId('game-mode-hide')}
                onchange={(v) => setGameMode({ hideLauncher: v })}
              />
            </div>
            {#if !gameModeMac}
              <div class="row">
                <div class="row-text">
                  <span class="row-label" id={labelId('game-mode-priority')}>{msg('settings.gameModePriorityLabel')}</span>
                  <span class="row-sub" id={subId('game-mode-priority')}>{msg('settings.gameModePrioritySub')}</span>
                </div>
                <Toggle
                  checked={current?.gameMode?.raisePriority ?? false}
                  labelledby={labelId('game-mode-priority')}
                  describedby={subId('game-mode-priority')}
                  onchange={(v) => setGameMode({ raisePriority: v })}
                />
              </div>
            {/if}
          </div>
        </Card>

        <Card title={msg('settings.overlayCardTitle')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('overlay-enabled')}>{msg('settings.overlayEnabledLabel')}</span>
                <span class="row-sub" id={subId('overlay-enabled')}>{msg('settings.overlayEnabledSub')}</span>
                <span class="row-sub">{msg('settings.overlayExclusiveNote')}</span>
              </div>
              <Toggle
                checked={current?.overlayEnabled ?? true}
                labelledby={labelId('overlay-enabled')}
                describedby={subId('overlay-enabled')}
                onchange={(v) => setOverlay({ overlayEnabled: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('overlay-hotkey')}>{msg('settings.overlayHotkeyLabel')}</span>
                <span class="row-sub">{msg('settings.overlayHotkeySub')}</span>
              </div>
              <Select
                labelledby={labelId('overlay-hotkey')}
                value={overlayHotkey}
                width="22rem"
                options={overlayHotkeyOptions}
                onchange={(id) => setOverlay({ overlayHotkey: id })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('screenshots-enabled')}>{msg('settings.screenshotsEnabledLabel')}</span>
                <span class="row-sub" id={subId('screenshots-enabled')}>{msg('settings.screenshotsEnabledSub')}</span>
              </div>
              <Toggle
                checked={current?.screenshotsEnabled ?? true}
                labelledby={labelId('screenshots-enabled')}
                describedby={subId('screenshots-enabled')}
                onchange={(v) => setScreenshots({ screenshotsEnabled: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('screenshot-hotkey')}>{msg('settings.screenshotHotkeyLabel')}</span>
                <span class="row-sub">{msg('settings.screenshotHotkeySub')}</span>
              </div>
              <Select
                labelledby={labelId('screenshot-hotkey')}
                value={screenshotHotkey}
                width="22rem"
                options={screenshotHotkeyOptions}
                onchange={(id) => setScreenshots({ screenshotHotkey: id })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.overlayStatusLabel')}</span>
                <span class="row-sub" class:row-error={overlayStatusFailed} role="status">{overlayStatusText}</span>
                {#if overlayError}<span class="row-sub row-error" role="alert">{overlayError}</span>{/if}
              </div>
            </div>
          </div>
        </Card>

        <Card title={msg('settings.generalLibraryCardTitle')}>
          <div class="rows">
            <div class="row folder-row">
              <span class="row-label folder-label">{msg('settings.generalLibraryPathLabel')}</span>
              <div class="folder-controls">
                <input
                  class="input sm"
                  type="text"
                  readonly
                  placeholder={msg('settings.generalLibraryPathPlaceholder')}
                  value={current?.libraryPath ?? ''}
                />
                <Button size="sm" onclick={openLibrarySetup}>
                  {current?.libraryPath ? msg('common.edit') : msg('common.select')}
                </Button>
                <IconButton
                  label={msg('settings.generalLibraryOpenFolderLabel')}
                  size="sm"
                  disabled={!current?.libraryPath}
                  onclick={() => openPath(current?.libraryPath)}
                >
                  <FolderOpen size="1.6rem" strokeWidth={1.8} />
                </IconButton>
              </div>
            </div>
            {#if current?.libraryPath}
              {#each folderRows as folder (folder.key)}
                <div class="row folder-row">
                  <span class="row-label folder-label">{folder.label}</span>
                  <div class="folder-controls">
                    <input class="input sm" type="text" readonly value={current?.[folder.key] ?? ''} />
                    <IconButton label={msg('settings.generalLibraryOpenFolderLabel')} size="sm" onclick={() => openPath(current?.[folder.key])}>
                      <FolderOpen size="1.6rem" strokeWidth={1.8} />
                    </IconButton>
                  </div>
                </div>
              {/each}
            {:else}
              <span class="row-sub">
                {msg('settings.generalLibraryEmptyHint')}
              </span>
            {/if}
            {#if current?.libraryPath}
              <LibraryFoldersRow />
            {/if}
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('diskusage.settingsLabel')}</span>
                <span class="row-sub">{msg('diskusage.settingsSub')}</span>
              </div>
              <Button size="sm" onclick={() => navigate('diskusage')}>
                <HardDrive size="1.5rem" strokeWidth={1.8} />
                {msg('diskusage.settingsOpen')}
              </Button>
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.extlibLabel')}</span>
                <span class="row-sub">{msg('settings.extlibSub')}</span>
              </div>
              {#if appInfo?.platform === 'windows'}
                <Button size="sm" disabled={importing} onclick={runImportExternal}>
                  <RefreshCw size="1.5rem" strokeWidth={1.8} />
                  {importing ? msg('settings.extlibRunning') : msg('settings.extlibLabel')}
                </Button>
              {:else}
                <span class="row-sub">{msg('settings.extlibWindowsOnly')}</span>
              {/if}
            </div>
            {#if importDetails.length > 0}
              <details class="import-details" open>
                <summary class="row-label">{msg('settings.extlibDetailsTitle')}</summary>
                <ul>
                  {#each importDetails as line, index (index)}
                    <li class="row-sub">{line}</li>
                  {/each}
                </ul>
              </details>
            {/if}
            <LibraryLocationRow />
          </div>
        </Card>

        <Card title={msg('settings.generalExperimentalCardTitle')}>
          <div class="rows">
            <LanSettingsRow />
          </div>
        </Card>
      </div>

      <div class="settings-column">
        <Card title={msg('settings.generalInterfaceCardTitle')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('language')}>{msg('settings.language')}</span>
                <span class="row-sub">{msg('settings.languageSub')}</span>
              </div>
              <Select
                labelledby={labelId('language')}
                value={$settings?.language ?? 'system'}
                width="22rem"
                options={[
                  { id: 'system', label: msg('settings.languageSystem') },
                  { id: 'ru', label: 'Русский' },
                  { id: 'en', label: 'English' },
                ]}
                onchange={(id) => set({ language: id })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('ui-scale')}>{msg('settings.generalUiScaleLabel')}</span>
                <span class="row-sub">{msg('settings.generalUiScaleSub')}</span>
              </div>
              <Select
                labelledby={labelId('ui-scale')}
                value={scaleValue}
                width="22rem"
                options={[
                  { id: '90', label: '90%' },
                  { id: '100', label: msg('settings.generalUiScaleDefaultOption') },
                  { id: '110', label: '110%' },
                  { id: '125', label: '125%' },
                ]}
                onchange={(id) => set({ uiScale: Number(id) / 100 })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-animations')}>{msg('settings.generalAnimationsLabel')}</span>
                <span class="row-sub" id={subId('general-animations')}>{msg('settings.generalAnimationsSub')}</span>
              </div>
              <Toggle
                checked={current?.animationsEnabled ?? true}
                labelledby={labelId('general-animations')}
                describedby={subId('general-animations')}
                onchange={(v) => set({ animationsEnabled: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-hw-accel')}>{msg('settings.generalHwAccelLabel')}</span>
                <span class="row-sub" id={subId('general-hw-accel')}>{msg('settings.generalHwAccelSub')}</span>
              </div>
              <Toggle
                checked={current?.hardwareAcceleration ?? true}
                labelledby={labelId('general-hw-accel')}
                describedby={subId('general-hw-accel')}
                onchange={(v) => set({ hardwareAcceleration: v })}
              />
            </div>
          </div>
        </Card>

        <Card title={msg('settings.generalPrivacyCardTitle')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-privacy-stats')}>{msg('settings.generalPrivacyStatsLabel')}</span>
                <span class="row-sub" id={subId('general-privacy-stats')}>{msg('settings.generalPrivacyStatsSub')}</span>
              </div>
              <Toggle
                checked={current?.anonymousUsageStats ?? false}
                labelledby={labelId('general-privacy-stats')}
                describedby={subId('general-privacy-stats')}
                onchange={(v) => set({ anonymousUsageStats: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-privacy-diag')}>{msg('settings.generalPrivacyDiagLabel')}</span>
                <span class="row-sub" id={subId('general-privacy-diag')}>{msg('settings.generalPrivacyDiagSub')}</span>
              </div>
              <Toggle
                checked={current?.anonymousDiagnostics ?? false}
                labelledby={labelId('general-privacy-diag')}
                describedby={subId('general-privacy-diag')}
                onchange={(v) => set({ anonymousDiagnostics: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-privacy-heartbeat')}>{msg('settings.generalPrivacyHeartbeatLabel')}</span>
                <span class="row-sub" id={subId('general-privacy-heartbeat')}>{msg('settings.generalPrivacyHeartbeatSub')}</span>
              </div>
              <Toggle
                checked={current?.sessionHeartbeat ?? true}
                labelledby={labelId('general-privacy-heartbeat')}
                describedby={subId('general-privacy-heartbeat')}
                onchange={(v) => set({ sessionHeartbeat: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.generalPrivacyShowSentLabel')}</span>
                <span class="row-sub">{msg('settings.generalPrivacyShowSentSub')}</span>
              </div>
              <Button size="sm" onclick={() => (sentDataOpen = true)}>
                <Eye size="1.5rem" strokeWidth={1.8} />
                {msg('settings.generalPrivacyShowSentButton')}
              </Button>
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.generalPrivacySessionStateLabel')}</span>
                <span class="row-sub">{msg('settings.generalPrivacySessionStateSub')}</span>
                <button
                  type="button"
                  class="privacy-link"
                  onclick={() => openLegalDoc({ id: 'privacy', title: msg('settings.generalPrivacyPolicyLinkLabel') })}
                >
                  {msg('settings.generalPrivacyPolicyLinkLabel')}
                </button>
              </div>
            </div>
          </div>
        </Card>

        <Card title={msg('settings.generalSyncCardTitle')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-sync-enable')}>{msg('settings.generalSyncEnableLabel')}</span>
                <span class="row-sub" id={subId('general-sync-enable')}>{msg('settings.generalSyncEnableSub')}</span>
                {#if !accountReady}
                  <span class="row-sub">{msg('settings.generalSyncNeedsAccountSub')}</span>
                {/if}
              </div>
              <Toggle
                checked={current?.accountSync ?? false}
                labelledby={labelId('general-sync-enable')}
                describedby={subId('general-sync-enable')}
                disabled={!accountReady}
                onchange={(v) => set({ accountSync: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('general-auto-away')}>{msg('settings.generalAutoAwayLabel')}</span>
                <span class="row-sub" id={subId('general-auto-away')}>{msg('settings.generalAutoAwaySub')}</span>
              </div>
              <Toggle
                checked={current?.presenceAutoAway ?? true}
                labelledby={labelId('general-auto-away')}
                describedby={subId('general-auto-away')}
                disabled={!accountReady || !current?.accountSync}
                onchange={(v) => set({ presenceAutoAway: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.generalSyncNowLabel')}</span>
                <span class="row-sub">{msg('settings.generalSyncNowSub')}</span>
              </div>
              <Button size="sm" disabled={!accountReady || !current?.accountSync || syncingNow} onclick={runSyncNow}>
                <RefreshCw size="1.5rem" strokeWidth={1.8} />
                {syncingNow ? msg('settings.generalSyncNowRunning') : msg('settings.generalSyncNowLabel')}
              </Button>
            </div>
            <div class="row danger-zone">
              <div class="row-text">
                <span class="row-label">{msg('settings.generalSyncForgetLabel')}</span>
                <span class="row-sub">{msg('settings.generalSyncForgetSub')}</span>
              </div>
              <Button
                size="sm"
                variant="danger"
                disabled={!accountReady || forgettingRemote}
                onclick={askForgetRemote}
              >
                <Trash2 size="1.5rem" strokeWidth={1.8} />
                {forgettingRemote ? msg('settings.generalSyncForgetRunning') : msg('settings.generalSyncForgetLabel')}
              </Button>
            </div>
          </div>
        </Card>

        {#if accountReady}
          <DiscordCard />
        {/if}
      </div>
    </div>
  {:else if tab === 'downloads'}
    <div class="settings-grid">
      <div class="settings-column">
        <Card title={msg('settings.downloadsTab')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('download-limit')}>{msg('settings.downloadsRateLimitLabel')}</span>
                <span class="row-sub">{msg('settings.downloadsRateLimitSub')}</span>
              </div>
              <RateLimitInput
                labelledby={labelId('download-limit')}
                value={current?.downloadRateLimit ?? 0}
                presets={downloadLimitPresets}
                onchange={(bytes) => set({ downloadRateLimit: bytes })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('upload-limit')}>{msg('settings.downloadsUploadLimitLabel')}</span>
                <span class="row-sub">{msg('settings.downloadsUploadLimitSub')}</span>
              </div>
              <RateLimitInput
                labelledby={labelId('upload-limit')}
                value={current?.uploadRateLimit ?? 0}
                presets={uploadLimitPresets}
                onchange={(bytes) => set({ uploadRateLimit: bytes })}
              />
            </div>
            <div class='row'>
              <div class='row-text'>
                <span class='row-label' id={labelId('play-mode')}>{msg('settings.downloadsPlayLabel')}</span>
                <span class='row-sub'>{msg('settings.downloadsPlaySub')}</span>
              </div>
              <Select labelledby={labelId('play-mode')} value={playModeValue} width='20rem' options={playModeOptions} onchange={pickPlayMode} />
            </div>
            {#if playModeValue === 'limit'}
              <div class='row'>
                <div class='row-text'>
                  <span class='row-label' id={labelId('play-download-limit')}>{msg('settings.downloadsPlayLimitDownLabel')}</span>
                </div>
                <RateLimitInput
                  labelledby={labelId('play-download-limit')}
                  value={current?.playDownloadRateLimit ?? 0}
                  presets={playDownloadPresets}
                  allowUnlimited={false}
                  onchange={(bytes) => set({ playDownloadRateLimit: bytes })}
                />
              </div>
              <div class='row'>
                <div class='row-text'>
                  <span class='row-label' id={labelId('play-upload-limit')}>{msg('settings.downloadsPlayLimitUpLabel')}</span>
                </div>
                <RateLimitInput
                  labelledby={labelId('play-upload-limit')}
                  value={current?.playUploadRateLimit ?? 0}
                  presets={playUploadPresets}
                  allowUnlimited={false}
                  onchange={(bytes) => set({ playUploadRateLimit: bytes })}
                />
              </div>
            {/if}
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.downloadsScheduleLabel')}</span>
                <span class="row-sub">{msg('settings.downloadsScheduleSub')}</span>
                {#if scheduleError}<span class="row-sub row-error" role="alert">{scheduleError}</span>{/if}
              </div>
              <div class="schedule-body">
                <ScheduleEditor windows={current?.downloadSchedule ?? []} onchange={setSchedule} />
              </div>
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('max-active')}>{msg('settings.downloadsMaxActiveLabel')}</span>
                <span class="row-sub">{msg('settings.downloadsMaxActiveSub')}</span>
              </div>
              <Select
                labelledby={labelId('max-active')}
                value={maxActiveValue}
                width="20rem"
                options={maxActiveDownloadOptions}
                onchange={(id) => set({ maxActiveDownloads: Number(id) })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-upload-while-downloading')}>{msg('settings.downloadsUploadWhileDownloadingLabel')}</span>
                <span class="row-sub" id={subId('downloads-upload-while-downloading')}>{msg('settings.downloadsUploadWhileDownloadingSub')}</span>
              </div>
              <Toggle
                checked={current?.uploadWhileDownloading ?? false}
                labelledby={labelId('downloads-upload-while-downloading')}
                describedby={subId('downloads-upload-while-downloading')}
                onchange={(v) => set({ uploadWhileDownloading: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-seed-after')}>{msg('settings.downloadsSeedAfterLabel')}</span>
                <span class="row-sub" id={subId('downloads-seed-after')}>{msg('settings.downloadsSeedAfterSub')}</span>
              </div>
              <Toggle
                checked={current?.seedAfterDownload ?? false}
                labelledby={labelId('downloads-seed-after')}
                describedby={subId('downloads-seed-after')}
                onchange={(v) => set({ seedAfterDownload: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('source-refresh')}>{msg('settings.downloadsSourceRefreshLabel')}</span>
                <span class="row-sub">{msg('settings.downloadsSourceRefreshSub')}</span>
              </div>
              <Select
                labelledby={labelId('source-refresh')}
                value={sourceRefreshInterval}
                width="20rem"
                options={sourceRefreshOptions}
                onchange={(id) => set({ sourceRefreshInterval: id })}
              />
            </div>
          </div>
        </Card>

        <NetworkSettingsCard />

        <Card title={msg('settings.downloadsUpdatesCardTitle')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-update-check-auto')}>{msg('settings.downloadsUpdateCheckAutoLabel')}</span>
                <span class="row-sub" id={subId('downloads-update-check-auto')}>{msg('settings.downloadsUpdateCheckAutoSub')}</span>
              </div>
              <Toggle
                checked={current?.updateCheckAutomatically ?? true}
                labelledby={labelId('downloads-update-check-auto')}
                describedby={subId('downloads-update-check-auto')}
                onchange={(v) => set({ updateCheckAutomatically: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-update-auto-download')}>{msg('settings.downloadsUpdateAutoDownloadLabel')}</span>
                <span class="row-sub" id={subId('downloads-update-auto-download')}>{msg('settings.downloadsUpdateAutoDownloadSub')}</span>
              </div>
              <Toggle
                checked={current?.updateAutoDownload ?? false}
                labelledby={labelId('downloads-update-auto-download')}
                describedby={subId('downloads-update-auto-download')}
                onchange={(v) => set({ updateAutoDownload: v })}
              />
            </div>
            {#if betaShown}
              <div class="row">
                <div class="row-text">
                  <span class="row-label" id={labelId('downloads-beta')}>{msg('settings.downloadsBetaLabel')}{#if onBeta} · {msg('settings.downloadsBetaChannelMark')}{/if}</span>
                  <span class="row-sub" id={subId('downloads-beta')}>{msg('settings.downloadsBetaSub')}</span>
                </div>
                <Toggle
                  checked={current?.updateBetaChannel ?? false}
                  labelledby={labelId('downloads-beta')}
                  describedby={subId('downloads-beta')}
                  onchange={setBetaChannel}
                />
              </div>
            {/if}
            <StableOffer />
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-save-backup')}>{msg('settings.downloadsSaveBackupLabel')}</span>
                <span class="row-sub" id={subId('downloads-save-backup')}>{msg('settings.downloadsSaveBackupSub')}</span>
              </div>
              <Toggle
                checked={current?.updateSaveBackup ?? true}
                labelledby={labelId('downloads-save-backup')}
                describedby={subId('downloads-save-backup')}
                onchange={(v) => set({ updateSaveBackup: v })}
              />
            </div>
            {#if $saveBackupsEnabled}
              <div class="row">
                <div class="row-text">
                  <span class="row-label" id={labelId('downloads-save-backup-after-session')}>{msg('settings.downloadsSaveBackupAfterSessionLabel')}</span>
                  <span class="row-sub" id={subId('downloads-save-backup-after-session')}>{msg('settings.downloadsSaveBackupAfterSessionSub')}</span>
                </div>
                <Toggle
                  checked={current?.saveBackupAfterSession ?? true}
                  labelledby={labelId('downloads-save-backup-after-session')}
                  describedby={subId('downloads-save-backup-after-session')}
                  onchange={(v) => set({ saveBackupAfterSession: v })}
                />
              </div>
              <div class="row">
                <div class="row-text">
                  <span class="row-label" id={labelId('downloads-save-backup-before-launch')}>{msg('settings.downloadsSaveBackupBeforeLaunchLabel')}</span>
                  <span class="row-sub" id={subId('downloads-save-backup-before-launch')}>{msg('settings.downloadsSaveBackupBeforeLaunchSub')}</span>
                </div>
                <Toggle
                  checked={current?.saveBackupBeforeLaunch ?? true}
                  labelledby={labelId('downloads-save-backup-before-launch')}
                  describedby={subId('downloads-save-backup-before-launch')}
                  onchange={(v) => set({ saveBackupBeforeLaunch: v })}
                />
              </div>
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.downloadsSaveBackupLimitLabel')}</span>
                  <span class="row-sub">{msg('settings.downloadsSaveBackupLimitSub')}</span>
                </div>
                <IntegerInput
                  value={current?.saveBackupLimit ?? 5}
                  min={1}
                  max={50}
                  label={msg('settings.downloadsSaveBackupLimitField')}
                  onchange={(n) => set({ saveBackupLimit: n })}
                />
              </div>
            {/if}
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('settings')}>{msg('cloudsaves.settingsLabel')}</span>
                <span class="row-sub" id={subId('settings')}>{msg('cloudsaves.settingsSub')}</span>
                {#if !accountReady}
                  <span class="row-sub">{msg('cloudsaves.settingsNeedsAccount')}</span>
                {:else if cloudUsage}
                  <span class="row-sub">{cloudUsage}{$cloudStatus?.pending ? ` · ${msg('cloudsaves.pending')}` : ''}</span>
                {/if}
                {#if cloudForgetFailed && !current?.cloudSaves}
                  <span class="row-sub">{msg('cloudsaves.retryForgetHint')}</span>
                {/if}
              </div>
              {#if cloudForgetFailed && !current?.cloudSaves}
                <Button size="sm" variant="danger" disabled={cloudBusy || !accountReady} onclick={retryCloudForget}>
                  {msg('cloudsaves.retryForget')}
                </Button>
              {/if}
              <Toggle
                checked={current?.cloudSaves ?? false}
                labelledby={labelId('settings')}
                describedby={subId('settings')}
                disabled={!accountReady && !current?.cloudSaves}
                onchange={changeCloudSaves}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('keep-previous')}>{msg('settings.downloadsKeepPreviousLabel')}</span>
                <span class="row-sub">{msg('settings.downloadsKeepPreviousSub')}</span>
              </div>
              <Select
                labelledby={labelId('keep-previous')}
                value={keepPreviousVersion}
                width="28rem"
                options={keepPreviousOptions}
                onchange={(id) => set({ keepPreviousVersion: id })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-torrent-reuse')}>{msg('settings.downloadsTorrentReuseLabel')}</span>
                <span class="row-sub" id={subId('downloads-torrent-reuse')}>{msg('settings.downloadsTorrentReuseSub')}</span>
              </div>
              <Toggle
                checked={current?.allowTorrentReuse ?? true}
                labelledby={labelId('downloads-torrent-reuse')}
                describedby={subId('downloads-torrent-reuse')}
                onchange={(v) => set({ allowTorrentReuse: v })}
              />
            </div>
          </div>
        </Card>
      </div>
      <div class="settings-column">
        <Card title={msg('settings.downloadsInstallCardTitle')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('after-install')}>{msg('settings.downloadsAfterInstallLabel')}</span>
                <span class="row-sub">{msg('settings.downloadsAfterInstallSub')}</span>
              </div>
              <Select
                labelledby={labelId('after-install')}
                value={cleanupPolicy}
                width="26rem"
                options={cleanupPolicyOptions}
                onchange={(id) => set({ installCleanupPolicy: id })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-auto-install')}>{msg('settings.downloadsAutoInstallLabel')}</span>
                <span class="row-sub" id={subId('downloads-auto-install')}>{msg('settings.downloadsAutoInstallSub')}</span>
              </div>
              <Toggle
                checked={current?.autoInstall ?? false}
                labelledby={labelId('downloads-auto-install')}
                describedby={subId('downloads-auto-install')}
                onchange={(v) => set({ autoInstall: v })}
              />
            </div>
            {#if appInfo && elevationSupported(appInfo)}
              <div class="row">
                <div class="row-text">
                  <span class="row-label" id={labelId('downloads-elevate-ahead')}>{msg('settings.downloadsElevateAheadLabel')}</span>
                  <span class="row-sub" id={subId('downloads-elevate-ahead')}>{msg('settings.downloadsElevateAheadSub')}</span>
                </div>
                <Toggle
                  checked={current?.elevateAhead ?? false}
                  labelledby={labelId('downloads-elevate-ahead')}
                  describedby={subId('downloads-elevate-ahead')}
                  onchange={(v) => set({ elevateAhead: v })}
                />
              </div>
            {/if}
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-skip-shortcuts')}>{msg('settings.downloadsSkipShortcutsLabel')}</span>
                <span class="row-sub" id={subId('downloads-skip-shortcuts')}>{msg('settings.downloadsSkipShortcutsSub')}</span>
              </div>
              <Toggle
                checked={current?.installSkipShortcuts ?? true}
                labelledby={labelId('downloads-skip-shortcuts')}
                describedby={subId('downloads-skip-shortcuts')}
                onchange={(v) => set({ installSkipShortcuts: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-desktop-shortcuts')}>{msg('settings.downloadsDesktopShortcutsLabel')}</span>
                <span class="row-sub" id={subId('downloads-desktop-shortcuts')}>{msg('settings.downloadsDesktopShortcutsSub')}</span>
              </div>
              <Toggle
                checked={current?.desktopShortcuts ?? true}
                labelledby={labelId('downloads-desktop-shortcuts')}
                describedby={subId('downloads-desktop-shortcuts')}
                onchange={(v) => set({ desktopShortcuts: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-skip-extras')}>{msg('settings.downloadsSkipExtrasLabel')}</span>
                <span class="row-sub" id={subId('downloads-skip-extras')}>{msg('settings.downloadsSkipExtrasSub')}</span>
              </div>
              <Toggle
                checked={current?.installSkipExtras ?? true}
                labelledby={labelId('downloads-skip-extras')}
                describedby={subId('downloads-skip-extras')}
                onchange={(v) => set({ installSkipExtras: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-verify-after-install')}>{msg('settings.downloadsVerifyAfterInstallLabel')}</span>
                <span class="row-sub" id={subId('downloads-verify-after-install')}>{msg('settings.downloadsVerifyAfterInstallSub')}</span>
              </div>
              <Toggle
                checked={current?.verifyAfterInstall ?? true}
                labelledby={labelId('downloads-verify-after-install')}
                describedby={subId('downloads-verify-after-install')}
                onchange={(v) => set({ verifyAfterInstall: v })}
              />
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label" id={labelId('downloads-verify-repack')}>{msg('settings.downloadsVerifyRepackLabel')}</span>
                <span class="row-sub" id={subId('downloads-verify-repack')}>{msg('settings.downloadsVerifyRepackSub')}</span>
              </div>
              <Toggle
                checked={current?.installVerifyRepack ?? false}
                labelledby={labelId('downloads-verify-repack')}
                describedby={subId('downloads-verify-repack')}
                onchange={(v) => set({ installVerifyRepack: v })}
              />
            </div>
          </div>
        </Card>

        <EmulatorsCard />
      </div>
    </div>
  {:else if tab === 'appearance'}
    <AppearanceTab />
  {:else if tab === 'about'}
    <div class="settings-grid">
      <div class="settings-column">
        <Card>
          <div class="about-logo">
            <img src="/typhon.png" alt="" width="44" height="44" draggable="false" />
            <div>
              <h3>Typhon Launcher</h3>
              <span class="row-sub">{msg(onBeta ? 'settings.aboutVersionBetaLabel' : 'settings.aboutVersionLabel', { version: appInfo?.version ?? '—' })} · {appInfo?.platform ?? ''}/{appInfo?.arch ?? ''}{appInfo?.devMock ? ' · devmock' : ''}</span>
            </div>
          </div>
          <div class="rows">
            {#if appInfo?.portable}
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutPortableLabel')}</span>
                  <span class="row-sub">{msg('settings.aboutPortableDataDir', { path: appInfo.dataDir })}</span>
                </div>
              </div>
            {/if}
            {#if systemInfo}
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutSystemLabel')}</span>
                  <span class="row-sub">{systemInfo.osUnknown ? msg('settings.aboutUnknown') : systemInfo.os}</span>
                </div>
              </div>
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutCpuLabel')}</span>
                  <span class="row-sub">{cpuLine(systemInfo)}</span>
                </div>
              </div>
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutMemoryLabel')}</span>
                  <span class="row-sub">{systemInfo.ramBytes > 0 ? bytesLabel(systemInfo.ramBytes) : msg('settings.aboutUnknown')}</span>
                </div>
              </div>
            {:else if systemFailed}
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutSystemLabel')}</span>
                  <span class="row-sub">{msg('settings.aboutSystemFailed')}</span>
                </div>
              </div>
            {/if}
            {#if wineStatus?.required}
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutRuntimeLabel')}</span>
                  <span class="row-sub">
                    {#if wineStatus.installed}
                      CrossOver {wineStatus.version}
                    {:else}
                      {msg('settings.aboutRuntimeMissing')}
                    {/if}
                  </span>
                </div>
              </div>
            {/if}
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.aboutCheckUpdatesLabel')}</span>
                <span class="row-sub">
                  {#if $selfUpdateChecking}
                    {msg('settings.aboutCheckUpdatesChecking')}
                  {:else}
                    {msg('settings.aboutInstalledVersion', { version: $selfUpdateStatus.currentVersion || appInfo?.version || '—' })}
                    {#if $selfUpdateStatus.checkedAt}
                      · {msg('settings.aboutCheckedAt', { date: relativeDate($selfUpdateStatus.checkedAt) })}
                    {/if}
                  {/if}
                </span>
              </div>
              <Button size="sm" disabled={$selfUpdateChecking} onclick={requestCheck}>
                <ListChecks size="1.5rem" strokeWidth={1.8} />
                {$selfUpdateChecking ? msg('settings.aboutCheckingEllipsis') : msg('settings.aboutCheckButtonLabel')}
              </Button>
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.aboutHistoryLabel')}</span>
                <span class="row-sub">
                  {#if $releaseNotesHistory.length > 0}
                    {msg('settings.aboutHistoryHasNotes')}
                  {:else}
                    {msg('settings.aboutHistoryEmpty')}
                  {/if}
                </span>
              </div>
              <Button size="sm" disabled={$releaseNotesHistory.length === 0} onclick={() => (historyOpen = true)}>
                <ScrollText size="1.5rem" strokeWidth={1.8} />
                {msg('settings.aboutHistoryButtonLabel')}
              </Button>
            </div>
            {#if rollbackShown($rollbackInfo)}
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutRollbackLabel')}</span>
                  <span class="row-sub">
                    {#if $rollbackInfo.available}
                      {msg('settings.aboutRollbackAvailable', { version: $rollbackInfo.version })}
                    {:else}
                      {rollbackReason($rollbackInfo)}
                    {/if}
                  </span>
                </div>
                <RollbackAction />
              </div>
            {/if}
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.aboutLicenseLabel')}</span>
                <span class="row-sub">{msg('settings.aboutLicenseText')}</span>
              </div>
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('discord.communityLabel')}</span>
                <span class="row-sub">{msg('discord.communitySub')}</span>
              </div>
              <DiscordJoinButton />
            </div>
            <UpdateBanner />
            <StableOffer />
          </div>
        </Card>

      </div>
      <div class="settings-column">
        <Card title={msg('settings.aboutDiagnosticsCardTitle')}>
          <div class="rows">
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.aboutLogsLabel')}</span>
                <span class="row-sub">
                  {#if logsBundle}
                    {logsBundle.name} · {bytesLabel(logsBundle.sizeBytes)} {msg('settings.aboutLogsAttachHint')}
                  {:else}
                    {msg('settings.aboutLogsSaveHint')}
                  {/if}
                </span>
              </div>
              <Button size="sm" disabled={logsSaving} onclick={saveLogs}>
                <Download size="1.5rem" strokeWidth={1.8} />
                {logsSaving ? msg('settings.aboutLogsSavingEllipsis') : msg('settings.aboutLogsDownloadButton')}
              </Button>
            </div>
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('settings.aboutLogsSendButton')}</span>
                <span class="row-sub">
                  {#if logsStage === 'preparing'}
                    {msg('settings.aboutLogsPreparingEllipsis')}
                  {:else if logsStage === 'sending' && logsProgress}
                    {msg('settings.aboutLogsSendingProgress', { percent: logsProgressPercent })}
                  {:else if logsStage === 'sending'}
                    {msg('settings.aboutLogsSendingEllipsis')}
                  {:else if logsStage === 'waiting'}
                    {msg('settings.aboutLogsWaitingEllipsis')}
                  {:else if logsSendFailure}
                    {logsSendFailure}
                  {:else if logsSendResult}
                    {msg('settings.aboutLogsSentStatus')}
                  {:else}
                    {msg('settings.aboutLogsSendConfirmNote')}
                  {/if}
                </span>
                {#if logsSending}
                  <div class="logs-upload-progress">
                    <ProgressBar
                      value={logsProgressPercent}
                      indeterminate={logsStage !== 'sending' || !logsProgress}
                      height={5}
                      label={msg('settings.aboutLogsLabel')}
                    />
                  </div>
                {/if}
              </div>
              <Button size="sm" disabled={logsSending} onclick={openSendLogsConfirm}>
                <Send size="1.5rem" strokeWidth={1.8} />
                {logsSending ? msg('settings.aboutLogsSendingEllipsis') : msg('settings.aboutLogsSendButton')}
              </Button>
            </div>
            {#if logsSendResult}
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutLogsSendResultLabel')}</span>
                  <span class="row-sub logs-send-id">{logsSendResult.id}</span>
                </div>
                <IconButton label={msg('settings.aboutLogsSendIdCopyLabel')} size="sm" onclick={copySendLogsId}>
                  <Copy size="1.5rem" strokeWidth={1.8} />
                </IconButton>
              </div>
              {#if logsSendResult.dropped.length > 0}
                <p class="row-sub">{msg('settings.aboutLogsSendDroppedNote', { files: logsSendResult.dropped.join(', ') })}</p>
              {/if}
            {/if}
            {#if logsSendFailure}
              <p class="row-sub">{msg('settings.aboutLogsSendFailedHint')}</p>
            {/if}
            <div class="row">
              <div class="row-text">
                <span class="row-label">{msg('tickets.settingsLabel')}</span>
                <span class="row-sub">{msg('tickets.settingsHint')}</span>
              </div>
              <Button size="sm" onclick={() => navigate('support', { view: 'new', topic: 'launcher' })}>
                <LifeBuoy size="1.5rem" strokeWidth={1.8} />
                {msg('tickets.writeSupport')}
              </Button>
            </div>
          </div>
        </Card>

        <Card title={msg('settings.aboutLegalCardTitle')}>
          {#if legalError}
            <p class="row-sub">{legalError}</p>
          {:else}
            <div class="rows">
              {#each legalDocs as meta (meta.id)}
                <div class="row">
                  <div class="row-text">
                    <span class="row-label">{meta.title}</span>
                  </div>
                  <Button size="sm" onclick={() => openLegalDoc(meta)}>{msg('common.open')}</Button>
                </div>
              {/each}
              <div class="row">
                <div class="row-text">
                  <span class="row-label">{msg('settings.aboutSourcesNoticeLabel')}</span>
                  <span class="row-sub">{msg('settings.aboutSourcesNoticeSub')}</span>
                </div>
                <Button size="sm" onclick={() => (sourcesNoticeReviewOpen = true)}>{msg('common.open')}</Button>
              </div>
            </div>
          {/if}
        </Card>
      </div>
    </div>
  {/if}
  </div>
  {/key}

  </div>

  <LibrarySetupModal
    bind:open={librarySetupOpen}
    title={current?.libraryPath ? msg('settings.generalLibrarySetupTitleChange') : msg('settings.generalLibrarySetupTitleNew')}
    note={current?.libraryPath
      ? msg('settings.generalLibrarySetupChangeNote')
      : ''}
  />

  <CloudConsentModal
    bind:open={cloudConsentOpen}
    busy={cloudBusy}
    onenable={enableCloudSaves}
    onpolicy={() => openLegalDoc({ id: 'privacy', title: msg('settings.generalPrivacyPolicyLinkLabel') })}
  />

  <CloudDisableModal
    bind:open={cloudDisableOpen}
    busy={cloudBusy}
    ondelete={() => disableCloudSaves(true)}
    onkeep={() => disableCloudSaves(false)}
  />

  <LegalDocumentModal bind:open={legalOpen} documentId={legalActiveId} title={legalActiveTitle} />

  <SourcesNoticeModal bind:open={sourcesNoticeReviewOpen} mode="review" />

  <SentDataModal bind:open={sentDataOpen} />

  {#if pending}
    <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
  {/if}

  <Modal bind:open={historyOpen} title={msg('settings.aboutHistoryLabel')} width="104rem">
    <ReleaseNotesList notes={$releaseNotesHistory} currentVersion={$selfUpdateStatus.currentVersion} />
  </Modal>
</Page>

<style>
  /* Stuck over the scrolled settings, the tabs are an island like the top bar's. */
  .tabs-wrap {
    position: sticky;
    top: 0;
    z-index: var(--z-sticky);
    margin: var(--space-2) 0 var(--space-6);
    padding: 0 var(--space-4);
    background: var(--island-fill);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .tab-body {
    animation: rise-in var(--dur-panel) var(--ease) backwards;
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row.danger-zone {
    margin-top: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid color-mix(in srgb, var(--danger) 30%, transparent);
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .row-label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .row-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .import-details {
    padding: var(--space-3) 0;
  }

  .import-details summary {
    cursor: pointer;
  }

  .import-details ul {
    margin: var(--space-2) 0 0;
    padding-left: var(--space-5);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .row-error {
    color: var(--danger);
  }

  .schedule-body {
    flex: 1 1 100%;
    min-width: 0;
  }

  .logs-upload-progress {
    width: min(28rem, 100%);
    margin-top: var(--space-2);
  }

  .folder-row {
    flex-direction: column;
    align-items: stretch;
    gap: var(--space-2);
  }

  .folder-label {
    font-size: var(--font-sm);
  }

  .folder-controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .about-logo {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .about-logo h3 {
    margin-bottom: 2px;
  }

  .privacy-link {
    align-self: flex-start;
    color: var(--accent-text);
    font-size: var(--font-xs);
    font-weight: 500;
    padding: 0.2rem 0.3rem;
    margin-left: -0.3rem;
    border-radius: var(--radius-xs);
  }

  .privacy-link:hover {
    color: var(--text);
  }
</style>
