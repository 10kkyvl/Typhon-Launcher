<script lang="ts">
  import { ExternalLink, FolderOpen } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { hasMessage, msg } from '../i18n';
  import { libraryErrorText } from '../library/errors';
  import {
    listPlatforms,
    openOfficialPage,
    selectEmulatorFile,
    setEmulatorPath,
    type PlatformStatus,
  } from '../services/emulators';
  import { emulatorPrompt } from '../stores/emulatorPrompt';

  const prompt = $derived($emulatorPrompt);

  let platforms = $state<PlatformStatus[]>([]);
  let failure = $state('');
  let busy = $state(false);

  const info = $derived(prompt ? platforms.find((item) => item.id === prompt.platform) : undefined);
  const reason = $derived(
    prompt && prompt.code !== 'emulators.not_configured' && hasMessage(prompt.code) ? msg(prompt.code) : '',
  );

  $effect(() => {
    const current = prompt;
    failure = '';
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
    return () => {
      stale = true;
    };
  });

  async function openSite() {
    if (!prompt) return;
    try {
      await openOfficialPage(prompt.platform);
    } catch (err) {
      failure = libraryErrorText(err, msg('emul.openSiteFailed'));
    }
  }

  async function choose() {
    const current = prompt;
    if (!current || busy) return;
    busy = true;
    failure = '';
    try {
      const path = await selectEmulatorFile(msg('emul.pickDialog', { emulator: info?.emulator ?? current.platform }));
      if (!path) return;
      await setEmulatorPath(current.platform, path);
      current.settle(true);
    } catch (err) {
      failure = libraryErrorText(err, msg('emul.saveFailed'));
    } finally {
      busy = false;
    }
  }
</script>

<Modal
  dismissible={!busy}
  open={Boolean(prompt)}
  title={msg('emul.neededTitle')}
  width="48rem"
  onclose={() => prompt?.settle(false)}
>
  {#if prompt}
    <p class="text">
      {info
        ? msg('emul.neededText', { console: info.name, emulator: info.emulator })
        : msg('emul.neededTextUnknown')}
    </p>
    {#if reason}<p class="reason">{reason}</p>{/if}
    <p class="hint">{msg('emul.neededHint')}</p>
    {#if failure}<p class="failure" role="alert">{failure}</p>{/if}
  {/if}
  {#snippet footer()}
    <Button disabled={busy} onclick={() => prompt?.settle(false)}>{msg('common.cancel')}</Button>
    <Button disabled={busy || !info} onclick={openSite}>
      <ExternalLink size="1.4rem" strokeWidth={1.8} />
      {msg('emul.neededOpenSite')}
    </Button>
    <Button variant="primary" disabled={busy} onclick={choose}>
      <FolderOpen size="1.4rem" strokeWidth={1.8} />
      {msg('emul.neededChoose')}
    </Button>
  {/snippet}
</Modal>

<style>
  .text {
    margin: 0 0 var(--space-3);
    font-size: var(--font-md);
    color: var(--text);
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
