<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { createAutosave, noteLength } from '../../lib/game/notes';
  import { errorCode, hasMessage, msg } from '../../lib/i18n';
  import { notesLimit, setNotes } from '../../lib/services/library';
  import { toast } from '../../lib/stores/toasts';

  let { gameId, stored }: { gameId: string; stored: string } = $props();

  type SaveState = 'idle' | 'saving' | 'saved' | 'error';

  let text = $state('');
  let limit = $state(0);
  let focused = $state(false);
  let dirty = $state(false);
  let phase = $state<SaveState>('idle');
  let errorText = $state('');
  let area = $state<HTMLTextAreaElement | null>(null);

  const length = $derived(noteLength(text));
  const over = $derived(limit > 0 && length > limit);

  const autosave = createAutosave(async (value) => {
    phase = 'saving';
    try {
      await setNotes(gameId, value);
      errorText = '';
      phase = 'saved';
    } catch (err) {
      errorText = describe(err);
      phase = 'error';
      throw err;
    }
  });

  function describe(err: unknown): string {
    const code = errorCode(err);
    return hasMessage(code) ? msg(code) : msg('games.notesSaveError');
  }

  $effect(() => {
    const next = stored;
    if (focused) return;
    if (dirty && next !== text.trimEnd()) return;
    text = next;
    dirty = false;
  });

  $effect(() => {
    text;
    if (area) {
      area.style.height = 'auto';
      area.style.height = `${area.scrollHeight}px`;
    }
  });

  onMount(async () => {
    try {
      limit = await notesLimit();
    } catch (err) {
      errorText = describe(err);
      phase = 'error';
    }
  });

  onDestroy(() => {
    autosave.flush().catch((err: unknown) => toast(describe(err), 'danger'));
  });

  function oninput() {
    dirty = true;
    phase = 'idle';
    if (over) return;
    autosave.schedule(text);
  }

  function onfocus() {
    focused = true;
  }

  function onblur() {
    focused = false;
    if (dirty && !over && text.trimEnd() !== stored) {
      autosave.schedule(text);
      autosave.flush().catch(() => undefined);
    }
  }
</script>

<section class="section">
  <h2 class="heading">{msg('games.notesHeading')}</h2>
  <textarea
    bind:this={area}
    bind:value={text}
    rows="4"
    placeholder={msg('games.notesPlaceholder')}
    aria-label={msg('games.notesHeading')}
    {oninput}
    {onfocus}
    {onblur}
  ></textarea>
  <div class="foot">
    <span class="status" class:error={phase === 'error'} role="status">
      {#if phase === 'error'}
        {errorText}
      {:else if phase === 'saving'}
        {msg('games.notesSaving')}
      {:else if phase === 'saved'}
        {msg('games.notesSaved')}
      {/if}
    </span>
    {#if limit > 0}
      <span class="counter" class:over>{msg('games.notesCounter', { count: length, limit })}</span>
    {/if}
  </div>
</section>

<style>
  .section {
    margin-top: var(--space-10);
  }

  .heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  textarea {
    width: 100%;
    min-height: 9rem;
    padding: var(--space-3);
    background: var(--surface-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    color: var(--text);
    font: inherit;
    font-size: var(--font-sm);
    line-height: 1.55;
    resize: none;
    overflow: hidden;
    user-select: text;
  }

  textarea:focus {
    outline: none;
    border-color: var(--accent);
  }

  .foot {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    margin-top: var(--space-2);
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .status.error,
  .counter.over {
    color: var(--danger);
  }
</style>
