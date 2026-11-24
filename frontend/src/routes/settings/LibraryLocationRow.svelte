<script lang="ts">
  import { FolderInput } from '@lucide/svelte';
  import { msg } from '../../lib/i18n';
  import Button from '../../lib/components/Button.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
  import { moveErrorText } from '../../lib/relocate/moveMessages';
  import { movePercent, moveSummary, stageLabel } from '../../lib/relocate/moveText';
  import { inWails } from '../../lib/services/backend';
  import { cancelMove, moveLibrary, selectMoveTargetFolder, type MoveJob } from '../../lib/services/relocate';
  import { activeMove } from '../../lib/stores/relocate';
  import { settings } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';

  let starting = $state(false);
  let cancelling = $state(false);
  let failure = $state('');

  const job = $derived($activeMove && $activeMove.scope === 'library' ? $activeMove : null);
  const libraryPath = $derived($settings?.libraryPath ?? '');

  // The running move in one sentence: its stage, the game being copied, the games after it, the totals.
  function progressLine(move: MoveJob): string {
    const stage = stageLabel(move.stage);
    const summary = moveSummary(move);
    const count = move.queue?.length ?? 0;
    if (move.title && count > 0) return msg('settings.generalLibraryMoveProgressQueue', { stage, title: move.title, count, summary });
    if (move.title) return msg('settings.generalLibraryMoveProgress', { stage, title: move.title, summary });
    if (count > 0) return msg('settings.generalLibraryMoveProgressQueueNoTitle', { stage, count, summary });
    return msg('settings.generalLibraryMoveProgressNoTitle', { stage, summary });
  }

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

{#snippet progress()}
  {#if job}
    <div class="progress">
      <ProgressBar value={movePercent(job)} label={msg('settings.generalLibraryMoveLabel')} />
      <div class="progress-line">
        <div class="progress-text">
          <span>{progressLine(job)}</span>
          <span>{msg('settings.generalLibraryMoveCancelHint')}</span>
        </div>
        <Button size="sm" variant="danger" disabled={cancelling} onclick={cancel}>
          {cancelling ? msg('settings.generalLibraryMoveCancellingEllipsis') : msg('settings.generalLibraryMoveCancelButton')}
        </Button>
      </div>
    </div>
  {/if}
  {#if failure}
    <div class:failure={!!job}><Notice tone="danger" compact>{failure}</Notice></div>
  {/if}
{/snippet}

<SettingsRow
  label={msg('settings.generalLibraryMoveLabel')}
  hint={libraryPath ? msg('settings.generalLibraryMoveCurrentSub', { path: libraryPath }) : undefined}
  disabledReason={libraryPath ? undefined : msg('settings.generalLibraryMoveEmptySub')}
  below={job || failure ? progress : undefined}
>
  {#snippet children({ hintId, disabled })}
    <!-- Busy from the press to the end of the move it started: it keeps focus all along. -->
    <Button size="sm" aria-describedby={hintId} {disabled} busy={starting || !!job} onclick={start}>
      <FolderInput size="1.5rem" strokeWidth={1.8} />
      {starting ? msg('settings.generalLibraryMoveStartingEllipsis') : msg('settings.generalLibraryMoveButtonLabel')}
    </Button>
  {/snippet}
</SettingsRow>

<style>
  .progress {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .progress-line {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .progress-text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .failure {
    margin-top: var(--space-2);
  }
</style>
