<script lang="ts">
  import Button from '../components/Button.svelte';
  import DropdownMenu from '../components/DropdownMenu.svelte';
  import Modal from '../components/Modal.svelte';
  import Tooltip from '../components/Tooltip.svelte';
  import type { Placement } from '../utils/position';

  // A control with a hint between two plain buttons, on the bare page (next to a menu button) or
  // inside a dialog, and a second dialog a test opens over it. For Tooltip.dom.test.ts.
  let {
    inDialog = false,
    describe = true,
    disabled = false,
    present = true,
    placement,
    other = false,
  }: {
    inDialog?: boolean;
    describe?: boolean;
    disabled?: boolean;
    present?: boolean;
    placement?: Placement;
    other?: boolean;
  } = $props();

  let settings = $state(true);
</script>

{#snippet toolbar()}
  <button type="button">Before</button>
  <Tooltip text="Removes the cached covers" {describe} {placement}>
    {#if present}
      <button type="button" {disabled}>Clear</button>
    {/if}
  </Tooltip>
  <button type="button">After</button>
{/snippet}

{#if inDialog}
  <Modal bind:open={settings} title="Settings">
    {@render toolbar()}
  </Modal>
{:else}
  <section>
    {@render toolbar()}
    <div class="list" data-testid="unrelated" style="overflow: auto"><p>Elsewhere</p></div>
    <DropdownMenu items={[{ id: 'alpha', label: 'Alphabet' }]}>
      {#snippet trigger({ props })}
        <Button {...props}>Sort</Button>
      {/snippet}
    </DropdownMenu>
  </section>
{/if}

<Modal open={other} title="Other">
  <p>Over it</p>
</Modal>
