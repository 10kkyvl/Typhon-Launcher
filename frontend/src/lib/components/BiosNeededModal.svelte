<script lang="ts">
  import { onDestroy } from 'svelte';
  import { FolderOpen, RefreshCw } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { errorCode, hasMessage, msg } from '../i18n';
  import { libraryErrorText } from '../library/errors';
  import { biosFolder, listPlatforms, openBiosFolder, type PlatformStatus } from '../services/emulators';
  import { biosPrompt } from '../stores/biosPrompt';
  import { toast } from '../stores/toasts';

  const prompt = $derived($biosPrompt);

  let platforms = $state<PlatformStatus[]>([]);
  let folder = $state('');
  let folderFailed = $state(false);
  let failure = $state('');
  let opening = $state(false);

  const info = $derived(prompt ? platforms.find((item) => item.id === prompt.platform) : undefined);
  // Open folder needs the folder: held, and reachable by the D-pad in Big Picture, until it is read.
  const openHeld = $derived(folder ? '' : folderFailed ? msg('emulators.bios_dir_unknown') : msg('common.loading'));

  // A launch can replace the prompt while the folder opens: the answer then belongs to no prompt on
  // screen and is said once in a toast naming the emulator. A destroyed prompt hears nothing either.
  let asked = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  $effect(() => {
    const current = prompt;
    asked += 1;
    failure = '';
    folder = '';
    folderFailed = false;
    opening = false;
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
        if (stale) return;
        folderFailed = true;
        failure = libraryErrorText(err, msg('emul.biosOpenFailed'));
      },
    );
    return () => {
      stale = true;
    };
  });

  async function openFolder() {
    const current = prompt;
    if (!current || opening || !folder) return;
    const at = asked;
    const heard = () => alive && at === asked;
    const emulator = info?.emulator ?? current.platform;
    opening = true;
    failure = '';
    try {
      await openBiosFolder(current.platform);
    } catch (err) {
      if (heard()) {
        failure = libraryErrorText(err, msg('emul.biosOpenFailed'));
      } else {
        const code = errorCode(err);
        toast(hasMessage(code) ? msg('emul.lateFailed', { emulator, reason: msg(code) }) : msg('emul.biosOpenFailedFor', { emulator }), 'danger');
      }
    }
    if (heard()) opening = false;
  }
</script>

<Modal open={Boolean(prompt)} title={msg('emul.biosTitle')} width="48rem" onclose={() => prompt?.settle(false)}>
  {#if prompt}
    <p class="text">
      {msg('emul.biosText', { emulator: info?.emulator ?? prompt.platform, console: info?.name ?? prompt.platform })}
    </p>
    {#if folder}<p class="folder">{folder}</p>{/if}
    {#if prompt.stillMissing}<div class="reason"><Notice tone="danger" compact>{msg('emul.biosStillMissing')}</Notice></div>{/if}
    <p class="hint">{msg('emul.biosHint')}</p>
    {#if failure}<div class="failure"><Notice tone="danger" compact>{failure}</Notice></div>{/if}
  {/if}
  {#snippet footer()}
    <Button onclick={() => prompt?.settle(false)}>{msg('common.cancel')}</Button>
    <Button disabled={!!openHeld} reason={openHeld} busy={opening} onclick={openFolder}>
      <FolderOpen size="1.4rem" strokeWidth={1.8} />
      {msg('emul.biosOpenFolder')}
    </Button>
    <Button variant="primary" onclick={() => prompt?.settle(true)}>
      <RefreshCw size="1.4rem" strokeWidth={1.8} />
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
    font-variant-ligatures: none;
    font-size: var(--font-sm);
    color: var(--text-2);
    overflow-wrap: anywhere;
  }

  .reason {
    margin: 0 0 var(--space-3);
  }

  .hint {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-3);
    line-height: 1.5;
  }

  .failure {
    margin: var(--space-3) 0 0;
  }
</style>
