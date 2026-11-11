<script lang="ts">
  import Modal from '../components/Modal.svelte';
  import { layer } from '../layers';

  // A select whose listbox is portalled out and registered as a non-modal layer owned by its trigger,
  // inside a dialog or on the bare page, with a submenu owned by one of its options: the shapes the
  // menus and selects pass builds. For layers.dom.test.ts.
  let {
    onpick,
    inDialog = true,
    dropTrigger = false,
  }: { onpick?: (value: string) => void; inDialog?: boolean; dropTrigger?: boolean } = $props();

  let open = $state(false);
  let listbox = $state(false);
  let submenu = $state(false);
  let trigger = $state<HTMLButtonElement>();
  let more = $state<HTMLElement>();
  let deleting = $state(false);
  let triggerShown = $state(true);
  let fallback = $state<HTMLButtonElement>();
</script>

{#snippet select()}
  <p>Language</p>
  {#if triggerShown}
    <button type="button" bind:this={trigger} aria-haspopup="listbox" onclick={() => (listbox = !listbox)}>Russian</button>
  {/if}
  <button type="button" bind:this={fallback}>Fallback</button>
  <button type="button">Save</button>
  <button type="button" aria-label="Help">
    <svg viewBox="0 0 10 10" aria-hidden="true"><path d="M0 0h10v10H0z" /></svg>
  </button>
{/snippet}

<button type="button" onclick={() => (open = true)}>Open settings</button>

{#if inDialog}
  <Modal bind:open title="Settings">
    {@render select()}
  </Modal>
{:else}
  <section>
    {@render select()}
  </section>
{/if}

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
    <li
      role="option"
      aria-selected="false"
      tabindex="0"
      onclick={() => {
        deleting = true;
        if (dropTrigger) triggerShown = false;
      }}
      onkeydown={() => {}}
    >
      Forget language
    </li>
    <li role="option" aria-selected="false" tabindex="0" bind:this={more} onclick={() => (submenu = true)} onkeydown={() => {}}>More languages</li>
  </ul>
{/if}

<Modal bind:open={deleting} title="Forget language" returnFocus={() => fallback}>
  <p>The language list is reset.</p>
</Modal>

{#if submenu}
  <ul class="listbox" role="listbox" aria-label="More languages" {@attach layer({ modal: false, owner: () => more, dismiss: () => (submenu = false) })}>
    <li role="option" aria-selected="false" tabindex="0" onkeydown={() => {}}>Deutsch</li>
  </ul>
{/if}

<style>
  .listbox {
    position: fixed;
    z-index: var(--z-layer);
  }
</style>
