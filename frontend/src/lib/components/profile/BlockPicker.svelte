<script lang="ts">
  import { Plus } from '@lucide/svelte';
  import Button from '../Button.svelte';
  import { msg } from '../../i18n';
  import { ADD_PRESETS, canAdd, layoutErrorText, type AddPreset } from '../../profile/layout';
  import { showcaseTitle } from '../../profile/layoutView';
  import type { ProfileLayout } from '../../services/account';
  import { blockLabel } from './blockLabels';

  let {
    layout,
    disabled = false,
    onadd,
  }: { layout: ProfileLayout; disabled?: boolean; onadd: (preset: AddPreset) => void } = $props();

  let open = $state(false);

  function label(preset: AddPreset): string {
    if (preset.id === 'collection') return msg('profile.presetCollection');
    if (preset.type === 'collection') return showcaseTitle(preset.id);
    return blockLabel(preset.type);
  }

  function add(preset: AddPreset) {
    onadd(preset);
    open = false;
  }
</script>

<div class="picker">
  <Button {disabled} pressed={open} onclick={() => (open = !open)}>
    <Plus size="1.5rem" strokeWidth={1.8} />{msg('profile.addBlock')}
  </Button>
  {#if open}
    <ul class="presets">
      {#each ADD_PRESETS as preset (preset.id)}
        {@const blocked = canAdd(layout, preset)}
        <li>
          <button class="preset" type="button" disabled={disabled || blocked !== null} onclick={() => add(preset)}>
            <span class="label">{label(preset)}</span>
            {#if blocked}<span class="reason">{layoutErrorText(blocked)}</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .picker {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-3);
  }

  .presets {
    list-style: none;
    margin: 0;
    padding: 0;
    width: 100%;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(18rem, 1fr));
    gap: var(--space-2);
  }

  .preset {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 0.2rem;
    width: 100%;
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
    transition:
      background var(--dur) var(--ease),
      border-color var(--dur) var(--ease);
  }

  .preset:hover:not(:disabled) {
    background: var(--hover-strong);
    border-color: var(--border-strong);
  }

  .preset:disabled {
    cursor: default;
    color: var(--text-3);
  }

  .label {
    font-size: var(--font-sm);
    font-weight: 500;
  }

  .reason {
    font-size: var(--font-xs);
    color: var(--text-3);
  }
</style>
