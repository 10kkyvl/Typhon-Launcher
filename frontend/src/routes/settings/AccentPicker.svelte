<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { get } from 'svelte/store';
  import { msg, type MessageKey } from '../../lib/i18n';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { applyPersonalAccent, displayedAccent, fallbackAccent } from '../../lib/theme/apply';
  import { accentPresets, validAccent } from '../../lib/theme/accent';
  import HexColorField from '../../lib/components/HexColorField.svelte';
  import ColorPalette from '../../lib/components/ColorPalette.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import { appearanceResetVersion } from '../../lib/stores/theme';
  import { nextIndex } from '../../lib/utils/roving';

  const uid = $props.id();
  const presetLabels: MessageKey[] = [
    'settings.accentPreset1', 'settings.accentPreset2', 'settings.accentPreset3', 'settings.accentPreset4',
    'settings.accentPreset5', 'settings.accentPreset6', 'settings.accentPreset7', 'settings.accentPreset8',
  ];
  let editing = $state(false);
  let draft = $state('');
  let lastValid = $state<string>(fallbackAccent);
  let busy = $state(false);
  const selected = $derived($settings?.accentColor ?? '');
  function cancel() {
    editing = false;
    applyPersonalAccent(get(settings)?.accentColor ?? '');
  }
  onDestroy(cancel);
  $effect(() => {
    $appearanceResetVersion;
    untrack(() => { editing = false; draft = ''; lastValid = fallbackAccent; });
  });
  function preview(value: string) {
    draft = value;
    if (validAccent(value)) { lastValid = value; applyPersonalAccent(value); }
  }
  async function save(color: string) {
    editing = false;
    busy = true;
    await updateSettings({ accentColor: color.toUpperCase() });
    applyPersonalAccent(get(settings)?.accentColor ?? '');
    busy = false;
  }
  async function tint(value: boolean) {
    busy = true;
    await updateSettings({ tintLogo: value });
    busy = false;
  }
  function startCustom() {
    draft = selected || (validAccent($displayedAccent) ? $displayedAccent : fallbackAccent);
    lastValid = draft;
    editing = true;
  }
  // The theme's own colour, the presets, then a colour of one's own: one choice of the three kinds.
  const choices = $derived([
    { id: 'theme', label: msg('settings.accentTheme'), color: undefined, chosen: !selected && !editing },
    ...accentPresets.map((color, i) => ({ id: color, label: msg(presetLabels[i]), color, chosen: selected.toUpperCase() === color && !editing })),
    { id: 'custom', label: msg('settings.accentCustom'), color: undefined, chosen: editing || (!!selected && !accentPresets.includes(selected.toUpperCase())) },
  ]);
  const stop = $derived(Math.max(0, choices.findIndex((choice) => choice.chosen)));
  let group = $state<HTMLElement>();
  function choose(index: number) {
    if (busy) return;
    if (index === 0) void save('');
    else if (index === choices.length - 1) startCustom();
    else void save(accentPresets[index - 1]);
  }
  function onkeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const radios = [...(group?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];
    const next = nextIndex(radios.indexOf(event.currentTarget as HTMLElement), radios.length, event.key, { orientation: 'both' });
    if (next === null) return;
    event.preventDefault();
    if (busy) return;
    choose(next);
    radios[next]?.focus();
  }
</script>

<Card title={msg('settings.accentTitle')}>
  <p class="hint">{msg('settings.accentHint')}</p>
  <div class="choices" role="radiogroup" aria-label={msg('settings.accentTitle')} bind:this={group}>
    {#each choices as choice, i (choice.id)}
      <button
        type="button"
        role="radio"
        class:swatch={choice.color !== undefined}
        class:chosen={choice.chosen}
        style={choice.color ? `--swatch: ${choice.color}` : undefined}
        aria-checked={choice.chosen}
        aria-label={choice.color ? choice.label : undefined}
        aria-disabled={busy || undefined}
        tabindex={i === stop ? 0 : -1}
        onclick={() => choose(i)}
        {onkeydown}
      >
        {#if choice.color}<span></span>{:else}{choice.label}{/if}
      </button>
    {/each}
  </div>
  {#if editing}
    <ColorPalette value={lastValid} onchange={preview} disabled={busy} />
    <div class="editor">
      <HexColorField value={draft} onchange={preview} disabled={busy} onkeydown={e => { if (e.key === 'Escape') cancel(); if (e.key === 'Enter' && validAccent(draft)) void save(draft); }} />
      <Button size="sm" variant="primary" disabled={!validAccent(draft) || busy} onclick={() => save(draft)}>{msg('settings.accentDone')}</Button>
      <Button size="sm" variant="secondary" onclick={cancel}>{msg('common.cancel')}</Button>
    </div>
    {#if !validAccent(draft)}<p class="hint">{msg('settings.accentInvalid')}</p>{/if}
  {:else}
    <div class="editor"><HexColorField value={selected || $displayedAccent} readonly /></div>
  {/if}
  <div class="tint"><span id="{uid}-tint-label">{msg('settings.accentTintLogo')}</span><Toggle checked={$settings?.tintLogo ?? false} disabled={busy || editing} labelledby="{uid}-tint-label" onchange={tint} /></div>
</Card>

<style>
  .hint { color: var(--text-2); font-size: var(--font-xs); margin-bottom: var(--space-3); }
  .choices, .editor { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .choices button { min-height: 3.6rem; padding: .5rem 1rem; border: 2px solid var(--border-strong); border-radius: var(--radius-md); background: var(--surface-2); color: var(--text); }
  .choices button.chosen { border-color: var(--accent-text); outline: 1px solid var(--accent-text); outline-offset: 2px; }
  .choices button[aria-disabled='true'] { opacity: .5; cursor: default; }
  .choices .swatch { padding: .4rem; }
  .swatch span { display: block; width: 2.4rem; height: 2.4rem; border-radius: 50%; background: var(--swatch); }
  .editor { margin-top: var(--space-4); }
  .tint { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); margin-top: var(--space-4); font-size: var(--font-md); }
</style>
