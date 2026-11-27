<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import Modal from './Modal.svelte';
  import GameProfiles from './GameProfiles.svelte';
  import PropertiesCompat from './PropertiesCompat.svelte';
  import PropertiesExecutable from './PropertiesExecutable.svelte';
  import PropertiesLaunch from './PropertiesLaunch.svelte';
  import PropertiesPlaying from './PropertiesPlaying.svelte';
  import PropertiesSaves from './PropertiesSaves.svelte';
  import { chooseSavesDir } from '../game/actions/saves';
  import { emulationPlatform, isEmulated } from '../library/emulation';
  import { selectImageFile } from '../services/emulators';
  import { errorCode, hasMessage, msg } from '../i18n';
  import { installErrorText } from '../install/installErrors';
  import { defaultWrapper, describeImports, presetsFor, problemActions, progressView } from '../game/compatMode';
  import {
    clearSavesDir,
    getLaunchArgsLine,
    selectGameExecutable,
    setExecutable,
    setLaunchArgsLine,
    setGameModeOverride,
    setHidePresence,
    setRunAsAdmin,
  } from '../services/library';
  import { executableCandidates, type Candidate } from '../services/install';
  import { appInfo, elevationSupported } from '../services/system';
  import {
    disableWrapper,
    disableWrapperKeepChanged,
    enableWrapper,
    inspectWrapper,
    onWrapperProgress,
    type WrapperId,
    type WrapperInfo,
    type WrapperProgress,
  } from '../services/wrappers';
  import { closeGameProperties, gamePropertiesTarget } from '../stores/gameProperties';
  import { libraryGames, runningGames } from '../stores/library';
  import { settings } from '../stores/settings';
  import { gameModeKeys, inheritLabel, withOverride, type GameModeKey, type OverrideState } from '../gamemode/gamemode';
  import { toast } from '../stores/toasts';
  import { focusableIn } from '../layers/focus';
  import { focusHandOff } from '../utils/focusHandOff';
  import { bytesSize } from '../utils/format';

  const target = $derived($gamePropertiesTarget);
  const game = $derived(target ? ($libraryGames.find((g) => g.id === target.gameId) ?? null) : null);

  const emulated = $derived(isEmulated(game));

  let candidates = $state<Candidate[]>([]);
  let candidatesLoading = $state(false);
  let candidatesError = $state('');
  // What sets the executable: a found file pressed, or the file dialog.
  let exeBusy = $state<'' | 'pick' | 'browse'>('');
  let exeError = $state('');

  let argsLine = $state('');
  let argsSaved = $state('');
  let argsLoading = $state(false);
  let argsBusy = $state(false);
  // A load that failed is said here; a save that failed is said by its toast and only kept here.
  let argsLoadError = $state('');
  let argsError = $state('');
  let argsInput = $state<HTMLInputElement>();

  let admin = $state(false);
  let adminBusy = $state(false);
  let adminShown = $state(false);
  let adminError = $state('');

  let presence = $state(true);
  let presenceBusy = $state(false);
  let presenceError = $state('');

  let gameModeBusy = $state(false);
  let gameModeError = $state('');
  let gameModeShown = $state(false);
  let gameModeMac = $state(false);

  const gameModeTitles = {
    highPerformance: 'settings.gameModePowerLabel',
    hideLauncher: 'settings.gameModeHideLabel',
    raisePriority: 'settings.gameModePriorityLabel',
  } as const;

  const gameModeRows = $derived(
    gameModeKeys
      .filter((key) => !(gameModeMac && key !== 'hideLauncher'))
      .map((key) => ({
        key,
        title: msg(gameModeTitles[key]),
        options: [
          { id: 'inherit', label: inheritLabel(Boolean($settings?.gameMode?.[key])) },
          { id: 'on', label: msg('modals.propertiesGameModeOn') },
          { id: 'off', label: msg('modals.propertiesGameModeOff') },
        ],
      })),
  );

  let compat = $state<WrapperInfo | null>(null);
  let compatLoading = $state(false);
  // Games whose compatibility mode is being switched. The component outlives the dialog (App mounts it
  // once), so a switch that goes on after the dialog closed is still known when it opens again.
  let compatBusyFor = $state<string[]>([]);
  let compatError = $state('');
  let compatOn = $state(false);
  let compatForce = $state(false);
  let compatWrapper = $state<WrapperId>('dgvoodoo');
  let compatPreset = $state('default');
  let compatProgress = $state<WrapperProgress | null>(null);
  let compatKeepOffer = $state(false);
  // Which problem action was pressed: that button stays focused and busy while the switch runs.
  let compatAction = $state<'' | 'revert' | 'keep'>('');

  const running = $derived(Boolean(game && $runningGames.has(game.id)));
  const compatBusy = $derived(game ? compatBusyFor.includes(game.id) : false);
  const compatVisible = $derived(Boolean(compat && (compat.supported || compat.active || compat.problem)));
  const compatSuggested = $derived(compat?.suggested ?? '');
  const compatLocked = $derived(!compat?.active && compatSuggested === '' && !compatForce);
  const compatApi = $derived(compat ? describeImports(compat.imports) : '');
  const compatView = $derived(compatProgress ? progressView(compatProgress, bytesSize) : null);
  const compatActions = $derived(compat?.problem ? problemActions(compat.problem.code) : null);
  const compatFiles = $derived((compat?.active?.files ?? []).join(', '));

  let savesBusy = $state<'' | 'browse' | 'reset'>('');
  let savesError = $state('');
  // The game whose saves folder the open question would reset: the one it was asked for.
  let resetAsk = $state<{ id: string; title: string; path: string } | null>(null);

  let loadedFor = '';
  let seq = 0;
  // One per opening, and per game shown: a write answered after its opening has passed paints
  // nothing and is said once in a toast naming its game. A destroyed dialog hears nothing either.
  let opening = 0;
  let alive = true;
  onDestroy(() => {
    alive = false;
  });

  const argsDirty = $derived(argsLine !== argsSaved);

  // Go's library and emulator reasons are lower-case clauses: said after the action that failed.
  // The other domains' messages are whole sentences.
  function errorText(err: unknown, fallback: string): string {
    const code = errorCode(err);
    if (/^(library|emulators)\./.test(code) && hasMessage(code)) return msg('modals.propertiesFailedReason', { action: fallback, reason: msg(code) });
    if (hasMessage(code)) return msg(code);
    return installErrorText(err, fallback);
  }

  /** A write answered after its opening passed: said once, in a toast naming its game. */
  function sayLate(title: string, failure?: string) {
    if (failure === undefined) toast(msg('modals.propertiesLateSaved', { title }), 'success');
    else toast(msg('modals.propertiesLateFailed', { title, reason: failure }), 'danger');
  }

  /** Taken at a press: whether the opening it was pressed in still hears its answer, and its game. */
  function pressedNow() {
    const at = opening;
    return { heard: () => alive && at === opening, title: game?.title ?? '' };
  }

  $effect(() => {
    const current = $gamePropertiesTarget;
    untrack(() => {
      if (!current) {
        loadedFor = '';
        seq++;
        opening++;
        resetAsk = null;
        return;
      }
      if (loadedFor === current.gameId) return;
      loadedFor = current.gameId;
      opening++;
      resetAsk = null;
      exeBusy = '';
      argsBusy = false;
      adminBusy = false;
      presenceBusy = false;
      gameModeBusy = false;
      savesBusy = '';
      compatAction = '';
      candidates = [];
      candidatesError = '';
      exeError = '';
      argsLine = '';
      argsSaved = '';
      argsLoadError = '';
      argsError = '';
      adminError = '';
      gameModeError = '';
      presenceError = '';
      savesError = '';
      compat = null;
      compatError = '';
      compatForce = false;
      compatProgress = null;
      compatKeepOffer = false;
      void load(current.gameId);
      void loadCompat(current.gameId);
    });
  });

  $effect(() => {
    admin = Boolean(game?.runAsAdmin);
  });

  $effect(() => {
    presence = !game?.hidePresence;
  });

  $effect(() => {
    compatOn = Boolean(compat?.active);
  });

  $effect(() => {
    const id = game?.id;
    if (!id) return;
    return onWrapperProgress((event) => {
      if (event.gameId === id) compatProgress = event;
    });
  });

  $effect(() => {
    if (target && game && (game.uninstalled || !game.installDir)) closeGameProperties();
  });

  async function load(gameId: string) {
    const mine = ++seq;
    candidatesLoading = true;
    argsLoading = true;
    const [list, line, info] = await Promise.allSettled([
      emulated ? Promise.resolve<Candidate[]>([]) : executableCandidates(gameId),
      emulated ? Promise.resolve('') : getLaunchArgsLine(gameId),
      appInfo(),
    ]);
    if (mine !== seq) return;
    candidatesLoading = false;
    argsLoading = false;
    if (list.status === 'fulfilled') {
      candidates = list.value;
    } else {
      candidatesError = errorText(list.reason, msg('modals.propertiesExeLoadFailed'));
    }
    if (line.status === 'fulfilled') {
      argsLine = line.value;
      argsSaved = line.value;
    } else {
      argsLoadError = errorText(line.reason, msg('modals.propertiesArgsLoadFailed'));
    }
    adminShown = info.status === 'fulfilled' && elevationSupported(info.value);
    gameModeShown = info.status === 'fulfilled';
    gameModeMac = info.status === 'fulfilled' && info.value.platform === 'darwin';
  }

  async function loadCompat(gameId: string) {
    if (emulated) return;
    const mine = seq;
    compatLoading = true;
    try {
      const info = await inspectWrapper(gameId);
      if (mine !== seq) return;
      compat = info;
      compatWrapper = info.active ? info.active.wrapper : defaultWrapper(info);
      compatPreset = info.active ? info.active.preset : 'default';
    } catch (err) {
      if (mine !== seq) return;
      compatError = errorText(err, msg('modals.compatInspectFailed'));
    } finally {
      if (mine === seq) compatLoading = false;
    }
  }

  function pickWrapper(id: string) {
    compatWrapper = id as WrapperId;
    if (!presetsFor(compatWrapper).includes(compatPreset)) compatPreset = 'default';
  }

  async function changeCompat(on: boolean, keepChanged = false, action: '' | 'revert' | 'keep' = '') {
    if (!game || compatBusy) return;
    const id = game.id;
    const { heard, title } = pressedNow();
    compatBusyFor = [...compatBusyFor, id];
    compatAction = action;
    compatError = '';
    compatKeepOffer = false;
    compatProgress = null;
    // The dialog cannot be closed while this runs, yet it may be turned to another game or closed from
    // outside (the game removed). The switch stays known as running by game, across openings; its
    // outcome is said in the opening it was pressed in, else in a toast naming the game.
    const shown = () => target?.gameId === id;
    // A problem's action goes with its notice once it worked: focus it held goes to the compatibility
    // switch, which then says the outcome, else to the control that followed the notice.
    const hand = action ? focusHandOff() : null;
    const after = action ? controlAfterNotice() : null;
    let failure: string | undefined;
    try {
      if (on) await enableWrapper(id, compatWrapper, compatPreset);
      else if (keepChanged) await disableWrapperKeepChanged(id);
      else await disableWrapper(id);
    } catch (err) {
      failure = errorText(err, msg(on ? 'modals.compatEnableFailed' : 'modals.compatDisableFailed'));
      if (heard()) {
        compatKeepOffer = !on && errorCode(err) === 'wrappers.tampered';
        compatError = failure;
      }
    } finally {
      compatBusyFor = compatBusyFor.filter((busy) => busy !== id);
      if (heard()) compatAction = '';
      if (shown()) compatProgress = null;
    }
    if (!heard()) sayLate(title, failure);
    if (shown()) await loadCompat(id);
    if (heard()) await hand?.(() => compatSection?.querySelector<HTMLElement>('[role="switch"]') ?? (after?.isConnected ? after : null));
  }

  let propsEl = $state<HTMLElement>();
  let compatSection = $state<HTMLElement>();

  function controlAfterNotice(): HTMLElement | null {
    const notice = document.activeElement?.closest('.notice');
    if (!notice || !propsEl) return null;
    const all = focusableIn(propsEl);
    const mine = all.filter((el) => notice.contains(el));
    return all[all.indexOf(mine[mine.length - 1]) + 1] ?? null;
  }

  async function pickCandidate(path: string) {
    if (!game || exeBusy) return;
    const id = game.id;
    const { heard, title } = pressedNow();
    exeBusy = 'pick';
    exeError = '';
    let failure: string | undefined;
    try {
      await setExecutable(id, path);
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesExeSaveFailed'));
    }
    if (!heard()) return sayLate(title, failure);
    exeBusy = '';
    exeError = failure ?? '';
  }

  async function browseExecutable() {
    if (!game || exeBusy) return;
    const current = game;
    const { heard, title } = pressedNow();
    exeBusy = 'browse';
    exeError = '';
    let failure: string | undefined;
    let changed = false;
    try {
      const dialogTitle = msg('modals.propertiesExeDialog', { title: current.title });
      const path = emulated
        ? await selectImageFile(dialogTitle, [emulationPlatform(current)])
        : await selectGameExecutable(dialogTitle, current.installDir, current.executable);
      if (path) {
        changed = true;
        await setExecutable(current.id, path);
      }
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesExeSaveFailed'));
    }
    if (!heard()) {
      if (changed || failure !== undefined) sayLate(title, failure);
      return;
    }
    exeBusy = '';
    exeError = failure ?? '';
  }

  async function saveArgs(event?: MouseEvent) {
    if (!game || argsBusy) return;
    const id = game.id;
    const line = argsLine;
    const { heard, title } = pressedNow();
    // Saved, Save has nothing left to do and stays but turns disabled: focus still on it then counts
    // as fallen, and goes back to the field, what the user would change next.
    const save = event?.currentTarget instanceof HTMLButtonElement ? event.currentTarget : null;
    const hand = focusHandOff(() => (save?.disabled ? save : null));
    argsBusy = true;
    argsError = '';
    let failure: string | undefined;
    try {
      await setLaunchArgsLine(id, line);
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesArgsSaveFailed'));
    }
    if (!heard()) return sayLate(title, failure);
    argsBusy = false;
    if (failure === undefined) {
      argsSaved = line;
      argsLoadError = '';
      toast(msg('modals.propertiesArgsSaved'), 'success');
    } else {
      argsError = failure;
    }
    await hand(() => argsInput);
  }

  async function changeAdmin(on: boolean) {
    if (!game || adminBusy) return;
    const id = game.id;
    const { heard, title } = pressedNow();
    admin = on;
    adminBusy = true;
    adminError = '';
    let failure: string | undefined;
    try {
      await setRunAsAdmin(id, on);
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesAdminFailed'));
    }
    if (!heard()) return sayLate(title, failure);
    adminBusy = false;
    if (failure !== undefined) {
      admin = !on;
      adminError = failure;
    }
  }

  async function changeGameMode(key: GameModeKey, state: OverrideState) {
    if (!game || gameModeBusy) return;
    const id = game.id;
    const { heard, title } = pressedNow();
    gameModeBusy = true;
    gameModeError = '';
    let failure: string | undefined;
    try {
      await setGameModeOverride(id, withOverride(game.gameMode, key, state));
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesGameModeFailed'));
    }
    if (!heard()) return sayLate(title, failure);
    gameModeBusy = false;
    gameModeError = failure ?? '';
  }

  async function changePresence(on: boolean) {
    if (!game || presenceBusy) return;
    const id = game.id;
    const { heard, title } = pressedNow();
    presence = on;
    presenceBusy = true;
    presenceError = '';
    let failure: string | undefined;
    try {
      await setHidePresence(id, !on);
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesPresenceFailed'));
    }
    if (!heard()) return sayLate(title, failure);
    presenceBusy = false;
    if (failure !== undefined) {
      presence = !on;
      presenceError = failure;
    }
  }

  async function browseSaves() {
    if (!game || savesBusy) return;
    const { heard, title } = pressedNow();
    savesBusy = 'browse';
    savesError = '';
    let failure: string | undefined;
    let dir = '';
    try {
      dir = await chooseSavesDir(game.id, game.title);
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesSavesFailed'));
    }
    if (!heard()) {
      if (dir || failure !== undefined) sayLate(title, failure);
      return;
    }
    savesBusy = '';
    savesError = failure ?? '';
  }

  function askResetSaves() {
    if (!game || savesBusy || !game.savesDir) return;
    savesError = '';
    resetAsk = { id: game.id, title: game.title, path: game.savesDir };
  }

  // Go's ClearSavesDir forgets the chosen folder and deletes no file: the question says so. A refusal
  // stays in the question; one that comes after the dialog moved on is said in a toast.
  async function resetSaves(ask: { id: string; title: string }) {
    const { heard } = pressedNow();
    savesBusy = 'reset';
    try {
      await clearSavesDir(ask.id);
    } catch (err) {
      if (heard()) {
        savesBusy = '';
        throw err;
      }
      return sayLate(ask.title, errorText(err, msg('modals.propertiesSavesFailed')));
    }
    if (!heard()) return sayLate(ask.title);
    savesBusy = '';
  }

  function closeResetAsk(ask: object) {
    if (resetAsk === ask) resetAsk = null;
  }
