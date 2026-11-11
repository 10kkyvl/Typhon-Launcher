<script lang="ts">
  import Button from '../components/Button.svelte';
  import Modal from '../components/Modal.svelte';
  import Popover from '../components/Popover.svelte';
  import type { Placement } from '../utils/position';

  // A popover opened by a button, on the bare page or inside a dialog, with controls before and after
  // its trigger and an icon button: the shapes Popover.dom.test.ts drives. Its body has three
  // controls: "First", "Remove me" (removes itself, as a list item that goes away under focus does)
  // and "Done" (closes it through `close`); `empty` gives it text only. `alone` leaves the trigger the
  // last control of its dialog; with `firstGoes` a press on "First" removes it.
  let {
    inDialog = false,
    alone = false,
    firstGoes = false,
    empty = false,
    placement,
    width,
    partId,
    onfirst,
  }: {
    inDialog?: boolean;
    alone?: boolean;
    firstGoes?: boolean;
    empty?: boolean;
    placement?: Placement;
    width?: string;
    partId?: string;
    onfirst?: () => void;
  } = $props();

  let settings = $state(false);
  let removable = $state(true);
  let first = $state(true);
</script>

{#snippet toolbar()}
  {#if !alone}<button type="button">Before</button>{/if}
  <Popover label="Details" {placement} {width} {partId}>
    {#snippet trigger({ props })}
      <Button {...props}>Details</Button>
    {/snippet}
    {#snippet children({ close })}
      {#if empty}
        <p>Nothing here</p>
      {:else}
        {#if first}
          <button
            type="button"
            onclick={() => {
              onfirst?.();
              if (firstGoes) first = false;
            }}>First</button
          >
        {/if}
        {#if removable}
          <button type="button" onclick={() => (removable = false)}>Remove me</button>
        {/if}
        <button type="button" onclick={close}>Done</button>
      {/if}
    {/snippet}
  </Popover>
  {#if !alone}
    <button type="button">After</button>
    <button type="button" aria-label="Help">
      <svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0 0h10v10H0z" /></svg>
    </button>
  {/if}
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
