<script lang="ts">
  import { onDestroy } from 'svelte';
  import { ExternalLink, FolderOpen } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { hasMessage, msg } from '../i18n';
  import { actionErrorText } from '../library/errors';
  import {
    listPlatforms,
    openOfficialPage,
    selectEmulatorFile,
    setEmulatorPath,
    type PlatformStatus,
  } from '../services/emulators';
  import { emulatorPrompt } from '../stores/emulatorPrompt';
  import { toast } from '../stores/toasts';

  const prompt = $derived($emulatorPrompt);

  let platforms = $state<PlatformStatus[]>([]);
  let listed = $state<'reading' | 'read' | 'failed'>('reading');
  let failure = $state('');
  // What runs: the emulator's file chosen and saved, or its site opened. That button stays busy.
  let busy = $state<'' | 'choose' | 'site'>('');

  const info = $derived(prompt ? platforms.find((item) => item.id === prompt.platform) : undefined);
  const reason = $derived(
    prompt && prompt.code !== 'emulators.not_configured' && hasMessage(prompt.code) ? msg(prompt.code) : '',
  );
  const running = $derived(busy ? msg('modals.actionRunning') : '');
  // The site is the emulator's own, known from the list: held until it is read.
  const siteHeld = $derived(running || (info ? '' : listed === 'failed' ? msg('emul.loadFailed') : msg('emul.loading')));

  // A launch can replace the prompt while its action runs: the answer then belongs to no prompt on
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
    busy = '';
    if (!current) return;
    let stale = false;
    listed = 'reading';
    listPlatforms().then(
      (list) => {
        if (stale) return;
        platforms = list;
        listed = 'read';
      },
      (err) => {
        console.error('list emulators', err);
        if (stale) return;
        listed = 'failed';
        failure = msg('emul.loadFailed');
      },
    );
    return () => {
      stale = true;
    };
  });

  async function openSite() {
    const current = prompt;
    if (!current || busy || !info) return;
    const at = opening;
    const emulator = info.emulator;
    busy = 'site';
    failure = '';
    try {
      await openOfficialPage(current.platform);
    } catch (err) {
      if (alive && at === opening) failure = actionErrorText(err, msg('emul.openSiteFailed'));
      else toast(actionErrorText(err, msg('emul.openSiteFailedFor', { emulator })), 'danger');
    }
    if (alive && at === opening) busy = '';
  }

  async function choose() {
    const current = prompt;
    if (!current || busy) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const emulator = info?.emulator ?? current.platform;
    busy = 'choose';
    failure = '';
    let saved = false;
    try {
      const path = await selectEmulatorFile(msg('emul.pickDialog', { emulator }));
      if (path) {
        await setEmulatorPath(current.platform, path);
        saved = true;
      }
    } catch (err) {
      if (!heard()) {
        toast(actionErrorText(err, msg('emul.saveFailedFor', { emulator })), 'danger');
        return;
      }
      failure = actionErrorText(err, msg('emul.saveFailed'));
    }
    if (!heard()) {
      if (saved) toast(msg('emul.pathSaved', { emulator }), 'success');
      return;
    }
    busy = '';
    if (saved) current.settle(true);
  }
</script>

<!-- Big Picture shows this prompt too: its D-pad walks the Tab order, so every button stays in it,
     held with a reason rather than disabled. -->
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
    {#if reason}<div class="reason"><Notice tone="danger" compact announce={false}>{reason}</Notice></div>{/if}
    <p class="hint">{msg('emul.neededHint')}</p>
    {#if failure}<div class="failure"><Notice tone="danger" compact>{failure}</Notice></div>{/if}
  {/if}
  {#snippet footer()}
    <Button disabled={!!busy} reason={running} onclick={() => prompt?.settle(false)}>{msg('common.cancel')}</Button>
    <Button disabled={!!siteHeld && busy !== 'site'} reason={busy === 'site' ? '' : siteHeld} busy={busy === 'site'} onclick={openSite}>
      <ExternalLink size="1.4rem" strokeWidth={1.8} />
      {msg('emul.neededOpenSite')}
    </Button>
    <Button variant="primary" disabled={busy === 'site'} reason={busy === 'site' ? running : ''} busy={busy === 'choose'} onclick={choose}>
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
