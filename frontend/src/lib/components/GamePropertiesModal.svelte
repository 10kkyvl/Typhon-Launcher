<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { get } from 'svelte/store';
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
  import { msg } from '../i18n';
  import { propertiesErrorText as errorText } from '../game/propertiesErrors';
  import { createGameModeWrites } from '../game/gameModeWrites.svelte';
  import { createCompatSwitch } from '../game/compatSwitch.svelte';
  import { createFileWrites, type FileWrite } from '../game/fileWrites.svelte';
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
  import { closeGameProperties, gamePropertiesTarget } from '../stores/gameProperties';
  import { libraryGames, runningGames } from '../stores/library';
  import type { GameModeKey, OverrideState } from '../gamemode/gamemode';
  import { toast } from '../stores/toasts';
  import { focusHandOff } from '../utils/focusHandOff';

  const target = $derived($gamePropertiesTarget);
  const game = $derived(target ? ($libraryGames.find((g) => g.id === target.gameId) ?? null) : null);

  const emulated = $derived(isEmulated(game));

  let candidates = $state<Candidate[]>([]);
  let candidatesLoading = $state(false);
  let candidatesError = $state('');
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

  const gameModes = createGameModeWrites(setGameModeOverride, (id) => get(libraryGames).find((g) => g.id === id)?.gameMode);
  let gameModeError = $state('');
  let gameModeShown = $state(false);
  let gameModeMac = $state(false);

  // The wrapper switch, the executable and the saves folder change what the game starts or its files.
  // Go's Enable takes the executable's folder at its start and places the files there, so these run
  // one at a time per game, across openings: while one runs the others are held, saying which one
  // they wait for.
  const fileWrites = createFileWrites();
  const fileWrite = $derived(fileWrites.of(game?.id));
  // What sets the executable: a found file pressed, or the file dialog.
  const exeBusy = $derived(fileWrite?.kind === 'exe' ? fileWrite.how : '');
  const savesBusy = $derived(fileWrite?.kind === 'saves' ? fileWrite.how : '');

  const running = $derived(Boolean(game && $runningGames.has(game.id)));
  const compat = createCompatSwitch({
    target: () => target,
    game: () => game,
    pressedNow,
    held: () => compatHeld,
    focus: { section: () => compatSection, props: () => propsEl },
    seq: () => seq,
    writes: fileWrites,
  });
  const compatBusy = $derived(compat.busy);

  let savesError = $state('');

  const heldReason = { compat: 'modals.propertiesHeldCompat', exe: 'modals.propertiesHeldExe', saves: 'modals.propertiesHeldSaves' } as const;
  const heldBy = (own: FileWrite['kind']) => (fileWrite && fileWrite.kind !== own ? msg(heldReason[fileWrite.kind]) : '');
  const exeHeld = $derived(heldBy('exe'));
  const compatHeld = $derived(heldBy('compat'));
  const savesHeld = $derived(heldBy('saves'));
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
      argsBusy = false;
      adminBusy = false;
      presenceBusy = false;
      gameModes.reset();
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
      compat.reset();
      void load(current.gameId);
      void compat.load(current.gameId);
    });
  });

  $effect(() => {
    admin = Boolean(game?.runAsAdmin);
  });

  $effect(() => {
    presence = !game?.hidePresence;
  });

  $effect(() => {
    void game?.gameMode;
    untrack(() => gameModes.settle());
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

  let propsEl = $state<HTMLElement>();
  let compatSection = $state<HTMLElement>();

  /** Compatibility mode reads the executable: a dialog that shows the game reads it again once it changed. */
  function exeChanged(id: string) {
    if (alive && target?.gameId === id) void compat.load(id);
  }

  async function pickCandidate(path: string) {
    if (!game || exeBusy || exeHeld) return;
    const id = game.id;
    const end = fileWrites.start(id, { kind: 'exe', how: 'pick' });
    if (!end) return;
    const { heard, title } = pressedNow();
    exeError = '';
    let failure: string | undefined;
    try {
      await setExecutable(id, path);
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesExeSaveFailed'));
    } finally {
      end();
    }
    if (!heard()) sayLate(title, failure);
    else exeError = failure ?? '';
    if (failure === undefined) exeChanged(id);
  }

  async function browseExecutable() {
    if (!game || exeBusy || exeHeld) return;
    const current = game;
    const end = fileWrites.start(current.id, { kind: 'exe', how: 'browse' });
    if (!end) return;
    const { heard, title } = pressedNow();
    exeError = '';
    let failure: string | undefined;
    let changed = false;
    try {
      const dialogTitle = msg('modals.propertiesExeDialog', { title: current.title });
      const path = emulated
        ? await selectImageFile(dialogTitle, [emulationPlatform(current)])
        : await selectGameExecutable(dialogTitle, current.installDir, current.executable);
      // A file picked after the dialog was torn down is not set: the dialog mounted since knows
      // nothing of this write and may be turning compatibility mode on.
      if (path && !alive) return toast(msg('modals.propertiesExeLateDropped', { title }), 'warning');
      if (path) {
        changed = true;
        await setExecutable(current.id, path);
      }
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesExeSaveFailed'));
    } finally {
      end();
    }
    if (!heard()) {
      if (changed || failure !== undefined) sayLate(title, failure);
    } else exeError = failure ?? '';
    if (changed && failure === undefined) exeChanged(current.id);
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

  // A change made while another is written waits its turn; changes still waiting when the opening
  // passes are said in one toast once the last is stored, a refusal in its own.
  async function changeGameMode(key: GameModeKey, state: OverrideState) {
    if (!game) return;
    const id = game.id;
    const { heard, title } = pressedNow();
    gameModeError = '';
    let failure: string | undefined;
    try {
      await gameModes.write(id, key, state);
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesGameModeFailed'));
    }
    if (!heard()) {
      if (failure !== undefined || gameModes.waiting(id) === 0) sayLate(title, failure);
      return;
    }
    if (failure !== undefined) gameModeError = failure;
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
    if (!game || savesBusy || savesHeld) return;
    const end = fileWrites.start(game.id, { kind: 'saves', how: 'browse' });
    if (!end) return;
    const { heard, title } = pressedNow();
    savesError = '';
    let failure: string | undefined;
    let dir = '';
    try {
      dir = await chooseSavesDir(game.id, game.title);
    } catch (err) {
      failure = errorText(err, msg('modals.propertiesSavesFailed'));
    } finally {
      end();
    }
    if (!heard()) {
      if (dir || failure !== undefined) sayLate(title, failure);
      return;
    }
    savesError = failure ?? '';
  }

  function askResetSaves() {
    if (!game || savesBusy || savesHeld || !game.savesDir) return;
    savesError = '';
    resetAsk = { id: game.id, title: game.title, path: game.savesDir };
  }

  // Go's ClearSavesDir forgets the chosen folder and deletes no file: the question says so. A refusal
  // stays in the question; one that comes after the dialog moved on is said in a toast.
  async function resetSaves(ask: { id: string; title: string }) {
    const end = fileWrites.start(ask.id, { kind: 'saves', how: 'reset' });
    // The question covers the dialog, so no other write of the game can have begun since it was asked.
    if (!end) return;
    const { heard } = pressedNow();
    try {
      await clearSavesDir(ask.id);
    } catch (err) {
      if (heard()) throw err;
      return sayLate(ask.title, errorText(err, msg('modals.propertiesSavesFailed')));
    } finally {
      end();
    }
    if (!heard()) sayLate(ask.title);
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
        heldBy={exeHeld}
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
          compat={compat.compat}
          loading={compat.loading}
          visible={compat.visible}
          on={compat.on}
          busy={compatBusy}
          heldBy={compatHeld}
          {running}
          locked={compat.locked}
          suggested={compat.suggested}
          api={compat.api}
          view={compat.view}
          actions={compat.actions}
          files={compat.files}
          force={compat.force}
          wrapper={compat.wrapper}
          bind:preset={compat.preset}
          action={compat.action}
          error={compat.error}
          keepOffer={compat.keepOffer}
          bind:section={compatSection}
          onswitch={compat.toggle}
          onforce={compat.toggleForce}
          onwrapper={compat.pickWrapper}
          onrevert={compat.revert}
          onkeep={compat.keep}
        />
      {/if}

      {#if !emulated}
        <GameProfiles {game} {candidates} {adminShown} />
      {/if}
      <PropertiesPlaying
        {game}
        {gameModeShown}
        {gameModeMac}
        {gameModeError}
        gameModeChosen={gameModes.chosen}
        ongamemode={changeGameMode}
        {presence}
        {presenceBusy}
        {presenceError}
        onpresence={(on) => void changePresence(on)}
      />

      {#if !emulated && (game.installDir || game.savesDir)}
        <PropertiesSaves {game} {savesBusy} heldBy={savesHeld} {savesError} onbrowse={browseSaves} onreset={askResetSaves} />
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
