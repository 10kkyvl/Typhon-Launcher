<script lang="ts">
  import { untrack } from 'svelte';
  import { FolderOpen } from '@lucide/svelte';
  import Tooltip from './Tooltip.svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import Modal from './Modal.svelte';
  import GameProfiles from './GameProfiles.svelte';
  import Notice from './Notice.svelte';
  import Select from './Select.svelte';
  import Spinner from './Spinner.svelte';
  import Toggle from './Toggle.svelte';
  import { chooseSavesDir } from '../game/actions/saves';
  import { emulationPlatform, isEmulated } from '../library/emulation';
  import { selectImageFile } from '../services/emulators';
  import { errorCode, hasMessage, msg } from '../i18n';
  import { installErrorText } from '../install/installErrors';
  import {
    WRAPPERS,
    defaultWrapper,
    describeImports,
    presetLabel,
    presetsFor,
    problemActions,
    problemText,
    progressView,
    wrapperName,
  } from '../game/compatMode';
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
  import {
    gameModeKeys,
    inheritLabel,
    overrideState,
    withOverride,
    type GameModeKey,
    type OverrideState,
  } from '../gamemode/gamemode';
  import { toast } from '../stores/toasts';
  import { bytesSize, truncateMiddle } from '../utils/format';

  const uid = $props.id();
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
  let argsError = $state('');

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

  let loadedFor = '';
  let seq = 0;

  const argsDirty = $derived(argsLine !== argsSaved);

  function errorText(err: unknown, fallback: string): string {
    const code = errorCode(err);
    if (hasMessage(code)) return msg(code);
    return installErrorText(err, fallback);
  }

  $effect(() => {
    const current = $gamePropertiesTarget;
    untrack(() => {
      if (!current) {
        loadedFor = '';
        seq++;
        return;
      }
      if (loadedFor === current.gameId) return;
      loadedFor = current.gameId;
      candidates = [];
      candidatesError = '';
      exeError = '';
      argsLine = '';
      argsSaved = '';
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
      argsError = errorText(line.reason, msg('modals.propertiesArgsLoadFailed'));
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
    compatBusyFor = [...compatBusyFor, id];
    compatAction = action;
    compatError = '';
    compatKeepOffer = false;
    compatProgress = null;
    // The dialog may have closed, or opened for another game, while this ran: a failure shows inline
    // where it is still shown, else as a toast, so the outcome is never lost.
    const shown = () => target?.gameId === id;
    try {
      if (on) await enableWrapper(id, compatWrapper, compatPreset);
      else if (keepChanged) await disableWrapperKeepChanged(id);
      else await disableWrapper(id);
    } catch (err) {
      const text = errorText(err, msg(on ? 'modals.compatEnableFailed' : 'modals.compatDisableFailed'));
      if (shown()) {
        compatKeepOffer = !on && errorCode(err) === 'wrappers.tampered';
        compatError = text;
      } else {
        toast(text, 'danger');
      }
    } finally {
      compatBusyFor = compatBusyFor.filter((busy) => busy !== id);
      compatAction = '';
      if (shown()) compatProgress = null;
    }
    if (shown()) await loadCompat(id);
  }

  async function pickCandidate(path: string) {
    if (!game || exeBusy) return;
    exeBusy = 'pick';
    exeError = '';
    try {
      await setExecutable(game.id, path);
    } catch (err) {
      exeError = errorText(err, msg('modals.propertiesExeSaveFailed'));
      toast(exeError, 'danger');
    } finally {
      exeBusy = '';
    }
  }

  async function browseExecutable() {
    if (!game || exeBusy) return;
    exeBusy = 'browse';
    exeError = '';
    try {
      const title = msg('modals.propertiesExeDialog', { title: game.title });
      const path = emulated
        ? await selectImageFile(title, [emulationPlatform(game)])
        : await selectGameExecutable(title, game.installDir, game.executable);
      if (path) await setExecutable(game.id, path);
    } catch (err) {
      exeError = errorText(err, msg('modals.propertiesExeSaveFailed'));
      toast(exeError, 'danger');
    } finally {
      exeBusy = '';
    }
  }

  async function saveArgs() {
    if (!game || argsBusy) return;
    argsBusy = true;
    argsError = '';
    try {
      await setLaunchArgsLine(game.id, argsLine);
      argsSaved = argsLine;
      toast(msg('modals.propertiesArgsSaved'), 'success');
    } catch (err) {
      argsError = errorText(err, msg('modals.propertiesArgsSaveFailed'));
      toast(argsError, 'danger');
    } finally {
      argsBusy = false;
    }
  }

  async function changeAdmin(on: boolean) {
    if (!game || adminBusy) return;
    adminBusy = true;
    adminError = '';
    try {
      await setRunAsAdmin(game.id, on);
    } catch (err) {
      admin = !on;
      adminError = errorText(err, msg('modals.propertiesAdminFailed'));
      toast(adminError, 'danger');
    } finally {
      adminBusy = false;
    }
  }

  async function changeGameMode(key: GameModeKey, state: OverrideState) {
    if (!game || gameModeBusy) return;
    gameModeBusy = true;
    gameModeError = '';
    try {
      await setGameModeOverride(game.id, withOverride(game.gameMode, key, state));
    } catch (err) {
      gameModeError = errorText(err, msg('modals.propertiesGameModeFailed'));
      toast(gameModeError, 'danger');
    } finally {
      gameModeBusy = false;
    }
  }

  async function changePresence(on: boolean) {
    if (!game || presenceBusy) return;
    presenceBusy = true;
    presenceError = '';
    try {
      await setHidePresence(game.id, !on);
    } catch (err) {
      presence = !on;
      presenceError = errorText(err, msg('modals.propertiesPresenceFailed'));
      toast(presenceError, 'danger');
    } finally {
      presenceBusy = false;
    }
  }

  async function browseSaves() {
    if (!game || savesBusy) return;
    savesBusy = 'browse';
    savesError = '';
    try {
      await chooseSavesDir(game.id, game.title);
    } catch (err) {
      savesError = errorText(err, msg('modals.propertiesSavesFailed'));
      toast(savesError, 'danger');
    } finally {
      savesBusy = '';
    }
  }

  async function resetSaves() {
    if (!game || savesBusy) return;
    savesBusy = 'reset';
    savesError = '';
    try {
      await clearSavesDir(game.id);
    } catch (err) {
      savesError = errorText(err, msg('modals.propertiesSavesFailed'));
      toast(savesError, 'danger');
    } finally {
      savesBusy = '';
    }
  }
</script>

<!-- dismissible while compatBusy: the wrapper download has no cancel and may take minutes; it goes on with the dialog closed, and reopening shows it still running. -->
<Modal
  open={Boolean(target && game)}
  title={game ? msg('modals.propertiesTitle', { title: game.title }) : msg('games.actionProperties')}
  width="56rem"
  onclose={closeGameProperties}
>
  {#if game}
    <div class="props">
      <section class="section">
        <h4>{msg('modals.propertiesExeTitle')}</h4>
        <p class="path">
          {game.executable ? msg('modals.propertiesExeCurrent', { path: game.executable }) : msg('modals.propertiesExeNone')}
        </p>
        {#if emulated}
          <p class="hint">{msg('emul.imageHint')}</p>
        {:else if candidatesLoading}
          <Spinner size="sm" label={msg('modals.propertiesExeLoading')} />
        {:else if candidatesError}
          <Notice tone="danger" compact>{candidatesError}</Notice>
        {:else if candidates.length === 0}
          <EmptyState size="inline" title={msg('modals.propertiesExeNoCandidates')} />
        {:else}
          <span class="label">{msg('modals.propertiesExeCandidates')}</span>
          <div class="candidates">
            {#each candidates as candidate (candidate.path)}
              <Tooltip text={candidate.path}>
                <button
                  type="button"
                  class="candidate"
                  aria-current={candidate.path === game.executable || undefined}
                  aria-disabled={exeBusy !== '' || undefined}
                  onclick={() => pickCandidate(candidate.path)}
                >
                  {truncateMiddle(candidate.path, 70)}
                </button>
              </Tooltip>
            {/each}
          </div>
        {/if}
        {#if exeError}
          <Notice tone="danger" compact announce={false}>{exeError}</Notice>
        {/if}
        <div>
          <Button size="sm" disabled={exeBusy === 'pick'} busy={exeBusy === 'browse'} onclick={browseExecutable}>
            <FolderOpen size="1.5rem" strokeWidth={1.8} />
            {msg('modals.propertiesExeBrowse')}
          </Button>
        </div>
      </section>

      {#if !emulated}
      <section class="section">
        <h4 id="{uid}-args-label">{msg('modals.propertiesArgsTitle')}</h4>
        <div class="row">
          <input
            class="input sm"
            type="text"
            aria-labelledby="{uid}-args-label"
            spellcheck="false"
            autocomplete="off"
            placeholder={msg('modals.propertiesArgsPlaceholder')}
            disabled={argsLoading}
            bind:value={argsLine}
            onkeydown={(e) => e.key === 'Enter' && argsDirty && saveArgs()}
          />
          <Button size="sm" variant="primary" disabled={argsLoading || (!argsDirty && !argsBusy)} busy={argsBusy} onclick={saveArgs}>
            {msg('modals.propertiesArgsSave')}
          </Button>
        </div>
        {#if argsError}
          <Notice tone="danger" compact announce={false}>{argsError}</Notice>
        {/if}
        <p class="hint">{msg('modals.propertiesArgsHint')}</p>
      </section>
      {/if}

      {#if adminShown && !emulated}
        <section class="section">
          <div class="row spread">
            <h4 id="{uid}-properties-admin-label">{msg('modals.propertiesAdminTitle')}</h4>
            <Toggle
              checked={admin}
              labelledby="{uid}-properties-admin-label"
              disabled={adminBusy}
              onchange={(on) => {
                admin = on;
                void changeAdmin(on);
              }}
            />
          </div>
          <p class="hint">{msg('modals.propertiesAdminHint')}</p>
          {#if adminError}
            <Notice tone="danger" compact announce={false}>{adminError}</Notice>
          {/if}
        </section>
      {/if}

      {#if !emulated && compatLoading && !compat}
        <section class="section">
          <h4>{msg('modals.compatTitle')}</h4>
          <Spinner size="sm" label={msg('modals.compatLoading')} />
        </section>
      {:else if !emulated && compatVisible && compat}
        <section class="section">
          <div class="row spread">
            <h4>{msg('modals.compatTitle')}</h4>
            <Toggle
              checked={compatOn}
              label={msg('modals.compatToggle')}
              disabled={compatBusy || running || (compatLocked && !compat.active)}
              onchange={(on) => {
                compatOn = on;
                void changeCompat(on);
              }}
            />
          </div>
          {#if compat.active}
            <p class="path">
              {msg('modals.compatActive', {
                wrapper: wrapperName(compat.active.wrapper),
                version: compat.active.version,
                files: compatFiles,
              })}
            </p>
            <p class="hint">{msg('modals.compatActivePreset', { preset: presetLabel(compat.active.preset) })}</p>
          {:else}
            <p class="path">{compatApi ? msg('modals.compatFound', { api: compatApi }) : msg('modals.compatFoundUnknown')}</p>
            <p class="hint">
              {compatSuggested
                ? msg('modals.compatSuggest', { wrapper: wrapperName(compatSuggested) })
                : msg('modals.compatNotNeeded')}
            </p>
            {#if compatSuggested === ''}
              <div>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-expanded={compatForce}
                  aria-controls="{uid}-compat-choice"
                  onclick={() => (compatForce = !compatForce)}
                >
                  {compatForce ? msg('modals.compatForceHide') : msg('modals.compatForce')}
                </Button>
              </div>
            {/if}
            <div class="choice" id="{uid}-compat-choice">
            {#if !compatLocked}
              <div class="row spread">
                <span class="label" id="{uid}-wrapper-label">{msg('modals.compatWrapper')}</span>
                <Select
                  labelledby="{uid}-wrapper-label"
                  value={compatWrapper}
                  width="26rem"
                  disabled={compatBusy || running}
                  options={WRAPPERS.map((id) => ({ id, label: wrapperName(id) }))}
                  onchange={pickWrapper}
                />
              </div>
              {#if presetsFor(compatWrapper).length > 1}
                <div class="row spread">
                  <span class="label" id="{uid}-preset-label">{msg('modals.compatPresetLabel')}</span>
                  <Select
                    labelledby="{uid}-preset-label"
                    bind:value={compatPreset}
                    width="26rem"
                    disabled={compatBusy || running}
                    options={presetsFor(compatWrapper).map((id) => ({ id, label: presetLabel(id) }))}
                  />
                </div>
              {/if}
            {/if}
            </div>
          {/if}
          {#if compatView}
            <p class="hint">{compatView.text}</p>
            <ProgressBar value={compatView.fraction ?? 0} max={1} indeterminate={compatView.fraction === null} label={msg('modals.compatTitle')} valueText={compatView.text} />
          {/if}
          {#if running}
            <p class="hint">{msg('modals.compatRunning')}</p>
          {/if}
          {#if compat.problem && compatActions?.dirMissing}
            <p class="hint">{problemText(compat.problem.code)}</p>
          {:else if compat.problem && compatActions}
            {@const actions = compatActions}
            <Notice tone="danger" compact title={msg('modals.compatProblemTitle')}>
              {problemText(compat.problem.code, compat.problem.file)}
              {#if actions.keepChanged}
                <span class="note">{msg('modals.compatKeepChangedHint')}</span>
              {/if}
              {#snippet action()}
                <div class="row">
                  <Button
                    size="sm"
                    disabled={(compatBusy && compatAction !== 'revert') || running}
                    busy={compatAction === 'revert'}
                    onclick={() => changeCompat(false, false, 'revert')}
                  >
                    {msg('modals.compatRevert')}
                  </Button>
                  {#if actions.keepChanged}
                    <Button
                      size="sm"
                      disabled={(compatBusy && compatAction !== 'keep') || running}
                      busy={compatAction === 'keep'}
                      onclick={() => changeCompat(false, true, 'keep')}
                    >
                      {msg('modals.compatKeepChanged')}
                    </Button>
                  {/if}
                </div>
              {/snippet}
            </Notice>
          {:else if compatKeepOffer}
            <Notice tone="warning" compact>
              {msg('modals.compatKeepChangedHint')}
              {#snippet action()}
                <Button
                  size="sm"
                  disabled={(compatBusy && compatAction !== 'keep') || running}
                  busy={compatAction === 'keep'}
                  onclick={() => changeCompat(false, true, 'keep')}
                >
                  {msg('modals.compatKeepChanged')}
                </Button>
              {/snippet}
            </Notice>
          {/if}
          {#if compatError}
            <Notice tone="danger" compact>{compatError}</Notice>
          {/if}
          <p class="hint">{msg('modals.compatHint')}</p>
        </section>
      {:else if !emulated && compatError}
        <section class="section">
          <h4>{msg('modals.compatTitle')}</h4>
          <Notice tone="danger" compact>{compatError}</Notice>
        </section>
      {/if}

      {#if !emulated}
        <GameProfiles {game} {candidates} {adminShown} />
      {/if}
      {#if gameModeShown}
        <section class="section">
          <h4>{msg('modals.propertiesGameModeTitle')}</h4>
          {#each gameModeRows as row (row.key)}
            <div class="row spread">
              <span class="label" id="{uid}-gamemode-{row.key}-label">{row.title}</span>
              <Select
                labelledby="{uid}-gamemode-{row.key}-label"
                value={overrideState(game.gameMode, row.key)}
                width="26rem"
                options={row.options}
                onchange={(id) => changeGameMode(row.key, id as OverrideState)}
              />
            </div>
          {/each}
          <p class="hint">{msg('modals.propertiesGameModeHint')}</p>
          {#if gameModeError}
            <Notice tone="danger" compact announce={false}>{gameModeError}</Notice>
          {/if}
        </section>
      {/if}

      <section class="section">
        <div class="row spread">
          <h4 id="{uid}-properties-presence-label">{msg('modals.propertiesPresenceTitle')}</h4>
          <Toggle
            checked={presence}
            labelledby="{uid}-properties-presence-label"
            disabled={presenceBusy}
            onchange={(on) => {
              presence = on;
              void changePresence(on);
            }}
          />
        </div>
        <p class="hint">{msg('modals.propertiesPresenceHint')}</p>
        {#if presenceError}
          <Notice tone="danger" compact announce={false}>{presenceError}</Notice>
        {/if}
      </section>

      {#if !emulated && (game.installDir || game.savesDir)}
        <section class="section">
          <h4>{msg('modals.propertiesSavesTitle')}</h4>
          <p class="path">{game.savesDir || msg('modals.propertiesSavesAuto')}</p>
          {#if savesError}
            <Notice tone="danger" compact announce={false}>{savesError}</Notice>
          {/if}
          <div class="row">
            <Button size="sm" disabled={savesBusy === 'reset'} busy={savesBusy === 'browse'} onclick={browseSaves}>
              <FolderOpen size="1.5rem" strokeWidth={1.8} />
              {msg('modals.propertiesSavesBrowse')}
            </Button>
            {#if game.savesDir}
              <Button size="sm" variant="ghost" disabled={savesBusy === 'browse'} busy={savesBusy === 'reset'} onclick={resetSaves}>
                {msg('modals.propertiesSavesReset')}
              </Button>
            {/if}
          </div>
        </section>
      {/if}
    </div>
  {/if}
</Modal>

<style>
  .props {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h4 {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text);
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .path {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .note {
    display: block;
    margin-top: var(--space-1);
  }

  /* Holds the wrapper choice the "force" disclosure shows, as a part of the section's own column. */
  .choice {
    display: contents;
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .row .input {
    flex: 1;
  }

  .spread {
    justify-content: space-between;
  }

  .candidates {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    max-height: 18rem;
    overflow-y: auto;
  }

  .candidate {
    text-align: left;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--text-2);
    font-size: var(--font-sm);
    cursor: pointer;
  }

  .candidate:hover:not([aria-disabled='true']) {
    border-color: var(--border-strong);
    color: var(--text);
  }

  .candidate[aria-current='true'] {
    border-color: var(--accent-bright);
    color: var(--text);
  }

  .candidate[aria-disabled='true'] {
    opacity: 0.6;
    cursor: default;
  }

  /* Inside the scrolling list: the shared ring is drawn inward, where the list does not clip it. */
  .candidate:focus-visible {
    outline-offset: -2px;
  }
</style>
