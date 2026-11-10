<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import { libraryErrorText } from '../library/errors';
  import { closeEmulator, listPlatforms, type PlatformStatus } from '../services/emulators';
  import { emulatorBusyPrompt } from '../stores/emulatorBusyPrompt';

  const prompt = $derived($emulatorBusyPrompt);

  let platforms = $state<PlatformStatus[]>([]);
  let failure = $state('');
  let closing = $state(false);

  const info = $derived(prompt ? platforms.find((item) => item.id === prompt.platform) : undefined);
  const emulator = $derived(info?.emulator ?? prompt?.platform ?? '');

  $effect(() => {
    const current = prompt;
    failure = '';
    closing = false;
    if (!current) return;
    let stale = false;
    listPlatforms().then(
      (list) => {
        if (!stale) platforms = list;
      },
      (err) => console.error('list emulators', err),
    );
    return () => {
      stale = true;
    };
  });

  async function closeAndPlay() {
    const current = prompt;
    if (!current || closing) return;
    closing = true;
    failure = '';
    try {
      await closeEmulator(current.platform);
      current.settle(true);
    } catch (err) {
      failure = libraryErrorText(err, msg('emul.busyCloseFailed', { emulator }));
    } finally {
      closing = false;
    }
  }
</script>

<Modal open={Boolean(prompt)} title={msg('emul.busyTitle', { emulator })} width="44rem" onclose={() => prompt?.settle(false)}>
  {#if prompt}
    <p class="text">{msg('emul.busyText', { emulator })}</p>
    <p class="hint">{msg('emul.busyHint')}</p>
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  {/if}
  {#snippet footer()}
    <Button disabled={closing} onclick={() => prompt?.settle(false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" disabled={closing} onclick={closeAndPlay}>
      {closing ? msg('emul.busyClosing', { emulator }) : msg('emul.busyConfirm', { emulator })}
    </Button>
  {/snippet}
</Modal>

<style>
  .text {
    margin: 0 0 var(--space-3);
    font-size: var(--font-md);
    color: var(--text);
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
