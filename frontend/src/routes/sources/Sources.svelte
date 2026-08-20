<script lang="ts">
  import {
    ChevronsUpDown,
    Database,
    Ellipsis,
    EllipsisVertical,
    FolderArchive,
    Gamepad2,
    Globe,
    Info,
    Plus,
    Puzzle,
    RefreshCw,
  } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { addSource, removeSource, sources, toggleSource } from '../../lib/stores/sources';
  import { toast } from '../../lib/stores/toasts';
  import { formatCount } from '../../lib/utils/format';
  import type { Source, SourceStatus } from '../../lib/mock/types';

  let modalOpen = $state(false);
  let newName = $state('');
  let newPath = $state('');

  const kindIcons = {
    core: Database,
    store: Gamepad2,
    mods: Puzzle,
    archive: FolderArchive,
    web: Globe,
  };

  const statusBadge: Record<SourceStatus, { kind: 'success' | 'neutral' | 'danger'; label: string }> = {
    active: { kind: 'success', label: 'Активен' },
    disabled: { kind: 'neutral', label: 'Отключен' },
    error: { kind: 'danger', label: 'Ошибка' },
  };

  function sourceMenu(source: Source) {
    return [
      { id: 'refresh', label: 'Обновить сейчас' },
      { id: 'toggle', label: source.status === 'disabled' ? 'Включить' : 'Отключить' },
      { id: 'folder', label: 'Открыть папку' },
      { id: 'remove', label: 'Удалить источник', danger: true, separator: true },
    ];
  }

  function onSourceMenu(source: Source, action: string) {
    if (action === 'toggle') toggleSource(source.id);
    else if (action === 'remove') {
      removeSource(source.id);
      toast(`Источник «${source.name}» удалён`);
    } else if (action === 'refresh') toast(`«${source.name}» обновляется...`);
    else toast('Действие недоступно в demo');
  }

  function submitSource() {
    if (!newName.trim()) return;
    addSource(newName.trim(), newPath.trim() || 'D:\\Aurora\\Sources\\Custom');
    toast(`Источник «${newName.trim()}» добавлен`, 'success');
    modalOpen = false;
    newName = '';
    newPath = '';
  }
</script>

<PageHeader
  title="Источники"
  subtitle="Источники — это каталоги с играми, дополнениями и файлами, которые Aurora использует для отображения, установки и обновления контента. Все источники хранятся локально на этом устройстве и обновляются клиентом."
