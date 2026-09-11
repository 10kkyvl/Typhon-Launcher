<script lang="ts">
  import { onDestroy } from 'svelte';
  import { get } from 'svelte/store';
  import { msg, type MessageKey } from '../../lib/i18n';
  import { settings, updateSettings } from '../../lib/stores/settings';
  import { applyPersonalAccent, displayedAccent } from '../../lib/theme/apply';
  import { accentPresets, validAccent } from '../../lib/theme/accent';
  import ColorPalette from '../../lib/components/ColorPalette.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  const presetLabels: MessageKey[] = [
    'settings.accentPreset1', 'settings.accentPreset2', 'settings.accentPreset3', 'settings.accentPreset4',
    'settings.accentPreset5', 'settings.accentPreset6', 'settings.accentPreset7', 'settings.accentPreset8',
  ];
  let editing = $state(false);
  let draft = $state('');
  let lastValid = $state('#6673F2');
  let busy = $state(false);
  const selected = $derived($settings?.accentColor ?? '');
  function cancel() {
    editing = false;
    applyPersonalAccent(get(settings)?.accentColor ?? '');
  }
  onDestroy(cancel);
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
</script>

<Card title={msg('settings.accentTitle')}>
  <p class="hint">{msg('settings.accentHint')}</p>
  <div class="choices" role="group" aria-label={msg('settings.accentTitle')}>
    <button class:chosen={!selected && !editing} disabled={busy} aria-pressed={!selected && !editing} onclick={() => save('')}>{msg('settings.accentTheme')}</button>
    {#each accentPresets as color, i}
      <button class="swatch" class:chosen={selected.toUpperCase() === color && !editing} style={`--swatch: ${color}`} disabled={busy} aria-label={msg(presetLabels[i])} aria-pressed={selected.toUpperCase() === color && !editing} onclick={() => save(color)}><span></span></button>
    {/each}
    <button class:chosen={editing || (!!selected && !accentPresets.includes(selected.toUpperCase()))} disabled={busy} aria-pressed={editing || (!!selected && !accentPresets.includes(selected.toUpperCase()))} onclick={() => { draft = selected || (validAccent($displayedAccent) ? $displayedAccent : '#6673F2'); lastValid = draft; editing = true; }}>{msg('settings.accentCustom')}</button>
  </div>
  {#if editing}
    <ColorPalette value={lastValid} onchange={preview} />
    <div class="editor">
      <label>HEX<input type="text" value={draft} placeholder="#RRGGBB" maxlength="7" spellcheck={false} aria-invalid={!validAccent(draft)} oninput={e => preview(e.currentTarget.value)} onkeydown={e => { if (e.key === 'Escape') cancel(); if (e.key === 'Enter' && validAccent(draft)) void save(draft); }} /></label>
      <Button size="sm" variant="primary" disabled={!validAccent(draft) || busy} onclick={() => save(draft)}>{msg('settings.accentDone')}</Button>
      <Button size="sm" variant="secondary" onclick={cancel}>{msg('common.cancel')}</Button>
    </div>
    {#if !validAccent(draft)}<p class="hint">{msg('settings.accentInvalid')}</p>{/if}
  {/if}
  <div class="tint"><span>{msg('settings.accentTintLogo')}</span><Toggle checked={$settings?.tintLogo ?? false} disabled={busy || editing} label={msg('settings.accentTintLogo')} onchange={tint} /></div>
</Card>

<style>
  .hint { color: var(--text-2); font-size: var(--font-xs); margin-bottom: var(--space-3); }
  .choices, .editor { display: flex; flex-wrap: wrap; align-items: center; gap: var(--space-2); }
  .choices button { min-height: 3.6rem; padding: .5rem 1rem; border: 2px solid var(--border-strong); border-radius: var(--radius-md); background: var(--surface-2); color: var(--text); }
  .choices button.chosen { border-color: var(--accent-text); outline: 1px solid var(--accent-text); outline-offset: 2px; }
  .choices button:disabled { opacity: .5; }
  .choices .swatch { padding: .4rem; }
  .swatch span { display: block; width: 2.4rem; height: 2.4rem; border-radius: 50%; background: var(--swatch); }
  .editor { margin-top: var(--space-4); }
  label { display: flex; align-items: center; gap: var(--space-2); font-size: var(--font-sm); }
  input[type=text] { width: 10rem; padding: .7rem; background: var(--surface-2); color: var(--text); border: 1px solid var(--border-strong); border-radius: var(--radius-sm); }
  .tint { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); margin-top: var(--space-4); font-size: var(--font-md); }
</style>
