<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import { layer } from '../layers';

  // A dialog with a select whose listbox is portalled out of it, registered as a non-modal layer owned
  // by the dialog: the shape the menus and selects pass builds. For layers.dom.test.ts.
  let { onpick }: { onpick?: (value: string) => void } = $props();

  let open = $state(false);
  let listbox = $state(false);
  let trigger = $state<HTMLButtonElement>();
</script>

<button type="button" onclick={() => (open = true)}>Open settings</button>

<Modal bind:open title="Settings">
  <p>Language</p>
  <button type="button" bind:this={trigger} aria-haspopup="listbox" onclick={() => (listbox = true)}>Russian</button>
  <button type="button">Save</button>
</Modal>

{#if listbox}
  <ul
    class="listbox"
    role="listbox"
    aria-label="Language"
    {@attach layer({ modal: false, owner: () => trigger, initialFocus: () => '[role=option]', dismiss: () => (listbox = false) })}
  >
    {#each ['Russian', 'English'] as value (value)}
      <li
        role="option"
        aria-selected={value === 'Russian'}
        tabindex="0"
        onclick={() => {
          onpick?.(value);
          listbox = false;
        }}
        onkeydown={(event) => event.key === 'Enter' && event.currentTarget.click()}
      >
        {value}
      </li>
    {/each}
  </ul>
{/if}

<style>
  .listbox {
    position: fixed;
    z-index: var(--z-layer);
  }
</style>
