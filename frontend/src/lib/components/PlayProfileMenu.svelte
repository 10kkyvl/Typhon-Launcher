<script lang="ts">
  import { ChevronDown } from '@lucide/svelte';
  import Button from './Button.svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import { lastProfileId, profileChoices } from '../game/profiles';
  import { msg } from '../i18n';
  import type { LibraryGame } from '../services/library';

  let {
    game,
    disabled = false,
    onpick,
  }: {
    game: Pick<LibraryGame, 'profiles' | 'lastProfile' | 'emulation'>;
    disabled?: boolean;
    onpick: (profileID: string) => void;
  } = $props();

  const choices = $derived(profileChoices(game));
  const last = $derived(lastProfileId(game));
  const items = $derived(choices.map((p) => ({ id: p.id, label: p.name, checked: p.id === last })));
</script>

{#if choices.length > 1}
  <DropdownMenu {items} align="left" onselect={onpick}>
    {#snippet trigger({ props })}
      <Button {...props} variant="primary" size="lg" icon join="start" aria-label={msg('games.playProfileChoose')} {disabled}>
        <ChevronDown size="1.8rem" strokeWidth={2} />
      </Button>
    {/snippet}
  </DropdownMenu>
{/if}
