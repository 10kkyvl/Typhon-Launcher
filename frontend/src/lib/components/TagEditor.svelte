<script lang="ts">
  import { flushSync } from 'svelte';
  import Button from './Button.svelte';
  import Chip from './Chip.svelte';
  import { msg } from '../i18n';

  let {
    tags = $bindable([]),
    suggestions = [],
    placeholder,
    disabled = false,
  }: { tags?: string[]; suggestions?: string[]; placeholder: string; disabled?: boolean } = $props();

  const listId = `tag-suggestions-${Math.random().toString(36).slice(2)}`;
  let draft = $state('');
  let field = $state<HTMLInputElement>();
  let list = $state<HTMLElement>();

  const fold = (value: string) => value.normalize('NFKC').toLocaleLowerCase();
  const offered = $derived(suggestions.filter((tag) => !tags.some((own) => fold(own) === fold(tag))));

  function add() {
    const tag = draft.trim();
    if (!tag) return;
    if (!tags.some((own) => fold(own) === fold(tag))) tags = [...tags, tag];
    draft = '';
  }

  export function commit() {
    add();
  }

  // The pressed remove button goes with its tag; focus moves to the tag that took its place, else the
  // one before it, else the field, so the keyboard stays in the list instead of falling to the page.
  function remove(tag: string) {
    const at = tags.indexOf(tag);
    flushSync(() => {
      tags = tags.filter((own) => own !== tag);
    });
    const buttons = list?.querySelectorAll<HTMLButtonElement>('button') ?? [];
    (buttons[at] ?? buttons[at - 1] ?? field)?.focus();
  }

  function onkeydown(event: KeyboardEvent) {
    if (event.key !== 'Enter' || event.isComposing) return;
    event.preventDefault();
    add();
  }
</script>

<div class="editor">
  <div class="row">
    <input class="input" type="text" list={listId} {placeholder} {disabled} bind:value={draft} bind:this={field} {onkeydown} />
    <Button disabled={disabled || !draft.trim()} onclick={add}>{msg('modals.gameTagsAdd')}</Button>
    <datalist id={listId}>
      {#each offered as tag (tag)}
        <option value={tag}></option>
      {/each}
    </datalist>
  </div>
  {#if tags.length > 0}
    <div class="chips" bind:this={list}>
      {#each tags as tag (tag)}
        <Chip selected onremove={() => remove(tag)} removeLabel={msg('modals.gameTagsRemove', { tag })}>{tag}</Chip>
      {/each}
    </div>
  {/if}
</div>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .row {
    display: flex;
    gap: var(--space-2);
  }

  .row .input {
    flex: 1;
    min-width: 0;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.6rem;
  }
</style>
