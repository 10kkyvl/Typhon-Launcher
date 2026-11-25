<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
  import OptionList from './OptionList.svelte';
  import { msg } from '../i18n';

  let {
    open = $bindable(false),
    title,
    candidates,
    onpick,
    onbrowse,
    returnFocus,
  }: {
    open?: boolean;
    title: string;
    candidates: string[];
    onpick: (dir: string) => void;
    onbrowse: () => void;
    /** Where focus goes on close when the element that opened the dialog is gone (a menu item). */
    returnFocus?: () => HTMLElement | null | undefined;
  } = $props();
  const uid = $props.id();

  // A choice, not a use: an arrow chooses in a radio group, so walking the list must not set the
  // folder. Select uses the chosen one.
  let chosen = $state('');
  const options = $derived(candidates.map((candidate) => ({ id: candidate, title: candidate })));

  // The dialog stays mounted between openings (the game menu, the saves view): each one starts with
  // no folder chosen.
  $effect(() => {
    const isOpen = open;
    void candidates;
    untrack(() => {
      if (!isOpen || !candidates.includes(chosen)) chosen = '';
    });
  });
</script>

<Modal bind:open title={msg('ui.savesDirTitle')} width="52rem" {returnFocus}>
  <p class="hint" id="{uid}-question">{msg('ui.savesMultipleCandidates', { title })}</p>
  <OptionList labelledby="{uid}-question" {options} bind:value={chosen} />
  {#snippet footer()}
    <Button onclick={() => (open = false)}>{msg('common.cancel')}</Button>
    <Button onclick={onbrowse}>{msg('ui.pickAnotherFolder')}</Button>
    <Button variant="primary" disabled={!chosen} onclick={() => onpick(chosen)}>{msg('common.select')}</Button>
  {/snippet}
</Modal>

<style>
  .hint {
    margin-bottom: var(--space-4);
    font-size: var(--font-sm);
    color: var(--text-2);
  }
</style>
