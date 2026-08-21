<script lang="ts">
  import { untrack } from 'svelte';
  import { ChevronLeft, ChevronRight } from '@lucide/svelte';
  import {
    getSourceDetails,
    queryReleases,
    type ReleaseView,
    type SourceDetails,
  } from '../services/sources';
  import { refresh as refreshSource, remove as removeSource, sources, toggle as toggleSource } from '../stores/sources';
  import { relativeDate, bytesSize } from '../utils/format';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ReleaseMatchModal from './ReleaseMatchModal.svelte';
  import SearchInput from './SearchInput.svelte';
  import Select from './Select.svelte';
  import StatusBadge from './StatusBadge.svelte';

  let { open = $bindable(false), sourceId }: { open?: boolean; sourceId: string | null } = $props();

  const source = $derived(sourceId ? $sources.find((s) => s.id === sourceId) : undefined);

  const statusOptions = [
    { id: 'all', label: 'Все' },
    { id: 'matched', label: 'Сопоставлено' },
    { id: 'review', label: 'На проверку' },
    { id: 'unmatched', label: 'Без совпадения' },
    { id: 'removed', label: 'Удалённые' },
    { id: 'new', label: 'Новые' },
  ];

  const statusBadge: Record<string, { kind: 'success' | 'warning' | 'neutral'; label: string }> = {
    active: { kind: 'success', label: 'Активен' },
    disabled: { kind: 'neutral', label: 'Отключен' },
    error: { kind: 'neutral', label: 'Ошибка' },
    updating: { kind: 'neutral', label: 'Обновление' },
  };

  let details = $state<SourceDetails | null>(null);
  let filterStatus = $state('all');
  let search = $state('');
  let page = $state(1);
  const pageSize = 50;
  let releases = $state<ReleaseView[]>([]);
  let total = $state(0);
  let loadingReleases = $state(false);
  let refreshing = $state(false);
  let searchTimer: ReturnType<typeof setTimeout> | undefined;

  let matchOpen = $state(false);
  let matchRelease = $state<ReleaseView | null>(null);

  $effect(() => {
    const id = sourceId;
    const isOpen = open;
    untrack(() => {
      if (isOpen && id) {
        filterStatus = 'all';
        search = '';
        page = 1;
        loadDetails(id);
        loadReleases(id);
      }
    });
  });

  async function loadDetails(id: string) {
    try {
      details = await getSourceDetails(id);
    } catch {
      details = null;
    }
  }

  async function loadReleases(id: string) {
    loadingReleases = true;
    try {
      const result = await queryReleases({ sourceId: id, search, status: filterStatus, page, pageSize });
      releases = result.items;
      total = result.total;
    } catch {
      releases = [];
      total = 0;
    } finally {
      loadingReleases = false;
    }
  }

  function reloadAll() {
    if (!sourceId) return;
    loadDetails(sourceId);
    loadReleases(sourceId);
  }

  function onFilterChange() {
    page = 1;
    if (sourceId) loadReleases(sourceId);
  }

  function onSearchInput() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      page = 1;
      if (sourceId) loadReleases(sourceId);
    }, 250);
  }

  function prevPage() {
    if (page <= 1) return;
    page -= 1;
    if (sourceId) loadReleases(sourceId);
  }

  function nextPage() {
    if (page * pageSize >= total) return;
    page += 1;
    if (sourceId) loadReleases(sourceId);
  }

  async function doRefresh() {
    if (!sourceId) return;
    refreshing = true;
    await refreshSource(sourceId);
    refreshing = false;
    reloadAll();
  }

  async function doToggle() {
    if (!source) return;
    await toggleSource(source.id, !source.enabled);
  }

  async function doRemove() {
    if (!source) return;
    if (!window.confirm(`Удалить источник «${source.name}»?`)) return;
    await removeSource(source.id);
    open = false;
  }

  function openMatch(view: ReleaseView) {
    if (view.release.availability === 'removed') return;
    if (view.release.matchStatus === 'review' || view.release.matchStatus === 'unmatched') {
      matchRelease = view;
      matchOpen = true;
    }
  }

  function matchLabel(view: ReleaseView) {
    if (view.release.matchStatus === 'matched') return { kind: 'success' as const, label: 'Сопоставлено' };
    if (view.release.matchStatus === 'review') return { kind: 'warning' as const, label: 'На проверку' };
    return { kind: 'neutral' as const, label: 'Без совпадения' };
  }

  const from = $derived(total === 0 ? 0 : (page - 1) * pageSize + 1);
  const to = $derived(Math.min(page * pageSize, total));
</script>

