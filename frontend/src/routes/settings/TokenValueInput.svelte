<script lang="ts">
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { TOKEN_RANGES, tokenRangeProblem, tokenRangeText } from '../../lib/theme/tokenRanges';

  let {
    id,
    name,
    value,
    describedBy,
    onchange,
  }: {
    id: string;
    name: string;
    value: string;
    describedBy?: string;
    onchange: (value: string) => void;
  } = $props();

  // A ranged number token gets a numeric field with its range; every other kind stays free text.
  const range = $derived(TOKEN_RANGES[name]);
  const problem = $derived(tokenRangeProblem(name, value));
  const problemId = $derived(`${id}-range`);
  const described = $derived([describedBy, problem ? problemId : ''].filter(Boolean).join(' ') || undefined);
</script>

<!-- The token's CSS name is the field's description: a theme author looks it up in THEMING.md. -->
<span class="token-field">
  <Tooltip text={name}>
  {#if range}
    <input
      {id}
      class="input sm token-value"
      type="number"
      min={range.min}
      max={range.max}
      step={range.step}
      aria-invalid={problem ? 'true' : undefined}
      aria-describedby={described}
      {value}
      oninput={(e) => onchange((e.currentTarget as HTMLInputElement).value)}
    />
  {:else}
    <input
      {id}
      class="input sm token-value"
      type="text"
      aria-describedby={describedBy}
      {value}
      oninput={(e) => onchange((e.currentTarget as HTMLInputElement).value)}
    />
  {/if}
  </Tooltip>
  {#if problem}
    <span class="token-problem" id={problemId} role="alert">{tokenRangeText(problem)}</span>
  {/if}
</span>

<style>
  .token-field {
    display: flex;
    flex: 1 1 auto;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .token-value {
    width: 100%;
    min-width: 0;
  }

  .token-value[aria-invalid='true'] {
    border-color: var(--danger);
  }

  .token-problem {
    font-size: var(--font-xs);
    color: var(--danger);
  }
</style>
