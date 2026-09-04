<script lang="ts">
  import { TriangleAlert } from '@lucide/svelte';
  import { respondTelemetryConsent } from '../stores/telemetryConsent';
  import { errorMessage } from '../utils/errors';
  import Button from './Button.svelte';
  import Toggle from './Toggle.svelte';

  let usageStats = $state(false);
  let saving = $state(false);
  let error = $state('');

  const errorReport = `{
  "installation_id": "6f1c2b90-3c7e-4a55-9c1e-2f0b8a7d4e11",
  "session_id": "b2d1e8a4-77c3-4f10-9e2a-51c7d3f9a806",
  "app_version": "0.2.4",
  "os": "windows",
  "arch": "amd64",
  "reports": [
    {
      "error_id": "3a9f0c21-64b8-4d7e-8f52-0c4a1b6e93d7",
      "component": "install",
      "operation": "extract_archive",
      "error_code": "disk_full",
      "message": "extract <path>: not enough space on disk",
      "stack": "typhon/internal/install.(*Service).extract\\n\\t<path>:214 +0x25",
      "timestamp": "2026-08-28T22:14:07Z",
      "fatal": false
    }
  ]
}`;

  const usageEvent = `{
  "installation_id": "6f1c2b90-3c7e-4a55-9c1e-2f0b8a7d4e11",
  "session_id": "b2d1e8a4-77c3-4f10-9e2a-51c7d3f9a806",
  "app_version": "0.2.4",
  "os": "windows",
  "arch": "amd64",
  "events": [
    {
      "type": "game_stopped",
      "timestamp": "2026-08-28T22:14:07Z",
      "properties": { "game_id": "1020", "duration_seconds": 3600 }
    }
  ]
}`;

  async function respond(diagnostics: boolean) {
    saving = true;
    error = '';
    try {
      await respondTelemetryConsent(usageStats, diagnostics);
    } catch (err) {
      error = errorMessage(err);
    } finally {
      saving = false;
    }
  }
</script>

<!--
  Deliberately not a Modal: Modal closes on Escape, on a backdrop click and on
  its own X, and this screen is rendered from a store that stays true until the
  answer is stored. Dismissing it would leave the window blank with no way
  forward. The screen goes away because the answer was written, not because it
  was closed, so there is no third silent option beside the two buttons.
-->
<div class="screen" role="dialog" aria-modal="true" aria-labelledby="consent-title">
  <div class="card">
    <div class="head">
      <h3 id="consent-title">Отправлять анонимные отчёты об ошибках?</h3>
    </div>

    <div class="body">
      <p class="text">
        Это помогает быстрее чинить баги. В отчёт попадает только то, что сломалось: пути, имя устройства и
        сетевые адреса удаляются перед отправкой.
      </p>

      <div class="row">
        <div class="row-text">
          <span class="row-title">Ещё и статистика использования</span>
          <span class="row-note">
            События о запусках игр, загрузках, установках и обновлениях: идентификатор игры, длительность,
            объём и код ошибки. Экраны, нажатия и поведение в интерфейсе не отслеживаются.
          </span>
        </div>
        <Toggle checked={usageStats} label="Анонимная статистика использования" onchange={(v) => (usageStats = v)} />
      </div>

      <details class="disclosure">
        <summary>Что именно отправляется</summary>
        <div class="examples">
          <div class="example">
            <span class="example-label">Отчёт об ошибке</span>
            <pre class="example-pre">{errorReport}</pre>
          </div>
          <div class="example">
            <span class="example-label">Событие статистики использования</span>
            <pre class="example-pre">{usageEvent}</pre>
          </div>
        </div>
      </details>

      {#if error}
        <p class="error">
          <TriangleAlert size="1.5rem" strokeWidth={1.8} />
          {error}
        </p>
      {/if}
    </div>

    <div class="foot">
      <Button size="lg" disabled={saving} onclick={() => respond(false)}>
        {saving ? 'Сохранение…' : 'Не отправлять'}
      </Button>
      <Button size="lg" disabled={saving} onclick={() => respond(true)}>
        {saving ? 'Сохранение…' : 'Да, отправлять'}
      </Button>
    </div>
  </div>
</div>

<style>
  .screen {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(4, 6, 10, 0.62);
    animation: fade var(--dur-panel) var(--ease);
  }

  .card {
    width: 52rem;
    max-width: calc(100vw - 4.8rem);
    max-height: calc(100vh - 8rem);
    display: flex;
    flex-direction: column;
    background: var(--surface-2);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-xl);
    box-shadow: var(--shadow-modal);
    animation: rise var(--dur-panel) var(--ease);
  }

  .head {
    padding: 1.8rem 2.4rem 0;
  }

  h3 {
    font-size: var(--font-lg);
    font-weight: 600;
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: 1.6rem 2.4rem 2.4rem;
    overflow-y: auto;
  }

  .foot {
    display: flex;
    justify-content: flex-end;
    gap: 0.8rem;
    padding: 1.4rem 2.4rem;
    border-top: 1px solid var(--border);
  }

  .text {
    font-size: var(--font-md);
    line-height: 1.6;
    color: var(--text-2);
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .row-title {
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .row-note {
    font-size: var(--font-xs);
    color: var(--text-3);
    line-height: 1.5;
  }

  .disclosure {
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: var(--space-3) var(--space-4);
  }

  .disclosure summary {
    cursor: pointer;
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
  }

  .examples {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }

  .example {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .example-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .example-pre {
    max-width: 100%;
    overflow-x: auto;
    padding: var(--space-3) var(--space-4);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
    font-size: var(--font-xs);
    line-height: 1.5;
    white-space: pre;
    color: var(--text-2);
  }

  .error {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    font-size: var(--font-sm);
    color: var(--danger);
  }

  @keyframes fade {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(0.4rem);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
