<script lang="ts">
  import Button from '../components/Button.svelte';
  import DropdownMenu from '../components/DropdownMenu.svelte';
  import Modal from '../components/Modal.svelte';
  import type { MenuItem } from '../components/menu';

  // A menu button on the bare page or inside a dialog, with controls before and after its trigger and
  // an icon button: the shapes DropdownMenu.dom.test.ts drives. An item with the id `confirm` opens a
  // dialog, as "Sign out" or "Remove" do at the call sites.
  let {
    items,
    onselect,
    inDialog = false,
    align,
    placement,
  }: {
    items: MenuItem[];
    onselect?: (id: string, item: MenuItem) => void;
    inDialog?: boolean;
    align?: 'left' | 'right';
    placement?: 'down' | 'up';
  } = $props();

  let settings = $state(false);
  let confirm = $state(false);

  function pick(id: string, item: MenuItem) {
    if (id === 'confirm') confirm = true;
    onselect?.(id, item);
  }
</script>

{#snippet toolbar()}
  <button type="button">Before</button>
  <DropdownMenu {items} {align} {placement} onselect={pick}>
    {#snippet trigger({ props })}
      <Button {...props}>Sort</Button>
    {/snippet}
  </DropdownMenu>
  <button type="button">After</button>
  <button type="button" aria-label="Help">
    <svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0 0h10v10H0z" /></svg>
  </button>
{/snippet}

<button type="button" onclick={() => (settings = true)}>Open settings</button>

{#if inDialog}
  <Modal bind:open={settings} title="Settings">
    {@render toolbar()}
  </Modal>
{:else}
  <section>
    {@render toolbar()}
  </section>
{/if}

<Modal bind:open={confirm} title="Confirm">
  <p>Sure?</p>
</Modal>
