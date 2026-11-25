<script lang="ts">
  import { Copy, Download, LifeBuoy, ListChecks, ScrollText, Send } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DiscordJoinButton from '../../lib/components/DiscordJoinButton.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import RollbackAction from '../../lib/components/RollbackAction.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
  import StableOffer from '../../lib/components/StableOffer.svelte';
  import UpdateBanner from '../../lib/components/UpdateBanner.svelte';
  import { msg } from '../../lib/i18n';
  import { uploadPercent } from '../../lib/support/progress';
  import { rollbackShown } from '../../lib/services/selfupdate';
  import { rollbackReason } from '../../lib/services/selfupdateMessages';
  import { isBeta } from '../../lib/services/selfupdateChannel';
  import type { SystemInfo } from '../../lib/services/system';
  import { releaseNotesHistory, requestCheck, rollbackInfo, selfUpdateChecking, selfUpdateStatus } from '../../lib/stores/selfupdate';
  import { navigate } from '../../lib/stores/router';
  import { bytesLabel, relativeDate } from '../../lib/utils/format';
  import type { SettingsPage } from './settingsPage.svelte';

  // The about tab: the version and the machine, the launcher's updates, the logs and the legal
  // documents. What outlives a switch of tabs (the facts read on opening, the logs export and send,
  // the dialogs) is the page's.
  let { page }: { page: SettingsPage } = $props();

  const logsProgressPercent = $derived(page.logsProgress ? uploadPercent(page.logsProgress) : 0);
  const onBeta = $derived(isBeta($selfUpdateStatus.channel) || isBeta(page.appInfo?.channel));
  const missing = $derived(msg('settings.aboutValueMissing'));

  // The rows that need the app facts say a failed read the way the system rows do; while it is
  // pending the version is a dash and the platform is left out.
  const buildLine = $derived.by(() => {
    if (page.appInfoFailed) return msg('settings.aboutSystemFailed');
    const version = msg(onBeta ? 'settings.aboutVersionBetaLabel' : 'settings.aboutVersionLabel', { version: page.appInfo?.version ?? missing });
    if (!page.appInfo) return version;
    const parts = { version, platform: page.appInfo.platform, arch: page.appInfo.arch };
    return msg(page.appInfo.devMock ? 'settings.aboutBuildLineDevmock' : 'settings.aboutBuildLine', parts);
  });

  function cpuLine(info: SystemInfo): string {
    const cpu = info.cpuUnknown ? msg('settings.aboutUnknown') : info.cpu;
    const threads = msg('settings.aboutCpuThreads', { cores: info.cores });
    if (info.physicalCoresUnknown) return msg('settings.aboutCpuLineNoCores', { cpu, threads });
    return msg('settings.aboutCpuLine', { cpu, cores: msg('settings.aboutCpuCores', { cores: info.physicalCores }), threads });
  }

  const checkHint = $derived.by(() => {
    if ($selfUpdateChecking) return msg('settings.aboutCheckUpdatesChecking');
    const version = $selfUpdateStatus.currentVersion || page.appInfo?.version;
    if (!version && page.appInfoFailed) return msg('settings.aboutSystemFailed');
    const installed = msg('settings.aboutInstalledVersion', { version: version || missing });
    if (!$selfUpdateStatus.checkedAt) return installed;
    return msg('settings.aboutInstalledCheckedAt', { installed, checked: msg('settings.aboutCheckedAt', { date: relativeDate($selfUpdateStatus.checkedAt) }) });
  });

  const logsSaveHint = $derived(
    page.logsBundle
      ? msg('settings.aboutLogsBundleLine', { name: page.logsBundle.name, size: bytesLabel(page.logsBundle.sizeBytes), hint: msg('settings.aboutLogsAttachHint') })
      : msg('settings.aboutLogsSaveHint'),
  );

  // Where the send has got to, under its row: the row's hint keeps saying where the file goes.
  const logsStageText = $derived.by(() => {
    if (page.logsStage === 'preparing') return msg('settings.aboutLogsPreparingEllipsis');
    if (page.logsStage === 'sending' && page.logsProgress) return msg('settings.aboutLogsSendingProgress', { percent: logsProgressPercent });
    if (page.logsStage === 'sending') return msg('settings.aboutLogsSendingEllipsis');
    if (page.logsStage === 'waiting') return msg('settings.aboutLogsWaitingEllipsis');
    if (page.logsSendResult) return msg('settings.aboutLogsSentStatus');
    return '';
  });
