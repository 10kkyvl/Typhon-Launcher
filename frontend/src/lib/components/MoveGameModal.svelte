<script lang="ts">
  import { untrack } from 'svelte';
  import { FolderOpen, HardDrive, ShieldCheck } from '@lucide/svelte';
  import { moveErrorText } from '../relocate/moveMessages';
  import { movePercent, moveSummary } from '../relocate/moveText';
  import { inWails } from '../services/backend';
  import { cancelMove, moveGame, selectMoveTargetFolder, type MoveJob } from '../services/relocate';
  import { getStorageInfoFor, type StorageInfo } from '../services/system';
  import { libraryGames } from '../stores/library';
  import { closeMoveGame, moves, moveTarget } from '../stores/relocate';
  import { toast } from '../stores/toasts';
  import { bytesSize, truncateMiddle } from '../utils/format';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import ProgressBar from './ProgressBar.svelte';

  const game = $derived(
    $moveTarget ? ($libraryGames.find((g) => g.id === $moveTarget?.gameId) ?? null) : null,
  );

  let jobId = $state<string | null>(null);
  let startedJob = $state<MoveJob | null>(null);
  const job = $derived(jobId ? ($moves.find((j) => j.id === jobId) ?? startedJob) : null);
  const jobFailed = $derived(job?.stage === 'failed' ? job : null);
  const jobActive = $derived(job && job.stage !== 'failed' && job.stage !== 'cancelled' && job.stage !== 'done');

  let picked = $state('');
  let diskInfo = $state<StorageInfo | null>(null);
  let picking = $state(false);
  let starting = $state(false);
  let cancelling = $state(false);
  let failure = $state('');
  let loadedFor = $state('');

  $effect(() => {
    const target = $moveTarget;
    untrack(() => {
      if (!target) {
        loadedFor = '';
        return;
      }
      if (loadedFor === target.gameId) return;
      loadedFor = target.gameId;
      picked = '';
      diskInfo = null;
      failure = '';
      jobId = null;
      startedJob = null;
      const existing = $moves.find(
        (j) =>
          j.scope === 'game' &&
          j.gameId === target.gameId &&
          j.stage !== 'done' &&
          j.stage !== 'cancelled' &&
          j.stage !== 'failed',
      );
      if (existing) jobId = existing.id;
    });
  });

  $effect(() => {
    if (job?.stage === 'done') closeMoveGame();
  });

  function message(err: unknown, fallback: string) {
    return moveErrorText(err, fallback);
  }

  async function pickFolder() {
    if (!inWails) {
      toast('Выбор папки доступен только в desktop-сборке');
      return;
    }
    picking = true;
    try {
      const folder = await selectMoveTargetFolder();
      if (!folder) return;
      picked = folder;
      failure = '';
      try {
        diskInfo = await getStorageInfoFor(folder);
      } catch (err) {
        diskInfo = null;
        failure = message(err, 'Не удалось определить свободное место');
      }
    } catch (err) {
      toast(message(err, 'Не удалось открыть диалог выбора папки'), 'danger');
    } finally {
      picking = false;
    }
  }

  async function start() {
    if (!game || !picked || starting) return;
    starting = true;
    failure = '';
    try {
      const created = await moveGame(game.id, picked);
      jobId = created.id;
      startedJob = created;
    } catch (err) {
      failure = message(err, 'Не удалось начать перенос');
    } finally {
      starting = false;
    }
  }

  async function cancel() {
    if (!job || cancelling) return;
    cancelling = true;
    try {
      await cancelMove(job.id);
    } catch (err) {
      toast(message(err, 'Не удалось отменить перенос'), 'danger');
    } finally {
      cancelling = false;
    }
  }

  function retry() {
    jobId = null;
    startedJob = null;
    failure = '';
  }

  function close() {
    closeMoveGame();
  }
</script>

<Modal open={!!$moveTarget} title="Перенос игры" width="52rem" onclose={close}>
  {#if game}
    <div class="move">
      <div class="field">
        <span class="field-label">Сейчас установлена в</span>
        <p class="path">{game.installDir}</p>
        {#if game.sizeBytes > 0}
          <span class="field-sub">Размер игры — {bytesSize(game.sizeBytes)}</span>
        {/if}
      </div>

      {#if jobActive && job}
        <div class="progress">
          <ProgressBar value={movePercent(job)} />
          <div class="progress-text">
            <span>{moveSummary(job)}</span>
            <span>{movePercent(job)}%</span>
          </div>
          {#if job.currentFile}
            <span class="field-sub mono">{truncateMiddle(job.currentFile, 64)}</span>
          {/if}
        </div>
      {:else}
        {#if jobFailed}
          <p class="failure">{moveErrorText(jobFailed.error, 'Не удалось перенести игру')}</p>
        {/if}

        <div class="field">
          <span class="field-label">Новая папка</span>
          <div class="pick-controls">
            <input class="input sm" type="text" readonly placeholder="Папка не выбрана" value={picked} />
            <Button size="sm" disabled={picking} onclick={pickFolder}>
              <FolderOpen size="1.5rem" strokeWidth={1.8} />
              Обзор
            </Button>
          </div>
        </div>

        {#if diskInfo}
          <div class="disk">
            <HardDrive size="1.7rem" strokeWidth={1.8} />
            <div class="disk-text">
              <span class="disk-name">Диск {diskInfo.volume || '—'}{diskInfo.filesystem ? ` · ${diskInfo.filesystem}` : ''}</span>
              <span class="disk-meta">Свободно {bytesSize(diskInfo.freeBytes)} из {bytesSize(diskInfo.totalBytes)}</span>
            </div>
          </div>
        {/if}

        <div class="safety">
          <ShieldCheck size="1.7rem" strokeWidth={1.8} />
          <p>
            Перенос безопасен: файлы копируются в новое место и проверяются по хешу, и только после этого
            исходная папка удаляется. Пока проверка не пройдёт, старые файлы не тронуты.
          </p>
        </div>

        <p class="hint">Перенос больших игр может занять продолжительное время — лаунчер можно свернуть.</p>

        {#if failure}
          <p class="failure">{failure}</p>
        {/if}
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    {#if jobActive && job}
      <Button variant="danger" disabled={cancelling} onclick={cancel}>
        {cancelling ? 'Отменяем…' : 'Отменить перенос'}
      </Button>
    {:else}
      <Button onclick={close}>Отмена</Button>
      {#if jobFailed}
        <Button variant="primary" onclick={retry}>Попробовать снова</Button>
      {:else}
        <Button variant="primary" disabled={!picked || starting} onclick={start}>
          {starting ? 'Начинаем…' : 'Перенести'}
        </Button>
      {/if}
    {/if}
  {/snippet}
</Modal>

<style>
  .move {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .field-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
    word-break: break-all;
  }

  .path {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .pick-controls {
    display: flex;
    gap: var(--space-2);
  }

  .pick-controls .input {
    flex: 1;
  }

  .disk {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    color: var(--text-2);
  }

  .disk-text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .disk-name {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .disk-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .safety {
    display: flex;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--accent-text);
  }

  .safety p {
    margin: 0;
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-2);
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .progress {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .progress-text {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .failure {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
