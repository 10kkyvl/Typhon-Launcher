<script lang="ts">
  import Button from './Button.svelte';
  import ContextMenu from './ContextMenu.svelte';
  import Modal from './Modal.svelte';
  import RemoveGameModal from './RemoveGameModal.svelte';
  import { quickActions, type QuickAction } from '../game/quickActions';
  import {
    createShortcut,
    locateSaves,
    playGame,
    removeShortcut,
    setSavesDir,
    stopGame,
    type LibraryGame,
    type SavesResult,
  } from '../services/library';
  import { openFolder, selectFolder } from '../services/settings';
  import { openMoveGame } from '../game/actions/move';
  import { share as shareLan, shares, unshare as unshareLan } from '../stores/lan';
  import { settings } from '../stores/settings';
  import { closeGameMenu, gameMenu } from '../stores/gameMenu';
  import { libraryGames, runningGames } from '../stores/library';
  import { navigate } from '../stores/router';
  import { toast } from '../stores/toasts';
  import { verify } from '../stores/updates';
  import { errorMessage } from '../utils/errors';
  import { truncateMiddle } from '../utils/format';

  const game = $derived($gameMenu ? ($libraryGames.find((g) => g.id === $gameMenu?.gameId) ?? null) : null);

  const items = $derived(
    game
      ? quickActions({
          installed: !game.uninstalled,
          running: $runningGames.has(game.id),
          hasExecutable: Boolean(game.executable),
          hasShortcut: Boolean(game.shortcutPath),
          lanEnabled: Boolean($settings?.lanSharing),
          lanShared: $shares.some((s) => s.gameId === game.id),
        })
      : [],
  );

  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('disk');
  let target = $state<LibraryGame | null>(null);

  let savesOpen = $state(false);
  let savesCandidates = $state<string[]>([]);

  function run(current: LibraryGame, action: QuickAction) {
    switch (action) {
      case 'play':
        return guard(() => playGame(current.id));
      case 'stop':
        return guard(() => stopGame(current.id));
      case 'folder':
        return guard(() => openFolder(current.installDir));
      case 'saves':
        return openSaves(current);
      case 'verify':
        navigate('game', { id: current.id });
        return verify(current.id);
      case 'move':
        openMoveGame(current.id);
        return;
      case 'lan-share':
        shareLan(current.id);
        return;
      case 'lan-unshare':
        return guard(() => unshareLan(current.id));
      case 'shortcut-create':
        return guard(() => createShortcut(current.id));
      case 'shortcut-remove':
        return guard(() => removeShortcut(current.id));
      case 'uninstall':
      case 'remove':
        target = current;
        removeMode = action === 'uninstall' ? 'disk' : 'library';
        removeOpen = true;
        return;
    }
  }

  async function guard(action: () => Promise<unknown>) {
    try {
      await action();
    } catch (err) {
      toast(errorMessage(err), 'danger');
    }
  }

  async function openSaves(current: LibraryGame) {
    let found: SavesResult;
    try {
      found = await locateSaves(current.id);
    } catch (err) {
      toast(errorMessage(err), 'danger');
      return;
    }
    const { path, candidates, unreadable } = found;
    if (path) {
      await guard(() => openFolder(path));
      return;
    }
    if (candidates && candidates.length > 0) {
      target = current;
      savesCandidates = candidates;
      savesOpen = true;
      return;
    }
    toast(
      unreadable > 0
        ? 'Часть папок прочитать не удалось. Укажите папку сохранений вручную'
        : 'Папка сохранений не найдена. Укажите её вручную',
    );
    await pickSaves(current);
  }

  async function pickSaves(current: LibraryGame) {
    await guard(async () => {
      const dir = await selectFolder(`Папка сохранений — ${current.title}`);
      if (!dir) return;
      await setSavesDir(current.id, dir);
      await openFolder(dir);
    });
  }

  async function useCandidate(dir: string) {
    if (!target) return;
    const current = target;
    savesOpen = false;
    await guard(async () => {
      await setSavesDir(current.id, dir);
      await openFolder(dir);
    });
  }

  function onSelect(id: string) {
    if (!game) return;
    void run(game, id as QuickAction);
  }
</script>

{#if $gameMenu && game && items.length > 0}
  <ContextMenu
    items={items}
    x={$gameMenu.x}
    y={$gameMenu.y}
    onselect={onSelect}
    onclose={closeGameMenu}
  />
{/if}

{#if target}
  <RemoveGameModal bind:open={removeOpen} bind:mode={removeMode} gameId={target.id} title={target.title} />

  <Modal bind:open={savesOpen} title="Папка сохранений" width="52rem">
    <p class="hint">Подходящих папок нашлось несколько. Выберите ту, что относится к «{target.title}».</p>
    <div class="candidates">
      {#each savesCandidates as candidate (candidate)}
        <button class="candidate" onclick={() => useCandidate(candidate)} title={candidate}>
          {truncateMiddle(candidate, 70)}
        </button>
      {/each}
    </div>
    {#snippet footer()}
      <Button onclick={() => (savesOpen = false)}>Отмена</Button>
      <Button
        variant="primary"
        onclick={() => {
          const current = target;
          savesOpen = false;
          if (current) void pickSaves(current);
        }}
      >
        Указать другую
      </Button>
    {/snippet}
  </Modal>
{/if}

<style>
  .hint {
    margin-bottom: var(--space-4);
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .candidates {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
  }

  .candidate {
    padding: 0.9rem 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    font-size: var(--font-sm);
    color: var(--text-2);
    text-align: left;
    transition:
      background var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease),
      color var(--dur-fast) var(--ease);
  }

  .candidate:hover {
    background: var(--hover-strong);
    border-color: var(--border-strong);
    color: var(--text);
  }
</style>
