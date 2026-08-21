<script lang="ts">
  import { untrack } from 'svelte';
  import { TriangleAlert } from '@lucide/svelte';
  import { add as addSource, errorMessage } from '../stores/sources';
  import { testSource, type SourcePreview } from '../services/sources';
  import { toast } from '../stores/toasts';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  let step = $state<'url' | 'checking' | 'preview'>('url');
  let url = $state('');
  let preview = $state<SourcePreview | null>(null);
  let error = $state('');
  let adding = $state(false);

  function reset() {
    step = 'url';
    url = '';
    preview = null;
    error = '';
    adding = false;
  }

  $effect(() => {
    const isOpen = open;
    untrack(() => {
      if (isOpen) reset();
    });
  });

  async function check() {
    const value = url.trim();
    if (!value) return;
    step = 'checking';
    error = '';
    try {
      preview = await testSource(value);
      step = 'preview';
    } catch (err) {
      error = errorMessage(err);
      step = 'url';
    }
  }

  async function submit() {
    const value = url.trim();
    if (!value) return;
    adding = true;
    try {
      const source = await addSource(value);
      toast(`Источник «${source.name}» добавлен`, 'success');
      open = false;
    } catch (err) {
      error = errorMessage(err);
      step = 'url';
    } finally {
      adding = false;
    }
  }
</script>

<Modal bind:open title="Добавить источник">
  {#if step === 'url'}
    <div class="form">
      <label class="field">
        <span class="field-label">URL источника</span>
        <input type="text" placeholder="https://example.com/feed.json" bind:value={url} />
      </label>
      {#if error}
        <p class="error">
          <TriangleAlert size="1.5rem" strokeWidth={1.8} />
          {error}
        </p>
      {/if}
    </div>
  {:else if step === 'checking'}
    <div class="loading">
      <span class="spinner"></span>
      <span class="loading-text">Проверка источника…</span>
      <span class="loading-note">Импорт большого фида может занять несколько секунд.</span>
    </div>
  {:else if preview}
    <div class="preview">
      <div class="preview-row">
        <span class="key">Название</span>
        <span class="value">{preview.name}</span>
      </div>
      <div class="preview-row">
        <span class="key">Версия фида</span>
        <span class="value">{preview.feedVersion}</span>
      </div>
      <div class="preview-row">
        <span class="key">Записей</span>
        <span class="value">{preview.entries}</span>
      </div>
      <div class="preview-row">
        <span class="key">Некорректных записей</span>
        <span class="value">{preview.invalid}</span>
      </div>
      {#if preview.duplicate}
        <p class="warn">
          <TriangleAlert size="1.5rem" strokeWidth={1.8} />
          Такой источник уже добавлен
        </p>
      {/if}
      {#if preview.warnings?.length}
        <div class="warnings">
          {#each preview.warnings as warning (warning)}
            <p class="warn">
              <TriangleAlert size="1.5rem" strokeWidth={1.8} />
              {warning}
            </p>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    {#if step === 'url'}
      <Button onclick={() => (open = false)}>Отмена</Button>
      <Button variant="primary" disabled={!url.trim()} onclick={check}>Проверить источник</Button>
    {:else if step === 'preview'}
      <Button onclick={() => (step = 'url')}>Назад</Button>
      <Button variant="primary" disabled={adding} onclick={submit}>{adding ? 'Добавление…' : 'Добавить'}</Button>
    {/if}
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.7rem;
  }

  .field-label {
    font-size: 1.4rem;
    font-weight: 500;
    color: var(--text-2);
  }

  .field input {
    height: var(--control-md);
    padding: 0 1.2rem;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: 1.5rem;
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

  .error {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    font-size: 1.4rem;
    color: var(--danger);
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
    font-size: 1.5rem;
    font-weight: 500;
  }

  .loading-note {
    font-size: 1.3rem;
    color: var(--text-3);
  }

  .preview {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .preview-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.9rem 0;
    font-size: 1.4rem;
  }

  .preview-row + .preview-row {
    border-top: 1px solid var(--border);
  }

  .key {
    color: var(--text-3);
  }

  .value {
    color: var(--text);
    font-variant-numeric: tabular-nums;
  }

  .warnings {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    margin-top: 0.6rem;
  }

  .warn {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    font-size: 1.3rem;
    color: var(--warning);
    background: var(--warning-subtle);
    border-radius: var(--radius-sm);
    padding: 0.8rem 1rem;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
