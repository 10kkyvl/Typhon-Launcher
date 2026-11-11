<script lang="ts">
  import Button from '../components/Button.svelte';
  import DropdownMenu from '../components/DropdownMenu.svelte';
  import Modal from '../components/Modal.svelte';
  import Select from '../components/Select.svelte';
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
    text = 'Removes the cached covers',
    expanded,
  }: {
    inDialog?: boolean;
    describe?: boolean;
    disabled?: boolean;
    present?: boolean;
    placement?: Placement;
    other?: boolean;
    text?: string;
    /** The control has a popup of its own and says whether it is open. */
    expanded?: boolean;
  } = $props();

  let settings = $state(true);
  // Changed by the page, not through the tooltip's props: the control's own description and the
  // control itself being created again.
  let note = $state('note-a');
  let generation = $state(0);
  let language = $state('en');
  const LANGUAGES = [
    { id: 'en', label: 'English' },
    { id: 'ru', label: 'Русский' },
  ];
</script>

{#snippet toolbar()}
  <button type="button">Before</button>
  <Tooltip {text} {describe} {placement}>
    {#if present}
      {#key generation}
        <button type="button" {disabled} aria-describedby={note} aria-expanded={expanded}>Clear</button>
      {/key}
    {/if}
  </Tooltip>
  <button type="button">After</button>
  <p id="note-a">Takes a minute</p>
  <p id="note-b">Cannot be undone</p>
  <button type="button" onclick={() => (note = 'note-b')}>Change note</button>
  <button type="button" onclick={() => (generation += 1)}>Rebuild</button>
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
    <Select options={LANGUAGES} bind:value={language} label="Language" />
    <input aria-label="Search" />
    <p data-testid="blank">Nothing to press here</p>
  </section>
{/if}

<Modal open={other} title="Other">
  <p>Over it</p>
</Modal>
