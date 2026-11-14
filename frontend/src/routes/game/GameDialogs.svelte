<script lang="ts">
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
</script>

{#if localGame}
  <GameStatusModal bind:open={statusOpen} game={localGame} />

  {#if tagsOpen}
    <GameTagsModal bind:open={tagsOpen} game={localGame} />
  {/if}

  <RemoveGameModal
    bind:open={removeOpen}
    bind:mode={removeMode}
    gameId={localGame.id}
    title={localGame.title}
    external={externalLabel(localGame)}
    emulated={emulated}
    onremoved={(removed) => {
      if (removed === 'library') navigate('installed');
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
