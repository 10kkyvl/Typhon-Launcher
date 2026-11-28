<script lang="ts">
  import { canRetryInstall, needsInteractiveInstall } from '../install/installErrors';
  import { downloadsById } from '../stores/downloads';
  import { installations } from '../stores/install';
  import { settings } from '../stores/settings';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import InstallChooseStep from './InstallChooseStep.svelte';
  import InstallOutcome from './InstallOutcome.svelte';
  import InstallPlanStep from './InstallPlanStep.svelte';
  import { createInstallDialog } from './installDialog.svelte';
  import { msg } from '../i18n';

  /**
   * Installs one download: its plan (destination, space, mode), the choice of the game's executable,
   * progress and the outcome. Every refusal is said in the dialog by code and takes the focus; while
   * an action runs the other controls are held with a reason. The dialog is reopened for one download
   * after another and can be unmounted with its page: an answer for an earlier opening paints nothing
   * here, and an install it started, or a refusal, is said once in a toast naming its package.
   */
  let { open = $bindable(false), downloadId }: { open?: boolean; downloadId: string | null } = $props();
  const uid = $props.id();
  const failureId = `${uid}-failure`;

  const dialog = createInstallDialog({
    get open() { return open; },
    set open(value) { open = value; },
    get downloadId() { return downloadId; },
    get installations() { return $installations; },
    get downloadsById() { return $downloadsById; },
    get settings() { return $settings; },
    failureId,
  });
</script>

