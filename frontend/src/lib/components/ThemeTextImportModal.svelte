<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import { importThemeText, type Theme } from '../services/theme';
  import { themeErrorText } from '../theme/themeErrors';

  let {
    open = $bindable(false),
    onimported,
  }: { open?: boolean; onimported: (theme: Theme) => void | Promise<void> } = $props();

  let text = $state('');
  let name = $state('');
  let error = $state('');
  let busy = $state(false);

  $effect(() => {
    if (!open) return;
    text = '';
    name = '';
    error = '';
  });

  async function save() {
    busy = true;
    error = '';
    try {
      const theme = await importThemeText(text, name.trim());
      open = false;
      await onimported(theme);
    } catch (err) {
      error = themeErrorText(err);
    } finally {
      busy = false;
    }
  }
</script>

<Modal bind:open title={msg('settings.appearanceImportTextTitle')} width="56rem">
  <div class="form">
    <label class="field">
      <span class="field-label">{msg('settings.appearanceImportTextLabel')}</span>
      <textarea
        class="input area"
        rows="12"
        spellcheck="false"
        placeholder={msg('settings.appearanceImportTextPlaceholder')}
        disabled={busy}
        bind:value={text}
      ></textarea>
      {#if error}<span class="error" role="alert">{error}</span>{/if}
    </label>
    <label class="field">
      <span class="field-label">{msg('settings.appearanceImportTextNameLabel')}</span>
      <input class="input" type="text" maxlength="64" disabled={busy} bind:value={name} />
      <span class="hint">{msg('settings.appearanceImportTextNameHint')}</span>
    </label>
  </div>

  {#snippet footer()}
    <Button disabled={busy} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={busy || !text.trim()} onclick={save}>{msg('common.save')}</Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
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

  .area {
    height: auto;
    padding: 0.9rem 1.2rem;
    resize: vertical;
    line-height: 1.5;
    font-family: ui-monospace, SFMono-Regular, Consolas, monospace;
    font-size: var(--font-sm);
  }

  .error {
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--danger);
  }

  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
