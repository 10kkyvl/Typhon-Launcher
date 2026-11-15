<script lang="ts">
  import { untrack } from 'svelte';
  import { msg } from '../i18n';

  // The field has one name: its own `label`, or an `id` a `<label for>` names it through (a settings
  // row's `labelFor`). Never both: an `aria-label` would take over from the visible label, so with an
  // `id` it is left off. IntegerInput.callers.test.ts holds every call site to a label for each id.
  type Named = { label: string; id?: undefined } | { id: string; label?: undefined };

  let {
    value,
    min,
    max,
    label,
    id,
    describedby,
    width = '12rem',
    onchange,
  }: {
    value: number;
    min: number;
    max: number;
    /** Id of the text that says what the field sets. */
    describedby?: string;
    width?: string;
    onchange: (value: number) => void;
  } & Named = $props();

  let draft = $state('');
  let editing = $state(false);
  let error = $state('');

  $effect(() => {
    const next = String(value);
    untrack(() => {
      if (!editing) draft = next;
    });
  });

  function commit() {
    editing = false;
    const raw = draft.trim();
    const parsed = Number(raw);
    if (raw === '' || !Number.isInteger(parsed) || parsed < min || parsed > max) {
      error = msg('ui.numberRangeHint', { min, max });
      return;
    }
    error = '';
    draft = String(parsed);
    if (parsed !== value) onchange(parsed);
  }

  function keydown(event: KeyboardEvent) {
    if (event.key === 'Enter') {
      event.preventDefault();
      commit();
      return;
    }
    if (event.key === 'Escape') {
      editing = false;
      error = '';
      draft = String(value);
    }
  }
</script>

<div class="integer" style:width>
  <input
    class="input"
    type="text"
    inputmode="numeric"
    {id}
    aria-label={id ? undefined : label}
    aria-describedby={describedby}
    aria-invalid={error !== ''}
    value={draft}
    oninput={(event) => {
      editing = true;
      draft = event.currentTarget.value;
      error = '';
    }}
    onblur={commit}
    onkeydown={keydown}
  />
  {#if error}
    <span class="error">{error}</span>
  {/if}
</div>

<style>
  .integer {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .error {
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
