<script lang="ts">
  import Tooltip from './Tooltip.svelte';

  let {
    kind = 'neutral',
    label,
    dot = true,
    plain = false,
    hint,
  }: {
    kind?: 'success' | 'warning' | 'danger' | 'accent' | 'neutral';
    label: string;
    dot?: boolean;
    plain?: boolean;
    /** What the state means here, beyond its label: the badge joins the Tab order and shows it as a tooltip, its description. */
    hint?: string;
  } = $props();
</script>

{#snippet badge(focusable: boolean)}
  <!-- svelte-ignore a11y_no_noninteractive_tabindex (A stop in the Tab order only with a hint, so the hint reaches keyboard users; pressing it does nothing.) -->
  <span class="badge {kind}" class:plain tabindex={focusable ? 0 : undefined}>
    {#if dot}<span class="dot"></span>{/if}
    {label}
  </span>
{/snippet}

{#if hint}
  <Tooltip text={hint}>{@render badge(true)}</Tooltip>
{:else}
  {@render badge(false)}
{/if}

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    height: 2.4rem;
    padding: 0 0.9rem;
    border-radius: var(--radius-sm);
    font-size: var(--font-xs);
    font-weight: 500;
    white-space: nowrap;
  }

  .badge.plain {
    height: auto;
    padding: 0;
    gap: 0.7rem;
    background: none;
    font-size: var(--font-xs);
    font-weight: 400;
    color: var(--text-2);
  }

  .dot {
    width: 0.6rem;
    height: 0.6rem;
    border-radius: var(--radius-round);
    background: currentColor;
    flex-shrink: 0;
  }

  .plain .dot {
    width: 0.7rem;
    height: 0.7rem;
  }

  .success {
    background: var(--success-subtle);
    color: var(--success);
  }

  .warning {
    background: var(--warning-subtle);
    color: var(--warning);
  }

  .danger {
    background: var(--danger-subtle);
    color: var(--danger);
  }

  .accent {
    background: var(--accent-subtle);
    color: var(--accent-text);
  }

  .neutral {
    background: var(--hover-strong);
    color: var(--text-2);
  }

  .plain.success .dot {
    background: var(--success);
  }

  .plain.warning .dot {
    background: var(--warning);
  }

  .plain.danger .dot {
    background: var(--danger);
  }

  .plain.accent .dot {
    background: var(--accent-bright);
  }

  .plain.neutral .dot {
    background: var(--text-3);
  }
</style>