>
  {#snippet actions()}
    <Button variant="primary" onclick={() => (modalOpen = true)}>
      <Plus size={16} strokeWidth={2} />
      Добавить источник
    </Button>
    <DropdownMenu
      items={[
        { id: 'refresh-all', label: 'Обновить все' },
        { id: 'export', label: 'Экспортировать список' },
        { id: 'import', label: 'Импортировать' },
      ]}
      onselect={(id) => toast(id === 'refresh-all' ? 'Обновление всех источников...' : 'Действие недоступно в demo')}
    >
      {#snippet trigger({ toggle })}
        <IconButton label="Ещё" onclick={toggle}>
          <Ellipsis size={18} strokeWidth={1.8} />
        </IconButton>
      {/snippet}
    </DropdownMenu>
  {/snippet}
</PageHeader>

{#if $sources.length === 0}
  <div class="empty-wrap">
    <EmptyState title="Источники ещё не добавлены" description="Добавьте первый источник, чтобы Aurora могла находить игры и обновления.">
      {#snippet icon()}
        <Database size={22} strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => (modalOpen = true)}>
          <Plus size={16} strokeWidth={2} />
          Добавить источник
        </Button>
      {/snippet}
    </EmptyState>
  </div>
{:else}
  <div class="table">
    <div class="thead">
      <span class="th">Источник</span>
      <span class="th sortable">Статус <ChevronsUpDown size={13} strokeWidth={1.8} /></span>
      <span class="th sortable">Последнее обновление <ChevronsUpDown size={13} strokeWidth={1.8} /></span>
      <span class="th sortable">Предметов <ChevronsUpDown size={13} strokeWidth={1.8} /></span>
      <span class="th sortable">Версия <ChevronsUpDown size={13} strokeWidth={1.8} /></span>
      <span class="th"></span>
    </div>
    {#each $sources as source (source.id)}
      {@const KindIcon = kindIcons[source.kind]}
      {@const badge = statusBadge[source.status]}
      <div class="row" class:disabled={source.status === 'disabled'}>
        <div class="source">
          <div class="source-icon {source.kind}">
            <KindIcon size={19} strokeWidth={1.8} />
          </div>
          <div class="source-text">
            <span class="source-name">{source.name}</span>
            <span class="source-path">{source.path}</span>
          </div>
        </div>
        <span class="cell"><StatusBadge kind={badge.kind} label={badge.label} /></span>
        <span class="cell">{source.lastUpdate}</span>
        <span class="cell nums">{formatCount(source.items)}</span>
        <span class="cell nums">{source.version}</span>
        <span class="cell menu-cell">
          <DropdownMenu items={sourceMenu(source)} onselect={(id) => onSourceMenu(source, id)}>
            {#snippet trigger({ toggle })}
              <IconButton label="Меню источника" size="sm" onclick={toggle}>
                <EllipsisVertical size={16} strokeWidth={1.8} />
              </IconButton>
            {/snippet}
          </DropdownMenu>
        </span>
      </div>
    {/each}
    <div class="tfoot">
      <span class="note">
        <Info size={15} strokeWidth={1.8} />
        Источники обновляются клиентом автоматически по расписанию.
      </span>
      <button class="check" onclick={() => toast('Проверка обновлений источников...')}>
        <RefreshCw size={15} strokeWidth={1.8} />
        Проверить обновления
      </button>
    </div>
  </div>
{/if}

<Modal bind:open={modalOpen} title="Добавить источник">
  <div class="form">
    <label class="field">
      <span class="field-label">Название</span>
      <input type="text" placeholder="Например, My Game Archive" bind:value={newName} />
    </label>
    <label class="field">
      <span class="field-label">Путь или URL</span>
      <input type="text" placeholder="D:\Aurora\Sources\Custom" bind:value={newPath} />
    </label>
    <p class="form-hint">Источник будет проверен и проиндексирован после добавления.</p>
  </div>
  {#snippet footer()}
    <Button onclick={() => (modalOpen = false)}>Отмена</Button>
    <Button variant="primary" disabled={!newName.trim()} onclick={submitSource}>Добавить</Button>
  {/snippet}
</Modal>

<style>
  .empty-wrap {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
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
    grid-template-columns: minmax(260px, 1.6fr) 130px 1fr 110px 90px 52px;
    align-items: center;
    gap: var(--space-4);
    padding: 0 var(--space-5);
  }

  .thead {
    height: 46px;
    border-bottom: 1px solid var(--border);
  }

  .th {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 12.5px;
    font-weight: 500;
    color: var(--text-3);
  }

  .th.sortable {
    cursor: default;
  }

  .th :global(svg) {
    opacity: 0.6;
  }

  .row {
    padding-top: 13px;
    padding-bottom: 13px;
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: rgba(255, 255, 255, 0.02);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row.disabled .source-name,
  .row.disabled .cell {
    color: var(--text-3);
  }

  .source {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
  }

  .source-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    border-radius: var(--radius-md);
    flex-shrink: 0;
    background: rgba(255, 255, 255, 0.05);
    color: var(--text-2);
  }

  .source-icon.core {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .source-icon.mods {
    background: rgba(155, 110, 220, 0.14);
    color: #b394e8;
  }

  .source-icon.store {
    background: var(--success-subtle);
    color: var(--success);
  }

  .source-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .source-name {
    font-size: 14px;
    font-weight: 550;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .source-path {
    font-size: 12px;
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell {
    font-size: 13.5px;
    color: var(--text-2);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .cell.nums {
    font-variant-numeric: tabular-nums;
  }

  .menu-cell {
    overflow: visible;
    justify-self: end;
  }

  .tfoot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 12px var(--space-5);
    border-top: 1px solid var(--border);
  }

  .note {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 12.5px;
    color: var(--text-3);
  }

  .check {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2);
    padding: 6px 10px;
    border-radius: var(--radius-sm);
    transition:
      background var(--dur) var(--ease),
      color var(--dur) var(--ease);
  }

  .check:hover {
    background: rgba(255, 255, 255, 0.05);
    color: var(--text);
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 7px;
  }

  .field-label {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-2);
  }

  .field input {
    height: 38px;
    padding: 0 12px;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: 14px;
    color: var(--text);
    outline: none;
    transition:
      border-color var(--dur) var(--ease),
      background var(--dur) var(--ease);
  }

  .field input:hover {
    border-color: var(--border-strong);
  }

  .field input:focus {
    border-color: rgba(104, 117, 232, 0.55);
    background: var(--surface-3);
  }

  .field input::placeholder {
    color: var(--text-3);
  }

  .form-hint {
    font-size: 12.5px;
    color: var(--text-3);
  }

  @media (max-width: 1300px) {
    .thead,
    .row {
      grid-template-columns: minmax(220px, 1.6fr) 120px 1fr 90px 52px;
    }

    .th:nth-child(5),
    .cell.nums:nth-of-type(4) {
      display: none;
    }
  }
</style>
