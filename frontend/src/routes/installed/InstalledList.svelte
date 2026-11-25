<script lang="ts">
  import { EllipsisVertical, FolderOpen, Gamepad2, Square } from '@lucide/svelte';
  import Artwork from '../../lib/components/Artwork.svelte';
  import Button from '../../lib/components/Button.svelte';
  import ConsoleTag from '../../lib/components/ConsoleTag.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import GameLink from '../../lib/components/GameLink.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import type { MenuItem } from '../../lib/components/menu';
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
  import { activeMoves, holdingMove, openMoveGame } from '../../lib/stores/relocate';
  import { toast } from '../../lib/stores/toasts';
  import { updatesByGame } from '../../lib/stores/updates';
  import { brokenStatus } from '../../lib/game/compat';
  import type { CompatStatus } from '../../lib/services/compat';
  import { relativeDate } from '../../lib/utils/format';
  import { msg } from '../../lib/i18n';
  import { heroFor, libraryErrorText, play, sizeLabel, stop, updateLabel } from './installedGame';

  // The list view: one row per game with its status, its main action, its folder and its menu.
  // Removing a game is asked of the page, which owns the dialog. The main action is one control
  // whose words and deed follow the game's state, run through the page's `act` (busy until Go answers).
  let {
    games,
    compat,
    busy,
    act,
    onremove,
  }: {
    games: LibraryGame[];
    compat: Map<string, CompatStatus>;
    busy: (id: string) => boolean;
    act: (id: string, run: () => Promise<void>) => void;
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
    } else if (action === 'move') {
      openMoveGame(game.id);
    } else if (action === 'uninstall' || action === 'remove') {
      onremove(game, action === 'uninstall' ? 'disk' : 'library');
    }
  }

  // A move of this game, or of the whole library, is running: Go refuses another move and a removal.
  function moving(game: LibraryGame): boolean {
    return holdingMove($activeMoves, game.id) !== null;
  }

  // Go moves neither an emulated game nor one of another launcher (library.CheckRelocatable,
  // relocate.gameSource), nor a running one; the game menu offers Move by the same rule.
  function menuItems(game: LibraryGame, running: boolean): MenuItem[] {
    const own = !isExternal(game) && !isEmulated(game);
    const items: MenuItem[] = [{ id: 'folder', label: msg('games.openFolder') }];
    if (own) items.push({ id: 'executable', label: msg('games.installedChooseExeLabel') });
    if (own && !running) items.push({ id: 'move', label: msg('games.actionMove') });
    const disk = own || isInstalledImage(game);
    if (disk) items.push({ id: 'uninstall', label: msg('games.actionUninstall'), danger: true, separator: true });
    items.push({ id: 'remove', label: msg('games.actionRemoveLibrary'), danger: true, separator: !disk });
    if (!moving(game)) return items;
    return items.map((item) =>
      item.id === 'move' || item.id === 'uninstall' || item.id === 'remove'
        ? { ...item, disabled: true, label: msg('installed.menuMoving', { action: item.label }) }
        : item,
    );
  }

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

  type Main = { kind: 'stop' | 'image' | 'file' | 'play'; label: string; run: () => Promise<void> };

  function mainAction(game: LibraryGame, running: boolean): Main {
    if (running) return { kind: 'stop', label: msg('games.installedStopButton'), run: () => stop(game) };
    if ($missingImages.has(game.id)) return { kind: 'image', label: msg('emul.pickImage'), run: () => chooseImage(game) };
    if (!isLaunchable(game)) return { kind: 'file', label: msg('games.installedSetExeButton'), run: () => chooseExecutable(game) };
    return { kind: 'play', label: msg('games.play'), run: () => play(game) };
  }

  function statusKind(game: LibraryGame, running: boolean, updateKind?: string): 'success' | 'warning' | 'accent' | 'neutral' {
    if (running) return 'accent';
    if (moving(game)) return 'accent';
    if ($missingImages.has(game.id)) return 'warning';
    if (!isLaunchable(game)) return 'warning';
    if (updateKind === 'update') return 'warning';
    if (updateKind === 'new_release') return 'neutral';
    return 'success';
  }

  function statusLabel(game: LibraryGame, running: boolean) {
    if (running) return msg('games.runningLabel');
    if (moving(game)) return msg('installed.moving');
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
    {@const main = mainAction(game, running)}
    <div class="row" role="presentation" data-game-row={game.id} {@attach gameContextMenu(game.id)}>
      <div class="game">
        <div class="thumb">
          <GameLink id={game.id} name={game.title} repeat>
            <Artwork src={heroFor(game, $gameArt)} alt="" label={game.title} radius="var(--radius-sm)" />
          </GameLink>
        </div>
        <div class="titles">
          <span class="title-line">
            <span class="title"><GameLink id={game.id} name={game.title} truncate /></span>
            {#if isEmulated(game)}<ConsoleTag platform={emulationPlatform(game)} />{/if}
          </span>
          <span class="path">{game.installDir}</span>
          {#if sizeLabel(game) || updateText || moving(game)}
            <span class="size">
              {sizeLabel(game)}
              {#if moving(game)}
                <span class="row-moving">
                  <StatusBadge kind="accent" label={msg('installed.moving')} plain />
                </span>
              {/if}
              {#if updateText}
                <span class="row-update">
                  <StatusBadge kind={update?.kind === 'update' ? 'warning' : 'neutral'} label={updateText} plain />
                </span>
              {/if}
            </span>
          {/if}
        </div>
      </div>
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
        <Button size="sm" variant={main.kind === 'play' || main.kind === 'stop' ? 'play' : 'secondary'} busy={busy(game.id)} onclick={() => act(game.id, main.run)}>
          {#if main.kind === 'stop'}
            <Square size="1.2rem" strokeWidth={2} fill="currentColor" />
          {:else if main.kind === 'play'}
            <Gamepad2 size="1.3rem" strokeWidth={2} />
          {:else}
            <FolderOpen size="1.3rem" strokeWidth={1.8} />
          {/if}
          {main.label}
        </Button>
        <Button size="sm" onclick={() => openInstallDir(game)}>
          <FolderOpen size="1.3rem" strokeWidth={1.8} />
          {msg('games.openFolder')}
        </Button>
        <DropdownMenu items={menuItems(game, running)} onselect={(id) => onMenu(game, id)}>
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
    gap: var(--space-1);
    min-width: 0;
  }

  .title-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .title {
    display: flex;
    min-width: 0;
    font-size: var(--font-lg);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
  }

  /* A path is shown whole: it wraps at any character rather than being cut. */
  .path {
    font-size: var(--font-xs);
    color: var(--text-3);
    overflow-wrap: anywhere;
  }

  .size {
    font-size: var(--font-xs);
    color: var(--text-3);
    font-variant-numeric: tabular-nums;
  }

  .status {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .status-label {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    justify-self: end;
  }

  .row-update {
    display: inline-flex;
    margin-left: var(--space-2);
    vertical-align: bottom;
  }

  /* The status column says "Moving" where it is shown; the size line says it where it is not. */
  .row-moving {
    display: none;
    margin-left: var(--space-2);
    vertical-align: bottom;
  }

  @media (max-width: 1400px) {
    .row {
      grid-template-columns: minmax(22rem, 1fr) auto;
    }

    .status {
      display: none;
    }

    .row-moving {
      display: inline-flex;
    }
  }
</style>
