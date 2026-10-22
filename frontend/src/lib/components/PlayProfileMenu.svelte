<script lang="ts">
  import { ChevronDown } from '@lucide/svelte';
  import DropdownMenu from './DropdownMenu.svelte';
  import { lastProfileId, profileChoices } from '../game/profiles';
  import { msg } from '../i18n';
  import type { LibraryGame } from '../services/library';

  let {
    game,
    disabled = false,
    onpick,
  }: {
    game: Pick<LibraryGame, 'profiles' | 'lastProfile'>;
    disabled?: boolean;
    onpick: (profileID: string) => void;
  } = $props();

  const choices = $derived(profileChoices(game));
  const last = $derived(lastProfileId(game));
  const items = $derived(choices.map((p) => ({ id: p.id, label: p.name, checked: p.id === last })));
</script>

{#if choices.length > 1}
  <DropdownMenu {items} align="left" onselect={onpick}>
    {#snippet trigger({ toggle })}
      <button class="chevron" aria-label={msg('games.playProfileChoose')} {disabled} onclick={toggle}>
        <ChevronDown size="1.8rem" strokeWidth={2} />
      </button>
    {/snippet}
  </DropdownMenu>
{/if}

<style>
  .chevron {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: var(--control-lg);
    height: var(--control-lg);
    margin-left: 1px;
    border: 1px solid transparent;
    border-radius: 0 var(--radius-md) var(--radius-md) 0;
    background: var(--accent);
    color: var(--accent-on, #fff);
    cursor: pointer;
    transition: background var(--dur) var(--ease);
  }

  .chevron:hover:not(:disabled) {
    background: var(--accent-hover);
  }

  .chevron:disabled {
    opacity: 0.45;
    cursor: default;
  }
</style>
