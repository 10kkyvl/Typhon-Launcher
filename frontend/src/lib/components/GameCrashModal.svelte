<script lang="ts">
  import { get } from 'svelte/store';
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import { msg } from '../i18n';
  import { isEmulated } from '../library/emulation';
  import { crashView, type CrashAction, type CrashRecord } from '../redist/crashView';
  import { crashReasonText } from '../redist/messages';
  import { exportLogs } from '../services/system';
  import { openFolder } from '../services/settings';
  import { closeCrash, crashTarget } from '../stores/crash';
  import { openGameProperties } from '../stores/gameProperties';
  import { libraryGames } from '../stores/library';
  import { openRedist } from '../stores/redist';
  import { navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { verify } from '../stores/updates';
  import { focusHandOff } from '../utils/focusHandOff';

  const record = $derived($crashTarget);
  const title = $derived(record ? ($libraryGames.find((g) => g.id === record.gameId)?.title ?? '') : '');
  const emulated = $derived(record ? isEmulated($libraryGames.find((g) => g.id === record.gameId)) : false);
  const view = $derived(record ? crashView(record, emulated) : null);
  const reason = $derived(view?.reason ? msg(view.reason.key, view.reason.params) : record ? crashReasonText(record.reasonCode) : '');

  const uid = $props.id();
  const failureId = `${uid}-failure`;

  let open = $state(false);

  $effect(() => {
    open = record !== null;
  });

  function dismiss() {
    open = false;
    closeCrash();
  }

  function checkFiles() {
    if (!record) return;
    const id = record.gameId;
    dismiss();
    navigate('game', { id });
    void verify(id);
  }

  function openCompat() {
    if (!record) return;
    const id = record.gameId;
    dismiss();
    openGameProperties(id);
  }

  let reporting = $state(false);
  // Why the report could not be collected, for the crash on screen.
  let failure = $state('');

  // A crash is one game's exit at one moment; the details that follow it keep both.
  const crashKey = (crash: CrashRecord | null) => (crash ? `${crash.gameId}@${crash.at}` : '');
  const shownKey = $derived(crashKey(record));
  $effect(() => {
    void shownKey;
    untrack(() => (failure = ''));
  });

  async function collectReport() {
    if (reporting) return;
    const asked = shownKey;
    // The focus the report button holds at the press goes to the words of a failure.
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    reporting = true;
    failure = '';
    try {
      const bundle = await exportLogs();
      toast(msg('redist.crashReportSaved'), 'success');
      await openFolder(bundle.dir);
    } catch {
      // A failure for a crash no longer on screen says nothing about the one that is.
      if (crashKey(get(crashTarget)) === asked) failure = msg('redist.crashReportFailed');
    } finally {
      reporting = false;
    }
    if (failure) await hand(() => document.getElementById(failureId));
  }

  const heldReason = $derived(reporting ? msg('redist.crashReportRunning') : '');

  const ACTIONS: Record<CrashAction, { label: 'redist.crashLibs' | 'redist.crashVerify' | 'redist.crashCompat' | 'redist.crashReport'; run: () => void }> = {
    libs: { label: 'redist.crashLibs', run: installLibs },
    verify: { label: 'redist.crashVerify', run: checkFiles },
    compat: { label: 'redist.crashCompat', run: openCompat },
    report: { label: 'redist.crashReport', run: () => void collectReport() },
  };

  function installLibs() {
    if (!record) return;
    const id = record.gameId;
    dismiss();
    openRedist(id);
  }
</script>

<Modal bind:open dismissible={!reporting} busy={reporting} title={view ? msg(view.title) : ''} width="48rem" onclose={dismiss}>
  {#if record}
    <div class="body">
      <p>{msg('redist.crashGame', { title, seconds: Math.round(record.sessionSeconds) })}</p>
      {#if reason}
        <p>{msg('redist.crashReason', { reason })}</p>
      {/if}
      {#if view}
        {#each view.tips as tip (tip)}
          <p>{msg(tip)}</p>
        {/each}
        {#each view.tech as line (line.key)}
          <p class="code">{msg(line.key, line.params)}</p>
        {/each}
      {/if}
      {#if record.count > 1}
        <p class="repeat">{msg('redist.crashRepeat', { count: record.count })}</p>
      {/if}
      {#if failure}
        <Notice tone="danger" compact id={failureId} tabindex={-1}>{failure}</Notice>
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    <Button disabled={reporting} reason={heldReason} onclick={dismiss}>{msg('common.close')}</Button>
    {#if view}
      {#each view.actions as action, i (action)}
        <Button
          variant={i === view.actions.length - 1 ? 'primary' : 'secondary'}
          disabled={reporting && action !== 'report'}
          reason={action === 'report' ? '' : heldReason}
          busy={reporting && action === 'report'}
          onclick={ACTIONS[action].run}>{msg(ACTIONS[action].label)}</Button
        >
      {/each}
    {/if}
  {/snippet}
</Modal>

<style>
  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  p {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .code {
    color: var(--text-3);
  }

  .repeat {
    color: var(--warning);
  }
</style>
