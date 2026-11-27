<script lang="ts">
  import Button from './Button.svelte';
  import OptionList from './OptionList.svelte';
  import { msg } from '../i18n';

  /**
   * InstallModal's choice of the game's executable among those the install found. The choice is only
   * sent by the dialog's Confirm, so the arrows choose; each row is the file's name, read with its
   * whole path. `held` holds the list and the manual pick while one of the dialog's actions runs.
   */
  let {
    candidatePaths,
    chosen = $bindable(),
    held,
    acting,
    onpickmanual,
  }: {
    candidatePaths: string[];
    chosen: string;
    /** Why the choice cannot be changed now ('' while it can). */
    held: string;
    /** The dialog's action that runs, '' while none does. */
    acting: string;
    onpickmanual: () => void;
  } = $props();

  function basename(path: string) {
    const parts = path.split(/[\\/]/);
    return parts[parts.length - 1] || path;
  }

  const manualHeld = $derived(acting && acting !== 'manual' ? held : '');
  const options = $derived(candidatePaths.map((path) => ({ id: path, title: basename(path), note: path })));
</script>

<div class="block">
  <span class="lead">{msg('modals.installChooseGameExecutable')}</span>
  <OptionList {options} bind:value={chosen} label={msg('modals.installExecutablesAriaLabel')} disabled={!!acting} />
  <Button size="sm" disabled={!!manualHeld} reason={manualHeld} busy={acting === 'manual'} onclick={onpickmanual}>{msg('modals.installPickManually')}</Button>
</div>

<style>
  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .lead {
    font-size: var(--font-md);
    font-weight: 500;
  }
</style>
