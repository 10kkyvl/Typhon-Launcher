<script lang="ts">
  import { EllipsisVertical, FolderOpen, Gamepad2, Square } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { selectGameExecutable, setExecutable, type LibraryGame } from '../../lib/services/library';
  import { openGameFolder } from '../../lib/services/settings';
  import { runningGames } from '../../lib/stores/library';
  import { emulationPlatform, isEmulated, isInstalledImage } from '../../lib/library/emulation';
  import { selectImageFile } from '../../lib/services/emulators';
  import { missingImages, refreshMissingImages } from '../../lib/stores/missingImages';
  import { isExternal, isLaunchable } from '../../lib/library/external';
  import { gameArt } from '../../lib/stores/metadata';
  import { gameContextMenu } from '../../lib/stores/gameMenu';
  import { navigate } from '../../lib/stores/router';
  import { toast } from '../../lib/stores/toasts';
  import { updatesByGame } from '../../lib/stores/updates';
  import { brokenStatus } from '../../lib/game/compat';
  import type { CompatStatus } from '../../lib/services/compat';
  import { relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import { heroFor, libraryErrorText, play, sizeLabel, stop, updateLabel } from './installedGame';

  // The list view: one row per game with its status, its main action, its folder and its menu.
  // Removing a game is asked of the page, which owns the dialog.
  let {
    games,
    compat,
    onremove,
  }: {
    games: LibraryGame[];
    compat: Map<string, CompatStatus>;
    onremove: (game: LibraryGame, mode: 'disk' | 'library') => void;
  } = $props();

  function brokenNote(game: LibraryGame): string {
    const status = brokenStatus(game, compat.get(game.id));
    if (!status) return '';
    const reason = libraryErrorText(status.lastError, '');
    return reason
      ? msg('games.compatBrokenWithReason', { reason })
      : msg('games.compatBroken');
  }

  async function chooseImage(game: LibraryGame) {
    try {
      const path = await selectImageFile(msg('emul.addImageDialog'), [emulationPlatform(game)]);
      if (!path) return;
      await setExecutable(game.id, path);
      await refreshMissingImages();
      toast(msg('emul.imageSaved'), 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.imageSaveFailed')), 'danger');
    }
  }

  async function openInstallDir(game: LibraryGame) {
    try {
      await openGameFolder(game.installDir, game.executable);
    } catch {
      toast(msg('games.errorFolderUnavailable'), 'danger');
    }
  }

  async function onMenu(game: LibraryGame, action: string) {
    if (action === 'folder') {
      await openInstallDir(game);
    } else if (action === 'executable') {
      await chooseExecutable(game);
    } else if (action === 'uninstall' || action === 'remove') {
      onremove(game, action === 'uninstall' ? 'disk' : 'library');
    }
  }

  const menuItems = [
    { id: 'folder', label: msg('games.openFolder') },
    { id: 'executable', label: msg('games.installedChooseExeLabel') },
    { id: 'uninstall', label: msg('games.actionUninstall'), danger: true, separator: true },
    { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true },
  ];

  const emulatedMenuItems = [
    { id: 'folder', label: msg('games.openFolder') },
    { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true, separator: true },
  ];

  const imageMenuItems = [
    { id: 'folder', label: msg('games.openFolder') },
    { id: 'uninstall', label: msg('games.actionUninstall'), danger: true, separator: true },
    { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true },
  ];

  const externalMenuItems = [
    { id: 'folder', label: msg('games.openFolder') },
    { id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true, separator: true },
  ];

  async function chooseExecutable(game: LibraryGame) {
    try {
      const path = await selectGameExecutable(
        msg('games.installedChooseExeDialog', { title: game.title }),
        game.installDir,
        game.executable,
      );
      if (!path) return;
      await setExecutable(game.id, path);
      toast(msg('games.installedExeSavedToast', { title: game.title }), 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('games.installedChooseExeError')), 'danger');
    }
  }

  function statusKind(game: LibraryGame, running: boolean, updateKind?: string): 'success' | 'warning' | 'accent' | 'neutral' {
    if (running) return 'accent';
    if ($missingImages.has(game.id)) return 'warning';
    if (!isLaunchable(game)) return 'warning';
    if (updateKind === 'update') return 'warning';
    if (updateKind === 'new_release') return 'neutral';
    return 'success';
  }

  function statusLabel(game: LibraryGame, running: boolean) {
    if (running) return msg('games.runningLabel');
    if ($missingImages.has(game.id)) return msg('emul.imageMissing');
    if (!isLaunchable(game)) return msg('games.installedNeedsExeLabel');
    return relativeDate(game.lastPlayed);
  }
</script>

<div class="list">
  {#each games as game (game.id)}
    {@const running = $runningGames.has(game.id)}
    {@const update = $updatesByGame.get(game.id)?.availability}
    {@const updateText = updateLabel(update)}
    <div class="row" role="presentation" data-game-row={game.id} {@attach gameContextMenu(game.id)}>
      <button class="game" onclick={() => navigate('game', { id: game.id })}>
        <div class="thumb" aria-hidden="true">
          <Artwork src={heroFor(game, $gameArt)} alt="" label={game.title} radius="var(--radius-sm)" />
        </div>
        <div class="titles">
          <span class="title-line">
            <span class="title">{game.title}</span>
            {#if isEmulated(game)}<ConsoleTag platform={emulationPlatform(game)} />{/if}
          </span>
          <span class="path">{game.installDir}</span>
          {#if sizeLabel(game) || updateText}
            <span class="size">
              {sizeLabel(game)}
              {#if updateText}
                <span class="row-update">
                  <StatusBadge kind={update?.kind === 'update' ? 'warning' : 'neutral'} label={updateText} plain />
                </span>
              {/if}
            </span>
          {/if}
        </div>
      </button>
      <div class="status">
        <span class="status-label">{msg('games.lastPlayedLabel')}</span>
        <StatusBadge kind={statusKind(game, running, update?.kind)} label={statusLabel(game, running)} plain />
        {#if brokenNote(game)}
          <span class="compat">
            <StatusBadge kind="danger" label={msg('games.compatBrokenBadge')} hint={brokenNote(game)} plain />
          </span>
        {/if}
      </div>
      <div class="actions">
        {#if running}
          <Button size="sm" onclick={() => stop(game)}>
            <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
            {msg('games.installedStopButton')}
          </Button>
        {:else if $missingImages.has(game.id)}
          <Button size="sm" onclick={() => chooseImage(game)}>
            <FolderOpen size="1.3rem" strokeWidth={1.8} />
            {msg('emul.pickImage')}
          </Button>
        {:else if !isLaunchable(game)}
          <Button size="sm" onclick={() => chooseExecutable(game)}>
            <FolderOpen size="1.3rem" strokeWidth={1.8} />
            {msg('games.installedSetExeButton')}
          </Button>
        {:else}
          <Button variant="primary" size="sm" onclick={() => play(game)}>
            <Gamepad2 size="1.3rem" strokeWidth={2} />
            {msg('games.play')}
          </Button>
        {/if}
        <Button size="sm" onclick={() => openInstallDir(game)}>
          <FolderOpen size="1.3rem" strokeWidth={1.8} />
          {msg('games.openFolder')}
        </Button>
        <DropdownMenu items={isExternal(game) ? externalMenuItems : isInstalledImage(game) ? imageMenuItems : isEmulated(game) ? emulatedMenuItems : menuItems} onselect={(id) => onMenu(game, id)}>
          {#snippet trigger({ props })}
            <IconButton {...props} label={msg('games.installedMenuFor', { title: game.title })} size="sm">
              <EllipsisVertical size="1.6rem" strokeWidth={1.8} />
            </IconButton>
          {/snippet}
        </DropdownMenu>
      </div>
    </div>
  {/each}
</div>

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .row {
    display: grid;
    grid-template-columns: minmax(30rem, 1fr) 17rem auto;
    align-items: center;
    gap: var(--space-5);
    padding: var(--space-4) var(--space-5);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
  }

  .game {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    min-width: 0;
    text-align: left;
  }

  .thumb {
    width: 10.4rem;
    height: 5.8rem;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .titles {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
    min-width: 0;
  }

  .title-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .title {
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .path {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .size {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .status {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    min-width: 0;
  }

  .status-label {
    font-size: 1.2rem;
    color: var(--text-3);
    white-space: nowrap;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    justify-self: end;
  }

  .row-update {
    display: inline-flex;
    margin-left: 0.8rem;
    vertical-align: bottom;
  }

  @media (max-width: 1400px) {
    .row {
      grid-template-columns: minmax(22rem, 1fr) auto;
    }

    .status {
      display: none;
    }
  }
</style>
