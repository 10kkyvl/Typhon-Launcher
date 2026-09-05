<script lang="ts">
  import { NOTE_LIMIT, noteLength, notePreview, trimNote } from '../social/note';
  import { msg } from '../i18n';
  import Button from './Button.svelte';

  let {
    note,
    own = false,
    saving = false,
    onsave,
  }: {
    note: string;
    own?: boolean;
    saving?: boolean;
    onsave?: (note: string) => void;
  } = $props();

  let editing = $state(false);
  let expanded = $state(false);
  let draft = $state('');

  const preview = $derived(notePreview(note));
  const shown = $derived(expanded ? note : preview.text);
  const left = $derived(NOTE_LIMIT - noteLength(draft));
  const dirty = $derived(trimNote(draft) !== note);

  function edit() {
    draft = note;
    editing = true;
  }

  function cancel() {
    editing = false;
    draft = '';
  }

  function save() {
    if (dirty) onsave?.(draft);
    cancel();
  }

  function remove() {
    onsave?.('');
    cancel();
  }
</script>

{#if editing}
  <div class="editor">
    <textarea
      bind:value={draft}
      maxlength={NOTE_LIMIT}
      rows="3"
      placeholder={msg('social.notePlaceholder')}
      aria-label={msg('social.notePlaceholder')}
    ></textarea>
    <div class="actions">
      <span class="left" class:low={left <= 50}>{msg('social.noteLeft', { count: left })}</span>
      {#if note}
        <Button size="sm" variant="ghost" disabled={saving} onclick={remove}>{msg('social.noteRemove')}</Button>
      {/if}
      <Button size="sm" variant="ghost" onclick={cancel}>{msg('social.noteCancel')}</Button>
      <Button size="sm" variant="primary" disabled={saving || !dirty} onclick={save}>{msg('social.noteSave')}</Button>
    </div>
  </div>
{:else if note}
  <div class="note">
    <p class="text">{shown}{#if preview.truncated && !expanded}…{/if}</p>
    <div class="actions">
      {#if preview.truncated}
        <button class="link" type="button" onclick={() => (expanded = !expanded)}>
          {expanded ? msg('social.noteLess') : msg('social.noteMore')}
        </button>
      {/if}
      {#if own}
        <button class="link" type="button" onclick={edit}>{msg('social.noteEdit')}</button>
      {/if}
    </div>
  </div>
{:else if own}
  <div class="actions">
    <button class="link" type="button" onclick={edit}>{msg('social.noteAdd')}</button>
  </div>
{/if}

<style>
  .note {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .text {
    font-size: var(--font-sm);
    line-height: 1.55;
    color: var(--text-2);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-3);
  }

  .link {
    font-size: var(--font-xs);
    font-weight: 500;
    color: var(--text-3);
    transition: color var(--dur) var(--ease);
  }

  .link:hover {
    color: var(--accent-text);
  }

  .editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .editor .actions {
    justify-content: flex-end;
  }

  textarea {
    width: 100%;
    padding: var(--space-3);
    background: var(--surface-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    color: var(--text);
    font: inherit;
    font-size: var(--font-sm);
    line-height: 1.55;
    resize: vertical;
    user-select: text;
  }

  textarea:focus {
    outline: none;
    border-color: var(--accent);
  }

  .left {
    margin-right: auto;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .left.low {
    color: var(--warning);
  }
</style>
