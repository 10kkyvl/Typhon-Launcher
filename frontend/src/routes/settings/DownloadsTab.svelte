<script lang="ts">
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import IntegerInput from '../../lib/components/IntegerInput.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import RateLimitInput from '../../lib/components/RateLimitInput.svelte';
  import ScheduleEditor from '../../lib/components/ScheduleEditor.svelte';
  import Select from '../../lib/components/Select.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
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
  import { focusHandOff } from '../../lib/utils/focusHandOff';
  import type { SettingsPage } from './settingsPage.svelte';

  // The downloads tab: speed, schedule and seeding, the network, updates and saves, installation and
  // the emulators. Cloud saves and the schedule's save error are the page's: they outlive a switch of
  // tabs, and the cloud dialogs are the page's.
  // `uid` is what the page still hands every tab; the rows here make their own ids.
  let { page }: { page: SettingsPage; uid?: string } = $props();

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

  const cloudLine = $derived(
    cloudUsage && $cloudStatus?.pending ? msg('settings.downloadsCloudUsagePending', { usage: cloudUsage, pending: msg('cloudsaves.pending') }) : cloudUsage,
  );
  // The page clears its failure as a retry starts: the retry stays, busy, until the answer, and goes
  // only once it succeeds, focus held on it then moving to the switch beside it.
  let retrying = $state(false);
  const forgetLeft = $derived((page.cloudForgetFailed || retrying) && !current?.cloudSaves);
  const cloudOff = $derived(!accountReady && !current?.cloudSaves);

  async function retryForget() {
    if (retrying) return;
    const handOff = focusHandOff();
    retrying = true;
    try {
      await page.retryCloudForget();
    } finally {
      retrying = false;
    }
    await handOff(() => document.getElementById(`${fieldId}-cloud`));
  }

  function set(patch: Partial<Settings>) {
    updateSettings(patch);
  }

  const fieldId = $props.id();

  const betaShown = $derived(betaToggleShown($authState, $currentUser));
  const onBeta = $derived(isBeta($selfUpdateStatus.channel) || isBeta(page.appInfo?.channel));
  const betaLabel = $derived(
    onBeta
      ? msg('settings.downloadsBetaLabelMarked', { label: msg('settings.downloadsBetaLabel'), mark: msg('settings.downloadsBetaChannelMark') })
      : msg('settings.downloadsBetaLabel'),
  );

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
        <SettingsRow label={msg('settings.downloadsRateLimitLabel')} hint={msg('settings.downloadsRateLimitSub')}>
          {#snippet children({ labelId, hintId })}
            <RateLimitInput labelledby={labelId} describedby={hintId} value={current?.downloadRateLimit ?? 0} presets={downloadLimitPresets} onchange={(bytes) => set({ downloadRateLimit: bytes })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsUploadLimitLabel')} hint={msg('settings.downloadsUploadLimitSub')}>
          {#snippet children({ labelId, hintId })}
            <RateLimitInput labelledby={labelId} describedby={hintId} value={current?.uploadRateLimit ?? 0} presets={uploadLimitPresets} onchange={(bytes) => set({ uploadRateLimit: bytes })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsPlayLabel')} hint={msg('settings.downloadsPlaySub')}>
          {#snippet children({ labelId, hintId })}
            <Select labelledby={labelId} describedby={hintId} value={playModeValue} width="20rem" options={playModeOptions} onchange={pickPlayMode} />
          {/snippet}
        </SettingsRow>
        {#if playModeValue === 'limit'}
          <SettingsRow label={msg('settings.downloadsPlayLimitDownLabel')}>
            {#snippet children({ labelId })}
              <RateLimitInput labelledby={labelId} value={current?.playDownloadRateLimit ?? 0} presets={playDownloadPresets} allowUnlimited={false} onchange={(bytes) => set({ playDownloadRateLimit: bytes })} />
            {/snippet}
          </SettingsRow>
          <SettingsRow label={msg('settings.downloadsPlayLimitUpLabel')}>
            {#snippet children({ labelId })}
              <RateLimitInput labelledby={labelId} value={current?.playUploadRateLimit ?? 0} presets={playUploadPresets} allowUnlimited={false} onchange={(bytes) => set({ playUploadRateLimit: bytes })} />
            {/snippet}
          </SettingsRow>
        {/if}
        <SettingsRow label={msg('settings.downloadsScheduleLabel')} hint={msg('settings.downloadsScheduleSub')}>
          {#snippet below({ labelId, hintId })}
            <ScheduleEditor windows={current?.downloadSchedule ?? []} labelledby={labelId} describedby={hintId} onchange={page.setSchedule} />
            {#if page.scheduleError}<div class="message"><Notice tone="danger" compact>{page.scheduleError}</Notice></div>{/if}
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsMaxActiveLabel')} hint={msg('settings.downloadsMaxActiveSub')}>
          {#snippet children({ labelId, hintId })}
            <Select labelledby={labelId} describedby={hintId} value={maxActiveValue} width="20rem" options={maxActiveDownloadOptions} onchange={(id) => set({ maxActiveDownloads: Number(id) })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsUploadWhileDownloadingLabel')} hint={msg('settings.downloadsUploadWhileDownloadingSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.uploadWhileDownloading ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ uploadWhileDownloading: v })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsSeedAfterLabel')} hint={msg('settings.downloadsSeedAfterSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.seedAfterDownload ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ seedAfterDownload: v })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsSourceRefreshLabel')} hint={msg('settings.downloadsSourceRefreshSub')}>
          {#snippet children({ labelId, hintId })}
            <Select labelledby={labelId} describedby={hintId} value={sourceRefreshInterval} width="20rem" options={sourceRefreshOptions} onchange={(id) => set({ sourceRefreshInterval: id })} />
          {/snippet}
        </SettingsRow>
    </Card>

    <NetworkSettingsCard />

    <Card title={msg('settings.downloadsUpdatesCardTitle')}>
        <SettingsRow label={msg('settings.downloadsUpdateCheckAutoLabel')} hint={msg('settings.downloadsUpdateCheckAutoSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.updateCheckAutomatically ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ updateCheckAutomatically: v })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsUpdateAutoDownloadLabel')} hint={msg('settings.downloadsUpdateAutoDownloadSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.updateAutoDownload ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ updateAutoDownload: v })} />
          {/snippet}
        </SettingsRow>
        {#if betaShown}
          <SettingsRow label={betaLabel} hint={msg('settings.downloadsBetaSub')}>
            {#snippet children({ labelId, hintId })}
              <Toggle checked={current?.updateBetaChannel ?? false} labelledby={labelId} describedby={hintId} onchange={setBetaChannel} />
            {/snippet}
          </SettingsRow>
        {/if}
        <StableOffer />
        <SettingsRow label={msg('settings.downloadsSaveBackupLabel')} hint={msg('settings.downloadsSaveBackupSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.updateSaveBackup ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ updateSaveBackup: v })} />
          {/snippet}
        </SettingsRow>
        {#if $saveBackupsEnabled}
          <SettingsRow label={msg('settings.downloadsSaveBackupAfterSessionLabel')} hint={msg('settings.downloadsSaveBackupAfterSessionSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.saveBackupAfterSession ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ saveBackupAfterSession: v })} />
          {/snippet}
        </SettingsRow>
          <SettingsRow label={msg('settings.downloadsSaveBackupBeforeLaunchLabel')} hint={msg('settings.downloadsSaveBackupBeforeLaunchSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.saveBackupBeforeLaunch ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ saveBackupBeforeLaunch: v })} />
          {/snippet}
        </SettingsRow>
          <SettingsRow label={msg('settings.downloadsSaveBackupLimitLabel')} hint={msg('settings.downloadsSaveBackupLimitSub')} labelFor="{fieldId}-backup-limit">
            {#snippet children({ hintId })}
              <IntegerInput id="{fieldId}-backup-limit" describedby={hintId} value={current?.saveBackupLimit ?? 5} min={1} max={50} onchange={(n) => set({ saveBackupLimit: n })} />
            {/snippet}
          </SettingsRow>
        {/if}
        <SettingsRow
          label={msg('cloudsaves.settingsLabel')}
          hint={msg('cloudsaves.settingsSub')}
          disabledReason={cloudOff ? msg('cloudsaves.settingsNeedsAccount') : undefined}
          note={!accountReady && !cloudOff ? msg('cloudsaves.settingsNeedsAccount') : forgetLeft ? msg('cloudsaves.retryForgetHint') : undefined}
        >
          {#snippet children({ labelId, hintId, disabled })}
            <Toggle id="{fieldId}-cloud" checked={current?.cloudSaves ?? false} labelledby={labelId} describedby={hintId} {disabled} onchange={page.changeCloudSaves} />
          {/snippet}
          {#snippet below({ disabled })}
            {#if accountReady && cloudLine}<p class="cloud-line">{cloudLine}</p>{/if}
            {#if forgetLeft}
              <Button size="sm" variant="danger" {disabled} busy={retrying || page.cloudBusy} onclick={retryForget}>{msg('cloudsaves.retryForget')}</Button>
            {/if}
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsKeepPreviousLabel')} hint={msg('settings.downloadsKeepPreviousSub')}>
          {#snippet children({ labelId, hintId })}
            <Select labelledby={labelId} describedby={hintId} value={keepPreviousVersion} width="28rem" options={keepPreviousOptions} onchange={(id) => set({ keepPreviousVersion: id })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsTorrentReuseLabel')} hint={msg('settings.downloadsTorrentReuseSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.allowTorrentReuse ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ allowTorrentReuse: v })} />
          {/snippet}
        </SettingsRow>
    </Card>
  </div>
  <div class="settings-column">
    <Card title={msg('settings.downloadsInstallCardTitle')}>
        <SettingsRow label={msg('settings.downloadsAfterInstallLabel')} hint={msg('settings.downloadsAfterInstallSub')}>
          {#snippet children({ labelId, hintId })}
            <Select labelledby={labelId} describedby={hintId} value={cleanupPolicy} width="26rem" options={cleanupPolicyOptions} onchange={(id) => set({ installCleanupPolicy: id })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsAutoInstallLabel')} hint={msg('settings.downloadsAutoInstallSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.autoInstall ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ autoInstall: v })} />
          {/snippet}
        </SettingsRow>
        {#if page.appInfo && elevationSupported(page.appInfo)}
          <SettingsRow label={msg('settings.downloadsElevateAheadLabel')} hint={msg('settings.downloadsElevateAheadSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.elevateAhead ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ elevateAhead: v })} />
          {/snippet}
        </SettingsRow>
        {/if}
        <SettingsRow label={msg('settings.downloadsSkipShortcutsLabel')} hint={msg('settings.downloadsSkipShortcutsSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.installSkipShortcuts ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ installSkipShortcuts: v })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsDesktopShortcutsLabel')} hint={msg('settings.downloadsDesktopShortcutsSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.desktopShortcuts ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ desktopShortcuts: v })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsSkipExtrasLabel')} hint={msg('settings.downloadsSkipExtrasSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.installSkipExtras ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ installSkipExtras: v })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsVerifyAfterInstallLabel')} hint={msg('settings.downloadsVerifyAfterInstallSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.verifyAfterInstall ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ verifyAfterInstall: v })} />
          {/snippet}
        </SettingsRow>
        <SettingsRow label={msg('settings.downloadsVerifyRepackLabel')} hint={msg('settings.downloadsVerifyRepackSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle checked={current?.installVerifyRepack ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ installVerifyRepack: v })} />
          {/snippet}
        </SettingsRow>
    </Card>

    <EmulatorsCard />
  </div>
</div>

<style>
  .message {
    margin-top: var(--space-2);
  }

  .cloud-line {
    margin: 0 0 var(--space-2);
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
