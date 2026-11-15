<script lang="ts">
  import { ChevronDown, LayoutGrid, List, Plus, RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Chip from '../../lib/components/Chip.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import SearchInput from '../../lib/components/SearchInput.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import { scanning } from '../../lib/stores/discovery';
  import { installedView } from '../../lib/stores/ui';
  import { msg } from '../../lib/i18n';
  import type { Sort } from './installedGame';

  // The header's controls: the search and the sorting (the page's, bound), the view switch, the scan
  // and the two ways to add a game by hand.
  let {
    search = $bindable(),
    sort = $bindable(),
    scanLabel,
    onfind,
    onaddimage,
    onaddgame,
  }: {
    search: string;
    sort: Sort;
    scanLabel: string;
    onfind: () => void;
    onaddimage: () => void;
    onaddgame: () => void;
  } = $props();

  const sortLabels: Record<Sort, string> = {
    recent: msg('games.recentLabel'),
    alpha: msg('games.sortAlpha'),
    size: msg('games.sortSize'),
  };
</script>

<div class="search-wrap">
  <SearchInput bind:value={search} label={msg('games.installedSearchPlaceholder')} placeholder={msg('games.installedSearchPlaceholder')} />
</div>
<DropdownMenu
  items={(['recent', 'alpha', 'size'] as const).map((id) => ({ id, label: sortLabels[id], checked: sort === id }))}
  onselect={(id) => (sort = id as Sort)}
>
  {#snippet trigger({ props })}
    <Chip {...props}>
      {msg('games.installedSortByLabel', { sort: sortLabels[sort] })}
      <ChevronDown size="1.4rem" strokeWidth={1.8} />
    </Chip>
  {/snippet}
</DropdownMenu>
<SegmentedControl
  label={msg('games.viewLabel')}
  bind:value={$installedView}
  options={[
    { id: 'grid', label: msg('games.viewGrid') },
    { id: 'list', label: msg('games.viewList') },
  ]}
>
  {#snippet item(option)}
    {#if option.id === 'grid'}
      <LayoutGrid size="1.6rem" strokeWidth={1.8} />
    {:else}
      <List size="1.6rem" strokeWidth={1.8} />
    {/if}
  {/snippet}
</SegmentedControl>
<Button onclick={onfind} busy={$scanning}>
  {#if !$scanning}<RefreshCw size="1.5rem" strokeWidth={2} />{/if}
  {$scanning ? scanLabel : msg('games.installedFindGames')}
</Button>
<Button onclick={onaddimage}>
  <Plus size="1.5rem" strokeWidth={2} />
  {msg('emul.addButton')}
</Button>
<Button variant="primary" onclick={onaddgame}>
  <Plus size="1.5rem" strokeWidth={2} />
  {msg('games.installedAddGameButton')}
</Button>

<style>
  .search-wrap {
    width: 26rem;
  }
</style>
