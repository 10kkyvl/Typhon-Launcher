<script lang="ts">
  import GameDialogs from '../../routes/game/GameDialogs.svelte';
  import type { GamePageData } from '../../routes/game/gamePageData.svelte';
  import type { LibraryGame } from '../services/library';

  // The game page's dialogs with an opener for each request, as GameActions and the page's menu give
  // them. For GameDialogs.dom.test.ts.
  let {
    game,
    matchMode = 'find',
    canonicalId = 'igdb:1',
    title = 'Hades',
    data = { metaView: null } as unknown as GamePageData,
  }: { game: LibraryGame | undefined; matchMode?: 'find' | 'change'; canonicalId?: string; title?: string; data?: GamePageData } = $props();

  let statusOpen = $state(false);
  let tagsOpen = $state(false);
  let removeOpen = $state(false);
  let removeMode = $state<'disk' | 'library'>('library');
  let pickerOpen = $state(false);
  let lightboxOpen = $state(false);
  let lightboxIndex = $state(0);
  let matchOpen = $state(false);
  let downloadModalOpen = $state(false);
  let installModalOpen = $state(false);
</script>

<main>
  <button type="button" onclick={() => (statusOpen = true)}>Open status</button>
  <button type="button" onclick={() => (tagsOpen = true)}>Open tags</button>
  <button type="button" onclick={() => (matchOpen = true)}>Open match</button>
  <button type="button" onclick={() => (pickerOpen = true)}>Open picker</button>
</main>

<GameDialogs
  {data}
  localGame={game}
  installed={false}
  emulated={false}
  {title}
  {canonicalId}
  shots={[]}
  downloadPool={[]}
  downloadRelease={() => {}}
  {matchMode}
  downloadSource=""
  downloadOrigin={undefined}
  installModalDownloadId={null}
  bind:statusOpen
  bind:tagsOpen
  bind:removeOpen
  bind:removeMode
  bind:pickerOpen
  bind:lightboxOpen
  bind:lightboxIndex
  bind:matchOpen
  bind:downloadModalOpen
  bind:installModalOpen
/>
