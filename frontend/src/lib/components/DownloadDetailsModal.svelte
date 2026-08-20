<script lang="ts">
  import { FolderOpen } from '@lucide/svelte';
  import { openFolder } from '../services/settings';
  import { downloadsById, statusLabels } from '../stores/downloads';
  import { toast } from '../stores/toasts';
  import { bytesSize, speedBytes } from '../utils/format';
  import IconButton from './IconButton.svelte';
  import Modal from './Modal.svelte';
  import ProgressBar from './ProgressBar.svelte';

  let { open = $bindable(false), id }: { open?: boolean; id: string | null } = $props();

  const download = $derived(id ? $downloadsById.get(id) : undefined);

  function addedLabel(iso: string) {
    const date = new Date(iso);
    return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('ru-RU');
  }

  async function openDestination(path: string) {
    try {
      await openFolder(path);
    } catch {
      toast('Папка недоступна', 'danger');
    }
  }
</script>

<Modal bind:open title={download?.name ?? 'Загрузка'} width="62rem">
  {#if download}
    <div class="sections">
      <section class="block">
        <h4>Общее</h4>
        <div class="rows">
          <div class="row">
            <span class="key">Состояние</span>
            <span class="value">{statusLabels[download.status]}</span>
          </div>
          <div class="row">
            <span class="key">Папка</span>
            <span class="value path">
              {download.destination}
              <IconButton label="Открыть папку" size="sm" onclick={() => openDestination(download.destination)}>
                <FolderOpen size="1.6rem" strokeWidth={1.8} />
              </IconButton>
            </span>
          </div>
          <div class="row">
            <span class="key">Размер</span>
            <span class="value">{bytesSize(download.total)}</span>
          </div>
          <div class="row">
            <span class="key">Прогресс</span>
            <span class="value">{Math.floor(download.progress * 100)}%</span>
          </div>
          <div class="row">
            <span class="key">Добавлено</span>
            <span class="value">{addedLabel(download.addedAt)}</span>
          </div>
          {#if download.error}
            <div class="row">
              <span class="key">Ошибка</span>
              <span class="value danger">{download.error}</span>
            </div>
          {/if}
        </div>
      </section>

      <section class="block">
        <h4>Сеть</h4>
        <div class="rows">
          <div class="row">
            <span class="key">Сиды</span>
            <span class="value">{download.seeders}</span>
          </div>
          <div class="row">
            <span class="key">Пиры</span>
            <span class="value">{download.peers}</span>
          </div>
          <div class="row">
            <span class="key">Скорость загрузки</span>
            <span class="value">{speedBytes(download.downloadSpeed)}</span>
          </div>
          <div class="row">
            <span class="key">Скорость отдачи</span>
            <span class="value">{speedBytes(download.uploadSpeed)}</span>
          </div>
        </div>
      </section>

      <section class="block">
        <h4>Файлы</h4>
        <div class="file-list">
          {#each download.files as file, i (i)}
            <div class="file" class:skipped={!file.selected}>
              <div class="file-head">
                <span class="file-path">{file.path}</span>
                {#if file.selected}
                  <span class="file-size">{bytesSize(file.bytesDone)} / {bytesSize(file.size)}</span>
                {:else}
                  <span class="file-size">Пропущен</span>
                {/if}
              </div>
              <ProgressBar
                value={file.selected && file.size > 0 ? (file.bytesDone / file.size) * 100 : 0}
                height={4}
                color={file.selected ? 'var(--accent)' : 'var(--text-3)'}
              />
            </div>
          {/each}
        </div>
      </section>
    </div>
  {/if}
</Modal>

<style>
  .sections {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .block h4 {
    font-size: 1.3rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-3);
    margin-bottom: var(--space-2);
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.8rem 0;
    font-size: 1.4rem;
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .key {
    color: var(--text-3);
    flex-shrink: 0;
  }

  .value {
    min-width: 0;
    color: var(--text);
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  .value.path {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
  }

  .value.danger {
    color: var(--danger);
  }

  .file-list {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    max-height: 26rem;
    overflow-y: auto;
    padding: 1rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .file {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .file.skipped {
    opacity: 0.45;
  }

  .file-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .file-path {
    min-width: 0;
    font-size: 1.4rem;
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-size {
    font-size: 1.3rem;
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
    flex-shrink: 0;
  }
</style>
