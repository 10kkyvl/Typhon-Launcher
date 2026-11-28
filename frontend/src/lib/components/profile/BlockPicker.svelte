<script lang="ts">
  import { Plus } from '@lucide/svelte';
  import Button from '../Button.svelte';
  import Notice from '../Notice.svelte';
  import { msg } from '../../i18n';
  import { ADD_PRESETS, canAdd, layoutErrorText, unknownBlockCount, type AddPreset } from '../../profile/layout';
  import { showcaseTitle } from '../../profile/layoutView';
  import { MAX_BLOCKS, type ProfileLayout } from '../../services/account';
  import { focusHandOff } from '../../utils/focusHandOff';
  import { blockLabel } from './blockLabels';

  let {
    layout,
    disabled = false,
    onadd,
  }: { layout: ProfileLayout; disabled?: boolean; onadd: (preset: AddPreset) => void } = $props();

  const uid = $props.id();
  const listId = `${uid}-presets`;
  let open = $state(false);

  const hiddenByVersion = $derived(layout.blocks.length >= MAX_BLOCKS && unknownBlockCount(layout) > 0);

  function label(preset: AddPreset): string {
    if (preset.id === 'collection') return msg('profile.presetCollection');
    if (preset.type === 'collection') return showcaseTitle(preset.id);
    return blockLabel(preset.type);
  }

  // The list closes with the press: focus held in it goes back to "Add block".
  function add(preset: AddPreset, held: boolean) {
    if (held) return;
    const handOff = focusHandOff();
    onadd(preset);
    open = false;
    void handOff(() => document.getElementById(`${uid}-add`));
  }
</script>

<div class="picker">
  <!-- Held, never disabled, while the draft saves: a pointer press on Save moves no focus in the macOS
       view, so this may still hold it. -->
  <Button
    id="{uid}-add"
    {disabled}
    reason={disabled ? msg('social.saving') : ''}
    aria-expanded={open}
    aria-controls={open ? listId : undefined}
    onclick={() => (open = !open)}
  >
    <Plus size="1.5rem" strokeWidth={1.8} />{msg('profile.addBlock')}
  </Button>
  {#if hiddenByVersion}
    <Notice tone="warning" compact announce={false}>
      {msg('profile.blocksFromNewer', { hidden: unknownBlockCount(layout), count: MAX_BLOCKS })}
    </Notice>
  {/if}
  {#if open}
    <ul class="presets" id={listId}>
      {#each ADD_PRESETS as preset (preset.id)}
        {@const blocked = canAdd(layout, preset)}
        {@const held = disabled || blocked !== null}
        <!-- A kind that cannot be added stays in the Tab order, so the keyboard reaches why. -->
        <li>
          <button
            class="preset"
            type="button"
            aria-disabled={held ? 'true' : undefined}
            aria-describedby={blocked ? `${uid}-${preset.id}-reason` : undefined}
            onclick={() => add(preset, held)}
          >
            <span class="label">{label(preset)}</span>
            {#if blocked}<span class="reason" id="{uid}-{preset.id}-reason">{layoutErrorText(blocked)}</span>{/if}
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

  .preset:hover:not([aria-disabled='true']) {
    background: var(--hover-strong);
    border-color: var(--border-strong);
  }

  .preset[aria-disabled='true'] {
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
