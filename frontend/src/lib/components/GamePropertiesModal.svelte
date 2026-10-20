<script lang="ts">
  import { untrack } from 'svelte';
  import { FolderOpen } from '@lucide/svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import GameProfiles from './GameProfiles.svelte';
  import Toggle from './Toggle.svelte';
  import { chooseSavesDir } from '../game/actions/saves';
  import { errorCode, hasMessage, msg } from '../i18n';
  import { installErrorText } from '../install/installErrors';
  import {
    clearSavesDir,
    getLaunchArgsLine,
    selectGameExecutable,
    setExecutable,
    setLaunchArgsLine,
    setHidePresence,
    setRunAsAdmin,
  } from '../services/library';
  import { executableCandidates, type Candidate } from '../services/install';
  import { appInfo, elevationSupported } from '../services/system';
  import { closeGameProperties, gamePropertiesTarget } from '../stores/gameProperties';
  import { libraryGames } from '../stores/library';
  import { toast } from '../stores/toasts';
  import { truncateMiddle } from '../utils/format';

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
      presenceError = '';
      savesError = '';
      void load(current.gameId);
    });
  });

  $effect(() => {
    admin = Boolean(game?.runAsAdmin);
  });

  $effect(() => {
    presence = !game?.hidePresence;
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

      <GameProfiles {game} {candidates} {adminShown} />

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
