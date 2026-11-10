<script lang="ts">
  import { FolderInput } from '@lucide/svelte';
  import { msg } from '../../lib/i18n';
  import Button from '../../lib/components/Button.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import { moveErrorText } from '../../lib/relocate/moveMessages';
  import { movePercent, moveSummary, stageLabel } from '../../lib/relocate/moveText';
  import { inWails } from '../../lib/services/backend';
  import { cancelMove, moveLibrary, selectMoveTargetFolder } from '../../lib/services/relocate';
  import { activeMove } from '../../lib/stores/relocate';
  import { settings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';

  let starting = $state(false);
  let cancelling = $state(false);
  let failure = $state('');

  const job = $derived($activeMove && $activeMove.scope === 'library' ? $activeMove : null);
  const queueLeft = $derived(job?.queue?.length ?? 0);

  async function start() {
    if (starting || job) return;
    if (!inWails) {
      toast(msg('settings.generalLibraryMoveDesktopOnlyToast'));
      return;
    }
    let parent = '';
    try {
      parent = await selectMoveTargetFolder();
    } catch (err) {
      toast(moveErrorText(err, msg('settings.generalLibraryMoveDialogFailed')), 'danger');
      return;
    }
    if (!parent) return;
    starting = true;
    failure = '';
    try {
      await moveLibrary(parent);
    } catch (err) {
      failure = moveErrorText(err, msg('settings.generalLibraryMoveStartFailed'));
    } finally {
      starting = false;
    }
  }

  async function cancel() {
    if (!job || cancelling) return;
    cancelling = true;
    try {
      await cancelMove(job.id);
    } catch (err) {
      toast(moveErrorText(err, msg('settings.generalLibraryMoveCancelFailed')), 'danger');
    } finally {
      cancelling = false;
    }
  }
</script>

<div class="row">
  <div class="row-text">
    <span class="row-label">{msg('settings.generalLibraryMoveLabel')}</span>
    <span class="row-sub">
      {#if $settings?.libraryPath}
        {msg('settings.generalLibraryMoveCurrentSub', { path: $settings.libraryPath })}
      {:else}
        {msg('settings.generalLibraryMoveEmptySub')}
      {/if}
    </span>
  </div>
  <Button size="sm" disabled={starting || !!job || !$settings?.libraryPath} onclick={start}>
    <FolderInput size="1.5rem" strokeWidth={1.8} />
    {starting ? msg('settings.generalLibraryMoveStartingEllipsis') : msg('settings.generalLibraryMoveButtonLabel')}
  </Button>
</div>

{#if job}
  <div class="row progress-row">
    <div class="progress-body">
      <ProgressBar value={movePercent(job)} />
      <span class="row-sub">
        {stageLabel(job.stage)}{job.title ? ` · ${job.title}` : ''}
        {#if queueLeft > 0}
          {msg('settings.generalLibraryMoveQueueLeft', { count: queueLeft })}
        {/if}
        — {moveSummary(job)}
      </span>
      <span class="row-sub">{msg('settings.generalLibraryMoveCancelHint')}</span>
    </div>
    <Button size="sm" variant="danger" disabled={cancelling} onclick={cancel}>
      {cancelling ? msg('settings.generalLibraryMoveCancellingEllipsis') : msg('settings.generalLibraryMoveCancelButton')}
    </Button>
  </div>
{/if}

{#if failure}
  <p class="failure">{failure}</p>
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
    border-top: 1px solid var(--border);
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .row-label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .row-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .progress-row {
    align-items: flex-start;
  }

  .progress-body {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .failure {
    margin: var(--space-2) 0 0;
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
