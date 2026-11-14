<script lang="ts">
  import { untrack, type Snippet } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import PlayProfileMenu from '../../lib/components/PlayProfileMenu.svelte';
  import ProgressBar from '../../lib/components/ProgressBar.svelte';
  import { profileChoices } from '../../lib/game/profiles';
  import type { PrimaryKind } from '../../lib/game/view';
  import { msg } from '../../lib/i18n';
  import type { LibraryGame } from '../../lib/services/library';
  import { focusHandOff } from '../../lib/utils/focusHandOff';

  /**
   * The game page's main action: one button for the whole life of the page, so keyboard focus and a
   * screen reader stay on it while it changes from Play to launching to Stop, or from Download to the
   * download's progress. Only Play offers the game's launch profiles: when there is a choice it attaches
   * the profile chevron, and the two read as one control, a group named "Play". Any other action (stop,
   * download, progress) is the plain button whatever profiles the game has. `busy` holds the button
   * while the game is launching or stopping and while its progress shows: it ignores presses and keeps
   * focus. `progress` (0 to 100) draws that progress inside the button, whose label says it in words.
   */
  let {
    kind,
    game,
    disabled = false,
    busy = false,
    progress,
    element = $bindable(),
    onclick,
    onpick,
    children,
  }: {
    kind: PrimaryKind;
    game?: Pick<LibraryGame, 'profiles' | 'lastProfile' | 'emulation'>;
    disabled?: boolean;
    busy?: boolean;
    progress?: { value: number; indeterminate?: boolean } | null;
    element?: HTMLButtonElement;
    onclick: (event: MouseEvent) => void;
    onpick: (profileID: string) => void;
    children: Snippet;
  } = $props();

  let lead = $state<HTMLSpanElement>();

  const launched = $derived(kind === 'play' && !busy ? game : undefined);
  const split = $derived(launched ? profileChoices(launched).length > 1 : false);

  // The chevron goes when Play turns into anything else; if it held focus (a profile was just picked
  // from its menu), focus moves to the main button instead of falling to the page. Runs before the
  // chevron leaves the page, while focus is still on it.
  $effect.pre(() => {
    if (split) return;
    untrack(() => {
      const active = document.activeElement;
      if (!element || !lead || !active || active === element || !lead.contains(active)) return;
      void focusHandOff()(() => element);
    });
  });

  const attach = (node: HTMLButtonElement) => {
    element = node;
  };

  // A held Enter repeats its press; the key that started a launch would then land on Stop.
  function holdBack(event: KeyboardEvent) {
    if (event.key === 'Enter' && event.repeat) event.preventDefault();
  }
</script>

<span class="lead" bind:this={lead} role={split ? 'group' : undefined} aria-label={split ? msg('games.play') : undefined}>
  <Button
    variant={progress ? 'secondary' : 'primary'}
    size="lg"
    minWidth="18rem"
    join={split ? 'end' : undefined}
    {disabled}
    {busy}
    {onclick}
    onkeydown={holdBack}
    {@attach attach}
  >
    {@render children()}
    {#if progress}
      <span class="meter">
        <ProgressBar value={progress.value} indeterminate={progress.indeterminate} stretch decorative />
      </span>
    {/if}
  </Button>
  {#if launched}
    <PlayProfileMenu game={launched} {disabled} {onpick} />
  {/if}
</span>

<style>
  .lead {
    position: relative;
    display: inline-flex;
  }

  .meter {
    position: absolute;
    inset: auto var(--space-5) var(--space-2);
    display: flex;
    pointer-events: none;
  }
</style>
