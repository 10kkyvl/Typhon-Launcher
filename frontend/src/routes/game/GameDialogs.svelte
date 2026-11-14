<script lang="ts">
  import { untrack } from 'svelte';
  import AddDownloadModal from '../../lib/components/AddDownloadModal.svelte';
  import GameStatusModal from '../../lib/components/GameStatusModal.svelte';
  import GameTagsModal from '../../lib/components/GameTagsModal.svelte';
  import InstallModal from '../../lib/components/InstallModal.svelte';
  import Lightbox from '../../lib/components/Lightbox.svelte';
  import MetadataMatchModal from '../../lib/components/MetadataMatchModal.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import ReleaseList from '../../lib/components/ReleaseList.svelte';
  import RemoveGameModal from '../../lib/components/RemoveGameModal.svelte';
  import type { GamePageData } from './gamePageData.svelte';
  import { msg } from '../../lib/i18n';
  import { externalLabel } from '../../lib/library/external';
  import type { DownloadOrigin } from '../../lib/services/downloads';
  import type { LibraryGame } from '../../lib/services/library';
  import type { MediaAsset } from '../../lib/services/metadata';
  import type { ReleaseGroup } from '../../lib/services/sources';
  import { navigate } from '../../lib/stores/router';

  /** The dialogs the game page opens: status, tags, removal, the release picker, the viewer and the rest. */
  let {
    data,
    localGame,
    installed,
    emulated,
    title,
    canonicalId,
    shots,
    downloadPool,
    downloadRelease,
    matchMode,
    downloadSource,
    downloadOrigin,
    installModalDownloadId,
    statusOpen = $bindable(),
    tagsOpen = $bindable(),
    removeOpen = $bindable(),
    removeMode = $bindable(),
    pickerOpen = $bindable(),
    lightboxOpen = $bindable(),
    lightboxIndex = $bindable(),
    matchOpen = $bindable(),
    downloadModalOpen = $bindable(),
    installModalOpen = $bindable(),
  }: {
    data: GamePageData;
    localGame: LibraryGame | undefined;
    installed: boolean;
    emulated: boolean;
    title: string;
    canonicalId: string | undefined;
    shots: MediaAsset[];
    downloadPool: ReleaseGroup[];
    downloadRelease: (group: ReleaseGroup) => void;
    matchMode: 'find' | 'change';
    downloadSource: string;
    downloadOrigin: DownloadOrigin | undefined;
    installModalDownloadId: string | null;
    statusOpen: boolean;
    tagsOpen: boolean;
    removeOpen: boolean;
    removeMode: 'disk' | 'library';
    pickerOpen: boolean;
    lightboxOpen: boolean;
    lightboxIndex: number;
    matchOpen: boolean;
    downloadModalOpen: boolean;
    installModalOpen: boolean;
  } = $props();

  // Go says the card is gone (library:updated) before the removal answers, so the dialog keeps the
  // card it was opened for, and where the page leaves to: Installed for an installed game, the library
  // for a card that holds no install.
  let removing = $state<{ game: LibraryGame; emulated: boolean; leaveTo: 'installed' | 'library' } | null>(null);

  $effect(() => {
    if (!removeOpen) return;
    untrack(() => {
      if (localGame) removing = { game: localGame, emulated, leaveTo: installed ? 'installed' : 'library' };
    });
  });
</script>

{#if localGame}
  <GameStatusModal bind:open={statusOpen} game={localGame} />

  {#if tagsOpen}
    <GameTagsModal bind:open={tagsOpen} game={localGame} />
  {/if}
{/if}

{#if removing}
  <RemoveGameModal
    bind:open={removeOpen}
    bind:mode={removeMode}
    gameId={removing.game.id}
    title={removing.game.title}
    external={externalLabel(removing.game)}
    emulated={removing.emulated}
    onremoved={(removed) => {
      if (removed === 'library') navigate(removing?.leaveTo ?? 'library');
    }}
  />
{/if}

<Modal bind:open={pickerOpen} title={msg('games.detailChooseDownloadTitle')} width="86rem">
  <ReleaseList
    groups={downloadPool}
    currentReleaseId={localGame?.releaseId ?? ''}
    ondownload={(group) => {
      pickerOpen = false;
      downloadRelease(group);
    }}
  />
</Modal>

<Lightbox
  bind:open={lightboxOpen}
  bind:index={lightboxIndex}
  items={shots.map((shot) => ({ id: shot.id, url: shot.url ?? '' }))}
  label={title}
/>

<MetadataMatchModal
  bind:open={matchOpen}
  gameId={canonicalId ?? ''}
  gameTitle={title}
  mode={matchMode}
  onapplied={(view) => (data.metaView = view)}
/>

<AddDownloadModal bind:open={downloadModalOpen} initialSource={downloadSource} origin={downloadOrigin} />
<InstallModal bind:open={installModalOpen} downloadId={installModalDownloadId} />
