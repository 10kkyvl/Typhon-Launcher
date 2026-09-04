<script lang="ts">
  import { ChevronDown, History, RotateCcw, X } from '@lucide/svelte';
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
  import { bytesSize, relativeDate } from '../utils/format';
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
  const headline = $derived(isUpdate ? msg('ui.updateAvailable') : msg('ui.newReleaseAvailable'));

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
    <div class="titles">
      <h3 class="card-title">{headline}</h3>
      <p class="versions">
        <span class="from">{availability.installedVersion || msg('ui.versionUnknown')}</span>
        <span class="arrow">→</span>
        <span class="to">{availability.targetVersion || msg('ui.newRelease')}</span>
      </p>
    </div>
    <div class="badges">
      {#if update.state === 'update_ready'}
        <StatusBadge kind="success" label={msg('ui.readyToInstall')} />
      {:else if busy}
        <StatusBadge kind="accent" label={stepLabels(update.step ?? 'download')} />
      {:else if !isUpdate}
        <StatusBadge kind="warning" label={msg('ui.versionsNotComparable')} dot={false} />
      {/if}
    </div>
  </div>

  {#if update.planning}
    <p class="muted">{msg('ui.calculatingDownloadSize')}</p>
  {:else if busy}
    <div class="progress">
      <ProgressBar value={update.progress * 100} />
      <span class="muted">{Math.round(update.progress * 100)}%</span>
    </div>
  {:else}
    <dl class="summary">
      <div>
        <dt>{msg('ui.downloadLabel')}</dt>
        <dd>{sizeLabel}</dd>
      </div>
      <div>
        <dt>{msg('ui.method')}</dt>
        <dd>{strategyLabel}</dd>
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
    </dl>
  {/if}

  {#if availability.reason && !isUpdate}
    <p class="muted reason">{availability.reason}</p>
  {/if}
  {#if update.error}
    <p class="error">{update.error}</p>
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
        <ChevronDown size="1.5rem" strokeWidth={1.8} />
      </Button>
    {:else}
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
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .card-title {
    font-size: var(--font-lg);
    font-weight: 600;
    margin: 0;
  }

  .versions {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    margin: 0.6rem 0 0;
    font-size: var(--font-md);
    font-variant-numeric: tabular-nums;
    min-width: 0;
  }

  .versions span {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .from {
    color: var(--text-3);
  }

  .arrow {
    color: var(--text-3);
  }

  .to {
    font-weight: 500;
  }

  .badges {
    display: flex;
    gap: 0.8rem;
  }

  .summary {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-5);
    margin: 0 0 var(--space-4);
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