<Modal dismissible={!dialog.busy} bind:open title={msg('modals.installTitle')} width="54rem">
  {#if dialog.phase === 'loading'}
    <div class="loading" role="status">
      <Spinner size="lg" />
      <span class="loading-text">{msg('modals.installAnalyzing')}</span>
    </div>
  {:else if dialog.phase === 'unreadable'}
    <Notice
      tone="danger"
      compact
      bind:retrying={() => dialog.readRetrying, (value) => (dialog.readRetrying = value)}
      retry={{ label: msg('common.retry'), run: () => (downloadId ? dialog.load(downloadId) : Promise.resolve(undefined)) }}
    >
      {dialog.readFailure}
    </Notice>
  {:else if dialog.phase === 'plan' && dialog.info && dialog.plan}
    <InstallPlanStep
      info={dialog.info}
      plan={dialog.plan}
      space={dialog.space}
      spacePending={dialog.spacePending}
      spaceError={dialog.spaceError}
      notEnoughSpace={dialog.notEnoughSpace}
      requirementsGameId={dialog.requirementsGameId}
      destination={dialog.destination}
      hint={dialog.hint}
      roots={dialog.roots}
      rootOptions={dialog.rootOptions}
      rootChoice={dialog.rootChoice}
      pickRoot={dialog.pickRoot}
      controlled={dialog.controlled}
      silent={dialog.silent}
      portable={dialog.portable}
      external={dialog.external}
      modeOptions={dialog.modeOptions}
      mode={dialog.mode}
      setMode={dialog.setMode}
      movesInPlace={dialog.movesInPlace}
      held={dialog.busy ? msg('modals.actionRunning') : ''}
      acting={dialog.busy ? dialog.actor : ''}
      ondestination={dialog.setDestination}
      onbrowse={dialog.browseDestination}
      onusesuggested={dialog.useSuggested}
      onopensource={dialog.openSource}
      onpickinstaller={dialog.pickInstaller}
      onpickexecutable={dialog.pickGameExecutable}
    />
  {:else if dialog.phase === 'choose' && dialog.installation}
    <InstallChooseStep
      candidatePaths={dialog.candidatePaths}
      bind:chosen={() => dialog.chosen, (value) => (dialog.chosen = value)}
      held={dialog.busy ? msg('modals.actionRunning') : ''}
      acting={dialog.busy ? dialog.actor : ''}
      onpickmanual={dialog.pickManual}
    />
  {:else if dialog.phase === 'image' || dialog.phase === 'progress' || dialog.phase === 'done' || dialog.phase === 'problem'}
    <InstallOutcome
      phase={dialog.phase}
      installation={dialog.installation}
      imageDownload={dialog.imageDownload}
      imageTitle={dialog.imageTitle}
      imagePlatform={dialog.imagePlatform}
      externalWait={dialog.externalWait}
      askCleanup={dialog.askCleanup}
      bind:cleanupResolved={() => dialog.cleanupResolved, (value) => (dialog.cleanupResolved = value)}
      seedingNow={dialog.seedingNow}
      own={dialog.own}
      other={dialog.other}
      ondelete={dialog.deleteData}
    />
  {/if}

  {#if dialog.failure}
    <div class="failure">
      <Notice tone="danger" compact id={failureId} tabindex={-1}>{dialog.failure}</Notice>
    </div>
  {/if}

  {#snippet footer()}
    {#if dialog.phase === 'plan' && dialog.plan}
      <Button disabled={dialog.busy} reason={dialog.busy ? msg('modals.actionRunning') : ''} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      {#if dialog.plan.type !== 'unknown'}
        <Button variant="primary" disabled={!!dialog.startReason} reason={dialog.startReason} busy={dialog.own('start')} onclick={dialog.start}>
          {dialog.primaryLabel}
        </Button>
      {/if}
    {:else if dialog.phase === 'image'}
      <Button disabled={dialog.busy} reason={dialog.busy ? msg('modals.actionRunning') : ''} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={dialog.other('image')} reason={dialog.heldFor('image')} busy={dialog.own('image')} onclick={dialog.startImage}>
        {msg('emul.installAction')}
      </Button>
    {:else if dialog.phase === 'unreadable'}
      <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    {:else if dialog.phase === 'progress'}
      <Button onclick={() => (open = false)}>{msg('modals.installMinimize')}</Button>
      <Button
        variant="danger"
        disabled={dialog.other('cancel') || dialog.externalWait}
        reason={dialog.heldFor('cancel') || (dialog.externalWait ? msg('modals.installWaitingExternal') : '')}
        busy={dialog.own('cancel')}
        onclick={dialog.cancelCurrent}
      >
        {msg('modals.installCancelInstall')}
      </Button>
    {:else if dialog.phase === 'choose'}
      <Button disabled={dialog.busy} reason={dialog.busy ? msg('modals.actionRunning') : ''} onclick={() => (open = false)}>{msg('common.close')}</Button>
      <Button variant="danger" disabled={dialog.other('cancel')} reason={dialog.heldFor('cancel')} busy={dialog.own('cancel')} onclick={dialog.cancelCurrent}>
        {msg('modals.installCancelInstall')}
      </Button>
      <Button
        variant="primary"
        disabled={dialog.other('confirm') || !dialog.chosen}
        reason={dialog.heldFor('confirm') || (dialog.chosen ? '' : msg('modals.installChooseGameExecutable'))}
        busy={dialog.own('confirm')}
        onclick={dialog.confirmChoice}
      >
        {msg('modals.installConfirm')}
      </Button>
    {:else if dialog.phase === 'done'}
      {#if dialog.installation?.platform && dialog.installation.gameId}
        <Button data-autofocus onclick={() => (open = false)}>{msg('common.done')}</Button>
        <Button variant="primary" disabled={dialog.other('play')} reason={dialog.heldFor('play')} busy={dialog.own('play')} onclick={dialog.playInstalled}>{msg('games.play')}</Button>
      {:else}
        <Button data-autofocus variant="primary" onclick={() => (open = false)}>{msg('common.done')}</Button>
      {/if}
    {:else if dialog.phase === 'problem'}
      <Button onclick={dialog.dismissCurrent} disabled={dialog.other('dismiss')} reason={dialog.heldFor('dismiss')} busy={dialog.own('dismiss')}>{msg('modals.installDismiss')}</Button>
      {#if dialog.installation?.platform}
        <Button onclick={dialog.openImageFolder} disabled={dialog.other('folder')} reason={dialog.heldFor('folder')} busy={dialog.own('folder')}>{msg('emul.installFolder')}</Button>
      {/if}
      {#if dialog.installation && needsInteractiveInstall(dialog.installation.error)}
        <Button variant="primary" onclick={dialog.retryManually} disabled={dialog.other('retry')} reason={dialog.heldFor('retry')} busy={dialog.own('retry')}>
          {msg('modals.installManually')}
        </Button>
      {:else if !dialog.installation || canRetryInstall(dialog.installation.error)}
        <Button variant="primary" onclick={dialog.retryCurrent} disabled={dialog.other('retry')} reason={dialog.heldFor('retry')} busy={dialog.own('retry')}>{msg('common.retry')}</Button>
      {/if}
    {/if}
  {/snippet}
</Modal>

<style>
  .loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1.2rem;
    padding: 3.2rem 0;
  }

  .loading-text {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .failure {
    margin-top: var(--space-4);
  }
</style>
