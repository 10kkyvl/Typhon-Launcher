<script lang="ts">
  import { Copy, Download, LifeBuoy, ListChecks, ScrollText, Send } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import DiscordJoinButton from '../../lib/components/DiscordJoinButton.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import RollbackAction from '../../lib/components/RollbackAction.svelte';
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

  function cpuLine(info: SystemInfo): string {
    const threads = msg('settings.aboutCpuThreads', { cores: info.cores });
    const cores = info.physicalCoresUnknown ? '' : `${msg('settings.aboutCpuCores', { cores: info.physicalCores })} · `;
    return `${info.cpuUnknown ? msg('settings.aboutUnknown') : info.cpu} · ${cores}${threads}`;
  }
</script>

<div class="settings-grid">
  <div class="settings-column">
    <Card>
      <div class="about-logo">
        <img src="/typhon.png" alt="" width="44" height="44" draggable="false" />
        <div>
          <h3>Typhon Launcher</h3>
          <span class="row-sub">{msg(onBeta ? 'settings.aboutVersionBetaLabel' : 'settings.aboutVersionLabel', { version: page.appInfo?.version ?? '—' })} · {page.appInfo?.platform ?? ''}/{page.appInfo?.arch ?? ''}{page.appInfo?.devMock ? ' · devmock' : ''}</span>
        </div>
      </div>
      <div class="rows">
        {#if page.appInfo?.portable}
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutPortableLabel')}</span>
              <span class="row-sub">{msg('settings.aboutPortableDataDir', { path: page.appInfo.dataDir })}</span>
            </div>
          </div>
        {/if}
        {#if page.systemInfo}
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutSystemLabel')}</span>
              <span class="row-sub">{page.systemInfo.osUnknown ? msg('settings.aboutUnknown') : page.systemInfo.os}</span>
            </div>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutCpuLabel')}</span>
              <span class="row-sub">{cpuLine(page.systemInfo)}</span>
            </div>
          </div>
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutMemoryLabel')}</span>
              <span class="row-sub">{page.systemInfo.ramBytes > 0 ? bytesLabel(page.systemInfo.ramBytes) : msg('settings.aboutUnknown')}</span>
            </div>
          </div>
        {:else if page.systemFailed}
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutSystemLabel')}</span>
              <span class="row-sub">{msg('settings.aboutSystemFailed')}</span>
            </div>
          </div>
        {/if}
        {#if page.wineStatus?.required}
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutRuntimeLabel')}</span>
              <span class="row-sub">
                {#if page.wineStatus.installed}
                  CrossOver {page.wineStatus.version}
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
                {msg('settings.aboutInstalledVersion', { version: $selfUpdateStatus.currentVersion || page.appInfo?.version || '—' })}
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
          <Button size="sm" disabled={$releaseNotesHistory.length === 0} onclick={() => (page.historyOpen = true)}>
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
              {#if page.logsBundle}
                {page.logsBundle.name} · {bytesLabel(page.logsBundle.sizeBytes)} {msg('settings.aboutLogsAttachHint')}
              {:else}
                {msg('settings.aboutLogsSaveHint')}
              {/if}
            </span>
          </div>
          <Button size="sm" disabled={page.logsSaving} onclick={page.saveLogs}>
            <Download size="1.5rem" strokeWidth={1.8} />
            {page.logsSaving ? msg('settings.aboutLogsSavingEllipsis') : msg('settings.aboutLogsDownloadButton')}
          </Button>
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.aboutLogsSendButton')}</span>
            <span class="row-sub">
              {#if page.logsStage === 'preparing'}
                {msg('settings.aboutLogsPreparingEllipsis')}
              {:else if page.logsStage === 'sending' && page.logsProgress}
                {msg('settings.aboutLogsSendingProgress', { percent: logsProgressPercent })}
              {:else if page.logsStage === 'sending'}
                {msg('settings.aboutLogsSendingEllipsis')}
              {:else if page.logsStage === 'waiting'}
                {msg('settings.aboutLogsWaitingEllipsis')}
              {:else if page.logsSendResult}
                {msg('settings.aboutLogsSentStatus')}
              {:else}
                {msg('settings.aboutLogsSendConfirmNote')}
              {/if}
            </span>
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
          </div>
          <Button size="sm" disabled={page.logsSending} onclick={page.openSendLogsConfirm}>
            <Send size="1.5rem" strokeWidth={1.8} />
            {page.logsSending ? msg('settings.aboutLogsSendingEllipsis') : msg('settings.aboutLogsSendButton')}
          </Button>
        </div>
        {#if page.logsSendResult}
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutLogsSendResultLabel')}</span>
              <span class="row-sub logs-send-id">{page.logsSendResult.id}</span>
            </div>
            <IconButton label={msg('settings.aboutLogsSendIdCopyLabel')} size="sm" onclick={page.copySendLogsId}>
              <Copy size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          </div>
          {#if page.logsSendResult.dropped.length > 0}
            <p class="row-sub">{msg('settings.aboutLogsSendDroppedNote', { files: page.logsSendResult.dropped.join(', ') })}</p>
          {/if}
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
      {#if page.legalState === 'failed'}
        <Notice tone="danger" compact>{msg('settings.aboutLegalLoadError')}</Notice>
      {:else if page.legalState === 'unavailable'}
        <Notice tone="info" compact announce={false}>{msg('settings.aboutLegalUnavailable')}</Notice>
      {:else}
        <div class="rows">
          {#each page.legalDocs as meta (meta.id)}
            <div class="row">
              <div class="row-text">
                <span class="row-label">{meta.title}</span>
              </div>
              <Button size="sm" onclick={() => page.openLegalDoc(meta)}>{msg('common.open')}</Button>
            </div>
          {/each}
          <div class="row">
            <div class="row-text">
              <span class="row-label">{msg('settings.aboutSourcesNoticeLabel')}</span>
              <span class="row-sub">{msg('settings.aboutSourcesNoticeSub')}</span>
            </div>
            <Button size="sm" onclick={() => (page.sourcesNoticeReviewOpen = true)}>{msg('common.open')}</Button>
          </div>
        </div>
      {/if}
    </Card>
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

  .logs-upload-progress {
    width: min(28rem, 100%);
    margin-top: var(--space-2);
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

  .message {
    margin-top: var(--space-2);
  }
</style>
