<script lang="ts">
  import { untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { FileSearch, FolderOpen, Gamepad2 } from '@lucide/svelte';
  import { addGame, selectExecutable } from '../services/library';
  import {
    cancelInstall,
    confirmExecutable,
    deleteDownloadData,
    dismissInstall,
    inspectDownload,
    isControlled,
    isExternal,
    retryInstall,
    startInstall,
    type PlanInfo,
  } from '../services/install';
  import { openFolder, selectFolder } from '../services/settings';
  import { downloadsById, errorMessage } from '../stores/downloads';
  import {
    installActive,
    installStatusLabels,
    installTypeLabels,
    installations,
    installationsByDownload,
    upsertInstallation,
  } from '../stores/install';
  import { settings } from '../stores/settings';
  import { toast } from '../stores/toasts';
  import { bytesSize, truncateMiddle } from '../utils/format';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import SegmentedControl from './SegmentedControl.svelte';

  let { open = $bindable(false), downloadId }: { open?: boolean; downloadId: string | null } = $props();

  const modeOptions = [
    { id: 'move', label: 'Переместить' },
    { id: 'copy', label: 'Копировать' },
  ];

  let installId = $state<string | null>(null);
  let info = $state<PlanInfo | null>(null);
  let loading = $state(false);
  let busy = $state(false);
  let destination = $state('');
  let mode = $state('copy');
  let chosen = $state('');
  let cleanupResolved = $state(false);
  let token = 0;

  const installation = $derived(installId ? $installations.find((i) => i.id === installId) : undefined);
  const plan = $derived(info?.plan);
  const controlled = $derived(plan ? isControlled(plan.type) : false);
  const external = $derived(plan ? isExternal(plan.type) : false);
  const portable = $derived(plan?.type === 'portable');
  const silent = $derived(plan?.silent ?? false);
  const notEnoughSpace = $derived(
    !!info && info.requiredBytes > 0 && info.freeBytes > 0 && info.freeBytes < info.requiredBytes,
  );

  const phase = $derived.by(() => {
    const item = installation;
    if (item) {
      if (item.status === 'waiting_for_user') return 'choose';
      if (item.status === 'completed') return 'done';
      if (installActive(item.status)) return 'progress';
      return 'problem';
    }
    if (info) return 'plan';
    return 'loading';
  });

  const externalWait = $derived(
    !!installation &&
      isExternal(installation.type) &&
      !installation.silent &&
      (installation.status === 'pending' || installation.status === 'preparing' || installation.status === 'installing'),
  );

  const candidatePaths = $derived.by(() => {
    const list = (installation?.candidates ?? []).map((c) => c.path);
    if (chosen && !list.includes(chosen)) list.push(chosen);
    return list;
  });

  const askCleanup = $derived($settings?.installCleanupPolicy === 'ask');
  const seedingNow = $derived(!!installation && ($downloadsById.get(installation.downloadId)?.seeding ?? false));

  const primaryLabel = $derived.by(() => {
    switch (plan?.type) {
      case 'portable':
        return mode === 'move' ? 'Переместить и установить' : 'Копировать и установить';
      case 'archive_zip':
      case 'archive_7z':
      case 'archive_rar':
        return 'Распаковать и установить';
      case 'exe_installer':
      case 'msi_installer':
        return silent ? 'Установить' : 'Запустить установщик';
      default:
        return '';
    }
  });

  function reset() {
    token++;
    installId = null;
    info = null;
    loading = false;
    busy = false;
    destination = '';
    mode = 'copy';
    chosen = '';
    cleanupResolved = false;
  }

  async function load(id: string) {
    const existing = get(installationsByDownload).get(id);
    if (existing) {
      installId = existing.id;
      return;
    }
    const current = ++token;
    loading = true;
    try {
      const result = await inspectDownload(id);
      if (!open || current !== token) return;
      info = result;
      destination = result.plan.destination;
      mode = 'copy';
    } catch (err) {
      if (!open || current !== token) return;
      toast(errorMessage(err), 'danger');
      open = false;
    } finally {
      if (current === token) loading = false;
    }
  }

  $effect(() => {
    const isOpen = open;
    const id = downloadId;
    untrack(() => {
      reset();
      if (isOpen && id) load(id);
    });
  });

  $effect(() => {
    const item = installation;
    if (!item || item.status !== 'waiting_for_user') return;
    untrack(() => {
      if (chosen) return;
      chosen = item.executable || item.candidates?.[0]?.path || '';
    });
  });

  function basename(path: string) {
    const parts = path.split(/[\\/]/);
    return parts[parts.length - 1] || path;
  }

  async function browseDestination() {
    try {
      const path = await selectFolder('Выберите папку установки');
      if (path) destination = path;
    } catch {
      toast('Не удалось открыть диалог выбора папки', 'danger');
    }
  }

  async function run(action: () => Promise<void>) {
    busy = true;
    try {
      await action();
    } catch (err) {
      toast(errorMessage(err), 'danger');
    }
    busy = false;
  }

  function start() {
    const current = info;
    if (!current) return;
    return run(async () => {
      const item = await startInstall(current.downloadId, {
        destination: controlled || silent ? destination : '',
        mode: portable ? (current.seeding ? 'copy' : mode) : '',
        type: current.plan.type,
        installerPath: current.plan.installerPath,
      });
      upsertInstallation(item);
      installId = item.id;
    });
  }

  function openSource() {
    const path = plan?.sourcePath;
    if (!path) return;
    return run(async () => {
      await openFolder(path);
    });
  }

  function pickInstaller() {
    const current = info;
    if (!current) return;
    return run(async () => {
      const path = await selectExecutable('Выберите установщик');
      if (!path) return;
      const item = await startInstall(current.downloadId, {
        type: 'exe_installer',
        installerPath: path,
      });
      upsertInstallation(item);
      installId = item.id;
    });
  }

  function pickGameExecutable() {
    const current = info;
    if (!current) return;
    return run(async () => {
      const path = await selectExecutable('Выберите исполняемый файл игры');
      if (!path) return;
      await addGame(path, current.name);
      toast('Игра добавлена в библиотеку', 'success');
      open = false;
    });
  }

  function pickManual() {
    return run(async () => {
      const path = await selectExecutable('Выберите исполняемый файл игры');
      if (path) chosen = path;
    });
  }

  function confirmChoice() {
    const item = installation;
    if (!item || !chosen) return;
    return run(() => confirmExecutable(item.id, chosen));
  }

  function cancelCurrent() {
    const item = installation;
    if (!item) return;
    return run(() => cancelInstall(item.id));
  }

  function retryCurrent() {
    const item = installation;
    if (!item) return;
    return run(() => retryInstall(item.id));
  }

  function dismissCurrent() {
    const item = installation;
    if (!item) return;
    return run(async () => {
      await dismissInstall(item.id);
      installId = null;
      if (downloadId) await load(downloadId);
    });
  }

  function deleteData() {
    const item = installation;
    if (!item) return;
    return run(async () => {
      await deleteDownloadData(item.downloadId);
      cleanupResolved = true;
      toast('Загруженные файлы удалены', 'success');
    });
  }
</script>

<Modal bind:open title="Установка" width="54rem">
  {#if phase === 'loading'}
    <div class="loading">
      <span class="spinner"></span>
      <span class="loading-text">Анализ пакета…</span>
    </div>
  {:else if phase === 'plan' && info && plan}
    <div class="block">
      <div class="pkg">
        <span class="pkg-name">{info.name}</span>
        <span class="pkg-type">{installTypeLabels[plan.type]}</span>
      </div>

      <div class="rows">
        <div class="row">
          <span class="key">Источник</span>
          <span class="value">{truncateMiddle(plan.sourcePath, 48)}</span>
        </div>
        {#if info.requiredBytes > 0}
          <div class="row">
            <span class="key">Требуется места</span>
            <span class="value" class:danger={notEnoughSpace}>{bytesSize(info.requiredBytes)}</span>
          </div>
        {/if}
        {#if info.freeBytes > 0}
          <div class="row">
            <span class="key">Свободно на диске</span>
            <span class="value" class:danger={notEnoughSpace}>{bytesSize(info.freeBytes)}</span>
          </div>
        {/if}
      </div>

      {#if notEnoughSpace}
        <p class="note danger">Недостаточно места на диске для установки.</p>
      {/if}

      {#if controlled || silent}
        <div class="field">
          <span class="field-label">Папка установки</span>
          <div class="field-controls">
            <input class="input sm" type="text" bind:value={destination} />
            <Button size="sm" onclick={browseDestination}>Обзор</Button>
          </div>
        </div>
      {/if}

      {#if portable}
        {#if info.seeding}
          <p class="note">Файлы раздаются — будет использовано копирование</p>
        {:else}
          <div class="field">
            <span class="field-label">Режим установки</span>
            <SegmentedControl options={modeOptions} bind:value={mode} />
          </div>
        {/if}
      {/if}

      {#if external}
        {#if silent}
          <p class="note">Игра установится сама в выбранную папку, окно установщика не появится</p>
        {:else}
          <p class="note">Установщик откроется в отдельном окне</p>
        {/if}
      {/if}

      {#if plan.type === 'unknown'}
        <div class="choices">
          <p class="note">Тип пакета не распознан. Выберите действие вручную.</p>
          <Button onclick={openSource}>
            <FolderOpen size="1.6rem" strokeWidth={1.8} />
            Открыть папку загрузки
          </Button>
          <Button onclick={pickInstaller}>
            <FileSearch size="1.6rem" strokeWidth={1.8} />
            Выбрать установщик…
          </Button>
          <Button onclick={pickGameExecutable}>
            <Gamepad2 size="1.6rem" strokeWidth={1.8} />
            Выбрать исполняемый файл…
          </Button>
        </div>
      {/if}
    </div>
  {:else if phase === 'progress' && installation}
    <div class="block">
      <div class="pkg">
        <span class="pkg-name">{installation.name}</span>
        <span class="pkg-type">{installStatusLabels[installation.status]}</span>
      </div>
      {#if externalWait}
        <p class="note">Ожидание завершения установщика...</p>
      {:else}
        <ProgressBar value={installation.progress * 100} />
        <div class="progress-foot">
          <span class="size">{bytesSize(installation.bytesDone)} / {bytesSize(installation.bytesTotal)}</span>
          <span class="pct">{Math.floor(installation.progress * 100)}%</span>
        </div>
        {#if installation.currentFile}
          <span class="current-file">{truncateMiddle(installation.currentFile, 56)}</span>
        {/if}
      {/if}
    </div>
  {:else if phase === 'choose' && installation}
    <div class="block">
      <span class="lead">Выберите исполняемый файл игры</span>
      <div class="cand-list" role="radiogroup" aria-label="Исполняемые файлы">
        {#each candidatePaths as path (path)}
          <button class="cand" role="radio" aria-checked={chosen === path} onclick={() => (chosen = path)}>
            <span class="radio" class:on={chosen === path}></span>
            <span class="cand-text">
              <span class="cand-name">{basename(path)}</span>
              <span class="cand-path">{path}</span>
            </span>
          </button>
        {/each}
      </div>
      <Button size="sm" onclick={pickManual}>Выбрать вручную…</Button>
    </div>
  {:else if phase === 'done' && installation}
    <div class="block">
      <div class="rows">
        <div class="row">
          <span class="key">Игра</span>
          <span class="value">{installation.name}</span>
        </div>
        <div class="row">
          <span class="key">Папка</span>
          <span class="value">{truncateMiddle(installation.destination, 44)}</span>
        </div>
        <div class="row">
          <span class="key">Версия</span>
          <span class="value">{installation.detectedVersion || 'Неизвестно'}</span>
        </div>
      </div>
      {#if askCleanup && !cleanupResolved}
        <div class="field">
          <span class="field-label">Загруженные файлы</span>
          {#if seedingNow}
            <p class="note danger">Файлы сейчас раздаются. Удаление остановит раздачу.</p>
          {/if}
          <div class="cleanup-actions">
            <Button size="sm" variant="danger" disabled={busy} onclick={deleteData}>Удалить загруженные файлы</Button>
            <Button size="sm" onclick={() => (cleanupResolved = true)}>Оставить</Button>
          </div>
        </div>
      {/if}
    </div>
  {:else if phase === 'problem' && installation}
    <div class="block">
      <div class="pkg">
        <span class="pkg-name">{installation.name}</span>
        <span class="pkg-type">{installStatusLabels[installation.status]}</span>
      </div>
      <p class="note danger">{installation.error || 'Установка не была завершена.'}</p>
    </div>
  {/if}

  {#snippet footer()}
    {#if phase === 'plan' && plan}
      <Button onclick={() => (open = false)}>Отмена</Button>
      {#if plan.type !== 'unknown'}
        <Button
          variant="primary"
          disabled={busy || notEnoughSpace || ((controlled || silent) && destination.trim() === '')}
          onclick={start}
        >
          {primaryLabel}
        </Button>
      {/if}
    {:else if phase === 'progress'}
      <Button onclick={() => (open = false)}>Свернуть</Button>
      <Button variant="danger" disabled={busy || externalWait} onclick={cancelCurrent}>Отменить установку</Button>
    {:else if phase === 'choose'}
      <Button onclick={() => (open = false)}>Закрыть</Button>
      <Button variant="primary" disabled={busy || !chosen} onclick={confirmChoice}>Подтвердить</Button>
    {:else if phase === 'done'}
      <Button variant="primary" onclick={() => (open = false)}>Готово</Button>
    {:else if phase === 'problem'}
      <Button onclick={dismissCurrent} disabled={busy}>Убрать</Button>
      <Button variant="primary" onclick={retryCurrent} disabled={busy}>Повторить</Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1.2rem;
    padding: 3.2rem 0;
  }

  .spinner {
    width: 3.2rem;
    height: 3.2rem;
    border-radius: 50%;
    border: 2px solid rgba(255, 255, 255, 0.1);
    border-top-color: var(--accent);
    animation: spin 800ms linear infinite;
  }

  .loading-text {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .pkg {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .pkg-name {
    font-size: var(--font-lg);
    font-weight: 600;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pkg-type {
    font-size: var(--font-xs);
    color: var(--text-3);
    flex-shrink: 0;
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
    font-size: var(--font-sm);
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
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  .value.danger {
    color: var(--danger);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    align-items: flex-start;
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .field-controls {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    width: 100%;
  }

  .field-controls .input {
    flex: 1;
  }

  .note {
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }

  .note.danger {
    color: var(--danger);
  }

  .choices {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 0.8rem;
    padding-top: var(--space-2);
    border-top: 1px solid var(--border);
  }

  .progress-foot {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .pct {
    color: var(--text-2);
    font-weight: 500;
  }

  .current-file {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .lead {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .cand-list {
    display: flex;
    flex-direction: column;
    max-height: 26rem;
    overflow-y: auto;
    padding: var(--space-1);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .cand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.8rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .cand:hover {
    background: var(--hover);
  }

  .cand[aria-checked='true'] {
    background: var(--hover-strong);
  }

  .radio {
    width: 1.6rem;
    height: 1.6rem;
    flex-shrink: 0;
    border-radius: 50%;
    border: 1px solid var(--border-strong);
    background: var(--surface-2);
    transition:
      background var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease),
      box-shadow var(--dur-fast) var(--ease);
  }

  .radio.on {
    background: var(--accent);
    border-color: var(--accent);
    box-shadow: inset 0 0 0 3px var(--surface-2);
  }

  .cand-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .cand-name {
    font-size: var(--font-sm);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cand-path {
    font-size: 1.2rem;
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cleanup-actions {
    display: flex;
    gap: 0.8rem;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
