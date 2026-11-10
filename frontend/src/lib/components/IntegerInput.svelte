<script lang="ts">
  import { untrack } from 'svelte';
  import { msg } from '../i18n';

  let {
    value,
    min,
    max,
    label,
    width = '12rem',
    onchange,
  }: {
    value: number;
    min: number;
    max: number;
    label: string;
    width?: string;
    onchange: (value: number) => void;
  } = $props();

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
    aria-label={label}
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
