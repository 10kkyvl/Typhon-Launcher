<script lang="ts">
  import ContextMenu from './ContextMenu.svelte';
  import GameStatusModal from './GameStatusModal.svelte';
  import GameTagsModal from './GameTagsModal.svelte';
  import RemoveGameModal from './RemoveGameModal.svelte';
  import SavesCandidatesModal from './SavesCandidatesModal.svelte';
  import ConfirmModal from './ConfirmModal.svelte';
  import { quickActions, steamEligible, type QuickAction } from '../game/quickActions';
  import { addToSteamPrompt } from '../confirm/prompts';
  import {
    addToSteam,
    createShortcut,
    locateSaves,
    markError,
    playGame,
    removeFromSteam,
    removeShortcut,
    setFavorite,
    setHidden,
    setSavesDir,
    steamStatus,
    stopGame,
    type LibraryGame,
    type SavesResult,
  } from '../services/library';
  import { openGameFolder, openFolder } from '../services/settings';
  import { openMoveGame } from '../game/actions/move';
  import { chooseSavesDir } from '../game/actions/saves';
  import { share as shareLan, shares, unshare as unshareLan } from '../stores/lan';
  import { settings } from '../stores/settings';
  import { closeGameMenu, gameMenu } from '../stores/gameMenu';
  import { libraryGames, runningGames } from '../stores/library';
  import { navigate } from '../stores/router';
  import { openGameProperties } from '../stores/gameProperties';
  import { openRedist } from '../stores/redist';
  import { openSaveBackups, saveBackupsEnabled } from '../stores/savebackup';
  import { get } from 'svelte/store';
  import { toast } from '../stores/toasts';
  import { verify } from '../stores/updates';
  import { libraryErrorText } from '../library/errors';
  import { externalLabel, isExternal, isLaunchable } from '../library/external';
  import { errorMessage } from '../utils/errors';
  import { msg } from '../i18n';

  const game = $derived($gameMenu ? ($libraryGames.find((g) => g.id === $gameMenu?.gameId) ?? null) : null);

  let steam = $state<{ id: string; added: boolean } | null>(null);
  let steamConfirmID = $state<string | null>(null);

  $effect(() => {
    const menu = $gameMenu;
    steam = null;
    if (!menu) return;
    const id = menu.gameId;
    const current = get(libraryGames).find((g) => g.id === id);
    if (
      !current ||
      !steamEligible({
        installed: !current.uninstalled,
        hasExecutable: Boolean(current.executable),
        fromSteam: current.external?.provider === 'steam',
      })
    ) {
      return;
    }
    let stale = false;
    steamStatus(id).then(
      (status) => {
        if (!stale && status.supported && status.installed) steam = { id, added: status.added };
      },
      () => {
        // The menu opens often; a failed probe only hides the item instead of nagging on every open.
      },
    );
    return () => {
      stale = true;
    };
  });

  const items = $derived(
    game
      ? quickActions({
          installed: !game.uninstalled,
          running: $runningGames.has(game.id),
          hasExecutable: Boolean(game.executable),
          launchable: isLaunchable(game),
          external: isExternal(game),
          hasShortcut: Boolean(game.shortcutPath),
          lanEnabled: Boolean($settings?.lanSharing),
          lanShared: $shares.some((s) => s.gameId === game.id),
          favorite: Boolean(game.favorite),
          status: game.status ?? '',
          hidden: Boolean(game.hidden),
          saveBackups: $saveBackupsEnabled,
          fromSteam: game.external?.provider === 'steam',
          steam: steam && steam.id === game.id ? { added: steam.added } : null,
        })
      : [],
  );

  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('disk');
  let target = $state<LibraryGame | null>(null);

  let savesOpen = $state(false);
  let savesCandidates = $state<string[]>([]);

  let statusOpen = $state(false);
  let statusID = $state<string | null>(null);
  const statusGame = $derived(statusID ? ($libraryGames.find((g) => g.id === statusID) ?? null) : null);

  let tagsOpen = $state(false);
  let tagsID = $state<string | null>(null);
  const tagsGame = $derived(tagsID ? ($libraryGames.find((g) => g.id === tagsID) ?? null) : null);

  function run(current: LibraryGame, action: QuickAction) {
    switch (action) {
      case 'play':
        return guard(() => playGame(current.id));
      case 'stop':
        return guard(() => stopGame(current.id));
      case 'favorite-add':
      case 'favorite-remove':
        return mark(() => setFavorite(current.id, action === 'favorite-add'), msg('ui.favoriteChangeFailed'));
      case 'status':
        statusID = current.id;
        statusOpen = true;
        return;
      case 'tags':
        tagsID = current.id;
        tagsOpen = true;
        return;
      case 'hide':
      case 'unhide':
        return toggleHidden(current.id, action === 'hide');
      case 'folder':
        return guard(() => openGameFolder(current.installDir, current.executable));
      case 'saves':
        return openSaves(current);
      case 'saves-backups':
        openSaveBackups(current.id);
        return;
      case 'verify':
        navigate('game', { id: current.id });
        return verify(current.id);
      case 'redist':
        openRedist(current.id);
        return;
      case 'move':
        openMoveGame(current.id);
        return;
      case 'properties':
        openGameProperties(current.id);
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
      case 'steam-add':
        steamConfirmID = current.id;
        return;
      case 'steam-remove':
        return steamAction(() => removeFromSteam(current.id), msg('games.steamRemoved'));
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

  async function steamAction(fn: () => Promise<unknown>, done: string) {
    try {
      await fn();
      toast(done, 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('games.steamFailed')), 'danger');
    }
  }

  async function confirmSteamAdd() {
    const id = steamConfirmID;
    if (!id) return;
    await steamAction(() => addToSteam(id), msg('games.steamAdded'));
  }

  async function mark(fn: () => Promise<unknown>, fallback: string) {
    try {
      await fn();
    } catch (err) {
      toast(markError(err, fallback), 'danger');
    }
  }

  async function toggleHidden(id: string, on: boolean) {
    try {
      await setHidden(id, on);
    } catch (err) {
      toast(libraryErrorText(err, msg('games.hideFailed')), 'danger');
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
        ? msg('ui.savesPartialUnreadable')
        : msg('ui.savesNotFound'),
    );
    await pickSaves(current);
  }

  async function pickSaves(current: LibraryGame) {
    await guard(async () => {
      const dir = await chooseSavesDir(current.id, current.title);
      if (dir) await openFolder(dir);
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

{#if steamConfirmID}
  <ConfirmModal prompt={addToSteamPrompt()} onconfirm={confirmSteamAdd} onclose={() => (steamConfirmID = null)} />
{/if}

{#if statusGame && statusOpen}
  <GameStatusModal bind:open={statusOpen} game={statusGame} />
{/if}

{#if tagsGame && tagsOpen}
  <GameTagsModal bind:open={tagsOpen} game={tagsGame} />
{/if}

{#if target}
  <RemoveGameModal bind:open={removeOpen} bind:mode={removeMode} gameId={target.id} title={target.title} external={externalLabel(target)} />

  <SavesCandidatesModal
    bind:open={savesOpen}
    title={target.title}
    candidates={savesCandidates}
    onpick={useCandidate}
    onbrowse={() => {
      const current = target;
      savesOpen = false;
      if (current) void pickSaves(current);
    }}
  />
{/if}
