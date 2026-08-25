<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Toggle from './Toggle.svelte';
  import { inspectRemoval, removeGame, type RemovalInfo } from '../services/install';
  import { toast } from '../stores/toasts';
  import { bytesSize } from '../utils/format';

  type Mode = 'disk' | 'library';

  let {
    open = $bindable(false),
    gameId,
    title,
    mode = $bindable<Mode>('disk'),
    onremoved,
  }: {
    open?: boolean;
    gameId: string;
    title: string;
    mode?: Mode;
    onremoved?: (mode: Mode) => void;
  } = $props();

  let info = $state<RemovalInfo | null>(null);
  let loading = $state(false);
  let working = $state(false);
  let failure = $state('');
  let deleteFiles = $state(false);
  let deleteDownload = $state(false);
  let loadedFor = $state('');

  const blocked = $derived(!!info && (info.running || info.busy));
  const canDeleteFiles = $derived(!!info && info.owned && !info.dirMissing);
  const canUninstall = $derived(!!info && (info.method === 'installer' || canDeleteFiles));

  $effect(() => {
    if (!open) {
      loadedFor = '';
      return;
    }
    if (loadedFor === gameId) return;
    loadedFor = gameId;
    void load();
  });

  $effect(() => {
    if (info && !canUninstall) mode = 'library';
  });

  async function load() {
    loading = true;
    failure = '';
    info = null;
    try {
      const loaded = await inspectRemoval(gameId);
      info = loaded;
      deleteFiles = loaded.owned && !loaded.dirMissing;
      deleteDownload = false;
    } catch (err) {
      failure = message(err, 'Не удалось собрать сведения об установке');
    } finally {
      loading = false;
    }
  }

  function message(err: unknown, fallback: string) {
    return err instanceof Error && err.message ? err.message : fallback;
  }

  async function confirm() {
    if (!info || working) return;
    const keepInLibrary = mode === 'disk';
    working = true;
    failure = '';
    try {
      await removeGame(gameId, {
        deleteFiles: keepInLibrary ? canDeleteFiles : deleteFiles && canDeleteFiles,
        deleteDownload,
        keepInLibrary,
      });
      open = false;
      toast(keepInLibrary ? `«${title}» удалена с компьютера` : `«${title}» удалена из библиотеки`);
      onremoved?.(mode);
    } catch (err) {
      failure = message(err, 'Не удалось удалить игру');
    } finally {
      working = false;
    }
  }

  const methodText = $derived.by(() => {
    if (!info) return '';
    if (info.method === 'installer' && info.quietUninstall) {
      return 'Игра поставлена установщиком — Typhon удалит её через деинсталлятор в тихом режиме. Некоторые игры всё равно задают свой вопрос, например про сохранения: тогда ответьте в его окне.';
    }
    if (info.method === 'installer') {
      return 'Игра поставлена установщиком — Typhon запустит её деинсталлятор. Тихий режим он не поддерживает, поэтому откроется окном и может задать вопросы.';
    }
    if (info.method === 'files') {
      return 'Игру устанавливал Typhon, поэтому её папку можно удалить целиком.';
    }
    if (info.uninstallUnknown) {
      return 'Деинсталлятор определить не удалось. Запись уберём из библиотеки, а саму игру удалите через «Установка и удаление программ».';
    }
    if (info.dirMissing) {
      return 'Игра не установлена — в библиотеке осталась только её карточка.';
    }
    return 'Эту установку делал не Typhon: запись уберём из библиотеки, файлы останутся на диске.';
  });

  const freedText = $derived.by(() => {
    if (!info) return '';
    if (info.sizeUnknown) return 'Размер папки определить не удалось';
    if (info.sizeBytes > 0) return `Освободится ${bytesSize(info.sizeBytes)}`;
    return 'Место на диске освободит деинсталлятор';
  });
</script>

