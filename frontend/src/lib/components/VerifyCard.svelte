<script lang="ts">
  import { FileCheck, Wrench } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import StatusBadge from './StatusBadge.svelte';
  import type { VerifyState } from '../services/updates';
  import { createManifest, repair, verify } from '../stores/updates';
  import { bytesSize, truncateMiddle } from '../utils/format';

  let { gameId, state, running }: { gameId: string; state: VerifyState | undefined; running: boolean } =
    $props();

  const busy = $derived(Boolean(state?.running || state?.repairing));
  const checked = $derived(Boolean(state?.checkedAt) && !busy);
  const unavailable = $derived(state?.method === 'unavailable');
  const damaged = $derived(checked && ((state?.missingFiles ?? 0) > 0 || (state?.corruptedPieces ?? 0) > 0));
  const percent = $derived(Math.round((state?.ratio ?? 0) * 1000) / 10);
</script>

<Card padding="var(--space-5) var(--space-6)">
  <div class="head">
    <h3 class="card-title">Целостность файлов</h3>
    {#if unavailable}
      <StatusBadge kind="neutral" label="Проверка недоступна" dot={false} />
    {:else if damaged}
      <StatusBadge kind="warning" label="Найдены повреждения" />
    {:else if checked}
      <StatusBadge kind="success" label="Файлы в порядке" dot={false} />
    {/if}
  </div>

  {#if unavailable}
    <p class="muted">
      Установка не связана с релизом — сверять не с чем. Можно записать манифест текущих файлов, тогда
      последующие проверки покажут, что изменилось.
    </p>
    <div class="actions">
      <Button disabled={busy} onclick={() => createManifest(gameId)}>
        <FileCheck size="1.5rem" strokeWidth={1.8} />
        Создать манифест
      </Button>
    </div>
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
        <dt>Проверено</dt>
        <dd>{percent}%</dd>
      </div>
      <div>
        <dt>Совпало</dt>
        <dd>{bytesSize(state?.okBytes ?? 0)} из {bytesSize(state?.totalBytes ?? 0)}</dd>
      </div>
      {#if (state?.missingFiles ?? 0) > 0}
        <div>
          <dt>Отсутствуют</dt>
          <dd>{state?.missingFiles} файлов</dd>
        </div>
      {/if}
      {#if (state?.corruptedPieces ?? 0) > 0}
        <div>
          <dt>Повреждены</dt>
          <dd>{state?.corruptedPieces} блоков</dd>
        </div>
      {/if}
    </dl>
  {:else}
    <p class="muted">Проверка сверяет файлы на диске с торрентом релиза или сохранённым манифестом.</p>
  {/if}

  {#if state?.error}
    <p class="error">{state.error}</p>
  {/if}

  {#if !unavailable}
    <div class="actions">
      <Button disabled={busy} onclick={() => verify(gameId)}>
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
    {#if damaged && state?.repairable && running}
      <p class="muted">Игра запущена. Закройте её перед восстановлением.</p>
    {/if}
  {/if}
</Card>

<style>
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
    font-size: 1.3rem;
    color: var(--text-dim);
    margin-bottom: 0.4rem;
  }

  .summary dd {
    margin: 0;
    font-size: var(--font-md);
    font-weight: 500;
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
    color: var(--text-dim);
    font-size: 1.3rem;
    margin: 0 0 var(--space-4);
  }

  .mono {
    font-family: var(--font-mono);
  }

  .error {
    color: var(--danger);
    font-size: 1.3rem;
    margin: 0 0 var(--space-4);
  }
</style>
