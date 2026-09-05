<script lang="ts">
  import { Download, Eye, FolderOpen, ListChecks, RefreshCw, ScrollText, Trash2 } from '@lucide/svelte';
  import { onMount, untrack } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import LegalDocumentModal from '../../lib/components/LegalDocumentModal.svelte';
  import LibrarySetupModal from '../../lib/components/LibrarySetupModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import RateLimitInput from '../../lib/components/RateLimitInput.svelte';
  import Select from '../../lib/components/Select.svelte';
  import { msg } from '../../lib/i18n';
  import SentDataModal from '../../lib/components/SentDataModal.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import ReleaseNotesList from '../../lib/components/ReleaseNotesList.svelte';
  import SourcesNoticeModal from '../../lib/components/SourcesNoticeModal.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import UpdateBanner from '../../lib/components/UpdateBanner.svelte';
  import AppearanceTab from './AppearanceTab.svelte';
  import LanSettingsRow from './LanSettingsRow.svelte';
  import LibraryLocationRow from './LibraryLocationRow.svelte';
  import { forgetRemote, syncNow } from '../../lib/services/accountSync';
  import { accountSyncReason } from '../../lib/services/accountSyncMessages';
  import { inWails } from '../../lib/services/backend';
  import { listLegalDocuments, type LegalMeta } from '../../lib/services/legal';
  import { logsReason } from '../../lib/services/logsMessages';
  import { getSettings, maxActiveDownloadOptions, openFolder, type Settings } from '../../lib/services/settings';
  import {
    exportLogs,
    getAppInfo,
    getSystemInfo,
    type AppInfo,
    type LogBundle,
    type SystemInfo,
  } from '../../lib/services/system';
  import { releaseNotesHistory, requestCheck, selfUpdateChecking, selfUpdateStatus } from '../../lib/stores/selfupdate';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { authState } from '../../lib/stores/user';
  import { bytesLabel, relativeDate } from '../../lib/utils/format';

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

  let legalDocs = $state<LegalMeta[]>([]);
  let legalError = $state('');
  let legalOpen = $state(false);
  let legalActiveId = $state<string | null>(null);
  let legalActiveTitle = $state('');
  let sourcesNoticeReviewOpen = $state(false);
  let sentDataOpen = $state(false);

  let logsBundle = $state<LogBundle | null>(null);
  let logsSaving = $state(false);

  const accountReady = $derived($authState === 'authenticated');
  let syncingNow = $state(false);
  let forgettingRemote = $state(false);

  onMount(async () => {
    appInfo = await getAppInfo();
    systemInfo = await getSystemInfo();
    try {
      legalDocs = await listLegalDocuments();
    } catch {
      legalError = inWails ? msg('settings.aboutLegalLoadError') : msg('settings.aboutLegalUnavailable');
    }
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

  async function runForgetRemote() {
    if (forgettingRemote) return;
    if (!window.confirm(msg('settings.generalSyncForgetConfirm'))) return;
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

<PageHeader title={msg('settings.pageTitle')} />

<div class="tabs-wrap">
  <Tabs {tabs} bind:value={tab} />
</div>

{#if tab === 'general'}
  <div class="columns">
    <div class="column">
      <Card title={msg('settings.generalStartupCardTitle')}>
        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.generalLaunchOnStartupLabel')}</span>
              <span class="row-sub">{msg('settings.generalLaunchOnStartupSub')}</span>
            </div>
            <Toggle
              checked={current?.launchOnStartup ?? false}
              label={msg('settings.generalLaunchOnStartupLabel')}
              onchange={(v) => set({ launchOnStartup: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.generalMinimizeToTrayLabel')}</span>
              <span class="row-sub">{msg('settings.generalMinimizeToTraySub')}</span>
            </div>
            <Toggle
              checked={current?.minimizeToTray ?? true}
              label={msg('settings.generalMinimizeToTrayLabel')}
              onchange={(v) => set({ minimizeToTray: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.generalDiscordRpcLabel')}</span>
              <span class="row-sub">{msg('settings.generalDiscordRpcSub')}</span>
            </div>
            <Toggle
              checked={current?.discordRichPresence ?? false}
              label={msg('settings.generalDiscordRpcLabel')}
              onchange={(v) => set({ discordRichPresence: v })}
            />
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
          <LibraryLocationRow />
        </div>
      </Card>

      <Card title={msg('settings.generalExperimentalCardTitle')}>
        <div class="rows">
          <LanSettingsRow />
        </div>
      </Card>
    </div>

    <div class="column">
      <Card title={msg('settings.generalInterfaceCardTitle')}>
        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.language')}</span>
              <span class="row-sub">{msg('settings.languageSub')}</span>
            </div>
            <Select
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
              <span class="row-label">{msg('settings.generalUiScaleLabel')}</span>
              <span class="row-sub">{msg('settings.generalUiScaleSub')}</span>
            </div>
            <Select
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
              <span class="row-label">{msg('settings.generalAnimationsLabel')}</span>
              <span class="row-sub">{msg('settings.generalAnimationsSub')}</span>
            </div>
            <Toggle
              checked={current?.animationsEnabled ?? true}
              label={msg('settings.generalAnimationsToggleLabel')}
              onchange={(v) => set({ animationsEnabled: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.generalHwAccelLabel')}</span>
              <span class="row-sub">{msg('settings.generalHwAccelSub')}</span>
            </div>
            <Toggle
              checked={current?.hardwareAcceleration ?? true}
              label={msg('settings.generalHwAccelLabel')}
              onchange={(v) => set({ hardwareAcceleration: v })}
            />
          </div>
        </div>
      </Card>

      <Card title={msg('settings.generalPrivacyCardTitle')}>
        <div class="rows">
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.generalPrivacyStatsLabel')}</span>
              <span class="row-sub">{msg('settings.generalPrivacyStatsSub')}</span>
            </div>
            <Toggle
              checked={current?.anonymousUsageStats ?? false}
              label={msg('settings.generalPrivacyStatsLabel')}
              onchange={(v) => set({ anonymousUsageStats: v })}
            />
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.generalPrivacyDiagLabel')}</span>
              <span class="row-sub">{msg('settings.generalPrivacyDiagSub')}</span>
            </div>
            <Toggle
              checked={current?.anonymousDiagnostics ?? false}
              label={msg('settings.generalPrivacyDiagLabel')}
              onchange={(v) => set({ anonymousDiagnostics: v })}
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
              <span class="row-label">{msg('settings.generalSyncEnableLabel')}</span>
              <span class="row-sub">{msg('settings.generalSyncEnableSub')}</span>
              {#if !accountReady}
                <span class="row-sub">{msg('settings.generalSyncNeedsAccountSub')}</span>
              {/if}
            </div>
            <Toggle
              checked={current?.accountSync ?? false}
              label={msg('settings.generalSyncEnableLabel')}
              disabled={!accountReady}
              onchange={(v) => set({ accountSync: v })}
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
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.generalSyncForgetLabel')}</span>
              <span class="row-sub">{msg('settings.generalSyncForgetSub')}</span>
            </div>
            <Button
              size="sm"
              variant="danger"
              disabled={!accountReady || forgettingRemote}
              onclick={runForgetRemote}
            >
              <Trash2 size="1.5rem" strokeWidth={1.8} />
              {forgettingRemote ? msg('settings.generalSyncForgetRunning') : msg('settings.generalSyncForgetLabel')}
            </Button>
          </div>
        </div>
      </Card>
    </div>
  </div>
{:else if tab === 'downloads'}
  <div class="single-column">
    <Card title={msg('settings.downloadsTab')}>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsRateLimitLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsRateLimitSub')}</span>
          </div>
          <RateLimitInput
            value={current?.downloadRateLimit ?? 0}
            presets={downloadLimitPresets}
            onchange={(bytes) => set({ downloadRateLimit: bytes })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsUploadLimitLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsUploadLimitSub')}</span>
          </div>
          <RateLimitInput
            value={current?.uploadRateLimit ?? 0}
            presets={uploadLimitPresets}
            onchange={(bytes) => set({ uploadRateLimit: bytes })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsMaxActiveLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsMaxActiveSub')}</span>
          </div>
          <Select
            value={maxActiveValue}
            width="20rem"
            options={maxActiveDownloadOptions}
            onchange={(id) => set({ maxActiveDownloads: Number(id) })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsUploadWhileDownloadingLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsUploadWhileDownloadingSub')}</span>
          </div>
          <Toggle
            checked={current?.uploadWhileDownloading ?? false}
            label={msg('settings.downloadsUploadWhileDownloadingToggle')}
            onchange={(v) => set({ uploadWhileDownloading: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsSeedAfterLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsSeedAfterSub')}</span>
          </div>
          <Toggle
            checked={current?.seedAfterDownload ?? false}
            label={msg('settings.downloadsSeedAfterToggle')}
            onchange={(v) => set({ seedAfterDownload: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsSourceRefreshLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsSourceRefreshSub')}</span>
          </div>
          <Select
            value={sourceRefreshInterval}
            width="20rem"
            options={sourceRefreshOptions}
            onchange={(id) => set({ sourceRefreshInterval: id })}
          />
        </div>
      </div>
    </Card>

    <Card title={msg('settings.downloadsInstallCardTitle')}>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsAfterInstallLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsAfterInstallSub')}</span>
          </div>
          <Select
            value={cleanupPolicy}
            width="26rem"
            options={cleanupPolicyOptions}
            onchange={(id) => set({ installCleanupPolicy: id })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsAutoInstallLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsAutoInstallSub')}</span>
          </div>
          <Toggle
            checked={current?.autoInstall ?? false}
            label={msg('settings.downloadsAutoInstallToggle')}
            onchange={(v) => set({ autoInstall: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsSkipShortcutsLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsSkipShortcutsSub')}</span>
          </div>
          <Toggle
            checked={current?.installSkipShortcuts ?? true}
            label={msg('settings.downloadsSkipShortcutsToggle')}
            onchange={(v) => set({ installSkipShortcuts: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsDesktopShortcutsLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsDesktopShortcutsSub')}</span>
          </div>
          <Toggle
            checked={current?.desktopShortcuts ?? true}
            label={msg('settings.downloadsDesktopShortcutsToggle')}
            onchange={(v) => set({ desktopShortcuts: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsSkipExtrasLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsSkipExtrasSub')}</span>
          </div>
          <Toggle
            checked={current?.installSkipExtras ?? true}
            label={msg('settings.downloadsSkipExtrasToggle')}
            onchange={(v) => set({ installSkipExtras: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsVerifyAfterInstallLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsVerifyAfterInstallSub')}</span>
          </div>
          <Toggle
            checked={current?.verifyAfterInstall ?? true}
            label={msg('settings.downloadsVerifyAfterInstallToggle')}
            onchange={(v) => set({ verifyAfterInstall: v })}
          />
        </div>
      </div>
    </Card>

    <Card title={msg('settings.downloadsUpdatesCardTitle')}>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsUpdateCheckAutoLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsUpdateCheckAutoSub')}</span>
          </div>
          <Toggle
            checked={current?.updateCheckAutomatically ?? true}
            label={msg('settings.downloadsUpdateCheckAutoToggle')}
            onchange={(v) => set({ updateCheckAutomatically: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsUpdateAutoDownloadLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsUpdateAutoDownloadSub')}</span>
          </div>
          <Toggle
            checked={current?.updateAutoDownload ?? false}
            label={msg('settings.downloadsUpdateAutoDownloadToggle')}
            onchange={(v) => set({ updateAutoDownload: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsSaveBackupLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsSaveBackupSub')}</span>
          </div>
          <Toggle
            checked={current?.updateSaveBackup ?? true}
            label={msg('settings.downloadsSaveBackupToggle')}
            onchange={(v) => set({ updateSaveBackup: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsKeepPreviousLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsKeepPreviousSub')}</span>
          </div>
          <Select
            value={keepPreviousVersion}
            width="28rem"
            options={keepPreviousOptions}
            onchange={(id) => set({ keepPreviousVersion: id })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.downloadsTorrentReuseLabel')}</span>
            <span class="row-sub">{msg('settings.downloadsTorrentReuseSub')}</span>
          </div>
          <Toggle
            checked={current?.allowTorrentReuse ?? true}
            label={msg('settings.downloadsTorrentReuseToggle')}
            onchange={(v) => set({ allowTorrentReuse: v })}
          />
        </div>
      </div>
    </Card>
  </div>
{:else if tab === 'appearance'}
  <AppearanceTab />
{:else if tab === 'about'}
  <div class="single-column">
    <Card>
      <div class="about-logo">
        <img src="/typhon.png" alt="" width="44" height="44" draggable="false" />
        <div>
          <h3>Typhon Launcher</h3>
          <span class="row-sub">{msg('settings.aboutVersionLabel', { version: appInfo?.version ?? '—' })} · {appInfo?.platform ?? ''}/{appInfo?.arch ?? ''}{appInfo?.devMock ? ' · devmock' : ''}</span>
        </div>
      </div>
      <div class="rows">
        {#if systemInfo}
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutSystemLabel')}</span>
              <span class="row-sub">{systemInfo.os}</span>
            </div>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutCpuLabel')}</span>
              <span class="row-sub">{systemInfo.cpu} · {msg('settings.aboutCpuThreads', { cores: systemInfo.cores })}</span>
            </div>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutMemoryLabel')}</span>
              <span class="row-sub">{bytesLabel(systemInfo.ramBytes)}</span>
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
        <UpdateBanner />
      </div>
    </Card>

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
{/if}

<LibrarySetupModal
  bind:open={librarySetupOpen}
  title={current?.libraryPath ? msg('settings.generalLibrarySetupTitleChange') : msg('settings.generalLibrarySetupTitleNew')}
  note={current?.libraryPath
    ? msg('settings.generalLibrarySetupChangeNote')
    : ''}
/>

<LegalDocumentModal bind:open={legalOpen} documentId={legalActiveId} title={legalActiveTitle} />
<SourcesNoticeModal bind:open={sourcesNoticeReviewOpen} mode="review" />
<SentDataModal bind:open={sentDataOpen} />
<Modal bind:open={historyOpen} title={msg('settings.aboutHistoryLabel')} width="52rem">
  <ReleaseNotesList notes={$releaseNotesHistory} currentVersion={$selfUpdateStatus.currentVersion} />
</Modal>

<style>
  .tabs-wrap {
    margin-bottom: var(--space-8);
  }

  .columns {
    display: grid;
    grid-template-columns: 1fr;
    gap: var(--space-8);
    align-items: start;
    max-width: 96rem;
  }

  .column {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    min-width: 0;
  }

  .single-column {
    display: flex;
    flex-direction: column;
    gap: var(--space-6);
    max-width: 96rem;
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

  @media (min-width: 1600px) {
    .columns {
      grid-template-columns: 1fr 1fr;
      gap: var(--space-12);
    }
  }
</style>
