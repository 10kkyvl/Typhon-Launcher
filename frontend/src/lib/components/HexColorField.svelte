<script lang="ts">
  import { Clipboard } from '@wailsio/runtime';
  import { Copy } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';
  import { validAccent } from '../theme/accent';
  import { msg } from '../i18n';
  import { toast } from '../stores/toasts';

  let { value, readonly = false, disabled = false, onchange, onkeydown }: {
    value: string; readonly?: boolean; disabled?: boolean;
    onchange?: (value: string) => void; onkeydown?: (event: KeyboardEvent) => void;
  } = $props();

  function input(event: Event) {
    const field = event.currentTarget as HTMLInputElement;
    const raw = field.value.trim();
    const normalized = /^[0-9a-f]{6}$/i.test(raw) ? `#${raw}` : raw;
    onchange?.(normalized.toUpperCase());
  }
  async function copy() {
    if (!validAccent(value) || disabled) return;
    try {
      await Clipboard.SetText(value.toUpperCase());
      toast(msg('social.copied'), 'info');
    } catch { toast(msg('social.copyFailed'), 'danger'); }
  }
</script>

<div class="hex-field">
  <label>HEX
    <input type="text" value={value.toUpperCase()} {readonly} {disabled} placeholder="#RRGGBB"
      spellcheck={false} autocapitalize="characters" autocomplete="off" aria-invalid={!validAccent(value)}
      oninput={input} {onkeydown} />
  </label>
  <IconButton size="sm" label={msg('settings.accentCopyHex')} disabled={disabled || !validAccent(value)} onclick={copy}><Copy size="1.5rem" /></IconButton>
</div>

<style>
  .hex-field { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  label { display: flex; align-items: center; gap: var(--space-2); color: var(--text-2); font-size: var(--font-xs); }
  input { width: 10rem; min-width: 0; padding: .7rem; background: var(--surface-2); color: var(--text); border: 1px solid var(--border-strong); border-radius: var(--radius-sm); font: inherit; font-variant-numeric: tabular-nums; }
  input[aria-invalid=true] { border-color: var(--danger); }
  input:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
</style>
