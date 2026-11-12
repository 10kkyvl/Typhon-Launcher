<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import PlayProfileMenu from '../../lib/components/PlayProfileMenu.svelte';
  import { profileChoices } from '../../lib/game/profiles';
  import type { PrimaryKind } from '../../lib/game/view';
  import { msg } from '../../lib/i18n';
  import type { LibraryGame } from '../../lib/services/library';

  /**
   * The game page's main action. Only Play offers the game's launch profiles: when there is a choice
   * it attaches the profile chevron, and the two read as one control, a group named "Play". Any other
   * action (stop, download) is a plain button whatever profiles the game has.
   */
  let {
    kind,
    game,
    disabled = false,
    onclick,
    onpick,
    children,
  }: {
    kind: PrimaryKind;
    game?: Pick<LibraryGame, 'profiles' | 'lastProfile' | 'emulation'>;
    disabled?: boolean;
    onclick: () => void;
    onpick: (profileID: string) => void;
    children: Snippet;
  } = $props();

  const launched = $derived(kind === 'play' ? game : undefined);
  const split = $derived(launched ? profileChoices(launched).length > 1 : false);
</script>

<span class="lead" role={split ? 'group' : undefined} aria-label={split ? msg('games.play') : undefined}>
  <Button variant="primary" size="lg" minWidth="18rem" join={split ? 'end' : undefined} {disabled} {onclick}>
    {@render children()}
  </Button>
  {#if launched}
    <PlayProfileMenu game={launched} {disabled} {onpick} />
  {/if}
</span>

<style>
  .lead {
    display: inline-flex;
  }
</style>
