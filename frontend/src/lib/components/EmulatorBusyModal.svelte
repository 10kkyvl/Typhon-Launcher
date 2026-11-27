<script lang="ts">
  import { onDestroy } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { errorCode, hasMessage, msg } from '../i18n';
  import { libraryErrorText } from '../library/errors';
  import { closeEmulator, listPlatforms, type PlatformStatus } from '../services/emulators';
  import { emulatorBusyPrompt } from '../stores/emulatorBusyPrompt';
  import { toast } from '../stores/toasts';

  const prompt = $derived($emulatorBusyPrompt);

  let platforms = $state<PlatformStatus[]>([]);
  let failure = $state('');
  let closing = $state(false);

  const info = $derived(prompt ? platforms.find((item) => item.id === prompt.platform) : undefined);
  const emulator = $derived(info?.emulator ?? prompt?.platform ?? '');

  // A launch can replace the prompt while the emulator closes: the answer then belongs to no prompt on
  // screen and is said once in a toast naming the emulator. A destroyed prompt hears nothing either.
  let opening = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  $effect(() => {
    const current = prompt;
    opening += 1;
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

  // CloseEmulator asks the emulator to close, waits, then ends its process (emul.busyHint says so).
  async function closeAndPlay() {
    const current = prompt;
    if (!current || closing) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const name = emulator;
    closing = true;
    failure = '';
    try {
      await closeEmulator(current.platform);
    } catch (err) {
      const code = errorCode(err);
      if (!heard()) {
        toast(hasMessage(code) ? msg('emul.lateFailed', { emulator: name, reason: msg(code) }) : msg('emul.busyCloseFailed', { emulator: name }), 'danger');
        return;
      }
      failure = libraryErrorText(err, msg('emul.busyCloseFailed', { emulator: name }));
      closing = false;
      return;
    }
    if (!heard()) {
      toast(msg('emul.busyClosedLate', { emulator: name }), 'success');
      return;
    }
    closing = false;
    current.settle(true);
  }

  const held = $derived(closing ? msg('modals.actionRunning') : '');
</script>

<Modal dismissible={!closing} open={Boolean(prompt)} title={msg('emul.busyTitle', { emulator })} width="44rem" onclose={() => prompt?.settle(false)}>
  {#if prompt}
    <p class="text">{msg('emul.busyText', { emulator })}</p>
    <p class="hint">{msg('emul.busyHint')}</p>
    {#if failure}<div class="failure"><Notice tone="danger" compact>{failure}</Notice></div>{/if}
  {/if}
  {#snippet footer()}
    <Button disabled={closing} reason={held} onclick={() => prompt?.settle(false)}>{msg('common.cancel')}</Button>
    <Button variant="primary" busy={closing} onclick={closeAndPlay}>
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
  }
</style>
