<script lang="ts">
  import Chip from '../components/Chip.svelte';
  import DropdownMenu from '../components/DropdownMenu.svelte';

  // The three things a chip is at the call sites: a toggle, the trigger of a menu (which may show
  // that a filter is applied), and a tag with a remove button. For Chip.dom.test.ts.
  let { onremove }: { onremove?: (tag: string) => void } = $props();

  let on = $state(false);
  let presses = $state(0);
</script>

<Chip selected={on} onclick={() => (on = !on)}>Installed</Chip>

<DropdownMenu items={[{ id: 'a', label: 'Alphabet' }]}>
  {#snippet trigger({ props })}
    <Chip {...props} selected>Sort</Chip>
  {/snippet}
</DropdownMenu>

<Chip onclick={() => presses++}>Plain {presses}</Chip>

<Chip selected onremove={() => onremove?.('rpg')} removeLabel="Remove the tag rpg">rpg</Chip>
