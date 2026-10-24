<script lang="ts">
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import { msg } from '../i18n';
  import { crashView, type CrashAction } from '../redist/crashView';
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

  const record = $derived($crashTarget);
  const title = $derived(record ? ($libraryGames.find((g) => g.id === record.gameId)?.title ?? '') : '');
  const view = $derived(record ? crashView(record) : null);
  const reason = $derived(view?.reason ? msg(view.reason.key, view.reason.params) : record ? crashReasonText(record.reasonCode) : '');

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

  async function collectReport() {
    if (reporting) return;
    reporting = true;
    try {
      const bundle = await exportLogs();
      toast(msg('redist.crashReportSaved'), 'success');
      await openFolder(bundle.dir);
    } catch {
      toast(msg('redist.crashReportFailed'), 'danger');
    } finally {
      reporting = false;
    }
  }

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

<Modal bind:open title={view ? msg(view.title) : ''} width="48rem" onclose={dismiss}>
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
    </div>
  {/if}

  {#snippet footer()}
    <Button onclick={dismiss}>{msg('common.close')}</Button>
    {#if view}
      {#each view.actions as action, i (action)}
        <Button variant={i === view.actions.length - 1 ? 'primary' : 'secondary'} onclick={ACTIONS[action].run}>{msg(ACTIONS[action].label)}</Button>
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
