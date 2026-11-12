<script lang="ts">
  import { Trash2 } from '@lucide/svelte';
  import { tick } from 'svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import type { ConfirmPrompt } from '../../lib/confirm/prompts';
  import { msg } from '../../lib/i18n';
  import type { Theme } from '../../lib/services/theme';
  import { activeTheme, selectTheme, themeList, themeMode } from '../../lib/stores/theme';
  import { fallbackAccent, fallbackBackground, themeDisplayName } from '../../lib/theme/apply';
  import { themeErrorText } from '../../lib/theme/themeErrors';
  import { nextIndex } from '../../lib/utils/roving';

  let {
    unsaved,
    name,
    deleting,
    onpick,
    onremove,
  }: {
    /** The draft being edited has unsaved edits: choosing another theme asks first. */
    unsaved: boolean;
    /** The draft's name, said in that question. */
    name: string;
    deleting: boolean;
    /** A theme was chosen: the page starts editing it. */
    onpick: (theme: Theme) => void;
    onremove: (theme: Theme) => void;
  } = $props();

  const list = $derived($themeList);
  const active = $derived($activeTheme);

  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);

  async function pickTheme(theme: Theme) {
    onpick(theme);
    await selectTheme(theme.id);
  }

  // The system choice, then every theme: one choice, a radio group of tiles.
  const presetChecked = $derived([
    $themeMode === 'system',
    ...list.map((theme) => $themeMode === 'theme' && active?.id === theme.id),
  ]);
  const presetStop = $derived(Math.max(0, presetChecked.indexOf(true)));
  let presetGrid = $state<HTMLElement>();

  // While a choice is applied the tiles refuse presses (aria-disabled keeps focus on them).
  let choosing = $state(false);
  // The tile a confirmed choice goes to: focus follows it once the question closes.
  let chosenAfterAsking: number | null = null;
  const presetRadios = () => [...(presetGrid?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];

  async function applyPreset(index: number) {
    choosing = true;
    try {
      if (index === 0) await selectTheme('system');
      else await pickTheme(list[index - 1]);
    } finally {
      choosing = false;
    }
  }

  // Choosing a theme replaces the draft: with unsaved edits it asks first. The system choice keeps
  // the draft. Returns whether the choice was made at once.
  function choosePreset(index: number): boolean {
    if (choosing || presetChecked[index]) return false;
    if (index > 0 && unsaved) {
      pending = {
        prompt: {
          title: msg('settings.themeDiscardTitle'),
          text: msg('settings.themeDiscardText', { name }),
          confirm: msg('settings.themeCustomRevert'),
          cancel: msg('settings.themeDiscardStay'),
        },
        run: async () => {
          await applyPreset(index);
          chosenAfterAsking = index;
        },
      };
      return false;
    }
    void applyPreset(index);
    return true;
  }

  async function closePending() {
    const target = chosenAfterAsking;
    chosenAfterAsking = null;
    pending = null;
    if (target === null) return;
    await tick();
    presetRadios()[target]?.focus();
  }

  function presetKeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const radios = presetRadios();
    const next = nextIndex(radios.indexOf(event.currentTarget as HTMLElement), radios.length, event.key, { orientation: 'both' });
    if (next === null) return;
    event.preventDefault();
    if (choosePreset(next)) radios[next]?.focus();
  }
</script>

<div class="preset-grid" role="radiogroup" aria-label={msg('settings.appearanceTab')} bind:this={presetGrid}>
  <button
    type="button"
    role="radio"
    class="preset"
    class:selected={presetChecked[0]}
    aria-checked={presetChecked[0]}
    aria-disabled={choosing || undefined}
    tabindex={presetStop === 0 ? 0 : -1}
    onclick={() => choosePreset(0)}
    onkeydown={presetKeydown}
  >
    <span class="preset-swatch system" style={`background: linear-gradient(135deg, ${fallbackBackground.dark} 50%, ${fallbackBackground.light} 50%);`}></span>
    <span class="preset-name">{msg('settings.appearanceSystemPresetLabel')}</span>
  </button>
  {#each list as theme, i (theme.id)}
    <div class="preset-cell">
      <button
        type="button"
        role="radio"
        class="preset"
        class:selected={presetChecked[i + 1]}
        aria-checked={presetChecked[i + 1]}
        aria-disabled={choosing || undefined}
        tabindex={presetStop === i + 1 ? 0 : -1}
        onclick={() => choosePreset(i + 1)}
        onkeydown={presetKeydown}
      >
        <span
          class="preset-swatch"
          style={`background: ${theme.tokens['--bg'] ?? (theme.base === 'light' ? fallbackBackground.light : fallbackBackground.dark)}; border-color: ${theme.tokens['--accent'] ?? fallbackAccent};`}
        ></span>
        <span class="preset-name">{themeDisplayName(theme)}</span>
        {#if !theme.builtIn}
          <span class="preset-tag"><StatusBadge kind="accent" dot={false} label={msg('settings.appearanceCustomThemeTag')} /></span>
        {/if}
      </button>
      {#if !theme.builtIn}
        <span class="preset-delete">
          <IconButton
            size="sm"
            label={msg('settings.appearanceDeleteTile', { name: themeDisplayName(theme) })}
            disabled={deleting}
            onclick={() => onremove(theme)}
          >
            <Trash2 size={14} />
          </IconButton>
        </span>
      {/if}
    </div>
  {/each}
</div>

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={closePending} errorText={(err) => themeErrorText(err)} />
{/if}

<style>
  .preset-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .preset-cell {
    position: relative;
    display: flex;
  }

  .preset-cell > .preset {
    flex: 1 1 auto;
    min-width: 0;
    padding-right: 3.4rem;
  }

  /* The delete button shows on the tile's hover and on keyboard focus. */
  .preset-delete {
    position: absolute;
    top: 50%;
    right: var(--space-2);
    transform: translateY(-50%);
    display: inline-flex;
    opacity: 0;
    transition: opacity var(--dur) var(--ease);
  }

  .preset-cell:hover .preset-delete,
  .preset-delete:focus-within {
    opacity: 1;
  }

  @media (hover: none) {
    .preset-delete {
      opacity: 1;
    }
  }

  .preset {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    text-align: left;
    transition:
      border-color var(--dur) var(--ease),
      background var(--dur) var(--ease);
  }

  .preset:hover {
    background: var(--surface-3);
  }

  .preset[aria-disabled='true'] {
    cursor: progress;
  }

  .preset.selected {
    border-color: var(--accent-bright);
    background: var(--accent-subtle);
  }

  .preset-swatch {
    width: 2.4rem;
    height: 2.4rem;
    border-radius: var(--radius-sm);
    border: 2px solid var(--border-strong);
    flex-shrink: 0;
  }

  .preset-name {
    font-size: var(--font-sm);
    font-weight: 500;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .preset-tag {
    display: inline-flex;
    margin-left: auto;
    flex-shrink: 0;
  }
</style>
