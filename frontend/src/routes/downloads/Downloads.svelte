<script lang="ts">
  import {
    ArrowDown,
    ArrowUp,
    ChevronDown,
    Clock,
    Download,
    FolderOpen,
    GripVertical,
    Play,
    Plus,
    Settings,
    X,
  } from '@lucide/svelte';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import Button from '../../lib/components/Button.svelte';
  import DownloadDetailsModal from '../../lib/components/DownloadDetailsModal.svelte';
  import DownloadItem from '../../lib/components/DownloadItem.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import EmptyState from '../../lib/components/EmptyState.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import InstallModal from '../../lib/components/InstallModal.svelte';
  import PageHeader from '../../lib/components/PageHeader.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { openFolder } from '../../lib/services/settings';
  import { active, completed, forceStart, moveDown, moveUp, queue, remove, stats } from '../../lib/stores/downloads';
  import { installActive, installStatusLabels, installationsByDownload } from '../../lib/stores/install';
  import { navigate } from '../../lib/stores/router';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { bytesSize, plural, rateLimitLabel, speedBytes } from '../../lib/utils/format';

  const MB = 1024 * 1024;

  const limitItems = [
    { id: 'none', label: 'Без ограничений' },
    { id: '10', label: '10 МБ/с' },
    { id: '25', label: '25 МБ/с' },
    { id: '50', label: '50 МБ/с' },
    { id: 'custom', label: 'Своё значение…' },
  ];

  const limitLabel = $derived(rateLimitLabel($settings?.downloadRateLimit ?? 0));

  function pickLimit(id: string) {
    if (id === 'custom') {
      navigate('settings', { tab: 'downloads' });
      return;
    }
    updateSettings({ downloadRateLimit: id === 'none' ? 0 : Number(id) * MB });
  }

  let addOpen = $state(false);
  let detailsOpen = $state(false);
  let detailsId = $state<string | null>(null);
  let installOpen = $state(false);
  let installDownloadId = $state<string | null>(null);

  const summary = $derived(
    [
      `↓ ${speedBytes($stats.downSpeed)}`,
      `↑ ${speedBytes($stats.upSpeed)}`,
      `${$stats.activeCount} ${plural($stats.activeCount, 'активная', 'активные', 'активных')}`,
      `${$queue.length} в очереди`,
    ].join(' · '),
  );

  function openDetails(id: string) {
    detailsId = id;
    detailsOpen = true;
  }

  function openInstall(id: string) {
    installDownloadId = id;
    installOpen = true;
  }

  function completedDate(iso: string | null) {
    if (!iso) return '—';
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? '—' : date.toLocaleDateString('ru-RU');
  }

  async function openDestination(path: string) {
    try {
      await openFolder(path);
    } catch {
      toast('Папка недоступна', 'danger');
    }
  }
</script>

