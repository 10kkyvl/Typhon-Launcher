<script lang="ts">
  import { tick } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import TextArea from './TextArea.svelte';
  import TextField from './TextField.svelte';
  import { errorCode, msg } from '../i18n';
  import { importThemeText, type Theme } from '../services/theme';
  import { REASONS, themeErrorText } from '../theme/themeErrors';

  /**
   * A theme imported from pasted text, under an optional name. Save sends one import per press and
   * keeps the focus, busy, while the fields refuse edits and Cancel waits with the reason. A refusal
   * is said where it belongs, in the launcher's words: a refused name the player gave at the name
   * field, anything else about the text at the text field, each taking the focus; a failure that is not
   * about either (the theme could not be written) in the dialog. What was pasted stays.
   */
  let {
    open = $bindable(false),
    onimported,
  }: { open?: boolean; onimported: (theme: Theme) => void | Promise<void> } = $props();
  const uid = $props.id();
  const failureId = `${uid}-failure`;

  let text = $state('');
  let name = $state('');
  let textError = $state('');
  let nameError = $state('');
  let failure = $state('');
  let busy = $state(false);
  let textarea = $state<HTMLTextAreaElement>();
  let input = $state<HTMLInputElement>();

  $effect(() => {
    if (!open) return;
    text = '';
    name = '';
    textError = '';
    nameError = '';
    failure = '';
  });

  const empty = $derived(!text.trim());

  async function save() {
    if (busy || empty) return;
    busy = true;
    textError = '';
    nameError = '';
    failure = '';
    const named = name.trim();
    let refusedAt: HTMLElement | undefined;
    try {
      const theme = await importThemeText(text, named);
      open = false;
      await onimported(theme);
    } catch (err) {
      const code = errorCode(err);
      if (code === 'theme.invalid_name' && named) {
        nameError = themeErrorText(err);
        refusedAt = input;
      } else if (REASONS[code]) {
        textError = themeErrorText(err);
        refusedAt = textarea;
      } else {
        console.error('theme text import', err);
        failure = msg('errLibrary.themeErrFallback');
      }
    } finally {
      busy = false;
    }
    if (!refusedAt) return;
    await tick();
    refusedAt.focus();
  }
</script>

<Modal dismissible={!busy} bind:open {busy} title={msg('settings.appearanceImportTextTitle')} width="56rem">
  <div class="form">
    <TextArea
      bind:value={text}
      bind:textarea
      label={msg('settings.appearanceImportTextLabel')}
      placeholder={msg('settings.appearanceImportTextPlaceholder')}
      rows={12}
      readonly={busy}
      error={textError}
      oninput={() => (textError = '')}
    />
    <TextField
      bind:value={name}
      bind:input
      label={msg('settings.appearanceImportTextNameLabel')}
      hint={msg('settings.appearanceImportTextNameHint')}
      maxlength={64}
      readonly={busy}
      error={nameError}
      oninput={() => (nameError = '')}
    />
    {#if failure}
      <Notice tone="danger" compact id={failureId}>{failure}</Notice>
    {/if}
  </div>

  {#snippet footer()}
    <Button disabled={busy} reason={busy ? msg('settings.appearanceImportTextBusy') : ''} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button
      variant="primary"
      disabled={!busy && empty}
      reason={!busy && empty ? msg('settings.appearanceImportTextEmpty') : ''}
      {busy}
      aria-describedby={failure ? failureId : undefined}
      onclick={save}
    >
      {msg('common.save')}
    </Button>
  {/snippet}
</Modal>

<style>
  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
</style>
