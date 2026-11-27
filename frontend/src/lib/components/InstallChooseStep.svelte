<script lang="ts">
  import Button from './Button.svelte';
  import { msg } from '../i18n';
  import { nextIndex } from '../utils/roving';

  /** InstallModal's choice of the game's executable among those the install found. */
  let {
    candidatePaths,
    chosen = $bindable(),
    onpickmanual,
  }: {
    candidatePaths: string[];
    chosen: string;
    onpickmanual: () => void;
  } = $props();

  function basename(path: string) {
    const parts = path.split(/[\\/]/);
    return parts[parts.length - 1] || path;
  }

  let candidateGroup = $state<HTMLElement>();

  // The found executables are a radio group: the arrows move the choice and the focus together.
  function moveChoice(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const radios = [...(candidateGroup?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];
    const next = nextIndex(radios.indexOf(event.currentTarget as HTMLElement), candidatePaths.length, event.key, { orientation: 'vertical' });
    if (next === null) return;
    event.preventDefault();
    chosen = candidatePaths[next];
    radios[next]?.focus();
  }
</script>

  <div class="block">
    <span class="lead">{msg('modals.installChooseGameExecutable')}</span>
    <div class="cand-list" role="radiogroup" aria-label={msg('modals.installExecutablesAriaLabel')} bind:this={candidateGroup}>
      {#each candidatePaths as path, index (path)}
        <button
          type="button"
          class="cand"
          role="radio"
          aria-checked={chosen === path}
          tabindex={chosen === path || (!candidatePaths.includes(chosen) && index === 0) ? 0 : -1}
          onclick={() => (chosen = path)}
          onkeydown={moveChoice}
        >
          <span class="radio" class:on={chosen === path} aria-hidden="true"></span>
          <span class="cand-text">
            <span class="cand-name">{basename(path)}</span>
            <span class="cand-path" title={path}>{path}</span>
          </span>
        </button>
      {/each}
    </div>
    <Button size="sm" onclick={onpickmanual}>{msg('modals.installPickManually')}</Button>
  </div>

<style>
  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .lead {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .cand-list {
    display: flex;
    flex-direction: column;
    max-height: 26rem;
    overflow-y: auto;
    padding: var(--space-1);
    background: var(--surface);
    border-radius: var(--radius-md);
  }

  .cand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    width: 100%;
    padding: 0.8rem;
    border-radius: var(--radius-sm);
    text-align: left;
    transition: background var(--dur-fast) var(--ease);
  }

  .cand:hover {
    background: var(--hover);
  }

  .cand[aria-checked='true'] {
    background: var(--hover-strong);
  }

  /* Inside the scrolling list: the shared ring is drawn inward, where the list does not clip it. */
  .cand:focus-visible {
    outline-offset: -2px;
  }

  .radio {
    width: 1.6rem;
    height: 1.6rem;
    flex-shrink: 0;
    border-radius: 50%;
    border: 1px solid var(--border-strong);
    background: var(--surface-2);
    transition:
      background var(--dur-fast) var(--ease),
      border-color var(--dur-fast) var(--ease),
      box-shadow var(--dur-fast) var(--ease);
  }

  .radio.on {
    background: var(--accent-bright);
    border-color: var(--accent-bright);
    box-shadow: inset 0 0 0 3px var(--surface-2);
  }

  .cand-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .cand-name {
    font-size: var(--font-sm);
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .cand-path {
    font-size: 1.2rem;
    color: var(--text-3);
    white-space: normal;
    overflow-wrap: anywhere;
    user-select: text;
  }
</style>
