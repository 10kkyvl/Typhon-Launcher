<script lang="ts">
  import { updateErrorText } from '../updates/updateErrors';
  import { ArrowUp, ChevronDown, History, RotateCcw, Sparkles, X } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import Modal from './Modal.svelte';
  import type { StrategyType, Update } from '../services/updates';
  import { getUpdateHistory, type UpdateHistory } from '../services/updates';
  import {
    abortUpdate,
    applyUpdate,
    preparePlan,
    restorePrevious,
    stepLabels,
    strategyLabels,
  } from '../stores/updates';
  import { bytesSize, progressPercent, relativeDate } from '../utils/format';
  import { updateReasonKey } from './updateReason';
  import { msg } from '../i18n';

  let { update, running }: { update: Update; running: boolean } = $props();

  let detailsOpen = $state(false);
  let confirmOpen = $state(false);
  let historyOpen = $state(false);
  let history = $state<UpdateHistory[]>([]);

  const availability = $derived(update.availability);
  const plan = $derived(update.plan ?? null);
  const busy = $derived(update.state === 'updating' || update.state === 'update_downloading');
  const isUpdate = $derived(availability.kind === 'update');
  const noRelease = $derived(availability.kind === 'none');
  const headline = $derived.by(() => {
    if (noRelease) {
      if (busy) return msg('ui.updateInProgress');
      if (update.state === 'update_failed') return msg('ui.updateFailedTitle');
      return msg('ui.latestVersionInstalled');
    }
    return isUpdate ? msg('ui.updateAvailable') : msg('ui.newReleaseAvailable');
  });
  const reasonKey = $derived(
    !isUpdate && availability.reason ? updateReasonKey(availability.reason) : 'ui.versionsNotComparable',
  );
  const cautious = $derived(
    !isUpdate &&
      !noRelease &&
      reasonKey !== 'ui.versionsNotComparable' &&
      reasonKey !== 'ui.newDistributionRevisionReason',
  );
  const explanation = $derived.by(() => {
    if (noRelease) return '';
    if (isUpdate) return msg('ui.updateAvailableHint');
    if (reasonKey === 'ui.versionsNotComparable') return msg('ui.versionsNotComparableHint');
    return msg(reasonKey);
  });
  const fromVersion = $derived(availability.installedVersion || (noRelease ? msg('ui.versionUnknown') : ''));
  const toVersion = $derived(noRelease ? '' : availability.targetVersion);
  const versionsLabel = $derived(
    fromVersion && toVersion ? 'games.detailFactVersion' : fromVersion ? 'ui.currentVersion' : 'ui.newVersion',
  );

  const sizeLabel = $derived.by(() => {
    if (plan) return bytesSize(plan.downloadBytes);
    if (availability.estimatedDownloadBytes > 0) return bytesSize(availability.estimatedDownloadBytes);
    return '—';
  });

  const strategyLabel = $derived(strategyLabels(plan?.strategy ?? availability.strategy));

  async function openHistory() {
    history = await getUpdateHistory(update.gameId);
    historyOpen = true;
  }

  function confirm() {
    confirmOpen = false;
    applyUpdate(update.gameId);
  }

  export function start() {
    if (busy || running) return;
    if (plan) {
      confirmOpen = true;
      return;
    }
    if (!update.planning) preparePlan(update.gameId);
  }
</script>

