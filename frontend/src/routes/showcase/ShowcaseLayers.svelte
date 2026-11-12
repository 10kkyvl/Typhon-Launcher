<script lang="ts">
  import { Bell, EllipsisVertical, Info } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Checkbox from '../../lib/components/Checkbox.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import ContextMenu from '../../lib/components/ContextMenu.svelte';
  import DropdownMenu from '../../lib/components/DropdownMenu.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import type { MenuItem } from '../../lib/components/menu';
  import Modal from '../../lib/components/Modal.svelte';
  import Popover from '../../lib/components/Popover.svelte';
  import Select from '../../lib/components/Select.svelte';
  import type { SelectOption } from '../../lib/components/select';
  import Toggle from '../../lib/components/Toggle.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import type { ConfirmPrompt } from '../../lib/confirm/prompts';
  import { toast, type ToastKind } from '../../lib/stores/toasts';
  import { contextMenuTrigger, type ContextRequest } from '../../lib/utils/contextmenu';
  import ShowcaseRow from './ShowcaseRow.svelte';
  import ShowcaseSection from './ShowcaseSection.svelte';

  const uid = $props.id();
  const KINDS: ToastKind[] = ['info', 'success', 'warning', 'danger'];
  const CONFIRM_MS = 800;

  let dialog = $state(false);
  let speed = $state('fast');
  const SPEEDS: SelectOption[] = [
    { id: 'fast', label: 'Fast' },
    { id: 'slow', label: 'Slow' },
  ];

  let confirm = $state<ConfirmPrompt | null>(null);
  let confirmFails = $state(true);
  const DANGER: ConfirmPrompt = {
    title: 'Remove the game?',
    text: 'Its files are deleted from the disk. Saves in the cloud stay.',
    note: 'The confirm action fails while "Make the action fail" is checked, then shows its error and offers to try again.',
    confirm: 'Remove',
    busy: 'Removing…',
  };
  const PRIMARY: ConfirmPrompt = {
    title: 'Add to Steam?',
    text: 'A shortcut to the game is added to your Steam library.',
    confirm: 'Add',
    busy: 'Adding…',
    tone: 'primary',
  };

  async function act() {
    await new Promise<void>((resolve) => setTimeout(resolve, CONFIRM_MS));
    if (confirmFails) throw new Error('showcase: the action failed as asked');
    toast('Done', 'success');
  }

  let sort = $state('name');
  let compact = $state(false);
  const menuItems = $derived<MenuItem[]>([
    { id: 'open', label: 'Open' },
    { id: 'rename', label: 'Rename' },
    { id: 'locked', label: 'Move (disabled)', disabled: true },
    { id: 'name', label: 'Sort by name', checked: sort === 'name', separator: true },
    { id: 'played', label: 'Sort by last played', checked: sort === 'played' },
    { id: 'compact', label: 'Compact rows', checked: compact, independent: true, separator: true },
    { id: 'remove', label: 'Remove', danger: true, separator: true },
  ]);

  function select(id: string) {
    if (id === 'name' || id === 'played') sort = id;
    else if (id === 'compact') compact = !compact;
    else toast(`${id} picked`, 'info');
  }

  let context = $state<ContextRequest | null>(null);
  const contextId = `${uid}-context`;
  const openerId = `${uid}-context-opener`;

  let alerts = $state(true);
</script>

<ShowcaseSection title="Layers" note="What opens over the page: hints, menus, a popover, dialogs and toasts.">
  <ShowcaseRow caption="Hints">
    <Tooltip text="Shows on hover and at once on keyboard focus">
      <Button>Hover or focus me</Button>
    </Tooltip>
    <Tooltip text="Placed below the control" placement="bottom">
      <Button variant="ghost">Below</Button>
    </Tooltip>
    <Tooltip text="Placed to the right" placement="right">
      <Button variant="ghost">Right</Button>
    </Tooltip>
    <Tooltip text="Repeats the name only, so it is shown and not announced twice" describe={false}>
      <Button icon aria-label="Repeats the name only, so it is shown and not announced twice"><Info size="1.6rem" /></Button>
    </Tooltip>
  </ShowcaseRow>

  <ShowcaseRow caption="Menus and a popover">
    <DropdownMenu items={menuItems} align="left" onselect={select}>
      {#snippet trigger({ props })}
        <Button {...props}>Menu</Button>
      {/snippet}
    </DropdownMenu>
    <DropdownMenu items={menuItems} onselect={select}>
      {#snippet trigger({ props })}
        <IconButton label="More" {...props}><EllipsisVertical size="1.8rem" /></IconButton>
      {/snippet}
    </DropdownMenu>
    <Button
      id={openerId}
      onclick={() => (context = { at: { anchor: () => document.getElementById(openerId) }, focus: 'first' })}
      {@attach contextMenuTrigger((request) => {
        context = request;
        return true;
      })}
    >
      Context menu (press, right-click or Shift+F10)
    </Button>
    <Popover label="Notifications" placement="bottom-start">
      {#snippet trigger({ props })}
        <IconButton label="Notifications" {...props}><Bell size="1.8rem" /></IconButton>
      {/snippet}
      {#snippet children({ close })}
        <div class="popover">
          <div class="switch-row">
            <span id="{uid}-alerts">Sound alerts</span>
            <Toggle labelledby="{uid}-alerts" bind:checked={alerts} />
          </div>
          <Button size="sm" onclick={close}>Done</Button>
        </div>
      {/snippet}
    </Popover>
  </ShowcaseRow>

  <ShowcaseRow caption="Dialogs">
    <Button variant="primary" onclick={() => (dialog = true)}>Open a dialog</Button>
    <Button variant="danger" onclick={() => (confirm = DANGER)}>Open a confirm dialog</Button>
    <Button onclick={() => (confirm = PRIMARY)}>Open a primary confirm</Button>
    <Checkbox bind:checked={confirmFails} label="Make the action fail" />
  </ShowcaseRow>

  <ShowcaseRow caption="Toasts">
    {#each KINDS as kind (kind)}
      <Button onclick={() => toast(`A ${kind} toast`, kind)}>{kind}</Button>
    {/each}
    <Button onclick={() => toast('A toast with an action', 'info', { label: 'Undo', run: () => toast('Undone', 'success') })}>With an action</Button>
  </ShowcaseRow>
</ShowcaseSection>

<Modal bind:open={dialog} title="A dialog" describedby="{uid}-dialog-text">
  <p class="text" id="{uid}-dialog-text">A dialog traps Tab, closes on Escape, a press on the scrim and the close button, and gives focus back.</p>
  <label class="text" for="{uid}-speed">Speed</label>
  <Select id="{uid}-speed" options={SPEEDS} bind:value={speed} />
  {#snippet footer()}
    <Button onclick={() => (dialog = false)}>Cancel</Button>
    <Button variant="primary" onclick={() => (dialog = false)}>Save</Button>
  {/snippet}
</Modal>

{#if confirm}
  <ConfirmModal prompt={confirm} onconfirm={act} onclose={() => (confirm = null)} errorText={() => 'The action failed. Uncheck "Make the action fail" and try again.'} />
{/if}

{#if context}
  <ContextMenu
    id={contextId}
    label="Context menu"
    items={menuItems}
    at={context.at}
    initial={context.focus}
    trigger={() => document.getElementById(openerId)}
    onselect={select}
    onclose={() => (context = null)}
  />
{/if}

<style>
  .popover {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4);
  }

  .switch-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    font-size: var(--font-md);
    color: var(--text);
  }

  .text {
    font-size: var(--font-sm);
    color: var(--text-2);
  }
</style>
