<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { createAutosave, noteLength } from '../../lib/game/notes';
  import { errorCode, hasMessage, msg } from '../../lib/i18n';
  import { notesLimit, setNotes } from '../../lib/services/library';
  import { toast } from '../../lib/stores/toasts';

  let { gameId, stored }: { gameId: string; stored: string } = $props();

  const uid = $props.id();

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
    class="input area"
    bind:this={area}
    bind:value={text}
    rows="4"
    placeholder={msg('games.notesPlaceholder')}
    aria-label={msg('games.notesHeading')}
    aria-invalid={over ? 'true' : undefined}
    aria-describedby={[phase === 'error' ? `${uid}-error` : '', limit > 0 ? `${uid}-counter` : ''].filter(Boolean).join(' ') || undefined}
    {oninput}
    {onfocus}
    {onblur}
  ></textarea>
  <div class="foot">
    {#if phase === 'error'}
      <span class="status error" id="{uid}-error" role="alert">{errorText}</span>
    {:else}
      <span class="status" role="status">
        {#if phase === 'saving'}
          {msg('games.notesSaving')}
        {:else if phase === 'saved'}
          {msg('games.notesSaved')}
        {/if}
      </span>
    {/if}
    {#if limit > 0}
      <span class="counter" class:over id="{uid}-counter">{msg('games.notesCounter', { count: length, limit })}</span>
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

  .area {
    height: auto;
    min-height: 9rem;
    padding: var(--space-3);
    font-size: var(--font-sm);
    line-height: 1.55;
    resize: none;
    overflow: hidden;
  }

  .foot {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    margin-top: var(--space-2);
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .counter {
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
  }

  .status.error {
    padding: 0 var(--space-1);
    border-radius: var(--radius-xs);
    background: var(--surface-2);
    color: var(--danger);
  }

  .counter.over {
    background: var(--surface-2);
    color: var(--danger);
  }
</style>
