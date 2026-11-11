<script lang="ts">
  import type { Snippet } from 'svelte';
  import Button from '../../lib/components/Button.svelte';
  import PlayProfileMenu from '../../lib/components/PlayProfileMenu.svelte';
  import { profileChoices } from '../../lib/game/profiles';
  import { msg } from '../../lib/i18n';
  import type { LibraryGame } from '../../lib/services/library';

  /**
   * The game page's main action. Given the game whose launch profiles it may offer, it attaches the
   * profile chevron when there is a choice, and the two read as one control: a group named "Play".
   */
  let {
    game,
    disabled = false,
    onclick,
    onpick,
    children,
  }: {
    game?: Pick<LibraryGame, 'profiles' | 'lastProfile' | 'emulation'>;
    disabled?: boolean;
    onclick: () => void;
    onpick: (profileID: string) => void;
    children: Snippet;
  } = $props();

  const split = $derived(game ? profileChoices(game).length > 1 : false);
</script>

<span class="lead" role={split ? 'group' : undefined} aria-label={split ? msg('games.play') : undefined}>
  <Button variant="primary" size="lg" minWidth="18rem" join={split ? 'end' : undefined} {disabled} {onclick}>
    {@render children()}
  </Button>
  {#if game}
    <PlayProfileMenu {game} {disabled} {onpick} />
  {/if}
</span>

<style>
  .lead {
    display: inline-flex;
  }
</style>
