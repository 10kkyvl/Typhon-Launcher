<script lang="ts">
  import { untrack } from 'svelte';
  import { FolderOpen } from '@lucide/svelte';
  import Button from './Button.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import Modal from './Modal.svelte';
  import GameProfiles from './GameProfiles.svelte';
  import Select from './Select.svelte';
  import Toggle from './Toggle.svelte';
  import { chooseSavesDir } from '../game/actions/saves';
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

  const target = $derived($gamePropertiesTarget);
  const game = $derived(target ? ($libraryGames.find((g) => g.id === target.gameId) ?? null) : null);

  let candidates = $state<Candidate[]>([]);
  let candidatesLoading = $state(false);
  let candidatesError = $state('');
  let exeBusy = $state(false);
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
  let compatBusy = $state(false);
  let compatError = $state('');
  let compatOn = $state(false);
  let compatForce = $state(false);
  let compatWrapper = $state<WrapperId>('dgvoodoo');
  let compatPreset = $state('default');
  let compatProgress = $state<WrapperProgress | null>(null);
  let compatKeepOffer = $state(false);

  const running = $derived(Boolean(game && $runningGames.has(game.id)));
  const compatVisible = $derived(Boolean(compat && (compat.supported || compat.active || compat.problem)));
  const compatSuggested = $derived(compat?.suggested ?? '');
  const compatLocked = $derived(!compat?.active && compatSuggested === '' && !compatForce);
  const compatApi = $derived(compat ? describeImports(compat.imports) : '');
  const compatView = $derived(compatProgress ? progressView(compatProgress, bytesSize) : null);
  const compatActions = $derived(compat?.problem ? problemActions(compat.problem.code) : null);
  const compatFiles = $derived((compat?.active?.files ?? []).join(', '));

  let savesBusy = $state(false);
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
      executableCandidates(gameId),
      getLaunchArgsLine(gameId),
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

  async function changeCompat(on: boolean, keepChanged = false) {
    if (!game || compatBusy) return;
    compatBusy = true;
    compatError = '';
    compatKeepOffer = false;
    compatProgress = null;
    try {
      if (on) await enableWrapper(game.id, compatWrapper, compatPreset);
      else if (keepChanged) await disableWrapperKeepChanged(game.id);
      else await disableWrapper(game.id);
    } catch (err) {
      compatKeepOffer = !on && errorCode(err) === 'wrappers.tampered';
      compatError = errorText(err, msg(on ? 'modals.compatEnableFailed' : 'modals.compatDisableFailed'));
    } finally {
      compatBusy = false;
      compatProgress = null;
    }
    await loadCompat(game.id);
  }

  async function pickCandidate(path: string) {
    if (!game || exeBusy) return;
    exeBusy = true;
    exeError = '';
    try {
      await setExecutable(game.id, path);
    } catch (err) {
      exeError = errorText(err, msg('modals.propertiesExeSaveFailed'));
      toast(exeError, 'danger');
    } finally {
      exeBusy = false;
    }
  }

  async function browseExecutable() {
    if (!game || exeBusy) return;
    exeBusy = true;
    exeError = '';
    try {
      const path = await selectGameExecutable(
        msg('modals.propertiesExeDialog', { title: game.title }),
        game.installDir,
        game.executable,
      );
      if (path) await setExecutable(game.id, path);
    } catch (err) {
      exeError = errorText(err, msg('modals.propertiesExeSaveFailed'));
      toast(exeError, 'danger');
    } finally {
      exeBusy = false;
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
    savesBusy = true;
    savesError = '';
    try {
      await chooseSavesDir(game.id, game.title);
    } catch (err) {
      savesError = errorText(err, msg('modals.propertiesSavesFailed'));
      toast(savesError, 'danger');
    } finally {
      savesBusy = false;
    }
  }

  async function resetSaves() {
    if (!game || savesBusy) return;
    savesBusy = true;
    savesError = '';
    try {
      await clearSavesDir(game.id);
    } catch (err) {
      savesError = errorText(err, msg('modals.propertiesSavesFailed'));
      toast(savesError, 'danger');
    } finally {
      savesBusy = false;
    }
  }
</script>

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
        {#if candidatesLoading}
          <p class="hint">{msg('modals.propertiesExeLoading')}</p>
        {:else if candidatesError}
          <p class="failure">{candidatesError}</p>
        {:else if candidates.length === 0}
          <p class="hint">{msg('modals.propertiesExeNoCandidates')}</p>
        {:else}
          <span class="label">{msg('modals.propertiesExeCandidates')}</span>
          <div class="candidates">
            {#each candidates as candidate (candidate.path)}
              <button
                class="candidate"
                class:current={candidate.path === game.executable}
                disabled={exeBusy}
                title={candidate.path}
                onclick={() => pickCandidate(candidate.path)}
              >
                {truncateMiddle(candidate.path, 70)}
              </button>
            {/each}
          </div>
        {/if}
        {#if exeError}
          <p class="failure">{exeError}</p>
        {/if}
        <div>
          <Button size="sm" disabled={exeBusy} onclick={browseExecutable}>
            <FolderOpen size="1.5rem" strokeWidth={1.8} />
            {msg('modals.propertiesExeBrowse')}
          </Button>
        </div>
      </section>

      <section class="section">
        <h4>{msg('modals.propertiesArgsTitle')}</h4>
        <div class="row">
          <input
            class="input sm"
            type="text"
            spellcheck="false"
            autocomplete="off"
            placeholder={msg('modals.propertiesArgsPlaceholder')}
            disabled={argsLoading}
            bind:value={argsLine}
            onkeydown={(e) => e.key === 'Enter' && argsDirty && saveArgs()}
          />
          <Button size="sm" variant="primary" disabled={argsLoading || argsBusy || !argsDirty} onclick={saveArgs}>
            {msg('modals.propertiesArgsSave')}
          </Button>
        </div>
        {#if argsError}
          <p class="failure">{argsError}</p>
        {/if}
        <p class="hint">{msg('modals.propertiesArgsHint')}</p>
      </section>

      {#if adminShown}
        <section class="section">
          <div class="row spread">
            <h4>{msg('modals.propertiesAdminTitle')}</h4>
            <Toggle
              bind:checked={admin}
              label={msg('modals.propertiesAdminTitle')}
              disabled={adminBusy}
              onchange={changeAdmin}
            />
          </div>
          <p class="hint">{msg('modals.propertiesAdminHint')}</p>
          {#if adminError}
            <p class="failure">{adminError}</p>
          {/if}
        </section>
      {/if}

      {#if compatLoading && !compat}
        <section class="section">
          <h4>{msg('modals.compatTitle')}</h4>
          <p class="hint">{msg('modals.compatLoading')}</p>
        </section>
      {:else if compatVisible && compat}
        <section class="section">
          <div class="row spread">
            <h4>{msg('modals.compatTitle')}</h4>
            <Toggle
              bind:checked={compatOn}
              label={msg('modals.compatToggle')}
              disabled={compatBusy || running || (compatLocked && !compat.active)}
              onchange={changeCompat}
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
                <Button size="sm" variant="ghost" onclick={() => (compatForce = !compatForce)}>
                  {compatForce ? msg('modals.compatForceHide') : msg('modals.compatForce')}
                </Button>
              </div>
            {/if}
            {#if !compatLocked}
              <div class="row spread">
                <span class="label">{msg('modals.compatWrapper')}</span>
                <Select
                  value={compatWrapper}
                  width="26rem"
                  options={WRAPPERS.map((id) => ({ id, label: wrapperName(id) }))}
                  onchange={pickWrapper}
                />
              </div>
              {#if presetsFor(compatWrapper).length > 1}
                <div class="row spread">
                  <span class="label">{msg('modals.compatPresetLabel')}</span>
                  <Select
                    bind:value={compatPreset}
                    width="26rem"
                    options={presetsFor(compatWrapper).map((id) => ({ id, label: presetLabel(id) }))}
                  />
                </div>
              {/if}
            {/if}
          {/if}
          {#if compatView}
            <p class="hint">{compatView.text}</p>
            <ProgressBar value={compatView.fraction ?? 0} max={1} indeterminate={compatView.fraction === null} />
          {/if}
          {#if running}
            <p class="hint">{msg('modals.compatRunning')}</p>
          {/if}
          {#if compat.problem && compatActions?.dirMissing}
            <p class="hint">{problemText(compat.problem.code)}</p>
          {:else if compat.problem && compatActions}
            <div class="warning" role="alert">
              <strong>{msg('modals.compatProblemTitle')}</strong>
              <span>{problemText(compat.problem.code, compat.problem.file)}</span>
              <div class="row">
                <Button size="sm" disabled={compatBusy || running} onclick={() => changeCompat(false)}>
                  {msg('modals.compatRevert')}
                </Button>
                {#if compatActions.keepChanged}
                  <Button size="sm" disabled={compatBusy || running} onclick={() => changeCompat(false, true)}>
                    {msg('modals.compatKeepChanged')}
                  </Button>
                {/if}
              </div>
              {#if compatActions.keepChanged}
                <span class="hint">{msg('modals.compatKeepChangedHint')}</span>
              {/if}
            </div>
          {:else if compatKeepOffer}
            <div class="warning" role="alert">
              <div class="row">
                <Button size="sm" disabled={compatBusy || running} onclick={() => changeCompat(false, true)}>
                  {msg('modals.compatKeepChanged')}
                </Button>
              </div>
              <span class="hint">{msg('modals.compatKeepChangedHint')}</span>
            </div>
          {/if}
          {#if compatError}
            <p class="failure">{compatError}</p>
          {/if}
          <p class="hint">{msg('modals.compatHint')}</p>
        </section>
      {:else if compatError}
        <section class="section">
          <h4>{msg('modals.compatTitle')}</h4>
          <p class="failure">{compatError}</p>
        </section>
      {/if}

      <GameProfiles {game} {candidates} {adminShown} />
      {#if gameModeShown}
        <section class="section">
          <h4>{msg('modals.propertiesGameModeTitle')}</h4>
          {#each gameModeRows as row (row.key)}
            <div class="row spread">
              <span class="label">{row.title}</span>
              <Select
                value={overrideState(game.gameMode, row.key)}
                width="26rem"
                options={row.options}
                onchange={(id) => changeGameMode(row.key, id as OverrideState)}
              />
            </div>
          {/each}
          <p class="hint">{msg('modals.propertiesGameModeHint')}</p>
          {#if gameModeError}
            <p class="failure">{gameModeError}</p>
          {/if}
        </section>
      {/if}

      <section class="section">
        <div class="row spread">
          <h4>{msg('modals.propertiesPresenceTitle')}</h4>
          <Toggle
            bind:checked={presence}
            label={msg('modals.propertiesPresenceTitle')}
            disabled={presenceBusy}
            onchange={changePresence}
          />
        </div>
        <p class="hint">{msg('modals.propertiesPresenceHint')}</p>
        {#if presenceError}
          <p class="failure">{presenceError}</p>
        {/if}
      </section>

      {#if game.installDir || game.savesDir}
        <section class="section">
          <h4>{msg('modals.propertiesSavesTitle')}</h4>
          <p class="path">{game.savesDir || msg('modals.propertiesSavesAuto')}</p>
          {#if savesError}
            <p class="failure">{savesError}</p>
          {/if}
          <div class="row">
            <Button size="sm" disabled={savesBusy} onclick={browseSaves}>
              <FolderOpen size="1.5rem" strokeWidth={1.8} />
              {msg('modals.propertiesSavesBrowse')}
            </Button>
            {#if game.savesDir}
              <Button size="sm" variant="ghost" disabled={savesBusy} onclick={resetSaves}>
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

  .failure {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .warning {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--danger);
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    color: var(--danger);
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

  .candidate:hover:not(:disabled) {
    border-color: var(--border-strong);
    color: var(--text);
  }

  .candidate.current {
    border-color: var(--accent);
    color: var(--text);
  }

  .candidate:disabled {
    opacity: 0.6;
    cursor: default;
  }
</style>