<Modal bind:open title={source?.name ?? 'Источник'} width="92rem">
  {#if source}
    <div class="sections">
      <section class="head">
        <div class="head-meta">
          <span class="url">{source.url}</span>
          <div class="head-badges">
            <StatusBadge kind={statusBadge[source.status]?.kind ?? 'neutral'} label={statusBadge[source.status]?.label ?? source.status} />
            <span class="updated">Обновлено: {relativeDate(source.lastUpdatedAt)}</span>
          </div>
          {#if source.lastError}
            <p class="error">{source.lastError}</p>
          {/if}
        </div>
        <div class="head-actions">
          <Button size="sm" disabled={refreshing || source.status === 'updating'} onclick={doRefresh}>
            {source.status === 'updating' ? 'Обновление…' : 'Обновить'}
          </Button>
          <Button size="sm" onclick={doToggle}>{source.enabled ? 'Отключить' : 'Включить'}</Button>
          <Button size="sm" variant="danger" onclick={doRemove}>Удалить</Button>
        </div>
      </section>

      {#if details}
        <section class="counters">
          <div class="counter"><span class="counter-value">{details.total}</span><span class="counter-label">Всего</span></div>
          <div class="counter"><span class="counter-value">{details.available}</span><span class="counter-label">Доступно</span></div>
          <div class="counter"><span class="counter-value">{details.removed}</span><span class="counter-label">Удалено</span></div>
          <div class="counter"><span class="counter-value">{details.new}</span><span class="counter-label">Новых</span></div>
          <div class="counter"><span class="counter-value">{details.ignored}</span><span class="counter-label">Игнор</span></div>
        </section>
      {/if}

      <section class="filters">
        <Select options={statusOptions} bind:value={filterStatus} width="18rem" onchange={onFilterChange} />
        <SearchInput bind:value={search} placeholder="Поиск релизов" oninput={onSearchInput} />
      </section>

      <section class="table">
        <div class="thead">
          <span class="th">Исходное название</span>
          <span class="th">Игра</span>
          <span class="th">Версия</span>
          <span class="th">Размер</span>
          <span class="th">Загружено</span>
          <span class="th">Статус</span>
        </div>
        {#if loadingReleases}
          <p class="empty">Загрузка…</p>
        {:else if releases.length === 0}
          <p class="empty">Релизов не найдено</p>
        {:else}
          {#each releases as view (view.release.id)}
            {@const badge = matchLabel(view)}
            {@const clickable = view.release.availability !== 'removed' && (view.release.matchStatus === 'review' || view.release.matchStatus === 'unmatched')}
            <button class="row" class:clickable onclick={() => openMatch(view)} disabled={!clickable}>
              <span class="cell title" title={view.release.rawTitle}>{view.release.rawTitle}</span>
              <span class="cell">{view.gameTitle || '—'}</span>
              <span class="cell">{view.release.edition || view.release.version || '—'}</span>
              <span class="cell nums">{bytesSize(view.release.size)}</span>
              <span class="cell">{relativeDate(view.release.uploadedAt)}</span>
              <span class="cell status">
                {#if view.release.availability === 'removed'}
                  <StatusBadge kind="danger" label="Недоступен" />
                {:else}
                  <StatusBadge kind={badge.kind} label={badge.label} />
                  {#if view.release.matchStatus === 'matched'}
                    <span class="pct">{Math.round(view.release.matchConfidence * 100)}%</span>
                  {/if}
                {/if}
                {#if view.release.new}
                  <StatusBadge kind="accent" label="Новое" dot={false} />
                {/if}
              </span>
            </button>
          {/each}
        {/if}
        <div class="tfoot">
          <span class="range">{from}–{to} из {total}</span>
          <div class="pager">
            <Button size="sm" disabled={page <= 1} onclick={prevPage}>
              <ChevronLeft size="1.5rem" strokeWidth={1.8} />
            </Button>
            <Button size="sm" disabled={page * pageSize >= total} onclick={nextPage}>
              <ChevronRight size="1.5rem" strokeWidth={1.8} />
            </Button>
          </div>
        </div>
      </section>
    </div>
  {/if}
</Modal>

<ReleaseMatchModal bind:open={matchOpen} release={matchRelease} onchanged={reloadAll} />

<style>
  .sections {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .head-meta {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
    min-width: 0;
  }

  .url {
    font-size: 1.4rem;
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .head-badges {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .updated {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .error {
    font-size: 1.3rem;
    color: var(--danger);
  }

  .head-actions {
    display: flex;
    gap: 0.8rem;
    flex-shrink: 0;
  }

  .counters {
    display: grid;
    grid-template-columns: repeat(5, 1fr);
    gap: var(--space-3);
  }

  .counter {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .counter-value {
    font-size: 1.9rem;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .counter-label {
    font-size: 1.2rem;
    color: var(--text-3);
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .filters :global(.search) {
    flex: 1;
  }

  .table {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    overflow: hidden;
  }

  .thead,
  .row {
    display: grid;
    grid-template-columns: minmax(16rem, 1.6fr) minmax(12rem, 1fr) 9rem 8rem 10rem 16rem;
    align-items: center;
    gap: var(--space-3);
    padding: 0 var(--space-4);
  }

  .thead {
    height: 4rem;
    border-bottom: 1px solid var(--border);
  }

  .th {
    font-size: 1.2rem;
    font-weight: 500;
    color: var(--text-3);
  }

  .row {
    width: 100%;
    height: 4.6rem;
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row.clickable {
    cursor: pointer;
  }

  .row.clickable:hover {
    background: rgba(255, 255, 255, 0.03);
  }

  .row:disabled {
    cursor: default;
  }

  .cell {
    font-size: 1.4rem;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cell.title {
    color: var(--text);
  }

  .cell.nums {
    font-variant-numeric: tabular-nums;
  }

  .cell.status {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    overflow: visible;
  }

  .pct {
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .empty {
    padding: var(--space-6);
    text-align: center;
    font-size: 1.4rem;
    color: var(--text-3);
  }

  .tfoot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 1.2rem var(--space-4);
    border-top: 1px solid var(--border);
  }

  .range {
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .pager {
    display: flex;
    gap: 0.6rem;
  }
</style>
