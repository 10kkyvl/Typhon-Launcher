<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';
  import { ArrowDown, ArrowLeft, ArrowUp, Check, CircleCheck, Download, FileUp, FolderOpen, Gamepad2, Pause, Play, RefreshCw, Trash2, X } from '@lucide/svelte';
  import ProgressBar from '../../components/ProgressBar.svelte';
  import type { BigPictureCommand } from '../input';
  import type { RequestText } from '../contracts';
  import { focusControl } from '../navigation';
  import {
    cancelFetchMetadata,
    cancelDownload,
    discardMetadata,
    fetchMetadata,
    forceStartDownload,
    listDownloads,
    moveDownloadDown,
    moveDownloadUp,
    pauseDownload,
    removeDownload,
    resumeDownload,
    selectTorrentFile,
    startDownloadFrom,
    type Download as DownloadRecord,
    type TorrentInfo,
  } from '../../services/downloads';
  import {
    cancelInstall,
    confirmExecutable,
    deleteDownloadData,
    dismissInstall,
    inspectDownload,
    isControlled,
    isExternal,
    retryInstall,
    startInstall,
    type InstallMode,
    type Installation,
    type PlanInfo,
  } from '../../services/install';
  import { addGame, selectExecutable } from '../../services/library';
  import { openFolder, openGameFolder, selectFolder } from '../../services/settings';
  import { appInfo, elevationSupported, type AppInfo } from '../../services/system';
  import { installErrorText } from '../../install/installErrors';
  import { installIndeterminate, installTotalUnknown } from '../../install/progress';
  import { active, completed, downloads, downloadsById, queue, statusLabels } from '../../stores/downloads';
  import { installStatusLabels, installTypeLabels, installations, installationsByDownload, upsertInstallation } from '../../stores/install';
  import { libraryGames } from '../../stores/library';
  import { settings } from '../../stores/settings';
  import { bytesSize, etaLabel, progressPercent, speedBytes, truncateMiddle } from '../../utils/format';
  import { t } from '../../i18n';
  import { createTransferActionRunner, transferJourneyStage } from '../transfers';

  let { onback, ongame, requestText }: {
    onback: () => void;
    ongame: (id: string) => void;
    requestText: RequestText;
  } = $props();

  type LocalDialog = 'metadata' | 'files' | 'install' | 'confirm';
  type Confirmation =
    | { kind: 'cancel-download'; downloadId: string }
    | { kind: 'remove-download'; downloadId: string }
    | { kind: 'cancel-install'; installId: string }
    | { kind: 'dismiss-install'; installId: string }
    | { kind: 'delete-files'; downloadId: string };

  let dialog = $state<LocalDialog | null>(null);
  let dialogElement = $state<HTMLElement | undefined>();
  let pageRoot: HTMLElement | undefined;
  let pageReturnFocus = '';
  let parentDialog: LocalDialog | null = null;
  let parentDialogFocus = '';
  let confirmation = $state<Confirmation | null>(null);
  let disposed = false;
  let addSession = $state(0);
  let metadataRequest = 0;
  let metadataFlight: Promise<void> = Promise.resolve();
  const startingHashes = new Set<string>();

  let torrentSource = $state('');
  let torrentInfo = $state<TorrentInfo | null>(null);
  let selectedFiles = $state<boolean[]>([]);
  let pendingMetadataHash = '';
  let downloadDestination = $state('');
  let autoInstall = $state(false);
  let elevateAhead = $state(false);
  let platformInfo = $state<AppInfo | null>(null);
  let installDownloadId = $state('');
  let installId = $state('');
  let installRequest = 0;
  let planInfo = $state<PlanInfo | null>(null);
  let installDestination = $state('');
  let installMode = $state<InstallMode>('copy');
  let chosenExecutable = $state('');
  let cleanupResolved = $state(false);

  const actions = createTransferActionRunner((error) => installErrorText(error, $t('bp.transfers.actionError')));
  const { state: actionStateStore, run: runAction } = actions;
  const retryActions = new Map<string, () => Promise<unknown>>();

  const failedDownloads = $derived($downloads.filter((item) => item.status === 'failed'));
  const orphanInstallations = $derived(
    [...$installationsByDownload.values()]
      .filter((item) => item.downloadId && !$downloadsById.has(item.downloadId))
      .sort((a, b) => Date.parse(b.startedAt) - Date.parse(a.startedAt)),
  );
  const selectedInstallation = $derived(installId ? $installations.find((item) => item.id === installId) : undefined);
  const selectedDownload = $derived(installDownloadId ? $downloadsById.get(installDownloadId) : undefined);
  const selectedFilesCount = $derived(selectedFiles.filter(Boolean).length);
  const selectedFilesSize = $derived((torrentInfo?.files ?? []).reduce((sum, file, index) => sum + (selectedFiles[index] ? file.size : 0), 0));
  const allFilesSelected = $derived(selectedFiles.length > 0 && selectedFilesCount === selectedFiles.length);
  const selectedPaths = $derived((torrentInfo?.files ?? []).filter((_, index) => selectedFiles[index]).map((file) => file.path));
  const canElevateAhead = $derived(
    autoInstall && platformInfo !== null && elevationSupported(platformInfo) &&
      selectedPaths.some((path) => /\.(?:exe|msi)$/i.test(path)),
  );
  const selectedStage = $derived(
    transferJourneyStage(selectedDownload?.status, selectedInstallation?.status, selectedInstallation?.gameId),
  );
  const installPlan = $derived(planInfo?.plan);
  const controlledPlan = $derived(installPlan ? isControlled(installPlan.type) : false);
  const externalPlan = $derived(installPlan ? isExternal(installPlan.type) : false);
  const portablePlan = $derived(installPlan?.type === 'portable');
  const silentPlan = $derived(installPlan?.silent ?? false);
  const notEnoughSpace = $derived(
    !!planInfo && planInfo.requiredBytes > 0 && planInfo.freeBytes > 0 && planInfo.freeBytes < planInfo.requiredBytes,
  );
  const seedingNow = $derived(!!selectedInstallation && ($downloadsById.get(selectedInstallation.downloadId)?.seeding ?? false));
  const askCleanup = $derived($settings?.installCleanupPolicy === 'ask');
  const externalWait = $derived(
    !!selectedInstallation && isExternal(selectedInstallation.type) && !selectedInstallation.silent &&
      ['pending', 'preparing', 'installing'].includes(selectedInstallation.status),
  );
  const candidatePaths = $derived.by(() => {
    const paths = (selectedInstallation?.candidates ?? []).map((candidate) => candidate.path);
    if (chosenExecutable && !paths.includes(chosenExecutable)) paths.push(chosenExecutable);
    return paths;
  });

  function actionKey(scope: string, id: string, action: string) {
    return `${scope}:${id}:${action}`;
  }

  function addActionKey(action: string) {
    return actionKey('add', String(addSession), action);
  }

  function clearActionPrefix(prefix: string) {
    actions.clear(prefix);
    for (const key of retryActions.keys()) if (key.startsWith(prefix)) retryActions.delete(key);
  }

  function pageIsActive() {
    const slot = pageRoot?.closest<HTMLElement>('[data-bp-page]');
    return !slot || (!slot.hidden && !slot.hasAttribute('inert'));
  }

  function isCurrentInstallView(downloadId: string, request = installRequest) {
    return !disposed && pageIsActive() && dialog === 'install' && installDownloadId === downloadId && request === installRequest;
  }

  function discardMetadataSafely(infoHash: string) {
    if (infoHash) void discardMetadata(infoHash).catch(() => undefined);
  }

  function cancelMetadataSafely(source: string) {
    if (source) void cancelFetchMetadata(source).catch(() => undefined);
  }

  function discardPendingTorrent() {
    metadataRequest++;
    if (dialog === 'metadata') cancelMetadataSafely(torrentSource);
    if (pendingMetadataHash && !startingHashes.has(pendingMetadataHash)) discardMetadataSafely(pendingMetadataHash);
    pendingMetadataHash = '';
    torrentInfo = null;
    selectedFiles = [];
    torrentSource = '';
    clearActionPrefix(`add:${addSession}:`);
    addSession++;
  }

  function pageBecameInactive() {
    installRequest++;
    if (dialog === 'metadata' || dialog === 'files') discardPendingTorrent();
    else {
      clearActionPrefix('add:');
      addSession++;
      metadataRequest++;
    }
    dialog = null;
    confirmation = null;
    parentDialog = null;
    parentDialogFocus = '';
    pageReturnFocus = '';
  }

  onMount(() => {
    const slot = pageRoot?.closest<HTMLElement>('[data-bp-page]');
    if (!slot) return;
    let wasActive = !slot.hidden && !slot.hasAttribute('inert');
    const observer = new MutationObserver(() => {
      const nowActive = !slot.hidden && !slot.hasAttribute('inert');
      if (wasActive && !nowActive) pageBecameInactive();
      wasActive = nowActive;
    });
    observer.observe(slot, { attributes: true, attributeFilter: ['hidden', 'inert'] });
    return () => observer.disconnect();
  });

  onDestroy(() => {
    disposed = true;
    installRequest++;
    if (dialog === 'metadata' || dialog === 'files') discardPendingTorrent();
    else {
      clearActionPrefix('add:');
      metadataRequest++;
    }
  });

  function actionPending(key: string) {
    return $actionStateStore.pending.includes(key);
  }

  function actionError(key: string) {
    return $actionStateStore.errors[key] ?? '';
  }

  async function perform(key: string, action: () => Promise<unknown>) {
    retryActions.set(key, action);
    const ok = await runAction(key, action);
    if (ok) retryActions.delete(key);
    return ok;
  }

  async function retryAction(key: string) {
    const action = retryActions.get(key);
    if (action) await perform(key, action);
  }

  function currentFocusKey() {
    return (document.activeElement as HTMLElement | null)?.dataset?.bpFocus ?? '';
  }

  async function focusDialog() {
    await tick();
    const controls = dialogElement?.querySelectorAll<HTMLButtonElement>('[data-bp-focus]:not(:disabled)') ?? [];
    const defaultControl = dialogElement?.querySelector<HTMLButtonElement>('[data-bp-default]:not(:disabled)');
    focusControl(defaultControl ?? controls[0]);
  }

  async function showDialog(next: LocalDialog, replace = false) {
    if (!dialog) {
      pageReturnFocus = currentFocusKey();
      parentDialog = null;
      parentDialogFocus = '';
    } else if (!replace) {
      parentDialog = dialog;
      parentDialogFocus = currentFocusKey();
    }
    dialog = next;
    await focusDialog();
  }

  async function closeDialog() {
    if (dialog === 'confirm' && confirmation && actionPending(confirmationActionKey(confirmation))) return;
    if (dialog === 'confirm' && parentDialog) {
      const next = parentDialog;
      const focus = parentDialogFocus;
      dialog = next;
      parentDialog = null;
      parentDialogFocus = '';
      confirmation = null;
      await focusDialog();
      const target = [...(dialogElement?.querySelectorAll<HTMLButtonElement>('[data-bp-focus]') ?? [])]
        .find((node) => node.dataset.bpFocus === focus && !node.disabled);
      if (target) focusControl(target);
      return;
    }

    if (dialog === 'install') installRequest++;
    if (dialog === 'metadata' || dialog === 'files') discardPendingTorrent();
    dialog = null;
    confirmation = null;
    parentDialog = null;
    parentDialogFocus = '';
    const focus = pageReturnFocus;
    pageReturnFocus = '';
    await tick();
    const target = [...document.querySelectorAll<HTMLButtonElement>('[data-bp-focus]')]
      .find((node) => node.dataset.bpFocus === focus && !node.disabled && !node.closest('[inert]'));
    if (target) focusControl(target);
  }

  export function handleCommand(command: BigPictureCommand): boolean {
    if (command !== 'back' || !dialog) return false;
    void closeDialog();
    return true;
  }

  async function beginAdd() {
    clearActionPrefix('add:');
    addSession++;
    metadataRequest++;
    const session = addSession;
    downloadDestination = $settings?.downloadsPath ?? '';
    autoInstall = $settings?.autoInstall ?? false;
    elevateAhead = $settings?.elevateAhead ?? false;
    let source: string | null;
    try {
      source = await requestText({ title: $t('bp.transfers.addSourceTitle'), maxLength: 4096 });
    } catch {
      return;
    }
    if (disposed || session !== addSession || !pageIsActive()) return;
    const value = source?.trim();
    if (!value) return;
    torrentSource = value;
    torrentInfo = null;
    pendingMetadataHash = '';
    await showDialog('metadata');
    await loadMetadata(value);
  }

  async function loadMetadata(source: string) {
    torrentSource = source;
    const session = addSession;
    const requestId = ++metadataRequest;
    const currentRequest = () => !disposed && session === addSession && requestId === metadataRequest && pageIsActive();
    const key = addActionKey('metadata');
    const previousFlight = metadataFlight;
    const operation = perform(key, async () => {
      await previousFlight;
      if (!currentRequest() || dialog !== 'metadata') return;
      let info: TorrentInfo;
      try {
        info = await fetchMetadata(source);
      } catch (error) {
        if (!currentRequest()) return;
        throw error;
      }
      if (!currentRequest() || dialog !== 'metadata') {
        discardMetadataSafely(info.infoHash);
        return;
      }
      torrentInfo = info;
      pendingMetadataHash = info.infoHash;
      selectedFiles = info.files.map(() => true);
    });
    metadataFlight = Promise.all([previousFlight, operation]).then(() => undefined);
    if (currentRequest() && dialog === 'metadata') await focusDialog();
    const ok = await operation;
    if (!ok || !currentRequest() || dialog !== 'metadata') {
      if (currentRequest() && dialog === 'metadata') await focusDialog();
      return;
    }
    await showDialog('files', true);
    if (currentRequest()) {
      void appInfo().then((info) => { if (currentRequest()) platformInfo = info; }).catch(() => { platformInfo = null; });
    }
  }

  async function retryMetadata() {
    await loadMetadata(torrentSource);
  }

  async function chooseTorrentFile() {
    const session = addSession;
    await perform(addActionKey('choose-torrent'), async () => {
      let path: string;
      try {
        path = await selectTorrentFile();
      } catch (error) {
        if (disposed || session !== addSession || !pageIsActive()) return;
        throw error;
      }
      if (!path || disposed || session !== addSession || !pageIsActive()) return;
      torrentSource = path;
      await loadMetadata(path);
    });
  }

  async function chooseDownloadFolder() {
    const key = addActionKey('download-folder');
    const session = addSession;
    await perform(key, async () => {
      try {
        const path = await selectFolder($t('modals.addDownloadChooseFolder'));
        if (path && !disposed && session === addSession && pageIsActive()) downloadDestination = path;
      } catch (error) {
        if (disposed || session !== addSession || !pageIsActive()) return;
        throw error;
      }
    });
  }

  async function editDownloadFolder() {
    const value = await requestText({
      title: $t('bp.transfers.destination'),
      initialValue: downloadDestination,
      maxLength: 2048,
    });
    if (value !== null) downloadDestination = value.trim();
  }

  async function startDownload() {
    if (!torrentInfo || selectedFilesCount === 0) return;
    const info = torrentInfo;
    const session = addSession;
    const indices = selectedFiles.map((selected, index) => selected ? index : -1).filter((index) => index >= 0);
    const key = addActionKey('start');
    const ok = await perform(key, async () => {
      startingHashes.add(info.infoHash);
      try {
        await startDownloadFrom(info.infoHash, downloadDestination, indices, {
          autoInstall,
          elevateAhead: autoInstall && canElevateAhead && elevateAhead,
        });
        if (pendingMetadataHash === info.infoHash) pendingMetadataHash = '';
        if (!disposed && pageIsActive() && session === addSession && dialog === 'files') await closeDialog();
      } finally {
        startingHashes.delete(info.infoHash);
      }
    });
    if (!ok && (disposed || !pageIsActive() || session !== addSession || dialog !== 'files')) {
      if (pendingMetadataHash === info.infoHash) pendingMetadataHash = '';
      discardMetadataSafely(info.infoHash);
    }
  }

  function toggleAllFiles() {
    selectedFiles = selectedFiles.map(() => !allFilesSelected);
  }

  async function pickInstallFolder() {
    if (!installDownloadId) return;
    const downloadId = installDownloadId;
    const request = installRequest;
    const key = actionKey('install', downloadId, 'folder');
    await perform(key, async () => {
      const path = await selectFolder($t('modals.installChooseFolder'));
      if (path && isCurrentInstallView(downloadId, request)) installDestination = path;
    });
  }

  async function editInstallFolder() {
    const downloadId = installDownloadId;
    const request = installRequest;
    const value = await requestText({
      title: $t('bp.transfers.installDestination'),
      initialValue: installDestination,
      maxLength: 2048,
    });
    if (value !== null && isCurrentInstallView(downloadId, request)) installDestination = value.trim();
  }

  async function inspectForInstall(downloadId: string) {
    const key = actionKey('install', downloadId, 'inspect');
    const request = ++installRequest;
    const currentRequest = () => !disposed && pageIsActive() && dialog === 'install' && installDownloadId === downloadId && request === installRequest;
    planInfo = null;
    installId = '';
    const existing = $installationsByDownload.get(downloadId);
    if (existing) {
      installId = existing.id;
      chosenExecutable = existing.executable || existing.candidates?.[0]?.path || '';
      if (currentRequest()) await focusDialog();
      return;
    }
    await perform(key, async () => {
      let info: PlanInfo;
      try {
        info = await inspectDownload(downloadId);
      } catch (error) {
        if (!currentRequest()) return;
        throw error;
      }
      if (!currentRequest()) return;
      planInfo = info;
      installDestination = info.plan.destination;
      installMode = 'copy';
      chosenExecutable = '';
      cleanupResolved = false;
    });
    if (currentRequest()) await focusDialog();
  }

  async function openInstall(downloadId: string) {
    clearActionPrefix('install:');
    installRequest++;
    installDownloadId = downloadId;
    installId = '';
    planInfo = null;
    chosenExecutable = '';
    cleanupResolved = false;
    await showDialog('install');
    await inspectForInstall(downloadId);
    if (dialog === 'install' && pageIsActive()) await focusDialog();
  }

  async function startInstallFromPlan() {
    if (!planInfo || !installPlan || !installDownloadId) return;
    const current = planInfo;
    const plan = current.plan;
    const request = installRequest;
    const key = actionKey('install', current.downloadId, 'start');
    await perform(key, async () => {
      const item = await startInstall(current.downloadId, {
        destination: controlledPlan || silentPlan ? installDestination : '',
        mode: portablePlan ? (current.seeding ? 'copy' : installMode) : '',
        type: plan.type,
        installerPath: plan.installerPath,
      });
      upsertInstallation(item);
      if (isCurrentInstallView(current.downloadId, request)) {
        installId = item.id;
        planInfo = null;
      }
    });
  }

  async function openInstallSource() {
    if (!installPlan?.sourcePath) return;
    const key = actionKey('install', installDownloadId, 'open-source');
    await perform(key, () => openGameFolder(installPlan!.sourcePath, installPlan!.installerPath || ''));
  }

  async function chooseInstaller() {
    if (!planInfo) return;
    const current = planInfo;
    const request = installRequest;
    const key = actionKey('install', current.downloadId, 'choose-installer');
    await perform(key, async () => {
      const path = await selectExecutable($t('modals.installChooseInstaller'));
      if (!path || !isCurrentInstallView(current.downloadId, request)) return;
      const item = await startInstall(current.downloadId, { type: 'exe_installer', installerPath: path });
      upsertInstallation(item);
      if (isCurrentInstallView(current.downloadId, request)) {
        installId = item.id;
        planInfo = null;
      }
    });
  }

  async function chooseGameExecutable() {
    if (!planInfo) return;
    const current = planInfo;
    const request = installRequest;
    const isCurrent = () => !disposed && pageIsActive() && dialog === 'install' && request === installRequest && installDownloadId === current.downloadId;
    const key = actionKey('install', current.downloadId, 'add-game');
    await perform(key, async () => {
      const path = await selectExecutable($t('modals.installChooseGameExecutable'));
      if (!path || !isCurrent()) return;
      const game = await addGame(path, current.name);
      if (isCurrent()) ongame(game.id);
    });
  }

  async function chooseManualExecutable() {
    const downloadId = installDownloadId;
    const request = installRequest;
    const key = actionKey('install', downloadId, 'choose-executable');
    await perform(key, async () => {
      const path = await selectExecutable($t('modals.installChooseGameExecutable'));
      if (path && isCurrentInstallView(downloadId, request)) chosenExecutable = path;
    });
  }

  async function enterExecutablePath() {
    const downloadId = installDownloadId;
    const request = installRequest;
    const value = await requestText({
      title: $t('bp.transfers.enterExecutablePath'),
      initialValue: chosenExecutable,
      maxLength: 4096,
    });
    if (value !== null && isCurrentInstallView(downloadId, request) && value.trim()) {
      chosenExecutable = value.trim();
    }
  }

  async function confirmExecutableChoice() {
    if (!selectedInstallation || !chosenExecutable) return;
    const item = selectedInstallation;
    const key = actionKey('install', item.id, 'confirm-executable');
    await perform(key, () => confirmExecutable(item.id, chosenExecutable));
  }

  async function retryInstallation() {
    if (!selectedInstallation) return;
    const item = selectedInstallation;
    const key = actionKey('install', item.id, 'retry');
    await perform(key, () => retryInstall(item.id));
  }

  function askConfirmation(value: Confirmation) {
    confirmation = value;
    void showDialog('confirm');
  }

  async function leaveConfirmation() {
    await closeDialog();
  }

  async function confirmDangerousAction() {
    if (!confirmation) return;
    const current = confirmation;
    const dismissedDownloadId = current.kind === 'dismiss-install'
      ? $installations.find((item) => item.id === current.installId)?.downloadId ?? ''
      : '';
    let key = '';
    let action: () => Promise<unknown>;
    switch (current.kind) {
      case 'cancel-download':
        key = actionKey('download', current.downloadId, 'cancel');
        action = () => cancelDownload(current.downloadId);
        break;
      case 'remove-download':
        key = actionKey('download', current.downloadId, 'remove');
        action = () => removeDownload(current.downloadId);
        break;
      case 'cancel-install':
        key = actionKey('install', current.installId, 'cancel');
        action = () => cancelInstall(current.installId);
        break;
      case 'dismiss-install':
        key = actionKey('install', current.installId, 'dismiss');
        action = () => dismissInstall(current.installId);
        break;
      case 'delete-files':
        key = actionKey('install', current.downloadId, 'delete-files');
        action = () => deleteDownloadData(current.downloadId);
        break;
    }

    if (!await perform(key, action)) return;
    if (disposed || !pageIsActive()) {
      confirmation = null;
      dialog = null;
      parentDialog = null;
      parentDialogFocus = '';
      pageReturnFocus = '';
      return;
    }
    confirmation = null;
    if (current.kind === 'cancel-install' || current.kind === 'dismiss-install' || current.kind === 'delete-files') {
      const next = parentDialog ?? 'install';
      dialog = next;
      parentDialog = null;
      parentDialogFocus = '';
      if (current.kind === 'dismiss-install' && dismissedDownloadId) {
        installId = '';
        planInfo = null;
        await inspectForInstall(dismissedDownloadId);
      } else if (current.kind === 'delete-files') {
        cleanupResolved = true;
      }
      await focusDialog();
      return;
    }
    await closeDialog();
  }

  function confirmationActionKey(value: Confirmation) {
    if (value.kind === 'cancel-download') return actionKey('download', value.downloadId, 'cancel');
    if (value.kind === 'remove-download') return actionKey('download', value.downloadId, 'remove');
    if (value.kind === 'cancel-install') return actionKey('install', value.installId, 'cancel');
    if (value.kind === 'dismiss-install') return actionKey('install', value.installId, 'dismiss');
    return actionKey('install', value.downloadId, 'delete-files');
  }

  async function openDownloadFolder(item: DownloadRecord) {
    const key = actionKey('download', item.id, 'open-folder');
    await perform(key, () => openFolder(item.destination));
  }

  function routeCompletedInstall(installation: Installation) {
    const game = $libraryGames.find((item) => item.id === installation.gameId);
    if (game) ongame(game.id);
    else if (installation.gameId) ongame(installation.gameId);
  }

  function queueAction(item: DownloadRecord, direction: 'up' | 'down') {
    const key = actionKey('download', item.id, `move-${direction}`);
    const action = direction === 'up' ? moveDownloadUp : moveDownloadDown;
    void perform(key, async () => {
      await action(item.id);
      downloads.set(await listDownloads());
    });
  }

  function requestCancelDownload(item: DownloadRecord) {
    askConfirmation({ kind: 'cancel-download', downloadId: item.id });
  }

  function requestRemoveDownload(item: DownloadRecord) {
    askConfirmation({ kind: 'remove-download', downloadId: item.id });
  }

