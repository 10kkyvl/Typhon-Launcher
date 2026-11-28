<script lang="ts">
  import Notice from './Notice.svelte';
  import Select from './Select.svelte';
  import Toggle from './Toggle.svelte';
  import { msg } from '../i18n';
  import { gameModeKeys, inheritLabel, overrideState, type GameModeKey, type OverrideState } from '../gamemode/gamemode';
  import { gameModeTitles } from '../game/gameModeWrites.svelte';
  import type { LibraryGame } from '../services/library';
  import { settings } from '../stores/settings';

  // What GamePropertiesModal sets for the time the game runs: the game mode overrides and whether
  // friends see it. The dialog holds their state and does their writes.
  let {
    game,
    gameModeShown,
    gameModeMac,
    gameModeError,
    gameModeChosen,
    ongamemode,
    presence,
    presenceBusy,
    presenceError,
    onpresence,
  }: {
    game: LibraryGame;
    gameModeShown: boolean;
    /** On macOS only hiding the launcher applies. */
    gameModeMac: boolean;
    gameModeError: string;
    /** The change a row shows while it waits, is sent, or until the library holds it. */
    gameModeChosen: (key: GameModeKey) => OverrideState | undefined;
    ongamemode: (key: GameModeKey, state: OverrideState) => void;
    presence: boolean;
    presenceBusy: boolean;
    presenceError: string;
    onpresence: (on: boolean) => void;
  } = $props();
  const uid = $props.id();
  const held = $derived(msg('modals.actionRunning'));

  const gameModeRows = $derived(
    gameModeKeys
      .filter((key) => !(gameModeMac && key !== 'hideLauncher'))
      .map((key) => ({
        key,
        title: msg(gameModeTitles[key]),
        options: [
          { id: 'inherit', label: inheritLabel(Boolean($settings?.gameMode?.[key])) },
          { id: 'on', label: msg('modals.propertiesGameModeOn') },
          { id: 'off', label: msg('modals.propertiesGameModeOff') },
        ],
      })),
  );
</script>

{#if gameModeShown}
  <section class="section">
    <h4>{msg('modals.propertiesGameModeTitle')}</h4>
    {#each gameModeRows as row (row.key)}
      <div class="row spread">
        <span class="label" id="{uid}-gamemode-{row.key}-label">{row.title}</span>
        <Select
          labelledby="{uid}-gamemode-{row.key}-label"
          value={gameModeChosen(row.key) ?? overrideState(game.gameMode, row.key)}
          width="26rem"
          options={row.options}
          onchange={(id) => ongamemode(row.key, id as OverrideState)}
        />
      </div>
    {/each}
    <p class="hint">{msg('modals.propertiesGameModeHint')}</p>
    {#if gameModeError}
      <Notice tone="danger" compact>{gameModeError}</Notice>
    {/if}
  </section>
{/if}

<section class="section">
  <div class="row spread">
    <div class="title">
      <h4 id="{uid}-properties-presence-label">{msg('modals.propertiesPresenceTitle')}</h4>
      {#if presenceBusy}
        <p class="hint" id="{uid}-presence-held">{held}</p>
      {/if}
    </div>
    <Toggle
      checked={presence}
      labelledby="{uid}-properties-presence-label"
      describedby={presenceBusy ? `${uid}-presence-held` : undefined}
      aria-disabled={presenceBusy ? 'true' : undefined}
      onchange={onpresence}
    />
  </div>
  <p class="hint">{msg('modals.propertiesPresenceHint')}</p>
  {#if presenceError}
    <Notice tone="danger" compact>{presenceError}</Notice>
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

  /* A switch's row: its heading, and the reason it is held while its change runs. */
  .title {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
</style>
