<script lang="ts">
  import { FolderOpen } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import EmulatorHint from '../../lib/components/EmulatorHint.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import VerifyCard from '../../lib/components/VerifyCard.svelte';
  import type { GamePageData } from './gamePageData.svelte';
  import { facts } from '../../lib/game/view';
  import { msg } from '../../lib/i18n';
  import { isInstalledImage, platformBadge } from '../../lib/library/emulation';
  import type { LibraryGame } from '../../lib/services/library';
  import { openGameFolder } from '../../lib/services/settings';
  import type { VerifyState } from '../../lib/services/updates';
  import { openRedist } from '../../lib/stores/redist';
  import { toast } from '../../lib/stores/toasts';
  import { bytesSize, playtime, relativeDate, truncateMiddle } from '../../lib/utils/format';

  /** The install block of an installed game: verification, where it lives, its facts and its settings. */
  let {
    data,
    uid,
    localGame,
    installed,
    managed,
    external,
    externalName,
    emulated,
    emulatedPlatform,
    imageMissing,
    verifyState,
    running,
  }: {
    data: GamePageData;
    uid: string;
    localGame: LibraryGame;
    installed: boolean;
    managed: boolean;
    external: boolean;
    externalName: string;
    emulated: boolean;
    emulatedPlatform: string;
    imageMissing: boolean;
    verifyState: VerifyState | undefined;
    running: boolean;
  } = $props();

  const installFacts = $derived.by(() => {
    if (!localGame || !installed) return [];
    const exe = localGame.executable.split(/[\\/]/).pop() ?? '';
    return facts([
      { label: msg('games.detailFactVersion'), value: localGame.version },
      { label: msg('games.detailFactSize'), value: localGame.sizeBytes > 0 ? bytesSize(localGame.sizeBytes) : '' },
      { label: emulated ? msg('emul.factImage') : msg('games.executableLabel'), value: exe, full: localGame.executable, mono: true },
      { label: msg('games.detailFactPlaytime'), value: playtime(localGame.playtimeSeconds) },
      { label: msg('games.lastPlayedLabel'), value: relativeDate(localGame.lastPlayed) },
      { label: msg('games.detailFactAdded'), value: relativeDate(localGame.installedAt) },
    ]);
  });

  async function reveal() {
    if (!localGame) return;
    try {
      await openGameFolder(localGame.installDir, localGame.executable);
    } catch {
      toast(msg('games.errorFolderUnavailable'), 'danger');
    }
  }
</script>

{#if managed}
  <section class="section">
    <VerifyCard gameId={localGame.id} state={verifyState} {running} />
  </section>
{/if}

<section class="section">
  <h2 class="heading">{msg('games.detailInstallHeading')}</h2>
  {#if external}
    <p class="muted">{msg('games.externalNote', { provider: externalName })}</p>
  {:else if emulated}
    <p class="muted">{msg(isInstalledImage(localGame) ? 'emul.gameNoteOwned' : 'emul.gameNote', { console: platformBadge(emulatedPlatform) })}</p>
    <EmulatorHint platform={emulatedPlatform} game={localGame} />
  {/if}
  <div class="path">
    <span class="path-value" title={localGame.installDir}>{truncateMiddle(localGame.installDir, 34)}</span>
    <IconButton label={msg('games.openFolder')} size="sm" onclick={reveal}>
      <FolderOpen size="1.6rem" strokeWidth={1.8} />
    </IconButton>
  </div>
  <dl class="facts">
    {#each installFacts as fact (fact.label)}
      <div class="fact">
        <dt>{fact.label}</dt>
        <dd class:mono={fact.mono} title={fact.full ?? undefined}>{fact.value}</dd>
      </div>
    {/each}
  </dl>
  {#if emulated}
    <div class="redist-row">
      {#if imageMissing}
        <Button size="sm" disabled={data.imageBusy} onclick={data.chooseImage}>{msg('emul.pickImage')}</Button>
      {/if}
      <Button size="sm" disabled={data.imageBusy} onclick={data.findCover}>{msg('emul.coverFind')}</Button>
    </div>
  {:else}
    <div class="redist-row">
      <Button size="sm" onclick={() => openRedist(localGame.id)}>{msg('redist.action')}</Button>
    </div>
  {/if}
  {#if data.wineStatus?.required && !emulated}
    <div class="steam-row">
      <div class="steam-row-text">
        <span class="steam-row-label" id="{uid}-detail-steam-bottle-label">{msg('games.detailSteamBottleLabel')}</span>
        <span class="steam-row-hint" id="{uid}-detail-steam-bottle-sub">{msg('games.detailSteamBottleHint')}</span>
      </div>
      <Toggle
        checked={data.requiresSteam}
        labelledby="{uid}-detail-steam-bottle-label"
        describedby="{uid}-detail-steam-bottle-sub"
        disabled={data.requiresSteamSaving}
        onchange={(on) => {
          data.requiresSteam = on;
          void data.toggleRequiresSteam(on);
        }}
      />
    </div>
  {/if}
</section>

<style>
  .section {
    margin-top: var(--space-10);
  }

  .heading {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
    margin-bottom: var(--space-4);
  }

  .path {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: var(--space-2);
  }

  .path-value {
    flex: 1;
    min-width: 0;
    font-size: var(--font-xs);
    color: var(--text-2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .facts {
    display: flex;
    flex-direction: column;
  }

  .fact {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.9rem 0;
    border-top: 1px solid var(--border);
  }

  .redist-row {
    display: flex;
    padding-top: var(--space-3);
    border-top: 1px solid var(--border);
  }

  .steam-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.9rem 0;
    border-top: 1px solid var(--border);
  }

  .steam-row-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .steam-row-label {
    font-size: var(--font-sm);
    color: var(--text);
  }

  .steam-row-hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  dt {
    font-size: var(--font-xs);
    color: var(--text-3);
    white-space: nowrap;
  }

  dd {
    font-size: var(--font-sm);
    color: var(--text);
    min-width: 0;
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    font-variant-numeric: tabular-nums;
  }

  dd.mono {
    font-size: var(--font-xs);
    color: var(--text-2);
  }
</style>
