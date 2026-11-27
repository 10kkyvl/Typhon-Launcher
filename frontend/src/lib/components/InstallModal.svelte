<script lang="ts">
  import { untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { addGame, launchGame, selectExecutable } from '../services/library';
  import { libraryErrorText } from '../library/errors';
  import {
    cancelInstall,
    confirmExecutable,
    deleteDownloadData,
    dismissInstall,
    inspectDownload,
    isControlled,
    isExternal,
    planSpace,
    retryInstall,
    retryInstallInteractive,
    startInstall,
    type PlanInfo,
    type SpaceInfo,
  } from '../services/install';
  import { canRetryInstall, installErrorText, needsInteractiveInstall } from '../install/installErrors';
  import { createSpacePlanner } from '../install/spacePlanner';
  import { adviseDestination, type DestinationHint, type DestinationOrigin } from '../install/destinationAdvice';
  import { libraryFolders, openGameFolder, selectFolder, type LibraryFolder } from '../services/settings';
  import { destinationInRoot, rootOfDestination } from '../library/folders';
  import { downloadsById } from '../stores/downloads';
  import type { Download } from '../services/downloads';
  import {
    installActive,
    installations,
    installationsByDownload,
    upsertInstallation,
  } from '../stores/install';
  import { settings } from '../stores/settings';
  import { toast } from '../stores/toasts';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import Spinner from './Spinner.svelte';
  import InstallChooseStep from './InstallChooseStep.svelte';
  import InstallOutcome from './InstallOutcome.svelte';
  import InstallPlanStep from './InstallPlanStep.svelte';
  import { bytesSize } from '../utils/format';
  import { msg } from '../i18n';

  let { open = $bindable(false), downloadId }: { open?: boolean; downloadId: string | null } = $props();
  const uid = $props.id();

  const modeOptions = $derived([
    { id: 'move', label: msg('modals.installModeMove') },
    { id: 'copy', label: msg('modals.installModeCopy') },
  ]);

  let installId = $state<string | null>(null);
  let imageDownload = $state<Download | null>(null);
  let info = $state<PlanInfo | null>(null);
  let loading = $state(false);
  let busy = $state(false);
  // Which button set `busy`: that one stays focused and busy, the others wait.
  let actor = $state('');
  let destination = $state('');
  let mode = $state('');
  let chosen = $state('');
  let cleanupResolved = $state(false);
  let space = $state<SpaceInfo | null>(null);
  let spacePending = $state(false);
  let spaceError = $state('');
  let folders = $state<LibraryFolder[]>([]);
  let hint = $state<DestinationHint | null>(null);
  let token = 0;
  let origin: DestinationOrigin = 'typed';

  const OTHER_ROOT = '__other__';

  const planner = createSpacePlanner({
    fetch: planSpace,
    onPending: () => {
      spacePending = true;
    },
    onResult: (result) => {
      const advice = adviseDestination(result, origin, destination);
      destination = advice.destination;
      hint = advice.hint;
      space = result;
      spaceError = '';
      spacePending = false;
    },
    onError: (err) => {
      spaceError = installErrorText(err);
      spacePending = false;
    },
  });

  const installation = $derived(installId ? $installations.find((i) => i.id === installId) : undefined);
  const imagePlatform = $derived(installation?.platform ?? imageDownload?.origin?.platform ?? '');
  const imageTitle = $derived(imageDownload?.origin?.title || imageDownload?.name || '');
  const plan = $derived(info?.plan);
  const controlled = $derived(plan ? isControlled(plan.type) : false);
  const external = $derived(plan ? isExternal(plan.type) : false);
  const portable = $derived(plan?.type === 'portable');
  const silent = $derived(plan?.silent ?? false);
  const spaceDriven = $derived(controlled || silent);
  const requestMode = $derived(portable ? (info?.seeding ? 'copy' : mode) : '');
  const notEnoughSpace = $derived(!spacePending && !spaceError && !!space && !space.enough);
  const spaceBlocked = $derived(notEnoughSpace || (!spacePending && spaceError !== ''));
  const roots = $derived(info?.roots ?? []);
  const rootChoice = $derived(rootOfDestination(roots, destination) || OTHER_ROOT);
  const rootOptions = $derived([
    ...roots.map((root) => {
      const folder = folders.find((f) => f.path === root);
      const label =
        folder?.available ? msg('modals.installRootOption', { path: root, free: bytesSize(folder.freeBytes) }) : root;
      return { id: root, label };
    }),
    { id: OTHER_ROOT, label: msg('modals.installRootOther') },
  ]);
  const movesInPlace = $derived(requestMode === 'move' && !spaceError && space?.mode === 'move' && space.sameVolume);

  const phase = $derived.by(() => {
    const item = installation;
    if (item) {
      if (item.status === 'waiting_for_user') return 'choose';
      if (item.status === 'completed') return 'done';
      if (installActive(item.status)) return 'progress';
      return 'problem';
    }
    if (info) return 'plan';
    if (imageDownload) return 'image';
    return 'loading';
  });

  const externalWait = $derived(
    !!installation &&
      isExternal(installation.type) &&
      !installation.silent &&
      (installation.status === 'pending' || installation.status === 'preparing' || installation.status === 'installing'),
  );

  const candidatePaths = $derived.by(() => {
    const list = (installation?.candidates ?? []).map((c) => c.path);
    if (chosen && !list.includes(chosen)) list.push(chosen);
    return list;
  });

  const requirementsGameId = $derived(downloadId ? ($downloadsById.get(downloadId)?.origin?.gameId ?? '') : '');

  const askCleanup = $derived($settings?.installCleanupPolicy === 'ask');
  const seedingNow = $derived(!!installation && ($downloadsById.get(installation.downloadId)?.seeding ?? false));

  const primaryLabel = $derived.by(() => {
    switch (plan?.type) {
      case 'portable':
        return requestMode === 'move' ? msg('modals.installMoveAndInstall') : msg('modals.installCopyAndInstall');
      case 'archive_zip':
      case 'archive_7z':
      case 'archive_rar':
        return msg('modals.installExtractAndInstall');
      case 'exe_installer':
      case 'msi_installer':
        return silent ? msg('modals.installInstallLabel') : msg('modals.installRunInstaller');
      default:
        return '';
    }
  });

  function reset() {
    token++;
    planner.cancel();
    installId = null;
    imageDownload = null;
    info = null;
    loading = false;
    busy = false;
    actor = '';
    destination = '';
    mode = '';
    chosen = '';
    cleanupResolved = false;
    space = null;
    spacePending = false;
    spaceError = '';
    folders = [];
    hint = null;
    origin = 'typed';
  }

  function replan(immediate: boolean, source: DestinationOrigin = 'typed') {
    const current = info;
    if (!current || !spaceDriven) return;
    origin = source;
    hint = null;
    const request = { downloadId: current.downloadId, destination, mode: requestMode };
    if (immediate) planner.now(request);
    else planner.schedule(request);
  }

  function setMode(next: string) {
    if (next === mode) return;
    mode = next;
    replan(true);
  }

  function pickRoot(id: string) {
    if (id === OTHER_ROOT) {
      void browseDestination();
      return;
    }
    destination = destinationInRoot(id, destination);
    replan(true, 'pick');
  }

  async function loadFolders(current: number) {
    try {
      const list = await libraryFolders();
      if (current === token) folders = list;
    } catch (err) {
      if (current === token) toast(installErrorText(err), 'danger');
    }
  }

  function onDestinationInput(event: Event & { currentTarget: HTMLInputElement }) {
    destination = event.currentTarget.value;
    replan(false);
  }

  async function load(id: string) {
    const existing = get(installationsByDownload).get(id);
    if (existing) {
      installId = existing.id;
      return;
    }
    const queued = get(downloadsById).get(id);
    if (queued?.origin?.platform) {
      imageDownload = queued;
      return;
    }
    const current = ++token;
    loading = true;
    try {
      const result = await inspectDownload(id);
      if (!open || current !== token) return;
      info = result;
      destination = result.plan.destination;
      mode = result.seeding ? 'copy' : result.mode;
      space = {
        requiredBytes: result.requiredBytes,
        freeBytes: result.freeBytes,
        sameVolume: result.sameVolume,
        mode: result.mode,
        enough: result.freeBytes >= result.requiredBytes,
        destinationBusy: false,
        suggested: '',
        reusable: false,
      };
      replan(true);
      if ((result.roots?.length ?? 0) > 1) void loadFolders(current);
    } catch (err) {
      if (!open || current !== token) return;
      toast(installErrorText(err), 'danger');
      open = false;
    } finally {
      if (current === token) loading = false;
    }
  }

  $effect(() => {
    const isOpen = open;
    const id = downloadId;
    untrack(() => {
      reset();
      if (isOpen && id) load(id);
    });
  });

  $effect(() => () => planner.cancel());

  $effect(() => {
    const item = installation;
    if (!item || item.status !== 'waiting_for_user') return;
    untrack(() => {
      if (chosen) return;
      chosen = item.executable || item.candidates?.[0]?.path || '';
    });
  });

  async function browseDestination() {
    try {
      const path = await selectFolder(msg('modals.installChooseFolder'));
      if (path) {
        destination = path;
        replan(true, 'pick');
      }
    } catch {
      toast(msg('modals.installFolderDialogFailed'), 'danger');
    }
  }

  function useSuggested() {
    const current = hint;
    if (!current) return;
    destination = current.path;
    replan(true);
  }

  async function run(action: () => Promise<void>, by = '', errorText: (err: unknown) => string = installErrorText) {
    busy = true;
    actor = by;
    try {
      await action();
    } catch (err) {
      toast(errorText(err), 'danger');
    }
    busy = false;
    actor = '';
  }

  const own = (by: string) => busy && actor === by;
  const other = (by: string) => busy && actor !== by;

  function start() {
    const current = info;
    if (!current) return;
    return run(async () => {
      const item = await startInstall(current.downloadId, {
        destination: controlled || silent ? destination : '',
        mode: requestMode,
        type: current.plan.type,
        installerPath: current.plan.installerPath,
      });
      upsertInstallation(item);
      installId = item.id;
    }, 'start');
  }

  function startImage() {
    const current = imageDownload;
    if (!current?.origin?.platform) return;
    return run(async () => {
      const item = await startInstall(current.id, {
        platform: current.origin.platform,
        title: current.origin.title || current.name,
        cover: current.origin.cover ?? '',
      });
      upsertInstallation(item);
      installId = item.id;
    }, 'image');
  }

  function playInstalled() {
    const item = installation;
    if (!item?.gameId) return;
    // A launch the player gave up (a question left unanswered) keeps the dialog where it was.
    return run(
      async () => {
        if (await launchGame(item.gameId)) open = false;
      },
      'play',
      (err) => libraryErrorText(err, msg('games.errorPlayFailed')),
    );
  }

  function openImageFolder() {
    const path = installation?.sourcePath || imageDownload?.destination;
    if (!path) return;
    return run(async () => {
      await openGameFolder(path, '');
    }, 'folder');
  }

  function openSource() {
    const path = plan?.sourcePath;
    if (!path) return;
    return run(async () => {
      await openGameFolder(path, plan?.installerPath || '');
    });
  }

  function pickInstaller() {
    const current = info;
    if (!current) return;
    return run(async () => {
      const path = await selectExecutable(msg('modals.installChooseInstaller'));
      if (!path) return;
      const item = await startInstall(current.downloadId, {
        type: 'exe_installer',
        installerPath: path,
      });
      upsertInstallation(item);
      installId = item.id;
    });
  }

  function pickGameExecutable() {
    const current = info;
    if (!current) return;
    return run(async () => {
      const path = await selectExecutable(msg('modals.installChooseGameExecutable'));
      if (!path) return;
      await addGame(path, current.name);
      toast(msg('modals.installGameAdded'), 'success');
      open = false;
    });
  }

  function pickManual() {
    return run(async () => {
      const path = await selectExecutable(msg('modals.installChooseGameExecutable'));
      if (path) chosen = path;
    });
  }

  function confirmChoice() {
    const item = installation;
    if (!item || !chosen) return;
    return run(() => confirmExecutable(item.id, chosen), 'confirm');
  }

  function cancelCurrent() {
    const item = installation;
    if (!item) return;
    return run(() => cancelInstall(item.id), 'cancel');
  }

  function retryCurrent() {
    const item = installation;
    if (!item) return;
    return run(() => retryInstall(item.id), 'retry');
  }

  function retryManually() {
    const item = installation;
    if (!item) return;
    return run(() => retryInstallInteractive(item.id), 'retry');
  }

  function dismissCurrent() {
    const item = installation;
    if (!item) return;
    return run(async () => {
      await dismissInstall(item.id);
      installId = null;
      if (downloadId) await load(downloadId);
    }, 'dismiss');
  }

  function deleteData() {
    const item = installation;
    if (!item) return;
    return run(async () => {
      await deleteDownloadData(item.downloadId);
      cleanupResolved = true;
      toast(msg('modals.installDownloadDataDeleted'), 'success');
    }, 'delete');
  }
</script>

<Modal dismissible={!busy} bind:open title={msg('modals.installTitle')} width="54rem">
  {#if phase === 'loading'}
    <div class="loading" role="status">
      <Spinner size="lg" />
      <span class="loading-text">{msg('modals.installAnalyzing')}</span>
    </div>
  {:else if phase === 'plan' && info && plan}
    <InstallPlanStep
      {info}
      {plan}
      {space}
      {spacePending}
      {spaceError}
      {notEnoughSpace}
      {requirementsGameId}
      {destination}
      {hint}
      {roots}
      {rootOptions}
      {rootChoice}
      {pickRoot}
      {controlled}
      {silent}
      {portable}
      {external}
      {modeOptions}
      {mode}
      {setMode}
      {movesInPlace}
      ondestinationinput={onDestinationInput}
      onbrowse={browseDestination}
      onusesuggested={useSuggested}
      onopensource={openSource}
      onpickinstaller={pickInstaller}
      onpickexecutable={pickGameExecutable}
    />
  {:else if phase === 'choose' && installation}
    <InstallChooseStep {candidatePaths} bind:chosen onpickmanual={pickManual} />
  {:else if phase === 'image' || phase === 'progress' || phase === 'done' || phase === 'problem'}
    <InstallOutcome
      {phase}
      {installation}
      {imageDownload}
      {imageTitle}
      {imagePlatform}
      {externalWait}
      {askCleanup}
      bind:cleanupResolved
      {seedingNow}
      {own}
      {other}
      ondelete={deleteData}
    />
  {/if}

  {#snippet footer()}
    {#if phase === 'plan' && plan}
      <Button disabled={busy} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      {#if plan.type !== 'unknown'}
        <Button
          variant="primary"
          disabled={other('start') || spaceBlocked || ((controlled || silent) && destination.trim() === '')}
          busy={own('start')}
          onclick={start}
        >
          {primaryLabel}
        </Button>
      {/if}
    {:else if phase === 'image'}
      <Button disabled={busy} onclick={() => (open = false)}>{msg('common.cancel')}</Button>
      <Button variant="primary" disabled={other('image')} busy={own('image')} onclick={startImage}>{msg('emul.installAction')}</Button>
    {:else if phase === 'progress'}
      <Button onclick={() => (open = false)}>{msg('modals.installMinimize')}</Button>
      <Button variant="danger" disabled={other('cancel') || externalWait} busy={own('cancel')} onclick={cancelCurrent}>
        {msg('modals.installCancelInstall')}
      </Button>
    {:else if phase === 'choose'}
      <Button onclick={() => (open = false)}>{msg('common.close')}</Button>
      <Button variant="danger" disabled={other('cancel')} busy={own('cancel')} onclick={cancelCurrent}>{msg('modals.installCancelInstall')}</Button>
      <Button variant="primary" disabled={other('confirm') || !chosen} busy={own('confirm')} onclick={confirmChoice}>
        {msg('modals.installConfirm')}
      </Button>
    {:else if phase === 'done'}
      {#if installation?.platform && installation.gameId}
        <Button onclick={() => (open = false)}>{msg('common.done')}</Button>
        <Button variant="primary" disabled={other('play')} busy={own('play')} onclick={playInstalled}>{msg('games.play')}</Button>
      {:else}
        <Button variant="primary" onclick={() => (open = false)}>{msg('common.done')}</Button>
      {/if}
    {:else if phase === 'problem'}
      <Button onclick={dismissCurrent} disabled={other('dismiss')} busy={own('dismiss')}>{msg('modals.installDismiss')}</Button>
      {#if installation?.platform}
        <Button onclick={openImageFolder} disabled={other('folder')} busy={own('folder')}>{msg('emul.installFolder')}</Button>
      {/if}
      {#if installation && needsInteractiveInstall(installation.error)}
        <Button variant="primary" onclick={retryManually} disabled={other('retry')} busy={own('retry')}>{msg('modals.installManually')}</Button>
      {:else if !installation || canRetryInstall(installation.error)}
        <Button variant="primary" onclick={retryCurrent} disabled={other('retry')} busy={own('retry')}>{msg('common.retry')}</Button>
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
</style>
