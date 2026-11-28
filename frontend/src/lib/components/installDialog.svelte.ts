import { onDestroy, untrack } from 'svelte';
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
  type Installation,
  type PlanInfo,
  type SpaceInfo,
} from '../services/install';
import { installErrorText } from '../install/installErrors';
import { createSpacePlanner } from '../install/spacePlanner';
import { adviseDestination, type DestinationHint, type DestinationOrigin } from '../install/destinationAdvice';
import { libraryFolders, openGameFolder, selectFolder, type LibraryFolder, type Settings } from '../services/settings';
import { destinationInRoot, rootOfDestination } from '../library/folders';
import { downloadsById } from '../stores/downloads';
import type { Download } from '../services/downloads';
import { installActive, installationsByDownload, upsertInstallation } from '../stores/install';
import { toast } from '../stores/toasts';
import { focusHandOff } from '../utils/focusHandOff';
import { bytesSize } from '../utils/format';
import { msg } from '../i18n';

export type InstallDialog = ReturnType<typeof createInstallDialog>;

/** What the dialog follows, read through getters; `open` is written back when the dialog closes itself. */
export interface InstallDialogInputs {
  open: boolean;
  readonly downloadId: string | null;
  readonly installations: Installation[];
  readonly downloadsById: Map<string, Download>;
  readonly settings: Settings | null;
  /** The id of the dialog's failure notice: a refusal said there takes the focus. */
  readonly failureId: string;
}

/**
 * InstallModal's state and actions: the plan, the space it needs, the executable's choice, the
 * install it starts and the actions on it. Called once while the dialog initialises: its effects and
 * its `onDestroy` are the dialog's.
 */
