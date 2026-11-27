<script lang="ts">
  import Notice from './Notice.svelte';
  import Select from './Select.svelte';
  import Toggle from './Toggle.svelte';
  import { msg } from '../i18n';
  import { overrideState, type GameModeKey, type OverrideState } from '../gamemode/gamemode';
  import type { SelectOption } from './select';
  import type { LibraryGame } from '../services/library';

  // What GamePropertiesModal sets for the time the game runs: the game mode overrides and whether
  // friends see it. The dialog holds their state and does their writes.
  let {
    game,
    gameModeShown,
    gameModeRows,
    gameModeError,
    ongamemode,
    presence,
    presenceBusy,
    presenceError,
    onpresence,
  }: {
    game: LibraryGame;
    gameModeShown: boolean;
    gameModeRows: { key: GameModeKey; title: string; options: SelectOption[] }[];
    gameModeError: string;
    ongamemode: (key: GameModeKey, state: OverrideState) => void;
    presence: boolean;
    presenceBusy: boolean;
    presenceError: string;
    onpresence: (on: boolean) => void;
  } = $props();
  const uid = $props.id();
</script>

{#if gameModeShown}
  <section class="section">
    <h4>{msg('modals.propertiesGameModeTitle')}</h4>
    {#each gameModeRows as row (row.key)}
      <div class="row spread">
        <span class="label" id="{uid}-gamemode-{row.key}-label">{row.title}</span>
        <Select
          labelledby="{uid}-gamemode-{row.key}-label"
          value={overrideState(game.gameMode, row.key)}
          width="26rem"
          options={row.options}
          onchange={(id) => ongamemode(row.key, id as OverrideState)}
        />
      </div>
    {/each}
    <p class="hint">{msg('modals.propertiesGameModeHint')}</p>
    {#if gameModeError}
      <Notice tone="danger" compact announce={false}>{gameModeError}</Notice>
    {/if}
  </section>
{/if}

<section class="section">
  <div class="row spread">
    <h4 id="{uid}-properties-presence-label">{msg('modals.propertiesPresenceTitle')}</h4>
    <Toggle checked={presence} labelledby="{uid}-properties-presence-label" disabled={presenceBusy} onchange={onpresence} />
  </div>
  <p class="hint">{msg('modals.propertiesPresenceHint')}</p>
  {#if presenceError}
    <Notice tone="danger" compact announce={false}>{presenceError}</Notice>
  {/if}
</section>

<style>
  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h4 {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text);
  }

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .spread {
    justify-content: space-between;
  }
</style>
