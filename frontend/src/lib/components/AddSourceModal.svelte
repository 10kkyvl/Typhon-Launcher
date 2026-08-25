<script lang="ts">
  import { untrack } from 'svelte';
  import { FileUp, TriangleAlert } from '@lucide/svelte';
  import { add as addSource, addFile as addSourceFile, errorMessage } from '../stores/sources';
  import { selectFeedFile, testSource, testSourceFile, type SourcePreview } from '../services/sources';
  import { toast } from '../stores/toasts';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  let { open = $bindable(false) }: { open?: boolean } = $props();

  let step = $state<'input' | 'checking' | 'preview'>('input');
  let mode = $state<'url' | 'file'>('url');
  let url = $state('');
  let path = $state('');
  let preview = $state<SourcePreview | null>(null);
  let error = $state('');
  let adding = $state(false);

  function reset() {
    step = 'input';
    mode = 'url';
    url = '';
    path = '';
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

  async function pickFile() {
    let selected = '';
    try {
      selected = await selectFeedFile();
    } catch (err) {
      error = errorMessage(err);
      return;
    }
    if (!selected) return;
    path = selected;
    mode = 'file';
    await check();
  }

  async function check() {
    const value = mode === 'file' ? path : url.trim();
    if (!value) return;
    step = 'checking';
    error = '';
    try {
      preview = mode === 'file' ? await testSourceFile(value) : await testSource(value);
      step = 'preview';
    } catch (err) {
      error = errorMessage(err);
      step = 'input';
    }
  }

  async function submit() {
    const value = mode === 'file' ? path : url.trim();
    if (!value) return;
    adding = true;
    try {
      const source = mode === 'file' ? await addSourceFile(value) : await addSource(value);
      toast(`Источник «${source.name}» добавлен`, 'success');
      open = false;
    } catch (err) {
      error = errorMessage(err);
      step = 'input';
    } finally {
      adding = false;
    }
  }

  function back() {
    preview = null;
    step = 'input';
  }
</script>

<Modal bind:open title="Добавить источник">
  {#if step === 'input'}
    <div class="form">
      <label class="field">
        <span class="field-label">URL источника</span>
        <input
          class="input"
          type="text"
          placeholder="https://example.com/feed.json"
          bind:value={url}
          oninput={() => (mode = 'url')}
        />
      </label>
      <div class="or">
        <span class="line"></span>
        <span class="or-text">или</span>
        <span class="line"></span>
      </div>
      <Button onclick={pickFile}>
        <FileUp size="1.6rem" strokeWidth={1.8} />
        Выбрать файл фида
      </Button>
      {#if path}
        <span class="picked" title={path}>{path}</span>
      {/if}
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
        <span class="key">{preview.type === 'file' ? 'Файл' : 'URL'}</span>
        <span class="value location" title={preview.type === 'file' ? preview.path : preview.url}>
          {preview.type === 'file' ? preview.path : preview.url}
        </span>
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
    {#if step === 'input'}
      <Button onclick={() => (open = false)}>Отмена</Button>
      <Button variant="primary" disabled={!url.trim() && !path} onclick={check}>Проверить источник</Button>
    {:else if step === 'preview'}
      <Button onclick={back}>Назад</Button>
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
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text-2);
  }

  .or {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .line {
    flex: 1;
    height: 1px;
    background: var(--border);
  }

  .or-text {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .picked {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .location {
    max-width: 28rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .error {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    font-size: var(--font-sm);
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
    font-size: var(--font-md);
    font-weight: 500;
  }

  .loading-note {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .preview {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 0.4rem var(--space-4);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .preview-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.9rem 0;
    font-size: var(--font-sm);
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
    font-size: var(--font-xs);
    color: var(--warning);
    background: var(--warning-subtle);
    border-radius: var(--radius-md);
    padding: 0.8rem 1rem;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
