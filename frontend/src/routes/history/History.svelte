<script lang="ts">
  import {
    CircleAlert,
    Download,
    History as HistoryIcon,
    Move,
    PackageCheck,
    PackageX,
    RefreshCw,
    RotateCcw,
    Trash2,
    Wifi,
  } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import Tabs from '../../lib/components/Tabs.svelte';
  import { filterHistory } from '../../lib/history/historyFilter';
  import { historyLabel } from '../../lib/history/historyText';
  import { Kind, type Record as HistoryRecord } from '../../lib/services/history';
  import { clearHistory, history, historyStatus } from '../../lib/stores/history';
  import { toast } from '../../lib/stores/toasts';
  import { errorMessage } from '../../lib/utils/errors';
  import { relativeDate } from '../../lib/utils/format';

  const segments: { id: string; label: string; kinds?: Kind[] }[] = [
    { id: 'all', label: 'Все' },
    { id: 'installs', label: 'Установки', kinds: [Kind.KindInstalled, Kind.KindInstallFailed] },
    {
      id: 'updates',
      label: 'Обновления',
      kinds: [Kind.KindUpdated, Kind.KindUpdateFailed, Kind.KindRolledBack],
    },
    { id: 'removals', label: 'Удаления', kinds: [Kind.KindRemoved, Kind.KindUninstalled] },
  ];


  let segment = $state('all');
  let query = $state('');
  let clearOpen = $state(false);
  let clearing = $state(false);

  const kindsFilter = $derived(segments.find((s) => s.id === segment)?.kinds);
  const filtered = $derived(filterHistory($history, { kinds: kindsFilter, query }));

  const tabs = $derived(
    segments.map((s) => ({
      id: s.id,
      label: s.label,
      count: s.kinds ? $history.filter((r) => s.kinds!.includes(r.kind)).length : $history.length,
    })),
  );

  const icons: Record<Kind, typeof HistoryIcon> = {
    [Kind.$zero]: HistoryIcon,
    [Kind.KindInstalled]: PackageCheck,
    [Kind.KindInstallFailed]: PackageX,
    [Kind.KindUpdated]: RefreshCw,
    [Kind.KindUpdateFailed]: PackageX,
    [Kind.KindRolledBack]: RotateCcw,
    [Kind.KindDownloaded]: Download,
    [Kind.KindUninstalled]: Trash2,
    [Kind.KindRemoved]: Trash2,
    [Kind.KindMoved]: Move,
    [Kind.KindLanReceived]: Wifi,
  };

  function iconFor(kind: Kind) {
    return icons[kind] ?? HistoryIcon;
  }

  function failed(kind: Kind) {
    return kind === Kind.KindInstallFailed || kind === Kind.KindUpdateFailed;
  }

  async function confirmClear() {
    clearing = true;
    try {
      await clearHistory();
      toast('История очищена', 'success');
      clearOpen = false;
    } catch (err) {
      toast(errorMessage(err), 'danger');
    } finally {
      clearing = false;
    }
  }
</script>

<Card surface="panel">
  <PageHeader title="История" subtitle="Установки, обновления и удаления игр на этом устройстве">
    {#snippet actions()}
      <Button variant="ghost" disabled={$history.length === 0} onclick={() => (clearOpen = true)}>
        <Trash2 size="1.5rem" strokeWidth={1.8} />
        Очистить историю
      </Button>
    {/snippet}
  </PageHeader>

  {#if $historyStatus.degraded}
    <div class="banner">
      <span class="icon"><CircleAlert size="1.6rem" strokeWidth={1.8} /></span>
      <span class="text">История не сохраняется: {$historyStatus.message}</span>
    </div>
  {/if}

  <Tabs {tabs} bind:value={segment} variant="pill" />

  <div class="toolbar">
    <div class="search-slot">
      <SearchInput bind:value={query} placeholder="Поиск по названию" />
    </div>
  </div>

  {#if $history.length === 0}
    <EmptyState title="История пока пуста" description="Здесь появятся установки, обновления и удаления игр.">
      {#snippet icon()}
        <HistoryIcon size="2rem" strokeWidth={1.8} />
      {/snippet}
    </EmptyState>
  {:else if filtered.length === 0}
    <EmptyState title="Ничего не найдено" description="Измените фильтр или поисковый запрос." />
  {:else}
    <div class="table">
      {#each filtered as record (record.id)}
        {@const label = historyLabel(record)}
        {@const Icon = iconFor(record.kind)}
        <div class="row" class:failed={failed(record.kind)}>
          <span class="icon-cell"><Icon size="1.7rem" strokeWidth={1.8} /></span>
          <div class="body">
            <span class="title">{label.title}</span>
            {#if label.detail}
              <span class="detail">{label.detail}</span>
            {/if}
          </div>
          <span class="when">{relativeDate(record.at)}</span>
        </div>
      {/each}
    </div>
  {/if}
</Card>

<Modal bind:open={clearOpen} title="Очистить историю">
  <p class="confirm-text">
    Записи об установках, обновлениях и удалениях игр будут удалены безвозвратно. Сами игры и их файлы это не
    затронет.
  </p>
  {#snippet footer()}
    <Button onclick={() => (clearOpen = false)}>Отмена</Button>
    <Button variant="danger" disabled={clearing} onclick={confirmClear}>
      {clearing ? 'Очищаем...' : 'Очистить историю'}
    </Button>
  {/snippet}
</Modal>

<style>
  .banner {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    margin-bottom: var(--space-5);
    background: var(--danger-subtle);
    border-radius: var(--radius-md);
  }

  .banner .icon {
    display: flex;
    flex-shrink: 0;
    color: var(--danger);
  }

  .banner .text {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .toolbar {
    margin: var(--space-5) 0 var(--space-6);
  }

  .search-slot {
    width: 28rem;
    flex-shrink: 0;
  }

  .table {
    display: flex;
    flex-direction: column;
    max-width: 100rem;
  }

  .row {
    display: grid;
    grid-template-columns: 4rem 1fr 12rem;
    align-items: center;
    gap: var(--space-4);
    padding: 1.1rem 1.2rem;
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .icon-cell {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 3.6rem;
    height: 3.6rem;
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text-2);
  }

  .row.failed .icon-cell {
    color: var(--danger);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .title {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .detail {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .when {
    text-align: right;
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  .confirm-text {
    color: var(--text-2);
    line-height: 1.55;
  }

  @media (max-width: 1200px) {
    .row {
      grid-template-columns: 4rem 1fr 9rem;
    }
  }
</style>