export function createInstallDialog(input: InstallDialogInputs) {
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
      // While an action runs the destination is what was sent: a busy folder is suggested, not taken.
      const advice = adviseDestination(result, busy ? 'typed' : origin, destination);
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

  const installation = $derived(installId ? input.installations.find((i) => i.id === installId) : undefined);
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
    if (readFailure) return 'unreadable';
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

  const requirementsGameId = $derived(input.downloadId ? (input.downloadsById.get(input.downloadId)?.origin?.gameId ?? '') : '');

  const askCleanup = $derived(input.settings?.installCleanupPolicy === 'ask');
  const seedingNow = $derived(!!installation && (input.downloadsById.get(installation.downloadId)?.seeding ?? false));

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

  // What the last action was refused with, and why the package could not be read: said in the dialog.
  let failure = $state('');
  let readFailure = $state('');
  let readRetrying = $state(false);
  let alive = true;
  onDestroy(() => {
    alive = false;
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
    failure = '';
    readFailure = '';
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

  // While an action runs, nothing that changes what it sent is taken: the plan shows what was sent.
  function setMode(next: string) {
    if (busy || next === mode) return;
    mode = next;
    replan(true);
  }

  function pickRoot(id: string) {
    if (busy) return;
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

  async function load(id: string): Promise<boolean | undefined> {
    const existing = get(installationsByDownload).get(id);
    if (existing) {
      installId = existing.id;
      return true;
    }
    const queued = get(downloadsById).get(id);
    if (queued?.origin?.platform) {
      imageDownload = queued;
      return true;
    }
    const current = ++token;
    loading = true;
    try {
      const result = await inspectDownload(id);
      if (!input.open || current !== token) return undefined;
      readFailure = '';
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
      return true;
    } catch (err) {
      if (!input.open || current !== token) return undefined;
      readFailure = installErrorText(err, msg('modals.installReadFailed'));
      return false;
    } finally {
      if (current === token) loading = false;
    }
  }

  // One per opening and download. `token` also moves with every read inside an opening. Before the
  // dialog draws: an install already known for the download is drawn at once, and the layer gives the
  // focus to its step, not to a loading view that is about to go.
  let opening = 0;

  $effect.pre(() => {
    const isOpen = input.open;
    const id = input.downloadId;
    untrack(() => {
      opening += 1;
      reset();
      if (isOpen && id) void load(id);
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
    if (busy) return;
    const at = opening;
    try {
      const path = await selectFolder(msg('modals.installChooseFolder'));
      if (path && alive && at === opening && !busy) {
        destination = path;
        replan(true, 'pick');
      }
    } catch (err) {
      console.warn('install dialog: folder picker', err);
      if (alive && at === opening) failure = msg('modals.installFolderDialogFailed');
    }
  }

  function useSuggested() {
    const current = hint;
    if (!current || busy) return;
    destination = current.path;
    replan(true);
  }

  function setDestination(value: string) {
    if (busy) return;
    destination = value;
    replan(false);
  }

  /** The package the dialog is about: what a toast names when an answer lands after it moved on. */
  const subject = () => installation?.name || info?.name || imageTitle;

  // One action at a time. The pressed control stays focused and busy; a refusal is said in the dialog
  // and takes the focus. `heard` tells the action whether the dialog is still the one it was pressed in.
  async function run(
    action: (heard: () => boolean) => Promise<void>,
    by = '',
    errorText: (err: unknown) => string = installErrorText,
  ) {
    if (busy) return;
    const at = opening;
    const heard = () => alive && at === opening;
    const title = subject();
    const pressed = document.activeElement;
    const hand = focusHandOff(() => pressed);
    busy = true;
    actor = by;
    failure = '';
    try {
      await action(heard);
    } catch (err) {
      if (!heard()) {
        toast(msg('modals.installRefusedFor', { title, reason: errorText(err) }), 'danger');
        return;
      }
      failure = errorText(err);
    }
    if (!heard()) return;
    busy = false;
    actor = '';
    if (failure) await hand(() => document.getElementById(input.failureId));
  }

  const own = (by: string) => busy && actor === by;
  const other = (by: string) => busy && actor !== by;
  const heldFor = (by: string) => (other(by) ? msg('modals.actionRunning') : '');

  // An install that starts after the dialog moved on is that package's: the store learns it, the dialog
  // does not show it, a toast names it.
  function started(item: Installation, heard: () => boolean, title: string) {
    upsertInstallation(item);
    if (heard()) installId = item.id;
    else toast(msg('modals.installStartedFor', { title }));
  }

  function start() {
    const current = info;
    if (!current) return;
    return run(async (heard) => {
      const item = await startInstall(current.downloadId, {
        destination: controlled || silent ? destination : '',
        mode: requestMode,
        type: current.plan.type,
        installerPath: current.plan.installerPath,
      });
      started(item, heard, current.name);
    }, 'start');
  }

  function startImage() {
    const current = imageDownload;
    if (!current?.origin?.platform) return;
    return run(async (heard) => {
      const item = await startInstall(current.id, {
        platform: current.origin.platform,
        title: current.origin.title || current.name,
        cover: current.origin.cover ?? '',
      });
      started(item, heard, current.origin.title || current.name);
    }, 'image');
  }

  function playInstalled() {
    const item = installation;
    if (!item?.gameId) return;
    // A launch the player gave up (a question left unanswered) keeps the dialog where it was.
    return run(
      async (heard) => {
        if ((await launchGame(item.gameId)) && heard()) input.open = false;
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
    }, 'source');
  }

  function pickInstaller() {
    const current = info;
    if (!current) return;
    return run(async (heard) => {
      const path = await selectExecutable(msg('modals.installChooseInstaller'));
      if (!path || !heard()) return;
      const item = await startInstall(current.downloadId, {
        type: 'exe_installer',
        installerPath: path,
      });
      started(item, heard, current.name);
    }, 'installer');
  }

  function pickGameExecutable() {
    const current = info;
    if (!current) return;
    return run(async (heard) => {
      const path = await selectExecutable(msg('modals.installChooseGameExecutable'));
      if (!path || !heard()) return;
      await addGame(path, current.name);
      toast(msg('modals.installGameAddedFor', { title: current.name }), 'success');
      if (heard()) input.open = false;
    }, 'executable');
  }

  function pickManual() {
    return run(async (heard) => {
      const path = await selectExecutable(msg('modals.installChooseGameExecutable'));
      if (path && heard()) chosen = path;
    }, 'manual');
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
    return run(async (heard) => {
      await dismissInstall(item.id);
      if (!heard()) return;
      installId = null;
      if (input.downloadId) await load(input.downloadId);
    }, 'dismiss');
  }

  function deleteData() {
    const item = installation;
    if (!item) return;
    return run(async (heard) => {
      await deleteDownloadData(item.downloadId);
      if (heard()) cleanupResolved = true;
      toast(msg('modals.installDownloadDataDeletedFor', { title: item.name }), 'success');
    }, 'delete');
  }

  // Why the primary action of the plan cannot be pressed now.
  const startReason = $derived.by(() => {
    if (other('start')) return msg('modals.actionRunning');
    if (notEnoughSpace) return msg('modals.installNotEnoughSpace');
    if (!spacePending && spaceError) return spaceError;
    if ((controlled || silent) && destination.trim() === '') return msg('errInstall.installEmptyDestination');
    return '';
  });

  return {
    get modeOptions() { return modeOptions; },
    get installation() { return installation; },
    get imageDownload() { return imageDownload; },
    get imageTitle() { return imageTitle; },
    get imagePlatform() { return imagePlatform; },
    get info() { return info; },
    get plan() { return plan; },
    get space() { return space; },
    get spacePending() { return spacePending; },
    get spaceError() { return spaceError; },
    get notEnoughSpace() { return notEnoughSpace; },
    get requirementsGameId() { return requirementsGameId; },
    get destination() { return destination; },
    get hint() { return hint; },
    get roots() { return roots; },
    get rootOptions() { return rootOptions; },
    get rootChoice() { return rootChoice; },
    get controlled() { return controlled; },
    get silent() { return silent; },
    get portable() { return portable; },
    get external() { return external; },
    get mode() { return mode; },
    get movesInPlace() { return movesInPlace; },
    get busy() { return busy; },
    get actor() { return actor; },
    get phase() { return phase; },
    get externalWait() { return externalWait; },
    get candidatePaths() { return candidatePaths; },
    get askCleanup() { return askCleanup; },
    get seedingNow() { return seedingNow; },
    get primaryLabel() { return primaryLabel; },
    get failure() { return failure; },
    get readFailure() { return readFailure; },
    get startReason() { return startReason; },
    get chosen() { return chosen; },
    set chosen(value: string) { chosen = value; },
    get cleanupResolved() { return cleanupResolved; },
    set cleanupResolved(value: boolean) { cleanupResolved = value; },
    get readRetrying() { return readRetrying; },
    set readRetrying(value: boolean) { readRetrying = value; },
    load,
    pickRoot,
    setMode,
    setDestination,
    browseDestination,
    useSuggested,
    openSource,
    pickInstaller,
    pickGameExecutable,
    pickManual,
    confirmChoice,
    cancelCurrent,
    retryCurrent,
    retryManually,
    dismissCurrent,
    deleteData,
    start,
    startImage,
    playInstalled,
    openImageFolder,
    own,
    other,
    heldFor,
  };
}
