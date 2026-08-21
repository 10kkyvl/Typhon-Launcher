<script lang="ts">
  import { ChevronDown, History, RotateCcw, X } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import Modal from './Modal.svelte';
  import type { Update } from '../services/updates';
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

  let { update, running }: { update: Update; running: boolean } = $props();

  let detailsOpen = $state(false);
  let confirmOpen = $state(false);
  let historyOpen = $state(false);
  let history = $state<UpdateHistory[]>([]);

  const availability = $derived(update.availability);
  const plan = $derived(update.plan ?? null);
  const busy = $derived(update.state === 'updating' || update.state === 'update_downloading');
  const isUpdate = $derived(availability.kind === 'update');
  const headline = $derived(isUpdate ? 'Доступно обновление' : 'Доступен новый релиз');

  const sizeLabel = $derived.by(() => {
    if (plan) return bytesSize(plan.downloadBytes);
    if (availability.estimatedDownloadBytes > 0) return bytesSize(availability.estimatedDownloadBytes);
    return '—';
  });

  const strategyLabel = $derived(strategyLabels[plan?.strategy ?? availability.strategy] ?? '—');

  async function openHistory() {
    history = await getUpdateHistory(update.gameId);
    historyOpen = true;
  }

  function confirm() {
    confirmOpen = false;
    applyUpdate(update.gameId);
  }
</script>

<Card padding="var(--space-5) var(--space-6)">
  <div class="head">
    <div class="titles">
      <h3 class="card-title">{headline}</h3>
      <p class="versions">
        <span class="from">{availability.installedVersion || 'версия неизвестна'}</span>
        <span class="arrow">→</span>
        <span class="to">{availability.targetVersion || 'новый релиз'}</span>
      </p>
    </div>
    <div class="badges">
      {#if update.state === 'update_ready'}
        <StatusBadge kind="success" label="Готово к установке" />
      {:else if busy}
        <StatusBadge kind="accent" label={stepLabels[update.step ?? 'download']} />
      {:else if !isUpdate}
        <StatusBadge kind="warning" label="Версии не сравнимы" dot={false} />
      {/if}
    </div>
  </div>

  {#if update.planning}
    <p class="muted">Расчёт объёма загрузки…</p>
  {:else if busy}
    <div class="progress">
      <ProgressBar value={update.progress * 100} />
      <span class="muted">{Math.round(update.progress * 100)}%</span>
    </div>
  {:else}
    <dl class="summary">
      <div>
        <dt>Загрузка</dt>
        <dd>{sizeLabel}</dd>
      </div>
      <div>
        <dt>Способ</dt>
        <dd>{strategyLabel}</dd>
      </div>
      {#if plan && plan.reusedBytes > 0}
        <div>
          <dt>Уже есть</dt>
          <dd>{bytesSize(plan.reusedBytes)}</dd>
        </div>
      {/if}
      {#if plan && availability.patchCount > 0}
        <div>
          <dt>Патчей</dt>
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
    <p class="muted reason">Игра запущена. Закройте её перед обновлением.</p>
  {/if}

  <div class="actions">
    {#if busy}
      <Button onclick={() => abortUpdate(update.gameId)}>
        <X size="1.5rem" strokeWidth={1.8} />
        Прервать
      </Button>
    {:else if plan}
      <Button variant="primary" disabled={running} onclick={() => (confirmOpen = true)}>Обновить</Button>
      <Button onclick={() => (detailsOpen = !detailsOpen)}>
        Подробности
        <ChevronDown size="1.5rem" strokeWidth={1.8} />
      </Button>
    {:else}
      <Button variant="primary" disabled={update.planning} onclick={() => preparePlan(update.gameId)}>
        Рассчитать обновление
      </Button>
    {/if}
    {#if update.canRollback}
      <Button onclick={() => restorePrevious(update.gameId)}>
        <RotateCcw size="1.5rem" strokeWidth={1.8} />
        Вернуть предыдущую версию
      </Button>
    {/if}
    <Button onclick={openHistory}>
      <History size="1.5rem" strokeWidth={1.8} />
      История
    </Button>
  </div>

  {#if detailsOpen && plan}
    <div class="details">
      <dl class="summary">
        <div>
          <dt>Текущая</dt>
          <dd>{plan.installedVersion || '—'}</dd>
        </div>
        <div>
          <dt>Целевая</dt>
          <dd>{plan.targetVersion || '—'}</dd>
        </div>
        <div>
          <dt>Нужно места</dt>
          <dd>{bytesSize(plan.requiredDiskBytes)}</dd>
        </div>
        <div>
          <dt>Резервная копия сохранений</dt>
          <dd>{plan.backupAvailable ? 'Будет создана' : 'Недоступна'}</dd>
        </div>
        <div>
          <dt>Откат</dt>
          <dd>{plan.rollbackAvailable ? 'Доступен' : 'Недоступен'}</dd>
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

<Modal bind:open={confirmOpen} title="Обновить игру?">
  <dl class="summary modal-summary">
    <div>
      <dt>Текущая версия</dt>
      <dd>{plan?.installedVersion || '—'}</dd>
    </div>
    <div>
      <dt>Новая версия</dt>
      <dd>{plan?.targetVersion || '—'}</dd>
    </div>
    <div>
      <dt>Загрузка</dt>
      <dd>{sizeLabel}</dd>
    </div>
    <div>
      <dt>Нужно места</dt>
      <dd>{plan ? bytesSize(plan.requiredDiskBytes) : '—'}</dd>
    </div>
    <div>
      <dt>Способ</dt>
      <dd>{strategyLabel}</dd>
    </div>
    <div>
      <dt>Резервная копия сохранений</dt>
      <dd>{plan?.backupAvailable ? 'Будет создана' : 'Недоступна'}</dd>
    </div>
  </dl>
  {#snippet footer()}
    <Button onclick={() => (confirmOpen = false)}>Отмена</Button>
    <Button variant="primary" onclick={confirm}>Обновить</Button>
  {/snippet}
</Modal>

<Modal bind:open={historyOpen} title="История обновлений">
  {#if history.length === 0}
    <p class="muted">Обновлений пока не было.</p>
  {:else}
    <ul class="history">
      {#each history as entry (entry.id)}
        <li>
          <span class="step-label">{entry.fromVersion || '—'} → {entry.toVersion || '—'}</span>
          <span class="muted">{strategyLabels[entry.strategy as keyof typeof strategyLabels] ?? entry.strategy}</span>
          <span class="muted">{relativeDate(entry.startedAt)}</span>
          <span class="muted">{entry.status}</span>
        </li>
      {/each}
    </ul>
  {/if}
  {#snippet footer()}
    <Button onclick={() => (historyOpen = false)}>Закрыть</Button>
  {/snippet}
</Modal>

<style>
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
  }

  .from {
    color: var(--text-dim);
  }

  .arrow {
    color: var(--text-dim);
  }

  .to {
    font-weight: 600;
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
    font-size: 1.3rem;
    color: var(--text-dim);
    margin-bottom: 0.4rem;
  }

  .summary dd {
    margin: 0;
    font-size: var(--font-md);
    font-weight: 500;
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
    color: var(--text-dim);
    font-size: 1.3rem;
    margin: 0;
  }

  .reason {
    margin-bottom: var(--space-4);
  }

  .error {
    color: var(--danger);
    font-size: 1.3rem;
    margin: 0 0 var(--space-4);
  }
</style>
