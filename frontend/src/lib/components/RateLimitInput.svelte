<script lang="ts">
  import { tick, untrack } from 'svelte';
  import Select from './Select.svelte';
  import { rateLimitLabel, rateMbText } from '../utils/format';
  import { msg } from '../i18n';

  let {
    value,
    presets,
    width = '20rem',
    onchange,
  }: {
    value: number;
    presets: number[];
    width?: string;
    onchange: (bytes: number) => void;
  } = $props();

  const MB = 1024 * 1024;
  const minMb = 0.1;
  const maxMb = 1000;

  const options = $derived([
    { id: 'none', label: rateLimitLabel(0) },
    ...presets.map((mb) => ({ id: String(mb), label: rateLimitLabel(mb * MB) })),
    { id: 'custom', label: msg('ui.customValue') },
  ]);

  const presetId = $derived.by(() => {
    if (!value || value <= 0) return 'none';
    const mb = String(value / MB);
    return presets.some((p) => String(p) === mb) ? mb : 'custom';
  });

  let picked = $state(false);
  let editing = $state(false);
  let draft = $state('');
  let error = $state('');
  let field = $state<HTMLInputElement | null>(null);

  const mode = $derived(picked || presetId === 'custom' ? 'custom' : presetId);

  $effect(() => {
    const next = value > 0 ? rateMbText(value) : '';
    untrack(() => {
      if (!editing) draft = next;
    });
  });

  async function select(id: string) {
    if (id === 'custom') {
      picked = true;
      await tick();
      field?.focus();
      return;
    }
    picked = false;
    editing = false;
    error = '';
    onchange(id === 'none' ? 0 : Number(id) * MB);
  }

  function commit() {
    editing = false;
    const raw = draft.trim().replace(',', '.');
    if (raw === '') {
      error = '';
      draft = value > 0 ? rateMbText(value) : '';
      return;
    }
    const mb = Number(raw);
    if (!Number.isFinite(mb) || mb < minMb || mb > maxMb) {
      error = msg('ui.numberRangeHint', { min: rateMbText(minMb * MB), max: rateMbText(maxMb * MB) });
      return;
    }
    error = '';
    const bytes = Math.round(mb * MB);
    draft = rateMbText(bytes);
    if (bytes !== value) onchange(bytes);
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
      draft = value > 0 ? rateMbText(value) : '';
    }
  }
</script>

<div class="limit" style:width>
  <Select value={mode} {options} width="100%" onchange={select} />
  {#if mode === 'custom'}
    <div class="custom">
      <input
        bind:this={field}
        class="input"
        type="text"
        inputmode="decimal"
        placeholder={rateMbText(50 * MB)}
        aria-label={msg('ui.speedMbps')}
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
      <span class="unit">{msg('units.mbs')}</span>
    </div>
    {#if error}
      <span class="error">{error}</span>
    {/if}
  {/if}
</div>

<style>
  .limit {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  .custom {
    position: relative;
  }

  .custom .input {
    padding-right: 5.4rem;
  }

  .unit {
    position: absolute;
    top: 50%;
    right: 1.2rem;
    transform: translateY(-50%);
    font-size: var(--font-sm);
    color: var(--text-3);
    pointer-events: none;
  }

  .error {
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