</script>

<div class="bp-page transfers-page" inert={dialog !== null} bind:this={pageRoot}>
  <header class="bp-header transfers-header">
    <div class="heading">
      <span class="eyebrow">{$t('bp.downloads')}</span>
      <h1>{$t('bp.transfers.title')}</h1>
    </div>
    <div class="bp-actions">
      <button class="bp-button" data-bp-focus="transfers:back" onclick={onback}>
        <ArrowLeft size={22} />{$t('bp.transfers.back')}
      </button>
      <button class="bp-button bp-primary" data-bp-focus="transfers:add" data-bp-default onclick={beginAdd}>
        <Download size={22} />{$t('bp.transfers.add')}
      </button>
    </div>
  </header>

  <main class="transfer-sections">
    {#if $active.length === 0 && $queue.length === 0 && $completed.length === 0 && failedDownloads.length === 0 && orphanInstallations.length === 0}
      <section class="bp-empty transfer-empty">
        <Download size={60} strokeWidth={1.5} />
        <h2>{$t('bp.transfers.emptyTitle')}</h2>
        <p>{$t('bp.transfers.emptyText')}</p>
        <button class="bp-button bp-primary" data-bp-focus="transfers:empty-add" onclick={beginAdd}>
          <Download size={22} />{$t('bp.transfers.add')}
        </button>
      </section>
    {/if}

    {#if $active.length > 0}
      <section class="transfer-section" aria-labelledby="transfers-active-heading">
        <h2 id="transfers-active-heading">{$t('bp.transfers.active')} <span class="count">{$active.length}</span></h2>
        <div class="transfer-list">
          {#each $active as item (item.id)}
            {@const pauseKey = actionKey('download', item.id, 'pause')}
            {@const resumeKey = actionKey('download', item.id, 'resume')}
            {@const cancelKey = actionKey('download', item.id, 'cancel')}
            <article class="bp-card transfer-card">
              <div class="card-heading">
                <h3>{item.name}</h3>
                <span class="status">{statusLabels(item.status)}</span>
              </div>
              <ProgressBar value={item.progress * 100} indeterminate={item.status === 'metadata' || item.status === 'verifying'} height={7} />
              <div class="metrics">
                <span>{$t('bp.transfers.downloaded', { done: bytesSize(item.downloaded), total: bytesSize(item.total) })}</span>
                <span>{$t('bp.transfers.network', { down: speedBytes(item.downloadSpeed), up: speedBytes(item.uploadSpeed) })}</span>
                {#if item.status === 'downloading' && item.etaSeconds >= 0}
                  <span>{$t('bp.transfers.eta', { time: etaLabel(item.etaSeconds) })}</span>
                {/if}
              </div>
              {#if item.error}<p class="bp-error" role="alert">{installErrorText(item.error)}</p>{/if}
              {#if actionError(pauseKey)}
                <div class="bp-error" role="alert"><span>{actionError(pauseKey)}</span><button data-bp-focus={`retry:${pauseKey}`} onclick={() => retryAction(pauseKey)}>{$t('bp.transfers.retryAction')}</button></div>
              {/if}
              {#if actionError(resumeKey)}
                <div class="bp-error" role="alert"><span>{actionError(resumeKey)}</span><button data-bp-focus={`retry:${resumeKey}`} onclick={() => retryAction(resumeKey)}>{$t('bp.transfers.retryAction')}</button></div>
              {/if}
              {#if actionError(cancelKey)}
                <div class="bp-error" role="alert"><span>{actionError(cancelKey)}</span><button data-bp-focus={`retry:${cancelKey}`} onclick={() => retryAction(cancelKey)}>{$t('bp.transfers.retryAction')}</button></div>
              {/if}
              <div class="bp-actions card-actions">
                {#if item.status === 'paused'}
                  <button class="bp-button" data-bp-focus={`download:${item.id}:toggle`} disabled={actionPending(resumeKey)} onclick={() => perform(resumeKey, () => resumeDownload(item.id))}>
                    <Play size={20} />{$t('bp.transfers.resume')}
                  </button>
                {:else if item.status === 'downloading'}
                  <button class="bp-button" data-bp-focus={`download:${item.id}:toggle`} disabled={actionPending(pauseKey)} onclick={() => perform(pauseKey, () => pauseDownload(item.id))}>
                    <Pause size={20} />{$t('bp.transfers.pause')}
                  </button>
                {/if}
                <button class="bp-button bp-danger" data-bp-focus={`download:${item.id}:cancel`} disabled={actionPending(cancelKey)} onclick={() => requestCancelDownload(item)}>
                  <X size={20} />{$t('bp.transfers.cancelDownload')}
                </button>
              </div>
            </article>
          {/each}
        </div>
      </section>
    {/if}

    {#if $queue.length > 0}
      <section class="transfer-section" aria-labelledby="transfers-queue-heading">
        <h2 id="transfers-queue-heading">{$t('bp.transfers.queue')} <span class="count">{$queue.length}</span></h2>
        <div class="transfer-list">
          {#each $queue as item, index (item.id)}
            {@const moveUpKey = actionKey('download', item.id, 'move-up')}
            {@const moveDownKey = actionKey('download', item.id, 'move-down')}
            {@const startKey = actionKey('download', item.id, 'start-now')}
            {@const cancelKey = actionKey('download', item.id, 'cancel')}
            <article class="bp-card transfer-card compact">
              <div class="card-heading">
                <h3>{item.name}</h3>
                <span class="status">{statusLabels(item.status)}</span>
              </div>
              <p class="muted">{$t('bp.transfers.downloaded', { done: bytesSize(item.downloaded), total: bytesSize(item.total) })}</p>
              {#if actionError(moveUpKey)}<div class="bp-error" role="alert"><span>{actionError(moveUpKey)}</span><button data-bp-focus={`retry:${moveUpKey}`} onclick={() => retryAction(moveUpKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
              {#if actionError(moveDownKey)}<div class="bp-error" role="alert"><span>{actionError(moveDownKey)}</span><button data-bp-focus={`retry:${moveDownKey}`} onclick={() => retryAction(moveDownKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
              {#if actionError(startKey)}<div class="bp-error" role="alert"><span>{actionError(startKey)}</span><button data-bp-focus={`retry:${startKey}`} onclick={() => retryAction(startKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
              {#if actionError(cancelKey)}<div class="bp-error" role="alert"><span>{actionError(cancelKey)}</span><button data-bp-focus={`retry:${cancelKey}`} onclick={() => retryAction(cancelKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
              <div class="bp-actions card-actions">
                {#if index > 0}
                  <button class="bp-button icon-button" data-bp-focus={`queue:${item.id}:up`} aria-label={$t('bp.transfers.moveUp')} disabled={actionPending(moveUpKey)} onclick={() => queueAction(item, 'up')}><ArrowUp size={21} />{$t('bp.transfers.moveUp')}</button>
                {/if}
                {#if index < $queue.length - 1}
                  <button class="bp-button icon-button" data-bp-focus={`queue:${item.id}:down`} aria-label={$t('bp.transfers.moveDown')} disabled={actionPending(moveDownKey)} onclick={() => queueAction(item, 'down')}><ArrowDown size={21} />{$t('bp.transfers.moveDown')}</button>
                {/if}
                <button class="bp-button bp-primary" data-bp-focus={`queue:${item.id}:start`} disabled={actionPending(startKey)} onclick={() => perform(startKey, () => forceStartDownload(item.id))}>
                  <Play size={20} />{$t('bp.transfers.startNow')}
                </button>
                <button class="bp-button bp-danger" data-bp-focus={`queue:${item.id}:cancel`} disabled={actionPending(cancelKey)} onclick={() => requestCancelDownload(item)}>
                  <X size={20} />{$t('bp.transfers.cancelDownload')}
                </button>
              </div>
            </article>
          {/each}
        </div>
      </section>
    {/if}

    {#if $completed.length > 0}
      <section class="transfer-section" aria-labelledby="transfers-completed-heading">
        <h2 id="transfers-completed-heading">{$t('bp.transfers.completed')} <span class="count">{$completed.length}</span></h2>
        <div class="transfer-list">
          {#each $completed as item (item.id)}
            {@const installation = $installationsByDownload.get(item.id)}
            {@const stage = transferJourneyStage(item.status, installation?.status, installation?.gameId)}
            {@const removeKey = actionKey('download', item.id, 'remove')}
            {@const folderKey = actionKey('download', item.id, 'open-folder')}
            <article class="bp-card transfer-card">
              <div class="card-heading">
                <h3>{item.name}</h3>
                <span class="complete-label"><CircleCheck size={21} />{bytesSize(item.total)}</span>
              </div>
              {#if installation && stage === 'install-progress'}
                <div class="install-inline">
                  <div class="card-heading"><span>{installStatusLabels(installation.status)}</span><span>{installTotalUnknown(installation) ? $t('bp.transfers.installWritten', { size: bytesSize(installation.bytesDone) }) : `${progressPercent(installation.progress)}%`}</span></div>
                  <ProgressBar value={installation.progress * 100} indeterminate={installIndeterminate(installation)} height={7} />
                </div>
              {:else if stage === 'library'}
                <p class="muted">{$t('bp.transfers.installComplete')}</p>
              {:else if stage === 'install-error' && installation}
                <p class="bp-error" role="alert">{installation.error ? installErrorText(installation.error) : $t('bp.transfers.installFailed')}</p>
              {:else if stage === 'choose-executable'}
                <p class="muted">{$t('bp.transfers.chooseExecutable')}</p>
              {:else}
                <p class="muted">{statusLabels(item.status)}</p>
              {/if}
              {#if actionError(folderKey)}<div class="bp-error" role="alert"><span>{actionError(folderKey)}</span><button data-bp-focus={`retry:${folderKey}`} onclick={() => retryAction(folderKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
              {#if actionError(removeKey)}<div class="bp-error" role="alert"><span>{actionError(removeKey)}</span><button data-bp-focus={`retry:${removeKey}`} onclick={() => retryAction(removeKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
              <div class="bp-actions card-actions">
                {#if !installation}
                  <button class="bp-button bp-primary" data-bp-focus={`download:${item.id}:install`} onclick={() => openInstall(item.id)}>
                    <Download size={20} />{$t('bp.transfers.install')}
                  </button>
                {:else if stage === 'choose-executable'}
                  <button class="bp-button bp-primary" data-bp-focus={`download:${item.id}:continue-install`} onclick={() => openInstall(item.id)}>
                    <Gamepad2 size={20} />{$t('bp.transfers.continueInstall')}
                  </button>
                {:else if stage === 'install-progress'}
                  <button class="bp-button" data-bp-focus={`download:${item.id}:install-progress`} onclick={() => openInstall(item.id)}>
                    <RefreshCw size={20} />{$t('bp.transfers.installProgress')}
                  </button>
                {:else if stage === 'install-error'}
                  <button class="bp-button bp-primary" data-bp-focus={`download:${item.id}:retry-install`} onclick={() => openInstall(item.id)}>
                    <RefreshCw size={20} />{$t('bp.transfers.retryInstall')}
                  </button>
                {:else if stage === 'library' && installation?.gameId}
                  <button class="bp-button bp-primary" data-bp-focus={`download:${item.id}:open-library`} onclick={() => routeCompletedInstall(installation)}>
                    <Gamepad2 size={20} />{$t('bp.transfers.openLibrary')}
                  </button>
                {:else if stage === 'installed'}
                  <span class="installed-label"><Check size={20} />{$t('bp.transfers.installed')}</span>
                {/if}
                <button class="bp-button" data-bp-focus={`download:${item.id}:folder`} disabled={actionPending(folderKey)} onclick={() => openDownloadFolder(item)}>
                  <FolderOpen size={20} />{$t('bp.transfers.showFolder')}
                </button>
                <button class="bp-button bp-danger" data-bp-focus={`download:${item.id}:remove`} disabled={actionPending(removeKey)} onclick={() => requestRemoveDownload(item)}>
                  <Trash2 size={20} />{$t('bp.transfers.removeDownload')}
                </button>
              </div>
            </article>
          {/each}
        </div>
      </section>
    {/if}

    {#if failedDownloads.length > 0}
      <section class="transfer-section" aria-labelledby="transfers-failed-heading">
        <h2 id="transfers-failed-heading">{$t('bp.transfers.failed')} <span class="count">{failedDownloads.length}</span></h2>
        <div class="transfer-list">
          {#each failedDownloads as item (item.id)}
            {@const resumeKey = actionKey('download', item.id, 'resume')}
            {@const cancelKey = actionKey('download', item.id, 'cancel')}
            <article class="bp-card transfer-card">
              <div class="card-heading"><h3>{item.name}</h3><span class="status danger-text">{statusLabels(item.status)}</span></div>
              <p class="bp-error" role="alert">{item.error ? installErrorText(item.error) : $t('bp.transfers.actionError')}</p>
              {#if actionError(resumeKey)}<div class="bp-error" role="alert"><span>{actionError(resumeKey)}</span><button data-bp-focus={`retry:${resumeKey}`} onclick={() => retryAction(resumeKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
              {#if actionError(cancelKey)}<div class="bp-error" role="alert"><span>{actionError(cancelKey)}</span><button data-bp-focus={`retry:${cancelKey}`} onclick={() => retryAction(cancelKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
              <div class="bp-actions card-actions">
                <button class="bp-button bp-primary" data-bp-focus={`failed:${item.id}:retry`} disabled={actionPending(resumeKey)} onclick={() => perform(resumeKey, () => resumeDownload(item.id))}>
                  <RefreshCw size={20} />{$t('bp.transfers.retryDownload')}
                </button>
                <button class="bp-button bp-danger" data-bp-focus={`failed:${item.id}:cancel`} disabled={actionPending(cancelKey)} onclick={() => requestCancelDownload(item)}>
                  <X size={20} />{$t('bp.transfers.cancelDownload')}
                </button>
              </div>
            </article>
          {/each}
        </div>
      </section>
    {/if}

    {#if orphanInstallations.length > 0}
      <section class="transfer-section" aria-labelledby="transfers-history-heading">
        <h2 id="transfers-history-heading">{$t('bp.transfers.installHistory')} <span class="count">{orphanInstallations.length}</span></h2>
        <div class="transfer-list">
          {#each orphanInstallations as installation (installation.id)}
            {@const stage = transferJourneyStage(undefined, installation.status, installation.gameId)}
            <article class="bp-card transfer-card compact">
              <div class="card-heading">
                <h3>{installation.name}</h3>
                <span class:complete-label={stage === 'library' || stage === 'installed'} class:status={stage !== 'library' && stage !== 'installed'}>
                  {stage === 'library' || stage === 'installed' ? $t('bp.transfers.installed') : installStatusLabels(installation.status)}
                </span>
              </div>
              {#if stage === 'install-progress'}
                <div class="install-inline">
                  <div class="card-heading"><span>{installStatusLabels(installation.status)}</span><span>{installTotalUnknown(installation) ? $t('bp.transfers.installWritten', { size: bytesSize(installation.bytesDone) }) : `${progressPercent(installation.progress)}%`}</span></div>
                  <ProgressBar value={installation.progress * 100} indeterminate={installIndeterminate(installation)} height={7} />
                </div>
              {:else if stage === 'install-error'}
                <p class="bp-error" role="alert">{installation.error ? installErrorText(installation.error) : $t('bp.transfers.installFailed')}</p>
              {:else if stage === 'choose-executable'}
                <p class="muted">{$t('bp.transfers.chooseExecutable')}</p>
              {/if}
              <div class="bp-actions card-actions">
                <button class="bp-button" data-bp-focus={`install-history:${installation.id}:details`} onclick={() => openInstall(installation.downloadId)}>
                  <RefreshCw size={20} />{$t('bp.transfers.installProgress')}
                </button>
                {#if installation.gameId}
                  <button class="bp-button bp-primary" data-bp-focus={`install-history:${installation.id}:game`} onclick={() => routeCompletedInstall(installation)}>
                    <Gamepad2 size={20} />{$t('bp.transfers.openLibrary')}
                  </button>
                {/if}
              </div>
            </article>
          {/each}
        </div>
      </section>
    {/if}
  </main>
</div>

{#if dialog}
  <div class="dialog-veil">
    <div class="transfer-dialog" role="dialog" aria-modal="true" aria-labelledby="transfer-dialog-title" data-bp-scope bind:this={dialogElement}>
      {#if dialog === 'metadata'}
        {@const metadataKey = addActionKey('metadata')}
        {@const torrentKey = addActionKey('choose-torrent')}
        <h2 id="transfer-dialog-title">{$t('bp.transfers.metadataTitle')}</h2>
        {#if actionPending(metadataKey)}
          <p class="dialog-note">{$t('bp.transfers.metadataLoading')}</p>
          <div class="bp-actions dialog-actions"><button class="bp-button" data-bp-focus="metadata:close" data-bp-default onclick={closeDialog}>{$t('bp.transfers.close')}</button></div>
        {:else if actionError(metadataKey)}
          <p class="bp-error" role="alert">{actionError(metadataKey)}</p>
          <div class="bp-actions dialog-actions">
            <button class="bp-button bp-primary" data-bp-focus="metadata:retry" data-bp-default onclick={retryMetadata}><RefreshCw size={20} />{$t('bp.transfers.retryAction')}</button>
            <button class="bp-button" data-bp-focus="metadata:torrent" disabled={actionPending(torrentKey)} onclick={chooseTorrentFile}><FileUp size={20} />{$t('bp.transfers.chooseTorrent')}</button>
            <button class="bp-button" data-bp-focus="metadata:close" onclick={closeDialog}>{$t('bp.transfers.close')}</button>
          </div>
          {#if actionError(torrentKey)}
            <div class="bp-error" role="alert"><span>{actionError(torrentKey)}</span><button data-bp-focus="metadata:torrent-retry" onclick={() => retryAction(torrentKey)}>{$t('bp.transfers.retryAction')}</button></div>
          {/if}
        {:else}
          <p class="dialog-note">{$t('bp.transfers.emptyText')}</p>
          <div class="bp-actions dialog-actions">
            <button class="bp-button bp-primary" data-bp-focus="metadata:torrent" data-bp-default disabled={actionPending(torrentKey)} onclick={chooseTorrentFile}><FileUp size={20} />{$t('bp.transfers.chooseTorrent')}</button>
            <button class="bp-button" data-bp-focus="metadata:close" onclick={closeDialog}>{$t('bp.transfers.close')}</button>
          </div>
          {#if actionError(torrentKey)}
            <div class="bp-error" role="alert"><span>{actionError(torrentKey)}</span><button data-bp-focus="metadata:torrent-retry" onclick={() => retryAction(torrentKey)}>{$t('bp.transfers.retryAction')}</button></div>
          {/if}
        {/if}
      {:else if dialog === 'files' && torrentInfo}
        {@const folderKey = addActionKey('download-folder')}
        {@const startKey = addActionKey('start')}
        <h2 id="transfer-dialog-title">{$t('bp.transfers.chooseFilesTitle')}</h2>
        <div class="torrent-summary"><strong>{torrentInfo.name}</strong><span>{bytesSize(torrentInfo.totalBytes)}</span></div>
        <div class="destination-row">
          <div><span class="field-label">{$t('bp.transfers.destination')}</span><span class="path-value">{downloadDestination || '—'}</span></div>
          <button class="bp-button" data-bp-focus="files:folder" disabled={actionPending(folderKey)} onclick={chooseDownloadFolder}><FolderOpen size={20} />{$t('bp.transfers.changeFolder')}</button>
          <button class="bp-button" data-bp-focus="files:path" onclick={editDownloadFolder}>{$t('bp.transfers.editPath')}</button>
        </div>
        {#if actionError(folderKey)}<div class="bp-error" role="alert"><span>{actionError(folderKey)}</span><button data-bp-focus="files:folder-retry" onclick={() => retryAction(folderKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
        <div class="file-list">
          <button class="file-choice all-files" role="checkbox" aria-checked={allFilesSelected} data-bp-focus="files:toggle-all" data-bp-default onclick={toggleAllFiles}>
            <span class="check-box" class:on={allFilesSelected}>{#if allFilesSelected}<Check size={18} />{/if}</span><span>{$t('bp.transfers.selectAll')}</span>
          </button>
          {#each torrentInfo.files as file, index (index)}
            <button class="file-choice" role="checkbox" aria-checked={selectedFiles[index]} data-bp-focus={`file:${index}`} onclick={() => (selectedFiles[index] = !selectedFiles[index])}>
              <span class="check-box" class:on={selectedFiles[index]}>{#if selectedFiles[index]}<Check size={18} />{/if}</span>
              <span class="file-path">{file.path}</span><span class="file-size">{bytesSize(file.size)}</span>
            </button>
          {/each}
        </div>
        <p class="muted">{$t('bp.transfers.selectedCount', { count: selectedFilesCount })} · {bytesSize(selectedFilesSize)}</p>
        <div class="option-list">
          <button class="file-choice option-choice" role="checkbox" aria-checked={autoInstall} data-bp-focus="files:auto-install" onclick={() => (autoInstall = !autoInstall)}>
            <span class="check-box" class:on={autoInstall}>{#if autoInstall}<Check size={18} />{/if}</span><span>{$t('bp.transfers.autoInstall')}<small>{$t('bp.transfers.autoInstallHint')}</small></span>
          </button>
          {#if canElevateAhead}
            <button class="file-choice option-choice" role="checkbox" aria-checked={elevateAhead} data-bp-focus="files:elevate-ahead" onclick={() => (elevateAhead = !elevateAhead)}>
              <span class="check-box" class:on={elevateAhead}>{#if elevateAhead}<Check size={18} />{/if}</span><span>{$t('bp.transfers.elevateAhead')}<small>{$t('bp.transfers.elevateAheadHint')}</small></span>
            </button>
          {/if}
        </div>
        {#if actionError(addActionKey('start'))}<div class="bp-error" role="alert"><span>{actionError(addActionKey('start'))}</span><button data-bp-focus="files:start-retry" onclick={startDownload}>{$t('bp.transfers.retryAction')}</button></div>{/if}
        <div class="bp-actions dialog-actions">
          <button class="bp-button" data-bp-focus="files:cancel" onclick={closeDialog}>{$t('bp.transfers.cancel')}</button>
          <button class="bp-button bp-primary" data-bp-focus="files:start" disabled={selectedFilesCount === 0 || actionPending(startKey)} onclick={startDownload}>
            <Download size={20} />{$t('bp.transfers.startDownload')}
          </button>
        </div>
      {:else if dialog === 'install'}
        {@const inspectKey = actionKey('install', installDownloadId, 'inspect')}
        {@const startKey = actionKey('install', installDownloadId, 'start')}
        {@const folderKey = actionKey('install', installDownloadId, 'folder')}
        {@const installStage = selectedStage}
        <h2 id="transfer-dialog-title">{#if installStage === 'install-ready' && planInfo}{$t('bp.transfers.installPlanTitle')}{:else if installStage === 'install-progress'}{$t('bp.transfers.installProgressTitle')}{:else if installStage === 'choose-executable'}{$t('bp.transfers.chooseExecutable')}{:else if installStage === 'library' || installStage === 'installed'}{$t('bp.transfers.installComplete')}{:else if installStage === 'install-error'}{$t('bp.transfers.failed')}{:else}{$t('bp.transfers.installAnalyze')}{/if}</h2>

        {#if actionPending(inspectKey) || (!planInfo && !selectedInstallation && !actionError(inspectKey))}
          <p class="dialog-note">{$t('bp.transfers.installAnalyze')}</p>
          <div class="bp-actions dialog-actions"><button class="bp-button" data-bp-focus="install:close" data-bp-default onclick={closeDialog}>{$t('bp.transfers.close')}</button></div>
        {:else if actionError(inspectKey) && !planInfo && !selectedInstallation}
          <p class="bp-error" role="alert">{actionError(inspectKey)}</p>
          <div class="bp-actions dialog-actions">
            <button class="bp-button bp-primary" data-bp-focus="install:inspect-retry" data-bp-default onclick={() => inspectForInstall(installDownloadId)}><RefreshCw size={20} />{$t('bp.transfers.retryAction')}</button>
            <button class="bp-button" data-bp-focus="install:close" onclick={closeDialog}>{$t('bp.transfers.close')}</button>
          </div>
        {:else if installStage === 'install-ready' && planInfo && installPlan}
          <div class="install-rows">
            <div class="install-row"><span>{$t('bp.transfers.installSource')}</span><strong>{truncateMiddle(installPlan.sourcePath, 62)}</strong></div>
            <div class="install-row"><span>{$t('bp.transfers.installType')}</span><strong>{installTypeLabels(installPlan.type)}</strong></div>
            {#if planInfo.requiredBytes > 0}<div class="install-row"><span>{$t('bp.transfers.requiredSpace')}</span><strong class:danger-text={notEnoughSpace}>{bytesSize(planInfo.requiredBytes)}</strong></div>{/if}
            {#if planInfo.freeBytes > 0}<div class="install-row"><span>{$t('bp.transfers.freeSpace')}</span><strong class:danger-text={notEnoughSpace}>{bytesSize(planInfo.freeBytes)}</strong></div>{/if}
          </div>
          {#if notEnoughSpace}<p class="bp-error" role="alert">{$t('bp.transfers.notEnoughSpace')}</p>{/if}
          {#if controlledPlan || silentPlan}
            <div class="install-field">
              <span class="field-label">{$t('bp.transfers.installDestination')}</span>
              <p class="path-value">{installDestination || '—'}</p>
              <div class="bp-actions">
                <button class="bp-button" data-bp-focus="install:edit-path" onclick={editInstallFolder}>{$t('bp.transfers.editPath')}</button>
                <button class="bp-button" data-bp-focus="install:folder" disabled={actionPending(folderKey)} onclick={pickInstallFolder}><FolderOpen size={20} />{$t('bp.transfers.changeFolder')}</button>
              </div>
              {#if actionError(folderKey)}<div class="bp-error" role="alert"><span>{actionError(folderKey)}</span><button data-bp-focus="install:folder-retry" onclick={() => retryAction(folderKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
            </div>
          {/if}
          {#if portablePlan}
            {#if planInfo.seeding}
              <p class="dialog-note">{$t('bp.transfers.seedingCopy')}</p>
            {:else}
              <div class="install-field"><span class="field-label">{$t('bp.transfers.installMode')}</span>
                <div class="bp-actions mode-actions">
                  <button class="bp-button" data-bp-focus="install:copy" aria-pressed={installMode === 'copy'} onclick={() => (installMode = 'copy')}>{$t('bp.transfers.copy')}</button>
                  <button class="bp-button" data-bp-focus="install:move" aria-pressed={installMode === 'move'} onclick={() => (installMode = 'move')}>{$t('bp.transfers.move')}</button>
                </div>
              </div>
            {/if}
          {/if}
          {#if externalPlan}
            <p class="dialog-note">{$t(silentPlan ? 'bp.transfers.silentNote' : 'bp.transfers.externalNote')}</p>
          {/if}
          {#if installPlan.type === 'unknown'}
            <p class="dialog-note">{$t('bp.transfers.unknownType')}</p>
            <div class="bp-actions dialog-actions vertical-actions">
              <button class="bp-button" data-bp-focus="install:source-folder" onclick={openInstallSource}><FolderOpen size={20} />{$t('bp.transfers.openSourceFolder')}</button>
              <button class="bp-button" data-bp-focus="install:pick-installer" onclick={chooseInstaller}><FileUp size={20} />{$t('bp.transfers.pickInstaller')}</button>
              <button class="bp-button" data-bp-focus="install:add-executable" data-bp-default onclick={chooseGameExecutable}><Gamepad2 size={20} />{$t('bp.transfers.pickGameExecutable')}</button>
            </div>
            {#each ['open-source', 'choose-installer', 'add-game'] as action}
              {@const errorKey = actionKey('install', installDownloadId, action)}
              {#if actionError(errorKey)}<div class="bp-error" role="alert"><span>{actionError(errorKey)}</span><button data-bp-focus={`retry:${errorKey}`} onclick={() => retryAction(errorKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
            {/each}
          {/if}
          <div class="bp-actions dialog-actions">
            <button class="bp-button" data-bp-focus="install:close-plan" onclick={closeDialog}>{$t('bp.transfers.close')}</button>
            {#if installPlan.type !== 'unknown'}
              <button class="bp-button bp-primary" data-bp-focus="install:start" data-bp-default disabled={actionPending(startKey) || notEnoughSpace || ((controlledPlan || silentPlan) && installDestination.trim() === '')} onclick={startInstallFromPlan}>
                <Play size={20} />{$t(portablePlan && installMode === 'move' ? 'bp.transfers.moveAndInstall' : portablePlan ? 'bp.transfers.copyAndInstall' : installPlan.type.startsWith('archive_') ? 'bp.transfers.extractAndInstall' : silentPlan ? 'bp.transfers.installAction' : 'bp.transfers.runInstaller')}
              </button>
            {/if}
          </div>
          {#if actionError(startKey)}<div class="bp-error" role="alert"><span>{actionError(startKey)}</span><button data-bp-focus="install:start-retry" onclick={() => retryAction(startKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
        {:else if installStage === 'install-progress' && selectedInstallation}
          {#if externalWait}
            <p class="dialog-note">{$t('bp.transfers.waitingExternal')}</p>
          {:else}
            <div class="install-progress">
              <div class="card-heading"><strong>{installStatusLabels(selectedInstallation.status)}</strong>{#if !installTotalUnknown(selectedInstallation)}<span>{progressPercent(selectedInstallation.progress)}%</span>{/if}</div>
              <ProgressBar value={selectedInstallation.progress * 100} indeterminate={installIndeterminate(selectedInstallation)} height={8} />
              {#if installTotalUnknown(selectedInstallation)}<p class="muted">{$t('bp.transfers.installWritten', { size: bytesSize(selectedInstallation.bytesDone) })}</p>{:else if selectedInstallation.status !== 'verifying'}<p class="muted">{$t('bp.transfers.installBytes', { done: bytesSize(selectedInstallation.bytesDone), total: bytesSize(selectedInstallation.bytesTotal) })}</p>{/if}
              {#if selectedInstallation.currentFile}<p class="path-value">{$t('bp.transfers.currentFile', { file: truncateMiddle(selectedInstallation.currentFile, 70) })}</p>{/if}
            </div>
          {/if}
          {#if actionError(actionKey('install', selectedInstallation.id, 'cancel'))}<p class="bp-error" role="alert">{actionError(actionKey('install', selectedInstallation.id, 'cancel'))}</p>{/if}
          <div class="bp-actions dialog-actions">
            <button class="bp-button" data-bp-focus="install:minimize" data-bp-default onclick={closeDialog}>{$t('bp.transfers.close')}</button>
            <button class="bp-button bp-danger" data-bp-focus="install:cancel" disabled={externalWait || actionPending(actionKey('install', selectedInstallation.id, 'cancel'))} onclick={() => askConfirmation({ kind: 'cancel-install', installId: selectedInstallation.id })}>
              <X size={20} />{$t('bp.transfers.cancelInstall')}
            </button>
          </div>
        {:else if installStage === 'choose-executable' && selectedInstallation}
          <p class="dialog-note">{$t('bp.transfers.chooseExecutable')}</p>
          <div class="candidate-list" role="radiogroup" aria-label={$t('bp.transfers.chooseExecutable')}>
            {#each candidatePaths as path, index (path)}
              <button class="candidate" role="radio" aria-checked={chosenExecutable === path} data-bp-focus={`executable:${index}`} onclick={() => (chosenExecutable = path)}>
                <span class="radio-dot" class:on={chosenExecutable === path}></span><span>{path}</span>
              </button>
            {/each}
          </div>
          {@const chooseKey = actionKey('install', selectedInstallation.id, 'choose-executable')}
          {@const confirmKey = actionKey('install', selectedInstallation.id, 'confirm-executable')}
          {#if actionError(chooseKey)}<div class="bp-error" role="alert"><span>{actionError(chooseKey)}</span><button data-bp-focus="executable:manual-retry" onclick={() => retryAction(chooseKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
          {#if actionError(confirmKey)}<div class="bp-error" role="alert"><span>{actionError(confirmKey)}</span><button data-bp-focus="executable:confirm-retry" onclick={() => retryAction(confirmKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
          <div class="bp-actions dialog-actions">
            <button class="bp-button" data-bp-focus="executable:manual" disabled={actionPending(chooseKey)} onclick={chooseManualExecutable}><FileUp size={20} />{$t('bp.transfers.pickManually')}</button>
            <button class="bp-button" data-bp-focus="executable:path" onclick={enterExecutablePath}>{$t('bp.transfers.enterExecutablePath')}</button>
            <button class="bp-button" data-bp-focus="executable:close" onclick={closeDialog}>{$t('bp.transfers.close')}</button>
            <button class="bp-button bp-primary" data-bp-focus="executable:confirm" data-bp-default disabled={!chosenExecutable || actionPending(confirmKey)} onclick={confirmExecutableChoice}>{$t('bp.transfers.confirmExecutable')}</button>
          </div>
        {:else if (installStage === 'library' || installStage === 'installed') && selectedInstallation}
          <div class="installed-summary">
            <span class="success-icon"><CircleCheck size={32} /></span>
            <p>{selectedInstallation.name}</p>
            <div class="install-rows">
              <div class="install-row"><span>{$t('bp.transfers.gameFolder')}</span><strong>{truncateMiddle(selectedInstallation.destination, 62)}</strong></div>
              <div class="install-row"><span>{$t('bp.transfers.gameVersion')}</span><strong>{selectedInstallation.detectedVersion || $t('bp.transfers.unknownVersion')}</strong></div>
            </div>
          </div>
          {#if askCleanup && selectedDownload && !cleanupResolved}
            <div class="cleanup-field">
              <strong>{$t('bp.transfers.cleanupTitle')}</strong><p class="dialog-note">{$t('bp.transfers.cleanupPrompt')}</p>
              {#if seedingNow}<p class="bp-error" role="alert">{$t('bp.transfers.seedingWarning')}</p>{/if}
              <div class="bp-actions">
                <button class="bp-button bp-danger" data-bp-focus="install:delete-files" onclick={() => askConfirmation({ kind: 'delete-files', downloadId: selectedInstallation.downloadId })}><Trash2 size={20} />{$t('bp.transfers.deleteFiles')}</button>
                <button class="bp-button" data-bp-focus="install:keep-files" data-bp-default onclick={() => (cleanupResolved = true)}>{$t('bp.transfers.keepFiles')}</button>
              </div>
            </div>
          {/if}
          {#if actionError(actionKey('install', selectedInstallation.downloadId, 'delete-files'))}<p class="bp-error" role="alert">{actionError(actionKey('install', selectedInstallation.downloadId, 'delete-files'))}</p>{/if}
          <div class="bp-actions dialog-actions">
            {#if selectedInstallation.gameId}
              <button class="bp-button bp-primary" data-bp-focus="install:open-library" data-bp-default={askCleanup && !cleanupResolved ? undefined : ''} onclick={() => routeCompletedInstall(selectedInstallation)}><Gamepad2 size={20} />{$t('bp.transfers.openLibrary')}</button>
            {/if}
            <button class="bp-button" data-bp-focus="install:close-done" data-bp-default={selectedInstallation.gameId ? undefined : ''} onclick={closeDialog}>{$t('common.done')}</button>
          </div>
        {:else if installStage === 'install-error' && selectedInstallation}
          <div class="install-error-summary">
            <strong>{selectedInstallation.name}</strong>
            <p class="bp-error" role="alert">{selectedInstallation.error ? installErrorText(selectedInstallation.error) : $t('bp.transfers.installFailed')}</p>
          </div>
          {@const retryKey = actionKey('install', selectedInstallation.id, 'retry')}
          {@const dismissKey = actionKey('install', selectedInstallation.id, 'dismiss')}
          {#if actionError(retryKey)}<div class="bp-error" role="alert"><span>{actionError(retryKey)}</span><button data-bp-focus="install:retry-action" onclick={() => retryAction(retryKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
          {#if actionError(dismissKey)}<div class="bp-error" role="alert"><span>{actionError(dismissKey)}</span><button data-bp-focus="install:dismiss-action" onclick={() => retryAction(dismissKey)}>{$t('bp.transfers.retryAction')}</button></div>{/if}
          <div class="bp-actions dialog-actions">
            <button class="bp-button" data-bp-focus="install:error-close" onclick={closeDialog}>{$t('bp.transfers.close')}</button>
            <button class="bp-button" data-bp-focus="install:dismiss" disabled={actionPending(dismissKey)} onclick={() => askConfirmation({ kind: 'dismiss-install', installId: selectedInstallation.id })}>{$t('bp.transfers.dismissInstall')}</button>
            <button class="bp-button bp-primary" data-bp-focus="install:retry" data-bp-default disabled={actionPending(retryKey)} onclick={retryInstallation}><RefreshCw size={20} />{$t('bp.transfers.retryInstall')}</button>
          </div>
        {/if}
      {:else if dialog === 'confirm' && confirmation}
        {@const confirmKey = confirmation.kind === 'cancel-download'
          ? actionKey('download', confirmation.downloadId, 'cancel')
          : confirmation.kind === 'remove-download'
            ? actionKey('download', confirmation.downloadId, 'remove')
            : confirmation.kind === 'cancel-install'
              ? actionKey('install', confirmation.installId, 'cancel')
              : confirmation.kind === 'dismiss-install'
                ? actionKey('install', confirmation.installId, 'dismiss')
                : actionKey('install', confirmation.downloadId, 'delete-files')}
        <h2 id="transfer-dialog-title">{confirmation.kind === 'cancel-download' ? $t('bp.transfers.cancelDownloadTitle') : confirmation.kind === 'remove-download' ? $t('bp.transfers.removeDownloadTitle') : confirmation.kind === 'cancel-install' ? $t('bp.transfers.cancelInstallTitle') : confirmation.kind === 'dismiss-install' ? $t('bp.transfers.dismissInstallTitle') : $t('bp.transfers.deleteFilesTitle')}</h2>
        <p class="dialog-note">{confirmation.kind === 'cancel-download' ? $t('bp.transfers.cancelDownloadText') : confirmation.kind === 'remove-download' ? $t('bp.transfers.removeDownloadText') : confirmation.kind === 'cancel-install' ? $t('bp.transfers.cancelInstallText') : confirmation.kind === 'dismiss-install' ? $t('bp.transfers.dismissInstallText') : $t('bp.transfers.deleteFilesText')}</p>
        {#if actionError(confirmKey)}<p class="bp-error" role="alert">{actionError(confirmKey)}</p><button class="bp-button" data-bp-focus="confirm:retry" onclick={confirmDangerousAction}>{$t('bp.transfers.retryAction')}</button>{/if}
        <div class="bp-actions dialog-actions">
          <button class="bp-button" data-bp-focus="confirm:cancel" data-bp-default disabled={actionPending(confirmKey)} onclick={leaveConfirmation}>{$t('bp.transfers.cancel')}</button>
          <button class="bp-button bp-danger" data-bp-focus="confirm:accept" disabled={actionPending(confirmKey)} onclick={confirmDangerousAction}>{$t('bp.transfers.confirm')}</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .transfers-page { display: flex; flex-direction: column; gap: 1.5rem; min-height: 0; }
  .transfers-header { display: flex; align-items: flex-end; justify-content: space-between; gap: 1.5rem; }
  .heading { display: flex; flex-direction: column; gap: .3rem; min-width: 0; }
  .eyebrow { color: var(--text-3, #a5afc0); font-size: .72em; letter-spacing: .1em; text-transform: uppercase; }
  h1 { margin: 0; font-size: clamp(1.7rem, 3vw, 2.8rem); line-height: 1.1; }
  .transfer-sections { overflow: auto; min-height: 0; display: flex; flex-direction: column; gap: 1.7rem; padding: .2rem .25rem 1.8rem; }
  .transfer-section { display: flex; flex-direction: column; gap: .75rem; }
  .transfer-section h2 { display: flex; align-items: baseline; gap: .6rem; margin: 0; font-size: 1.1em; }
  .count, .muted, .status { color: var(--text-3, #a5afc0); font-size: .83em; }
  .transfer-list { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, max(36rem, 18em)), 1fr)); gap: 1rem; }
  .transfer-card { display: flex; flex-direction: column; gap: .8rem; padding: 1rem 1.1rem; min-width: 0; }
  .transfer-card.compact { gap: .55rem; }
  .card-heading { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: .4rem .8rem; min-width: 0; }
  .card-heading h3 { flex: 1 1 12em; min-width: 0; margin: 0; font-size: 1em; line-height: 1.35; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; overflow: hidden; overflow-wrap: anywhere; }
  .card-heading .status, .complete-label { max-width: 100%; overflow-wrap: anywhere; }
  .metrics { display: flex; flex-wrap: wrap; gap: .35rem 1rem; color: var(--text-2, #d3dae6); font-size: .78em; font-variant-numeric: tabular-nums; }
  .card-actions { flex-wrap: wrap; justify-content: flex-start; gap: .55rem; margin-top: .1rem; }
  .bp-button { min-height: 2.65rem; display: inline-flex; align-items: center; justify-content: center; gap: .5rem; padding: .45rem .85rem; border: 1px solid var(--border-strong, #43506a); border-radius: .65rem; background: var(--surface-3, #263147); color: var(--text, #f4f6fb); font: inherit; font-size: .85em; line-height: 1.2; text-align: left; }
  .bp-button:hover:not(:disabled) { background: var(--surface-4, #33415b); }
  .bp-button:disabled { opacity: .5; cursor: wait; }
  .bp-primary { background: var(--accent, #5866dc); color: var(--accent-on, #fff); border-color: transparent; }
  .bp-primary:hover:not(:disabled) { background: var(--accent-hover, #6977e8); }
  .bp-danger { color: var(--danger, #ed8888); border-color: color-mix(in srgb, var(--danger, #ed8888) 45%, transparent); }
  .bp-error { display: flex; align-items: center; justify-content: space-between; gap: .8rem; color: var(--danger, #f09393); font-size: .84em; line-height: 1.4; overflow-wrap: anywhere; }
  .bp-error button { flex: none; color: inherit; text-decoration: underline; font: inherit; }
  .danger-text { color: var(--danger, #f09393) !important; }
  .complete-label, .installed-label { display: inline-flex; align-items: center; gap: .4rem; color: var(--success, #8ed9a9); font-size: .84em; }
  .install-inline { display: flex; flex-direction: column; gap: .45rem; }
  .transfer-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; gap: .75rem; padding: 4rem 1rem; text-align: center; }
  .transfer-empty h2, .transfer-empty p { margin: 0; }
  .transfer-empty p { max-width: 34rem; color: var(--text-3, #a5afc0); }

  .dialog-veil { position: fixed; inset: 0; z-index: 150; display: flex; align-items: center; justify-content: center; padding: 2rem; background: rgba(4, 7, 12, .78); }
  .transfer-dialog { width: min(64rem, 100%); max-height: min(90vh, 62rem); overflow: auto; display: flex; flex-direction: column; gap: 1rem; padding: 1.5rem; border: 1px solid var(--border-strong, #43506a); border-radius: 1.1rem; background: var(--surface-2, #172132); color: var(--text, #f4f6fb); box-shadow: 0 1.5rem 5rem rgba(0, 0, 0, .45); }
  .transfer-dialog h2 { margin: 0; font-size: 1.25em; }
  .dialog-note { margin: 0; color: var(--text-2, #d3dae6); font-size: .9em; line-height: 1.5; }
  .dialog-actions { justify-content: flex-end; flex-wrap: wrap; margin-top: .25rem; }
  .vertical-actions { flex-direction: column; align-items: stretch; }
  .vertical-actions .bp-button { justify-content: flex-start; }
  .torrent-summary { display: flex; justify-content: space-between; align-items: baseline; gap: 1rem; font-size: .95em; }
  .torrent-summary strong { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; }
  .torrent-summary span { flex: none; color: var(--text-3, #a5afc0); }
  .destination-row { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: end; gap: .6rem; }
  .destination-row > div { display: flex; flex-direction: column; gap: .25rem; min-width: 0; }
  .field-label { color: var(--text-3, #a5afc0); font-size: .76em; }
  .path-value { display: block; margin: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-2, #d3dae6); font-size: .82em; }
  .file-list { max-height: 25vh; overflow: auto; display: flex; flex-direction: column; gap: .2rem; padding: .45rem; background: var(--surface, #101927); border-radius: .75rem; }
  .file-choice { width: 100%; display: flex; align-items: center; gap: .65rem; padding: .65rem .6rem; border-radius: .45rem; color: var(--text, #f4f6fb); text-align: left; font: inherit; font-size: .82em; }
  .file-choice:hover, .candidate:hover { background: var(--hover, rgba(255, 255, 255, .09)); }
  .file-choice[aria-checked='true'] { background: var(--hover-strong, rgba(255, 255, 255, .13)); }
  .check-box { width: 1.2rem; height: 1.2rem; flex: none; display: inline-flex; align-items: center; justify-content: center; border: 1px solid var(--border-strong, #596780); border-radius: .3rem; color: white; }
  .check-box.on { background: var(--accent, #5866dc); border-color: var(--accent, #5866dc); }
  .all-files { border-bottom: 1px solid var(--border, #344157); }
  .file-path { min-width: 0; flex: 1; overflow-wrap: anywhere; }
  .file-size { flex: none; color: var(--text-3, #a5afc0); }
  .option-list { display: flex; flex-direction: column; gap: .2rem; }
  .option-choice { align-items: flex-start; }
  .option-choice > span:last-child { display: flex; flex-direction: column; gap: .2rem; }
  .option-choice small { color: var(--text-3, #a5afc0); font-size: .9em; }

  .install-rows { display: flex; flex-direction: column; gap: .3rem; }
  .install-row { display: flex; align-items: baseline; justify-content: space-between; gap: 1rem; padding: .55rem 0; border-bottom: 1px solid var(--border, #344157); font-size: .84em; }
  .install-row span { flex: none; color: var(--text-3, #a5afc0); }
  .install-row strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 500; text-align: right; }
  .install-field, .cleanup-field { display: flex; flex-direction: column; gap: .6rem; }
  .mode-actions .bp-button[aria-pressed='true'] { border-color: var(--accent, #5866dc); background: color-mix(in srgb, var(--accent, #5866dc) 28%, var(--surface-3, #263147)); }
  .install-progress { display: flex; flex-direction: column; gap: .7rem; }
  .candidate-list { max-height: 35vh; overflow: auto; display: flex; flex-direction: column; gap: .25rem; padding: .4rem; border-radius: .7rem; background: var(--surface, #101927); }
  .candidate { display: flex; align-items: center; gap: .7rem; padding: .7rem; border-radius: .4rem; color: var(--text, #f4f6fb); text-align: left; font: inherit; overflow-wrap: anywhere; }
  .candidate[aria-checked='true'] { background: var(--hover-strong, rgba(255, 255, 255, .13)); }
  .radio-dot { width: 1.1rem; height: 1.1rem; flex: none; border: 1px solid var(--border-strong, #596780); border-radius: 50%; }
  .radio-dot.on { background: var(--accent, #5866dc); box-shadow: inset 0 0 0 3px var(--surface-2, #172132); }
  .installed-summary { display: flex; flex-direction: column; gap: .8rem; }
  .installed-summary > p { margin: 0; font-size: 1.05em; font-weight: 600; }
  .success-icon { color: var(--success, #8ed9a9); }
  .install-error-summary { display: flex; flex-direction: column; gap: .7rem; }
  .install-error-summary > strong { font-size: 1em; }

  @media (max-width: 780px) {
    .transfers-header { align-items: flex-start; flex-direction: column; }
    .destination-row { grid-template-columns: 1fr 1fr; }
    .destination-row > div { grid-column: 1 / -1; }
  }
</style>