<div class="section">
<Card>
  <div class="head">
    <span class="mark" class:cautious aria-hidden="true">
      {#if isUpdate}
        <ArrowUp size="2rem" strokeWidth={2} />
      {:else}
        <Sparkles size="2rem" strokeWidth={1.8} />
      {/if}
    </span>
    <div class="titles">
      <h3 class="card-title">{headline}</h3>
      {#if explanation}
        <p class="explain" class:cautious>{explanation}</p>
      {/if}
    </div>
    {#if update.state === 'update_ready'}
      <div class="badges">
        <StatusBadge kind="success" label={msg('ui.readyToInstall')} />
      </div>
    {:else if busy}
      <div class="badges">
        <StatusBadge kind="accent" label={stepLabels(update.step ?? 'download')} />
      </div>
    {/if}
  </div>

  <div class="facts-clip">
    <dl class="summary facts">
      {#if fromVersion || toVersion}
        <div>
          <dt>{msg(versionsLabel)}</dt>
          <dd class="versions">
            {#if fromVersion}
              <span class="version" class:from={Boolean(toVersion)} title={fromVersion}>{fromVersion}</span>
            {/if}
            {#if fromVersion && toVersion}
              <span class="arrow" aria-hidden="true">→</span>
            {/if}
            {#if toVersion}
              <span class="version" title={toVersion}>{toVersion}</span>
            {/if}
          </dd>
        </div>
      {/if}
      {#if !update.planning && !busy && (!noRelease || plan)}
        <div>
          <dt>{msg('ui.downloadLabel')}</dt>
          <dd>{sizeLabel}</dd>
        </div>
        <div>
          <dt>{msg('ui.method')}</dt>
          <dd title={strategyLabel}>{strategyLabel}</dd>
        </div>
        {#if plan && plan.reusedBytes > 0}
          <div>
            <dt>{msg('ui.alreadyHave')}</dt>
            <dd>{bytesSize(plan.reusedBytes)}</dd>
          </div>
        {/if}
        {#if plan && availability.patchCount > 0}
          <div>
            <dt>{msg('ui.patches')}</dt>
            <dd>{availability.patchCount}</dd>
          </div>
        {/if}
      {/if}
    </dl>
  </div>

  {#if update.planning}
    <p class="muted reason">{msg('ui.calculatingDownloadSize')}</p>
  {:else if busy}
    <div class="progress">
      <ProgressBar value={update.progress * 100} />
      <span class="muted">{progressPercent(update.progress)}%</span>
    </div>
  {/if}

  {#if update.error}
    <p class="error">{updateErrorText(update.error)}</p>
  {/if}
  {#if running}
    <p class="muted reason">{msg('ui.gameRunningBeforeUpdate')}</p>
  {/if}

  <div class="actions">
    {#if busy}
      <Button onclick={() => abortUpdate(update.gameId)}>
        <X size="1.5rem" strokeWidth={1.8} />
        {msg('ui.abort')}
      </Button>
    {:else if plan}
      <Button variant="primary" disabled={running} onclick={() => (confirmOpen = true)}>{msg('ui.updateAction')}</Button>
      <Button onclick={() => (detailsOpen = !detailsOpen)}>
        {msg('ui.details')}
        <span class="chevron" class:open={detailsOpen}>
          <ChevronDown size="1.5rem" strokeWidth={1.8} />
        </span>
      </Button>
    {:else if !noRelease}
      <Button variant="primary" disabled={update.planning} onclick={() => preparePlan(update.gameId)}>
        {msg('ui.calculateUpdate')}
      </Button>
    {/if}
    {#if update.canRollback}
      <Button onclick={() => restorePrevious(update.gameId)}>
        <RotateCcw size="1.5rem" strokeWidth={1.8} />
        {msg('ui.restorePreviousVersion')}
      </Button>
    {/if}
    <Button onclick={openHistory}>
      <History size="1.5rem" strokeWidth={1.8} />
      {msg('ui.history')}
    </Button>
  </div>

  {#if detailsOpen && plan}
    <div class="details">
      <dl class="summary">
        <div>
          <dt>{msg('ui.current')}</dt>
          <dd>{plan.installedVersion || '—'}</dd>
        </div>
        <div>
          <dt>{msg('ui.target')}</dt>
          <dd>{plan.targetVersion || '—'}</dd>
        </div>
        <div>
          <dt>{msg('ui.spaceNeeded')}</dt>
          <dd>{bytesSize(plan.requiredDiskBytes)}</dd>
        </div>
        <div>
          <dt>{msg('ui.saveBackup')}</dt>
          <dd>{plan.backupAvailable ? msg('ui.backupWillBeCreated') : msg('ui.backupUnavailable')}</dd>
        </div>
        <div>
          <dt>{msg('ui.rollback')}</dt>
          <dd>{plan.rollbackAvailable ? msg('ui.available') : msg('ui.notAvailable')}</dd>
        </div>
        {#if update.savesBackup}
          <div>
            <dt>{msg('ui.saveBackupPath')}</dt>
            <dd class="path">{update.savesBackup}</dd>
          </div>
        {/if}
      </dl>
      <ol class="steps">
        {#each plan.steps as step, index (index)}
          <li>
            <span class="step-label">{step.label}</span>
            {#if step.bytes}<span class="muted">{bytesSize(step.bytes)}</span>{/if}
          </li>
        {/each}
      </ol>
    </div>
  {/if}
</Card>
</div>

<Modal bind:open={confirmOpen} title={msg('ui.updateGameTitle')}>
  <dl class="summary modal-summary">
    <div>
      <dt>{msg('ui.currentVersion')}</dt>
      <dd>{plan?.installedVersion || '—'}</dd>
    </div>
    <div>
      <dt>{msg('ui.newVersion')}</dt>
      <dd>{plan?.targetVersion || '—'}</dd>
    </div>
    <div>
      <dt>{msg('ui.downloadLabel')}</dt>
      <dd>{sizeLabel}</dd>
    </div>
    <div>
      <dt>{msg('ui.spaceNeeded')}</dt>
      <dd>{plan ? bytesSize(plan.requiredDiskBytes) : '—'}</dd>
    </div>
    <div>
      <dt>{msg('ui.method')}</dt>
      <dd>{strategyLabel}</dd>
    </div>
    <div>
      <dt>{msg('ui.saveBackup')}</dt>
      <dd>{plan?.backupAvailable ? msg('ui.backupWillBeCreated') : msg('ui.backupUnavailable')}</dd>
    </div>
  </dl>
  {#snippet footer()}
    <Button onclick={() => (confirmOpen = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" onclick={confirm}>{msg('ui.updateAction')}</Button>
  {/snippet}
</Modal>

<Modal bind:open={historyOpen} title={msg('ui.updateHistoryTitle')}>
  {#if history.length === 0}
    <p class="muted">{msg('ui.noUpdatesYet')}</p>
  {:else}
    <ul class="history">
      {#each history as entry (entry.id)}
        <li>
          <span class="step-label">{entry.fromVersion || '—'} → {entry.toVersion || '—'}</span>
          <span class="muted">{strategyLabels(entry.strategy as StrategyType) ?? entry.strategy}</span>
          <span class="muted">{relativeDate(entry.startedAt)}</span>
          <span class="muted">{entry.status}</span>
        </li>
      {/each}
    </ul>
  {/if}
  {#snippet footer()}
    <Button onclick={() => (historyOpen = false)}>{msg('common.close')}</Button>
  {/snippet}
</Modal>

<style>
  .section {
    max-width: 120rem;
    margin-bottom: var(--space-6);
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-5);
  }

  .mark {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 4.4rem;
    height: 4.4rem;
    flex-shrink: 0;
    border-radius: 50%;
    background: var(--accent-subtle);
    color: var(--accent-text);
    animation: mark-in var(--dur-slow) var(--ease-spring) both;
  }

  .titles {
    flex: 1;
    min-width: 0;
  }

  .card-title {
    font-size: var(--font-lg);
    font-weight: 600;
    margin: 0;
  }

  .explain {
    margin: 0.4rem 0 0;
    font-size: var(--font-sm);
    line-height: 1.45;
    color: var(--text-2);
  }

  .badges {
    display: flex;
    gap: 0.8rem;
    flex-shrink: 0;
    align-self: flex-start;
  }

  .summary {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-5);
    margin: 0 0 var(--space-4);
  }

  .mark.cautious {
    background: var(--warning-subtle);
    color: var(--warning);
  }

  .explain.cautious {
    color: var(--warning);
  }

  .facts-clip {
    overflow: hidden;
    margin-bottom: var(--space-5);
  }

  .facts {
    gap: var(--space-4) 0;
    margin: 0 0 0 calc(var(--space-5) * -1 - 1px);
  }

  .facts > div {
    min-width: 0;
    max-width: 100%;
    padding: 0 var(--space-5);
    border-left: 1px solid var(--border);
  }

  .facts dd {
    max-width: 28ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .facts .versions {
    display: flex;
    align-items: baseline;
    gap: 0.8rem;
    max-width: none;
  }

  .version {
    min-width: 0;
    max-width: 24ch;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .version.from,
  .arrow {
    color: var(--text-2);
  }

  .arrow {
    flex-shrink: 0;
  }

  .chevron {
    display: inline-flex;
    transition: transform var(--dur) var(--ease);
  }

  .chevron.open {
    transform: rotate(180deg);
  }

  @keyframes mark-in {
    from {
      opacity: 0;
      transform: scale(0.6);
    }
  }

  @keyframes details-in {
    from {
      opacity: 0;
      transform: translateY(-0.6rem);
    }
  }

  .summary dt {
    font-size: var(--font-xs);
    color: var(--text-3);
    margin-bottom: 0.4rem;
  }

  .summary dd {
    margin: 0;
    font-size: var(--font-md);
    font-weight: 500;
    font-variant-numeric: tabular-nums;
  }

  .summary dd.path {
    font-size: var(--font-sm);
    font-weight: 400;
    max-width: 32ch;
    overflow-wrap: anywhere;
  }

  .modal-summary {
    margin-bottom: 0;
  }

  .progress {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .progress :global(.track) {
    flex: 1;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .details {
    margin-top: var(--space-5);
    padding-top: var(--space-5);
    border-top: 1px solid var(--border);
    animation: details-in var(--dur-panel) var(--ease) both;
  }

  .steps {
    margin: 0;
    padding-left: 2.2rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .steps li,
  .history li {
    display: flex;
    gap: var(--space-4);
    align-items: baseline;
  }

  .history {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .step-label {
    font-size: var(--font-md);
  }

  .muted {
    color: var(--text-3);
    font-size: var(--font-xs);
    margin: 0;
  }

  .reason {
    margin-bottom: var(--space-4);
  }

  .error {
    color: var(--danger);
    font-size: var(--font-xs);
    margin: 0 0 var(--space-4);
  }
</style>