</script>

<!-- Not dismissible while a wrapper change runs (it rewrites the game's files and has no cancel) or
     the saves folder is reset: the outcome is said in the dialog. -->
<Modal
  open={Boolean(target && game)}
  dismissible={!compatBusy && savesBusy !== 'reset'}
  busy={compatBusy || savesBusy === 'reset'}
  title={game ? msg('modals.propertiesTitle', { title: game.title }) : msg('games.actionProperties')}
  width="56rem"
  onclose={closeGameProperties}
>
  {#if game}
    <div class="props" bind:this={propsEl}>
      <PropertiesExecutable
        {game}
        {emulated}
        {candidates}
        {candidatesLoading}
        {candidatesError}
        {exeBusy}
        {exeError}
        onpick={pickCandidate}
        onbrowse={browseExecutable}
      />

      <PropertiesLaunch
        {emulated}
        {adminShown}
        bind:argsLine
        bind:argsInput
        {argsLoading}
        {argsBusy}
        {argsDirty}
        {argsLoadError}
        {argsError}
        onsave={saveArgs}
        {admin}
        {adminBusy}
        {adminError}
        onadmin={(on) => void changeAdmin(on)}
      />

      {#if !emulated}
        <PropertiesCompat
          {compat}
          loading={compatLoading}
          visible={compatVisible}
          on={compatOn}
          busy={compatBusy}
          {running}
          locked={compatLocked}
          suggested={compatSuggested}
          api={compatApi}
          view={compatView}
          actions={compatActions}
          files={compatFiles}
          force={compatForce}
          wrapper={compatWrapper}
          bind:preset={compatPreset}
          action={compatAction}
          error={compatError}
          keepOffer={compatKeepOffer}
          bind:section={compatSection}
          onswitch={(on) => {
            if (compatBusy) return;
            compatOn = on;
            void changeCompat(on);
          }}
          onforce={() => (compatForce = !compatForce)}
          onwrapper={pickWrapper}
          onrevert={() => changeCompat(false, false, 'revert')}
          onkeep={() => changeCompat(false, true, 'keep')}
        />
      {/if}

      {#if !emulated}
        <GameProfiles {game} {candidates} {adminShown} />
      {/if}
      <PropertiesPlaying
        {game}
        {gameModeShown}
        {gameModeRows}
        {gameModeError}
        ongamemode={changeGameMode}
        {presence}
        {presenceBusy}
        {presenceError}
        onpresence={(on) => void changePresence(on)}
      />

      {#if !emulated && (game.installDir || game.savesDir)}
        <PropertiesSaves {game} {savesBusy} {savesError} onbrowse={browseSaves} onreset={askResetSaves} />
      {/if}
    </div>
  {/if}
</Modal>

{#if resetAsk}
  {@const ask = resetAsk}
  <ConfirmModal
    prompt={{
      title: msg('modals.propertiesSavesResetTitle'),
      text: msg('modals.propertiesSavesResetText', { path: ask.path, title: ask.title }),
      note: msg('modals.propertiesSavesResetNote'),
      confirm: msg('modals.propertiesSavesResetConfirm'),
      busy: msg('modals.propertiesSavesResetting'),
    }}
    onconfirm={() => resetSaves(ask)}
    onclose={() => closeResetAsk(ask)}
    errorText={(err) => errorText(err, msg('modals.propertiesSavesFailed'))}
    returnFocus={() => propsEl?.querySelector<HTMLElement>('[data-saves-browse]')}
  />
{/if}

<style>
  .props {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
</style>
