<script lang="ts">
  import { Clipboard } from '@wailsio/runtime';
  import { Copy } from '@lucide/svelte';
  import IconButton from './IconButton.svelte';
  import { validAccent } from '../theme/accent';
  import { msg } from '../i18n';
  import { toast } from '../stores/toasts';

  /**
   * A colour typed as #RRGGBB, with a copy button. Several stand in one panel, so each is named by
   * the colour it holds (`label`), the visible "HEX" joining the name. `id` and `describedby` let the
   * page tie its own message (an invalid colour) to the field. `optional`: an empty value is valid
   * (the colour falls back to another), so it is not called invalid while nothing on screen says so.
   */
  let {
    value,
    label,
    id,
    describedby,
    optional = false,
    readonly = false,
    disabled = false,
    onchange,
    onkeydown,
  }: {
    value: string;
    /** What colour this is ("Frame color"): the field's name with "HEX". */
    label: string;
    id?: string;
    /** Id of the page's text that describes the field (its invalid-colour message). */
    describedby?: string;
    optional?: boolean;
    readonly?: boolean;
    disabled?: boolean;
    onchange?: (value: string) => void;
    onkeydown?: (event: KeyboardEvent) => void;
  } = $props();

  const uid = $props.id();
  const inputId = $derived(id ?? `${uid}-input`);
  const invalid = $derived(!(optional && value === '') && !validAccent(value));

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
    } catch {
      toast(msg('social.copyFailed'), 'danger');
    }
  }
</script>

<div class="hex-field">
  <span id="{uid}-name" hidden>{label}</span>
  <label class="tag" id="{uid}-hex" for={inputId}>HEX</label>
  <input
    id={inputId}
    class="input sm"
    type="text"
    value={value.toUpperCase()}
    {readonly}
    {disabled}
    placeholder="#RRGGBB"
    spellcheck={false}
    autocapitalize="characters"
    autocomplete="off"
    aria-labelledby="{uid}-name {uid}-hex"
    aria-describedby={describedby}
    aria-invalid={invalid || undefined}
    oninput={input}
    {onkeydown}
  />
  <IconButton size="sm" label={msg('settings.copyHexOf', { name: label })} disabled={disabled || !validAccent(value)} onclick={copy}><Copy size="1.5rem" /></IconButton>
</div>

<style>
  .hex-field { display: flex; align-items: center; gap: var(--space-2); min-width: 0; }
  .tag { color: var(--text-2); font-size: var(--font-xs); }
  .input { width: 10rem; padding: 0 var(--space-2); border-radius: var(--radius-sm); font-variant-numeric: tabular-nums; }
  .input[aria-invalid='true'] { border-color: var(--danger); }
</style>