</script>

<div class="settings-grid">
  <div class="settings-column">
    <Card>
      <div class="about-logo">
        <img src="/typhon.png" alt="" width="44" height="44" draggable="false" />
        <div>
          <!-- The product's name, as every screen writes it: not translated. -->
          <h3>Typhon Launcher</h3>
          <span class="build">{buildLine}</span>
        </div>
      </div>
      {#if page.appInfo?.portable}
        <SettingsRow label={msg('settings.aboutPortableLabel')} hint={msg('settings.aboutPortableDataDir', { path: page.appInfo.dataDir })} />
      {/if}
      {#if page.systemInfo}
        <SettingsRow
          label={msg('settings.aboutSystemLabel')}
          hint={page.systemInfo.osUnknown ? msg('settings.aboutUnknown') : page.systemInfo.os}
        />
        <SettingsRow label={msg('settings.aboutCpuLabel')} hint={cpuLine(page.systemInfo)} />
        <SettingsRow
          label={msg('settings.aboutMemoryLabel')}
          hint={page.systemInfo.ramBytes > 0 ? bytesLabel(page.systemInfo.ramBytes) : msg('settings.aboutUnknown')}
        />
      {:else if page.systemFailed}
        <SettingsRow label={msg('settings.aboutSystemLabel')} hint={msg('settings.aboutSystemFailed')} />
      {/if}
      {#if page.wineStatus?.required}
        <SettingsRow
          label={msg('settings.aboutRuntimeLabel')}
          hint={page.wineStatus.installed
            ? msg('settings.aboutRuntimeCrossOver', { version: page.wineStatus.version })
            : msg('settings.aboutRuntimeMissing')}
        />
      {/if}
      <SettingsRow label={msg('settings.aboutCheckUpdatesLabel')} hint={checkHint}>
        {#snippet children({ hintId })}
          <Button size="sm" aria-describedby={hintId} busy={$selfUpdateChecking} onclick={requestCheck}>
            <ListChecks size="1.5rem" strokeWidth={1.8} />
            {$selfUpdateChecking ? msg('settings.aboutCheckingEllipsis') : msg('settings.aboutCheckButtonLabel')}
          </Button>
        {/snippet}
      </SettingsRow>
      <SettingsRow
        label={msg('settings.aboutHistoryLabel')}
        hint={msg('settings.aboutHistoryHasNotes')}
        disabledReason={$releaseNotesHistory.length > 0 ? undefined : msg('settings.aboutHistoryEmpty')}
      >
        {#snippet children({ hintId, disabled })}
          <Button size="sm" aria-describedby={hintId} {disabled} onclick={() => (page.historyOpen = true)}>
            <ScrollText size="1.5rem" strokeWidth={1.8} />
            {msg('settings.aboutHistoryButtonLabel')}
          </Button>
        {/snippet}
      </SettingsRow>
      {#if rollbackShown($rollbackInfo)}
        <SettingsRow
          label={msg('settings.aboutRollbackLabel')}
          hint={$rollbackInfo.available ? msg('settings.aboutRollbackAvailable', { version: $rollbackInfo.version }) : rollbackReason($rollbackInfo)}
        >
          {#snippet children()}
            <RollbackAction />
          {/snippet}
        </SettingsRow>
      {/if}
      <SettingsRow label={msg('settings.aboutLicenseLabel')} hint={msg('settings.aboutLicenseText')} />
      <SettingsRow label={msg('discord.communityLabel')} hint={msg('discord.communitySub')}>
        {#snippet children({ hintId })}
          <DiscordJoinButton describedby={hintId} />
        {/snippet}
      </SettingsRow>
      <UpdateBanner />
      <StableOffer />
    </Card>
  </div>
  <div class="settings-column">
    <Card title={msg('settings.aboutDiagnosticsCardTitle')}>
      <SettingsRow label={msg('settings.aboutLogsLabel')} hint={logsSaveHint}>
        {#snippet children({ hintId })}
          <Button size="sm" aria-describedby={hintId} busy={page.logsSaving} onclick={page.saveLogs}>
            <Download size="1.5rem" strokeWidth={1.8} />
            {page.logsSaving ? msg('settings.aboutLogsSavingEllipsis') : msg('settings.aboutLogsDownloadButton')}
          </Button>
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.aboutLogsSendButton')} hint={msg('settings.aboutLogsSendConfirmNote')}>
        {#snippet children({ hintId })}
          <Button size="sm" aria-describedby={hintId} busy={page.logsSending} onclick={page.openSendLogsConfirm}>
            <Send size="1.5rem" strokeWidth={1.8} />
            {page.logsSending ? msg('settings.aboutLogsSendingEllipsis') : msg('settings.aboutLogsSendButton')}
          </Button>
        {/snippet}
        {#snippet below()}
          {#if logsStageText}<p class="stage">{logsStageText}</p>{/if}
          {#if page.logsSendFailure}
            <div class="message">
              <Notice tone="danger" compact title={page.logsSendFailure}>{msg('settings.aboutLogsSendFailedHint')}</Notice>
            </div>
          {/if}
          {#if page.logsSending}
            <div class="logs-upload-progress">
              <ProgressBar
                value={logsProgressPercent}
                indeterminate={page.logsStage !== 'sending' || !page.logsProgress}
                height={5}
                label={msg('settings.aboutLogsLabel')}
              />
            </div>
          {/if}
        {/snippet}
      </SettingsRow>
      {#if page.logsSendResult}
        {@const result = page.logsSendResult}
        <SettingsRow label={msg('settings.aboutLogsSendResultLabel')} hint={result.id}>
          {#snippet children({ hintId })}
            <IconButton label={msg('settings.aboutLogsSendIdCopyLabel')} size="sm" aria-describedby={hintId} onclick={page.copySendLogsId}>
              <Copy size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          {/snippet}
          {#snippet below()}
            {#if result.dropped.length > 0}
              <p class="stage">{msg('settings.aboutLogsSendDroppedNote', { files: result.dropped.join(', ') })}</p>
            {/if}
          {/snippet}
        </SettingsRow>
      {/if}
      <SettingsRow label={msg('tickets.settingsLabel')} hint={msg('tickets.settingsHint')}>
        {#snippet children({ hintId })}
          <Button size="sm" aria-describedby={hintId} onclick={() => navigate('support', { view: 'new', topic: 'launcher' })}>
            <LifeBuoy size="1.5rem" strokeWidth={1.8} />
            {msg('tickets.writeSupport')}
          </Button>
        {/snippet}
      </SettingsRow>
    </Card>

    <Card title={msg('settings.aboutLegalCardTitle')}>
      {#if page.legalState === 'failed'}
        <Notice tone="danger" compact>{msg('settings.aboutLegalLoadError')}</Notice>
      {:else if page.legalState === 'unavailable'}
        <Notice tone="info" compact announce={false}>{msg('settings.aboutLegalUnavailable')}</Notice>
      {:else}
        {#each page.legalDocs as meta (meta.id)}
          <SettingsRow label={meta.title}>
            {#snippet children()}
              <Button size="sm" aria-label={msg('settings.aboutLegalOpenNamed', { title: meta.title })} onclick={() => page.openLegalDoc(meta)}>
                {msg('common.open')}
              </Button>
            {/snippet}
          </SettingsRow>
        {/each}
        <SettingsRow label={msg('settings.aboutSourcesNoticeLabel')} hint={msg('settings.aboutSourcesNoticeSub')}>
          {#snippet children({ hintId })}
            <Button
              size="sm"
              aria-label={msg('settings.aboutLegalOpenNamed', { title: msg('settings.aboutSourcesNoticeLabel') })}
              aria-describedby={hintId}
              onclick={() => (page.sourcesNoticeReviewOpen = true)}
            >
              {msg('common.open')}
            </Button>
          {/snippet}
        </SettingsRow>
      {/if}
    </Card>
  </div>
</div>

<style>
  .about-logo {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .about-logo h3 {
    margin-bottom: 2px;
  }

  .build,
  .stage {
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }

  .stage {
    margin: 0;
  }

  .logs-upload-progress {
    width: min(28rem, 100%);
    margin-top: var(--space-2);
  }

  .message {
    margin-top: var(--space-2);
  }
</style>
