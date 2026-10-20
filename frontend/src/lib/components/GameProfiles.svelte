<script lang="ts">
  import { untrack } from 'svelte';
  import { FolderOpen, Pencil, Plus, Trash2 } from '@lucide/svelte';
  import Button from './Button.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import Toggle from './Toggle.svelte';
  import { addToSteamPrompt, deleteProfilePrompt } from '../confirm/prompts';
  import { errorCode, hasMessage, msg } from '../i18n';
  import { installErrorText } from '../install/installErrors';
  import {
    addProfile,
    addProfileToSteam,
    createProfileShortcut,
    getProfiles,
    profileSteamStatus,
    removeProfile,
    removeProfileFromSteam,
    selectGameExecutable,
    updateProfile,
    type LibraryGame,
    type ProfileView,
    type SteamStatus,
  } from '../services/library';
  import type { Candidate } from '../services/install';
  import { toast } from '../stores/toasts';
  import { truncateMiddle } from '../utils/format';

  let {
    game,
    candidates,
    adminShown,
  }: {
    game: LibraryGame;
    candidates: Candidate[];
    adminShown: boolean;
  } = $props();

  interface Editor {
    id: string | null;
    name: string;
    executable: string;
    argsLine: string;
    admin: boolean;
  }

  let profiles = $state<ProfileView[]>([]);
  let steam = $state<Record<string, SteamStatus>>({});
  let loading = $state(false);
  let loadError = $state('');
  let editor = $state<Editor | null>(null);
  let editorError = $state('');
  let busy = $state(false);
  let rowError = $state('');
  let deleting = $state<ProfileView | null>(null);
  let steamAdding = $state<string | null>(null);
  let seq = 0;

  const signature = $derived(
    JSON.stringify([game.id, game.profiles, game.lastProfile, game.executable, game.launchArgs, game.runAsAdmin, game.shortcutPath]),
  );

  function errorText(err: unknown, fallback: string): string {
    const code = errorCode(err);
    if (hasMessage(code)) return msg(code);
    return installErrorText(err, fallback);
  }

  $effect(() => {
    void signature;
    untrack(() => void load(game.id));
  });

  async function load(gameId: string) {
    const mine = ++seq;
    loading = true;
    loadError = '';
    let list: ProfileView[];
    try {
      list = await getProfiles(gameId);
    } catch (err) {
      if (mine !== seq) return;
      loading = false;
      loadError = errorText(err, msg('modals.profilesLoadFailed'));
      return;
    }
    if (mine !== seq) return;
    profiles = list;
    loading = false;
    const states = await Promise.allSettled(list.map((p) => profileSteamStatus(gameId, p.id)));
    if (mine !== seq) return;
    const next: Record<string, SteamStatus> = {};
    states.forEach((state, i) => {
      if (state.status === 'fulfilled') next[list[i].id] = state.value;
      else loadError = errorText(state.reason, msg('games.steamFailed'));
    });
    steam = next;
  }

  function startAdd() {
    editorError = '';
    editor = { id: null, name: '', executable: game.executable, argsLine: '', admin: false };
  }

  function startEdit(p: ProfileView) {
    editorError = '';
    editor = { id: p.id, name: p.name, executable: p.executable, argsLine: p.argsLine, admin: p.runAsAdmin };
  }

  async function browse() {
    if (!editor || busy) return;
    editorError = '';
    try {
      const path = await selectGameExecutable(
        msg('modals.profilesExeDialog', { title: game.title }),
        game.installDir,
        editor.executable,
      );
      if (path && editor) editor.executable = path;
    } catch (err) {
      editorError = errorText(err, msg('modals.profilesSaveFailed'));
    }
  }

  async function save() {
    if (!editor || busy) return;
    const current = editor;
    busy = true;
    editorError = '';
    const input = {
      name: current.name.trim(),
      executable: current.executable,
      argsLine: current.argsLine,
      runAsAdmin: current.admin,
    };
    try {
      if (current.id === null) await addProfile(game.id, input);
      else await updateProfile(game.id, current.id, input);
      editor = null;
    } catch (err) {
      editorError = errorText(err, msg('modals.profilesSaveFailed'));
    } finally {
      busy = false;
    }
  }

  async function confirmDelete() {
    const target = deleting;
    if (!target) return;
    try {
      await removeProfile(game.id, target.id);
      toast(msg('modals.profilesDeleted'), 'success');
    } catch (err) {
      toast(errorText(err, msg('modals.profilesDeleteFailed')), 'danger');
    }
  }

  async function shortcut(p: ProfileView) {
    if (busy) return;
    busy = true;
    rowError = '';
    try {
      await createProfileShortcut(game.id, p.id);
      toast(msg('modals.profilesShortcutDone'), 'success');
    } catch (err) {
      rowError = errorText(err, msg('modals.profilesShortcutFailed'));
      toast(rowError, 'danger');
    } finally {
      busy = false;
    }
  }

  async function steamToggle(p: ProfileView, added: boolean) {
    if (busy) return;
    busy = true;
    rowError = '';
    try {
      if (added) {
        await removeProfileFromSteam(game.id, p.id);
        toast(msg('games.steamRemoved'), 'success');
      } else {
        await addProfileToSteam(game.id, p.id);
        toast(msg('games.steamAdded'), 'success');
      }
      steam = { ...steam, [p.id]: { ...steam[p.id], added: !added } };
    } catch (err) {
      rowError = errorText(err, msg('games.steamFailed'));
      toast(rowError, 'danger');
    } finally {
      busy = false;
    }
  }

  const canSave = $derived(Boolean(editor && editor.name.trim() !== '' && editor.executable !== ''));
