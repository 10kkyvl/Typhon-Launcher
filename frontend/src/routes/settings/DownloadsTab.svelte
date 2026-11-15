<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import IntegerInput from '../../lib/components/IntegerInput.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import RateLimitInput from '../../lib/components/RateLimitInput.svelte';
  import ScheduleEditor from '../../lib/components/ScheduleEditor.svelte';
  import Select from '../../lib/components/Select.svelte';
  import StableOffer from '../../lib/components/StableOffer.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import EmulatorsCard from './EmulatorsCard.svelte';
  import NetworkSettingsCard from './NetworkSettingsCard.svelte';
  import { msg } from '../../lib/i18n';
  import { maxActiveDownloadOptions, type Settings } from '../../lib/services/settings';
  import { defaultPlayDownloadLimit, defaultPlayUploadLimit } from '../../lib/services/playThrottle';
  import { elevationSupported } from '../../lib/services/system';
  import { betaToggleShown, isBeta } from '../../lib/services/selfupdateChannel';
  import { requestCheck, selfUpdateStatus } from '../../lib/stores/selfupdate';
  import { cloudStatus } from '../../lib/stores/cloudsaves';
  import { saveBackupsEnabled } from '../../lib/stores/savebackup';
  import { settings, updateSettings, updateSettingsReporting } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { authState, currentUser } from '../../lib/stores/user';
  import { bytesSize } from '../../lib/utils/format';
  import type { SettingsPage } from './settingsPage.svelte';

  // The downloads tab: speed, schedule and seeding, the network, updates and saves, installation and
  // the emulators. Cloud saves and the schedule's save error are the page's: they outlive a switch of
  // tabs, and the cloud dialogs are the page's.
  let { page, uid }: { page: SettingsPage; uid: string } = $props();

  const current = $derived($settings);
  const accountReady = $derived($authState === 'authenticated');

  const cloudUsage = $derived.by(() => {
    const status = $cloudStatus;
    if (!current?.cloudSaves || !status) return '';
    if (status.quotaBytes > 0) {
      return msg('cloudsaves.usage', { used: bytesSize(status.usedBytes), quota: bytesSize(status.quotaBytes) });
    }
    return msg('cloudsaves.usageUnknown', { used: bytesSize(status.usedBytes) });
  });

  function set(patch: Partial<Settings>) {
    updateSettings(patch);
  }

  const labelId = (name: string) => `${uid}-${name}-label`;
  const subId = (name: string) => `${uid}-${name}-sub`;

  const betaShown = $derived(betaToggleShown($authState, $currentUser));
  const onBeta = $derived(isBeta($selfUpdateStatus.channel) || isBeta(page.appInfo?.channel));

  async function setBetaChannel(value: boolean) {
    const saved = await updateSettingsReporting({ updateBetaChannel: value }, () => toast(msg('state.settingsSaveFailed'), 'danger'));
    if (saved) await requestCheck();
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
            {#if page.scheduleError}<div class="message"><Notice tone="danger" compact>{page.scheduleError}</Notice></div>{/if}
          </div>
          <div class="schedule-body">
            <ScheduleEditor windows={current?.downloadSchedule ?? []} onchange={page.setSchedule} />
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
            {#if page.cloudForgetFailed && !current?.cloudSaves}
              <span class="row-sub">{msg('cloudsaves.retryForgetHint')}</span>
            {/if}
          </div>
          {#if page.cloudForgetFailed && !current?.cloudSaves}
            <Button size="sm" variant="danger" disabled={page.cloudBusy || !accountReady} onclick={page.retryCloudForget}>
              {msg('cloudsaves.retryForget')}
            </Button>
          {/if}
          <Toggle
            checked={current?.cloudSaves ?? false}
            labelledby={labelId('settings')}
            describedby={subId('settings')}
            disabled={!accountReady && !current?.cloudSaves}
            onchange={page.changeCloudSaves}
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
        {#if page.appInfo && elevationSupported(page.appInfo)}
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

<style>
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

  .schedule-body {
    flex: 1 1 100%;
    min-width: 0;
  }

  .message {
    margin-top: var(--space-2);
  }
</style>