<PageHeader title="Загрузки" subtitle={summary}>
  {#snippet actions()}
    <DropdownMenu
      items={limitItems}
      onselect={pickLimit}
    >
      {#snippet trigger({ toggle })}
        <Button variant="ghost" onclick={toggle}>
          <Clock size="1.5rem" strokeWidth={1.8} />
          {limitLabel}
          <ChevronDown size="1.4rem" strokeWidth={1.8} />
        </Button>
      {/snippet}
    </DropdownMenu>
    <IconButton label="Настройки загрузок" onclick={() => navigate('settings', { tab: 'downloads' })}>
      <Settings size="1.7rem" strokeWidth={1.8} />
    </IconButton>
    <Button variant="primary" onclick={() => (addOpen = true)}>
      <Plus size="1.5rem" strokeWidth={2} />
      Добавить загрузку
    </Button>
  {/snippet}
</PageHeader>

<section class="section">
  <h2>Активные <span class="count">{$active.length}</span></h2>
  {#if $active.length === 0}
    <EmptyState title="Нет активных загрузок" description="Добавьте торрент или выберите релиз на странице игры.">
      {#snippet icon()}
        <Download size="2rem" strokeWidth={1.8} />
      {/snippet}
      {#snippet actions()}
        <Button variant="primary" onclick={() => (addOpen = true)}>Добавить загрузку</Button>
      {/snippet}
    </EmptyState>
  {:else}
    <div class="downloads">
      {#each $active as download (download.id)}
        <DownloadItem {download} onopen={(d) => openDetails(d.id)} />
      {/each}
    </div>
  {/if}
</section>

<section class="section">
  <h2>В очереди <span class="count">{$queue.length}</span></h2>
  {#if $queue.length === 0}
    <p class="muted">Очередь пуста.</p>
  {:else}
    <div class="rows">
      {#each $queue as q, i (q.id)}
        <div class="row">
          <span class="grip">
            <GripVertical size="1.6rem" strokeWidth={1.8} />
          </span>
          <span class="row-title">{q.name}</span>
          <span class="row-size">{bytesSize(q.total)}</span>
          <div class="row-actions">
            <IconButton label="Выше" size="sm" disabled={i === 0} onclick={() => moveUp(q.id)}>
              <ArrowUp size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton label="Ниже" size="sm" disabled={i === $queue.length - 1} onclick={() => moveDown(q.id)}>
              <ArrowDown size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton label="Начать сейчас" size="sm" onclick={() => forceStart(q.id)}>
              <Play size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton label="Убрать из очереди" size="sm" onclick={() => remove(q.id)}>
              <X size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</section>

{#if $completed.length > 0}
  <section class="section">
    <h2>Завершённые <span class="count">{$completed.length}</span></h2>
    <div class="rows">
      {#each $completed as item (item.id)}
        {@const install = $installationsByDownload.get(item.id)}
        <div class="row">
          <button class="row-title link" onclick={() => openDetails(item.id)}>{item.name}</button>
          <span class="row-size">{bytesSize(item.total)}</span>
          <span class="row-date">{completedDate(item.completedAt)}</span>
          <span class="row-state">
            {#if item.seeding}
              <StatusBadge kind="success" label="Раздаётся" plain />
            {/if}
          </span>
          <div class="install-cell">
            {#if !install}
              <Button size="sm" variant="primary" onclick={() => openInstall(item.id)}>Установить</Button>
            {:else if installActive(install.status)}
              <div class="install-progress">
                <span class="install-status">{installStatusLabels[install.status]}</span>
                <ProgressBar value={install.progress * 100} height={4} />
              </div>
            {:else if install.status === 'waiting_for_user'}
              <Button size="sm" variant="primary" onclick={() => openInstall(item.id)}>Продолжить установку</Button>
            {:else if install.status === 'completed'}
              <StatusBadge kind="success" label="Установлено" plain />
            {:else if install.status === 'failed'}
              <Button size="sm" variant="danger" onclick={() => openInstall(item.id)}>Установка: ошибка</Button>
            {:else}
              <Button size="sm" onclick={() => openInstall(item.id)}>
                {install.status === 'cancelled' ? 'Отменено' : 'Прервано'}
              </Button>
            {/if}
          </div>
          <div class="row-actions">
            <IconButton label="Открыть папку" size="sm" onclick={() => openDestination(item.destination)}>
              <FolderOpen size="1.5rem" strokeWidth={1.8} />
            </IconButton>
            <IconButton label="Удалить из списка" size="sm" onclick={() => remove(item.id)}>
              <X size="1.5rem" strokeWidth={1.8} />
            </IconButton>
          </div>
        </div>
      {/each}
    </div>
  </section>
{/if}

<AddDownloadModal bind:open={addOpen} />
<DownloadDetailsModal bind:open={detailsOpen} id={detailsId} />
<InstallModal bind:open={installOpen} downloadId={installDownloadId} />

<style>
  .section {
    margin-bottom: var(--space-10);
    max-width: 140rem;
  }

  .section h2 {
    display: flex;
    align-items: baseline;
    gap: 0.8rem;
    font-size: var(--font-xl);
    margin-bottom: var(--space-4);
  }

  .count {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .downloads {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
  }

  .muted {
    font-size: var(--font-sm);
    color: var(--text-3);
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-height: 5.2rem;
    padding: 0.6rem 1.2rem;
    margin: 0 -1.2rem;
    border-radius: var(--radius-md);
    transition: background var(--dur) var(--ease);
  }

  .row:hover {
    background: var(--hover);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .grip {
    display: inline-flex;
    color: var(--text-3);
    cursor: grab;
    flex-shrink: 0;
    opacity: 0.5;
  }

  .row:hover .grip {
    opacity: 1;
  }

  .row-title {
    flex: 1;
    min-width: 0;
    font-size: var(--font-md);
    font-weight: 500;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .row-title.link {
    text-align: left;
    transition: color var(--dur) var(--ease);
  }

  .row-title.link:hover {
    color: var(--accent-text);
  }

  .row-size,
  .row-date {
    font-size: var(--font-sm);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
    text-align: right;
  }

  .row-size {
    min-width: 7rem;
  }

  .row-date {
    min-width: 9rem;
  }

  .row-state {
    min-width: 9rem;
    display: inline-flex;
    justify-content: flex-end;
  }

  .row-actions {
    display: flex;
    gap: 2px;
    margin-left: var(--space-2);
  }

  .install-cell {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    min-width: 17rem;
    flex-shrink: 0;
  }

  .install-progress {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    width: 100%;
  }

  .install-status {
    font-size: var(--font-xs);
    color: var(--text-3);
    text-align: right;
  }

  @media (max-width: 1300px) {
    .row-date,
    .row-state {
      display: none;
    }
  }
</style>
