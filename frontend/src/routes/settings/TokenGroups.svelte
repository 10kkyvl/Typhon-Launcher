<script lang="ts">
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import { msg } from '../../lib/i18n';
  import {
    groupedTokenNames,
    groupTitleKey,
    isHexColor,
    tokenGroups,
    tokenHintKey,
    tokenKey,
    tokenLabelKey,
    type TokenGroupId,
  } from '../../lib/theme/tokens';
  import TokenValueInput from './TokenValueInput.svelte';

  /** The draft theme's tokens by group: a colour picker where the value is a colour, and its value field. */
  let { tokens, onchange }: { tokens: Record<string, string>; onchange: (name: string, value: string) => void } = $props();

  interface TokenSection {
    id: TokenGroupId | 'other';
    colors: boolean;
    collapsed: boolean;
    names: string[];
  }

  const sections = $derived.by<TokenSection[]>(() => {
    const present = new Set(Object.keys(tokens));
    const found: TokenSection[] = tokenGroups.map((group) => ({
      id: group.id,
      colors: group.colors,
      collapsed: group.collapsed,
      names: group.tokens.filter((name) => present.has(name)),
    }));
    found.push({
      id: 'other',
      colors: false,
      collapsed: false,
      names: [...present].filter((name) => !groupedTokenNames.has(name)),
    });
    return found.filter((section) => section.names.length > 0);
  });
</script>

<div class="token-groups">
  {#each sections as section (section.id)}
    <details class="token-group" open={!section.collapsed}>
      <summary>{msg(groupTitleKey(section.id))}</summary>
      <div class="token-rows">
        {#each section.names as name (name)}
          {@const value = tokens[name] ?? ''}
          {@const known = groupedTokenNames.has(name)}
          {@const label = known ? msg(tokenLabelKey(name)) : name}
          {@const fieldId = `theme-token-${tokenKey(name)}`}
          <div class="token-row">
            <div class="token-text">
              <label class="token-label" for={fieldId}>{label}</label>
              {#if known}
                <span class="token-hint" id={`${fieldId}-hint`}>{msg(tokenHintKey(name))}</span>
              {/if}
            </div>
            <div class="token-controls">
              {#if section.colors}
                {#if isHexColor(value)}
                  <Tooltip text={name}>
                    <input
                      class="color-input"
                      type="color"
                      aria-label={label}
                      {value}
                      oninput={(e) => onchange(name, (e.currentTarget as HTMLInputElement).value)}
                    />
                  </Tooltip>
                {:else}
                  <span class="swatch" style:background={value} aria-hidden="true"></span>
                {/if}
              {/if}
              <TokenValueInput
                id={fieldId}
                {name}
                {value}
                describedBy={known ? `${fieldId}-hint` : undefined}
                onchange={(next) => onchange(name, next)}
              />
            </div>
          </div>
        {/each}
      </div>
    </details>
  {/each}
</div>

<style>
  .token-groups {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .token-group {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .token-group > summary {
    padding: var(--space-3) var(--space-4);
    font-size: var(--font-sm);
    font-weight: 600;
    cursor: pointer;
  }

  .token-rows {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: 0 var(--space-4) var(--space-4);
  }

  .token-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-4);
  }

  .token-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex: 1 1 16rem;
    min-width: 0;
  }

  .token-label {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
  }

  .token-hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .token-controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 1 22rem;
    min-width: 0;
  }

  .swatch {
    width: 3.4rem;
    height: var(--control-sm);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    flex-shrink: 0;
  }

  .color-input {
    width: 3.4rem;
    height: var(--control-sm);
    padding: 0.2rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    flex-shrink: 0;
  }
</style>