</script>

<section class="section">
  <div class="row spread">
    <h4>{msg('modals.profilesTitle')}</h4>
    <Button size="sm" disabled={loading || busy || editor !== null} onclick={startAdd}>
      <Plus size="1.5rem" strokeWidth={1.8} />
      {msg('modals.profilesAdd')}
    </Button>
  </div>
  <p class="hint">{msg('modals.profilesHint')}</p>

  {#if loadError}
    <p class="failure">{loadError}</p>
  {/if}
  {#if rowError}
    <p class="failure">{rowError}</p>
  {/if}

  <ul class="list">
    {#each profiles as p (p.id)}
      {@const status = steam[p.id]}
      <li class="item">
        <div class="head">
          <span class="name">{p.isDefault ? msg('games.profileDefault') : p.name}</span>
          {#if p.isLast}<span class="tag">{msg('modals.profilesLast')}</span>{/if}
          {#if p.runAsAdmin}<span class="tag">{msg('modals.propertiesAdminTitle')}</span>{/if}
          {#if !p.isDefault}
            <span class="grow"></span>
            <button class="icon" title={msg('modals.profilesEdit')} aria-label={msg('modals.profilesEdit')} disabled={busy || editor !== null} onclick={() => startEdit(p)}>
              <Pencil size="1.5rem" strokeWidth={1.8} />
            </button>
            <button class="icon" title={msg('modals.profilesDelete')} aria-label={msg('modals.profilesDelete')} disabled={busy} onclick={() => (deleting = p)}>
              <Trash2 size="1.5rem" strokeWidth={1.8} />
            </button>
          {/if}
        </div>
        <p class="path" title={p.executable}>{truncateMiddle(p.executable, 70)}</p>
        {#if p.argsLine}<p class="hint">{p.argsLine}</p>{/if}
        {#if p.isDefault}<p class="hint">{msg('modals.profilesDefaultNote')}</p>{/if}
        <div class="row">
          {#if p.shortcutPath}
            <span class="hint">{msg('modals.profilesShortcutDone')}</span>
          {:else}
            <Button size="sm" variant="ghost" disabled={busy} onclick={() => shortcut(p)}>
              {msg('modals.profilesShortcut')}
            </Button>
          {/if}
          {#if status && status.supported && status.installed}
            <Button
              size="sm"
              variant="ghost"
              disabled={busy}
              onclick={() => (status.added ? steamToggle(p, true) : (steamAdding = p.id))}
            >
              {status.added ? msg('modals.profilesSteamRemove') : msg('modals.profilesSteamAdd')}
            </Button>
          {/if}
        </div>
      </li>
    {/each}
  </ul>

  {#if editor}
    <div class="editor">
      <label class="field">
        <span class="label">{msg('modals.profilesName')}</span>
        <input
          class="input sm"
          type="text"
          maxlength="40"
          spellcheck="false"
          autocomplete="off"
          placeholder={msg('modals.profilesNamePlaceholder')}
          bind:value={editor.name}
        />
      </label>

      <div class="field">
        <span class="label">{msg('modals.profilesExe')}</span>
        <p class="path">{editor.executable || msg('modals.profilesExeNone')}</p>
        {#if candidates.length > 0}
          <div class="candidates">
            {#each candidates as candidate (candidate.path)}
              <button
                class="candidate"
                class:current={candidate.path === editor.executable}
                title={candidate.path}
                onclick={() => editor && (editor.executable = candidate.path)}
              >
                {truncateMiddle(candidate.path, 70)}
              </button>
            {/each}
          </div>
        {/if}
        <div>
          <Button size="sm" disabled={busy} onclick={browse}>
            <FolderOpen size="1.5rem" strokeWidth={1.8} />
            {msg('modals.profilesExeBrowse')}
          </Button>
        </div>
      </div>

      <label class="field">
        <span class="label">{msg('modals.profilesArgs')}</span>
        <input
          class="input sm"
          type="text"
          spellcheck="false"
          autocomplete="off"
          placeholder={msg('modals.propertiesArgsPlaceholder')}
          bind:value={editor.argsLine}
        />
      </label>

      {#if adminShown}
        <div class="row spread">
          <span class="label">{msg('modals.propertiesAdminTitle')}</span>
          <Toggle bind:checked={editor.admin} label={msg('modals.propertiesAdminTitle')} />
        </div>
      {/if}

      {#if editorError}
        <p class="failure">{editorError}</p>
      {/if}

      <div class="row">
        <Button size="sm" variant="primary" disabled={busy || !canSave} onclick={save}>
          {msg('modals.profilesSave')}
        </Button>
        <Button size="sm" variant="ghost" disabled={busy} onclick={() => (editor = null)}>
          {msg('common.cancel')}
        </Button>
      </div>
    </div>
  {/if}
</section>

{#if deleting}
  <ConfirmModal prompt={deleteProfilePrompt(deleting.name)} onconfirm={confirmDelete} onclose={() => (deleting = null)} />
{/if}

{#if steamAdding !== null}
  {@const target = profiles.find((p) => p.id === steamAdding)}
  {#if target}
    <ConfirmModal
      prompt={addToSteamPrompt()}
      onconfirm={() => steamToggle(target, false)}
      onclose={() => (steamAdding = null)}
    />
  {/if}
{/if}

<style>
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

  .spread {
    justify-content: space-between;
  }

  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .item {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
  }

  .head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .name {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .tag {
    font-size: var(--font-xs);
    color: var(--accent);
  }

  .grow {
    flex: 1;
  }

  .icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-1);
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-3);
    cursor: pointer;
  }

  .icon:hover:not(:disabled) {
    color: var(--text);
  }

  .icon:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
    border: 1px solid var(--accent);
    border-radius: var(--radius-md);
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .candidates {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    max-height: 12rem;
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

  .candidate:hover {
    border-color: var(--border-strong);
    color: var(--text);
  }

  .candidate.current {
    border-color: var(--accent);
    color: var(--text);
  }
</style>
