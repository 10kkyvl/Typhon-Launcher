<script lang="ts">
  import { NOTE_LIMIT, noteLength, notePreview, trimNote } from '../social/note';
  import { msg } from '../i18n';
  import Button from './Button.svelte';
  import TextLink from './TextLink.svelte';

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

  const uid = $props.id();

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
      class="input"
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
    <p class="text" id="{uid}-text">{shown}{#if preview.truncated && !expanded}…{/if}</p>
    <div class="actions">
      {#if preview.truncated}
        <TextLink tone="muted" aria-expanded={expanded} aria-controls="{uid}-text" onclick={() => (expanded = !expanded)}>
          {expanded ? msg('social.noteLess') : msg('social.noteMore')}
        </TextLink>
      {/if}
      {#if own}
        <TextLink tone="muted" onclick={edit}>{msg('social.noteEdit')}</TextLink>
      {/if}
    </div>
  </div>
{:else if own}
  <div class="actions">
    <TextLink tone="muted" onclick={edit}>{msg('social.noteAdd')}</TextLink>
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
    font-size: var(--font-xs);
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
    height: auto;
    padding: var(--space-3);
    font: inherit;
    font-size: var(--font-sm);
    line-height: 1.55;
    resize: vertical;
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