<Modal bind:open title="Удаление игры">
  {#if loading}
    <p class="text">Смотрим, как установлена «{title}»...</p>
  {:else if info}
    <div class="modes">
      <button
        class="mode"
        class:selected={mode === 'disk'}
        disabled={!canUninstall}
        onclick={() => (mode = 'disk')}
      >
        <span class="mode-label">Удалить с компьютера</span>
        <span class="mode-sub">
          {#if canUninstall}
            Файлы уйдут с диска, карточка останется в библиотеке
          {:else if info.dirMissing}
            Файлов игры на диске уже нет
          {:else}
            Эту установку Typhon не делал и удалить её не может
          {/if}
        </span>
      </button>
      <button class="mode" class:selected={mode === 'library'} onclick={() => (mode = 'library')}>
        <span class="mode-label">Удалить из библиотеки</span>
        <span class="mode-sub">Карточка исчезнет вместе с наигранным временем</span>
      </button>
    </div>

    <p class="text">{methodText}</p>
    {#if info.installDir && !info.dirMissing}
      <p class="path">{info.installDir}</p>
    {/if}

    {#if info.running}
      <p class="warn">Игра сейчас запущена — закройте её.</p>
    {:else if info.busy}
      <p class="warn">По этой игре идёт установка или обновление.</p>
    {/if}

    <div class="options">
      {#if mode === 'disk'}
        <div class="row">
          <div class="row-text">
            <span class="row-label">Игра останется в библиотеке</span>
            <span class="row-sub">{freedText}</span>
          </div>
        </div>
      {:else}
        <div class="row" class:off={!canDeleteFiles}>
          <div class="row-text">
            <span class="row-label">
              {info.method === 'installer' ? 'Удалить остатки папки' : 'Удалить файлы игры'}
            </span>
            <span class="row-sub">
              {#if info.dirMissing}
                Папки установки уже нет на диске
              {:else if !info.owned && info.method === 'installer'}
                Папку удалит сам деинсталлятор
              {:else if !info.owned}
                Эти файлы Typhon не создавал, удалять их он не будет
              {:else if info.sizeUnknown}
                Размер папки определить не удалось
              {:else if info.method === 'installer'}
                Если деинсталлятор что-то оставит — {bytesSize(info.sizeBytes)}
              {:else}
                Освободится {bytesSize(info.sizeBytes)}
              {/if}
            </span>
          </div>
          <Toggle bind:checked={deleteFiles} label="Удалить файлы игры" disabled={!canDeleteFiles} />
        </div>
      {/if}

      {#if info.downloadPresent}
        <div class="row" class:off={info.downloadSeeding}>
          <div class="row-text">
            <span class="row-label">Удалить скачанные файлы, из которых ставили</span>
            <span class="row-sub">
              {#if info.downloadSeeding}
                Раздача активна — сначала остановите её в загрузках
              {:else}
                {bytesSize(info.downloadBytes)} в папке загрузок{info.downloadPath ? `: ${info.downloadPath}` : ''}
              {/if}
            </span>
          </div>
          <Toggle
            bind:checked={deleteDownload}
            label="Удалить скачанные файлы"
            disabled={info.downloadSeeding}
          />
        </div>
      {/if}
    </div>
  {/if}

  {#if failure}
    <p class="error">{failure}</p>
  {/if}

  {#snippet footer()}
    <Button onclick={() => (open = false)}>Отмена</Button>
    <Button variant="danger" onclick={confirm} disabled={!info || blocked || working}>
      {#if working}
        Удаляем...
      {:else if mode === 'disk'}
        Удалить с компьютера
      {:else}
        Удалить из библиотеки
      {/if}
    </Button>
  {/snippet}
</Modal>

<style>
  .text {
    margin: var(--space-5) 0 0;
    color: var(--text-2);
    line-height: 1.5;
  }

  .path {
    margin: var(--space-3) 0 0;
    font-size: 1.3rem;
    color: var(--text-3);
    word-break: break-all;
  }

  .warn {
    margin: var(--space-4) 0 0;
    color: var(--warning);
  }

  .error {
    margin: var(--space-4) 0 0;
    color: var(--danger);
  }

  .modes {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-3);
  }

  .mode {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-1);
    padding: var(--space-4);
    text-align: left;
    background: var(--surface-2);
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .mode:hover:not(:disabled) {
    background: var(--surface-3);
  }

  .mode.selected {
    border-color: var(--accent);
  }

  .mode:disabled {
    opacity: 0.5;
  }

  .mode-label {
    font-weight: 500;
  }

  .mode-sub {
    font-size: 1.3rem;
    line-height: 1.4;
    color: var(--text-3);
  }

  .options {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-5);
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-4);
    background: var(--surface-2);
    border-radius: var(--radius-md);
  }

  .row.off {
    opacity: 0.6;
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .row-label {
    font-weight: 500;
  }

  .row-sub {
    font-size: 1.3rem;
    color: var(--text-3);
  }
</style>
