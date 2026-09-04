<script lang="ts">
  import { FileCheck, Wrench } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import type { VerifyState } from '../services/updates';
  import { createManifest, repair, verify } from '../stores/updates';
  import { bytesSize, relativeDate, truncateMiddle } from '../utils/format';
  import { msg } from '../i18n';

  let { gameId, state, running }: { gameId: string; state: VerifyState | undefined; running: boolean } =
    $props();

  const busy = $derived(Boolean(state?.running || state?.repairing));
  const unavailable = $derived(state?.method === 'unavailable');
  const checked = $derived(!unavailable && !busy && Boolean(state?.checkedAt));
  const pending = $derived(!unavailable && !busy && !state?.checkedAt);
  const missing = $derived(checked ? (state?.missingFiles ?? 0) : 0);
  const corrupted = $derived(checked ? (state?.corruptedPieces ?? 0) : 0);
  const unreadable = $derived(checked ? (state?.unreadableFiles ?? 0) : 0);
  const damaged = $derived(missing > 0 || corrupted > 0);
  const percent = $derived(Math.round((state?.ratio ?? 0) * 1000) / 10);
  const methodLabel = $derived(
    state?.method === 'torrent' ? 'по торренту релиза' : 'по сохранённому манифесту',
  );
</script>

<div class="section">
<Card>
  <div class="head">
    <h3 class="card-title">Целостность файлов</h3>
    {#if unavailable}
      <StatusBadge kind="neutral" label="Проверка недоступна" dot={false} />
    {:else if pending}
      <StatusBadge kind="neutral" label="Не проверялось" dot={false} />
    {:else if damaged}
      <StatusBadge kind="warning" label="Найдены повреждения" />
    {:else if unreadable > 0}
      <StatusBadge kind="neutral" label="Проверено не всё" dot={false} />
    {:else if checked}
      <StatusBadge kind="success" label="Файлы в порядке" dot={false} />
    {/if}
  </div>

  {#if unavailable}
    <p class="muted">
      Сверять не с чем: релиз не даёт пригодного для проверки состава файлов, а манифест ещё не записан.
      Запишите манифест текущих файлов — последующие проверки покажут, что изменилось.
    </p>
    <div class="actions">
      <Button disabled={busy || running} onclick={() => createManifest(gameId)}>
        <FileCheck size="1.5rem" strokeWidth={1.8} />
        Создать манифест
      </Button>
    </div>
    {#if running}
      <p class="muted">Игра запущена. Закройте её, чтобы записать манифест.</p>
    {/if}
  {:else if busy}
    <div class="progress">
      <ProgressBar value={(state?.progress ?? 0) * 100} />
      <span class="muted">{Math.round((state?.progress ?? 0) * 100)}%</span>
    </div>
    {#if state?.currentFile}
      <p class="muted mono">{truncateMiddle(state.currentFile, 64)}</p>
    {/if}
  {:else if checked}
    <dl class="summary">
      <div>
        <dt>Совпало</dt>
        <dd>{percent}%</dd>
      </div>
      <div>
        <dt>Проверено</dt>
        <dd>{bytesSize(state?.okBytes ?? 0)} из {bytesSize(state?.totalBytes ?? 0)}</dd>
      </div>
      {#if missing > 0}
        <div>
          <dt>Отсутствуют</dt>
          <dd>{msg('verify.missingFiles', { count: missing })}</dd>
        </div>
      {/if}
      {#if corrupted > 0}
        <div>
          <dt>Повреждены</dt>
          <dd>{msg('verify.corruptedBlocks', { count: corrupted })}</dd>
        </div>
      {/if}
      {#if unreadable > 0}
        <div>
          <dt>Не прочитаны</dt>
          <dd>{msg('verify.unreadableFiles', { count: unreadable })}</dd>
        </div>
      {/if}
    </dl>
    <p class="muted">
      Сверка {methodLabel}, {relativeDate(state?.checkedAt ?? null).toLowerCase()}.
      {#if unreadable > 0}
        Часть файлов была занята другой программой — это не повреждение, повторите проверку позже.
      {/if}
    </p>
  {:else}
    <p class="muted">
      Проверка сверяет файлы на диске с торрентом релиза или с сохранённым манифестом. Результат
      относится к текущей версии и каталогу установки.
    </p>
  {/if}

  {#if state?.error}
    <p class="error">{state.error}</p>
  {/if}

  {#if !unavailable}
    <div class="actions">
      <Button disabled={busy || running} onclick={() => verify(gameId)}>
        <FileCheck size="1.5rem" strokeWidth={1.8} />
        Проверить файлы
      </Button>
      {#if damaged && state?.repairable}
        <Button variant="primary" disabled={busy || running} onclick={() => repair(gameId)}>
          <Wrench size="1.5rem" strokeWidth={1.8} />
          Восстановить
        </Button>
      {/if}
    </div>
    {#if running}
      <p class="muted">Игра запущена. Закройте её перед проверкой файлов.</p>
    {/if}
  {/if}
</Card>
</div>

<style>
  .section {
    max-width: 120rem;
    margin-bottom: var(--space-6);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    margin-bottom: var(--space-4);
  }

  .card-title {
    font-size: var(--font-lg);
    font-weight: 600;
    margin: 0;
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

  .progress {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    margin-bottom: var(--space-3);
  }

  .progress :global(.track) {
    flex: 1;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
  }

  .muted {
    color: var(--text-3);
    font-size: var(--font-xs);
    margin: 0 0 var(--space-4);
  }

  .mono {
    word-break: break-all;
  }

  .error {
    color: var(--danger);
    font-size: var(--font-xs);
    margin: 0 0 var(--space-4);
  }
</style>
