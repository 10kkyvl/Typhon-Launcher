<script lang="ts">
  import { FolderOpen, RefreshCw } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import { libraryErrorText } from '../library/errors';
  import { biosFolder, listPlatforms, openBiosFolder, type PlatformStatus } from '../services/emulators';
  import { biosPrompt } from '../stores/biosPrompt';

  const prompt = $derived($biosPrompt);

  let platforms = $state<PlatformStatus[]>([]);
  let folder = $state('');
  let failure = $state('');

  const info = $derived(prompt ? platforms.find((item) => item.id === prompt.platform) : undefined);

  $effect(() => {
    const current = prompt;
    failure = '';
    folder = '';
    if (!current) return;
    let stale = false;
    listPlatforms().then(
      (list) => {
        if (!stale) platforms = list;
      },
      (err) => {
        console.error('list emulators', err);
        if (!stale) failure = msg('emul.loadFailed');
      },
    );
    biosFolder(current.platform).then(
      (dir) => {
        if (!stale) folder = dir;
      },
      (err) => {
        console.warn('bios folder', err);
        if (!stale) failure = libraryErrorText(err, msg('emul.biosOpenFailed'));
      },
    );
    return () => {
      stale = true;
    };
  });

  async function openFolder() {
    if (!prompt) return;
    failure = '';
    try {
      await openBiosFolder(prompt.platform);
    } catch (err) {
      failure = libraryErrorText(err, msg('emul.biosOpenFailed'));
    }
  }
</script>

<Modal open={Boolean(prompt)} title={msg('emul.biosTitle')} width="52.8rem" onclose={() => prompt?.settle(false)}>
  {#if prompt}
    <p class="text">
      {msg('emul.biosText', { emulator: info?.emulator ?? prompt.platform, console: info?.name ?? prompt.platform })}
    </p>
    {#if folder}<p class="folder">{folder}</p>{/if}
    {#if prompt.stillMissing}<p class="reason" role="alert">{msg('emul.biosStillMissing')}</p>{/if}
    <p class="hint">{msg('emul.biosHint')}</p>
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  {/if}
  {#snippet footer()}
    <Button onclick={() => prompt?.settle(false)}>{msg('common.cancel')}</Button>
    <Button disabled={!folder} onclick={openFolder}>
      <FolderOpen size="1.54rem" strokeWidth={1.8} />
      {msg('emul.biosOpenFolder')}
    </Button>
    <Button variant="primary" onclick={() => prompt?.settle(true)}>
      <RefreshCw size="1.54rem" strokeWidth={1.8} />
      {msg('emul.biosRetry')}
    </Button>
  {/snippet}
</Modal>

<style>
  .text {
    margin: 0 0 var(--space-3);
    font-size: var(--font-md);
    color: var(--text);
  }

  .folder {
    margin: 0 0 var(--space-3);
    font-family: var(--font-mono);
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .reason {
    margin: 0 0 var(--space-3);
    font-size: var(--font-sm);
    color: var(--danger);
  }

  .hint {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-3);
    line-height: 1.5;
  }

  .failure {
    margin: var(--space-3) 0 0;
    font-size: var(--font-sm);
    color: var(--danger);
  }
</style>
