<script lang="ts">
  import MenuList from '../components/MenuList.svelte';
  import type { MenuItem } from '../components/menu';

  // A menu whose "More" item opens a second menu owned by that item: the submenu shape MenuList's
  // one-menu rule must keep open together. No built-in menu nests yet.
  const top: MenuItem[] = [
    { id: 'more', label: 'More' },
    { id: 'plain', label: 'Plain' },
  ];
  const nested: MenuItem[] = [{ id: 'deep', label: 'Deep' }];

  let first = $state(false);
  let second = $state(false);
  let trigger = $state<HTMLButtonElement | null>(null);

  const moreItem = () => document.querySelector<HTMLElement>('#top-menu [role="menuitem"]');
</script>

<button type="button" bind:this={trigger} onclick={() => (first = !first)}>Open</button>

{#if first}
  <MenuList
    items={top}
    id="top-menu"
    label="Top"
    owner={() => trigger}
    onpick={(item) => {
      if (item.id === 'more') second = true;
    }}
    onclose={() => {
      first = false;
      second = false;
    }}
  />
{/if}

{#if second}
  <MenuList items={nested} id="nested-menu" label="Nested" initial="first" owner={moreItem} onpick={() => {}} onclose={() => (second = false)} />
{/if}
