<script lang="ts">
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import TextField from './TextField.svelte';
  import Toggle from './Toggle.svelte';
  import { msg } from '../i18n';
  import { composing } from '../utils/shortcut';

  // The launch arguments and the run-as-administrator switch of GamePropertiesModal, which holds their
  // state and does their writes.
  let {
    emulated,
    adminShown,
    argsLine = $bindable(''),
    argsInput = $bindable(),
    argsLoading,
    argsBusy,
    argsDirty,
    argsLoadError,
    argsError,
    onsave,
    admin,
    adminBusy,
    adminError,
    onadmin,
  }: {
    emulated: boolean;
    adminShown: boolean;
    argsLine?: string;
    argsInput?: HTMLInputElement;
    argsLoading: boolean;
    argsBusy: boolean;
    argsDirty: boolean;
    argsLoadError: string;
    argsError: string;
    onsave: (event?: MouseEvent) => void;
    admin: boolean;
    adminBusy: boolean;
    adminError: string;
    onadmin: (on: boolean) => void;
  } = $props();
  const uid = $props.id();
  const held = $derived(msg('modals.actionRunning'));
</script>

{#if !emulated}
<section class="section">
  <h4 id="{uid}-args-label">{msg('modals.propertiesArgsTitle')}</h4>
  <div class="row args">
    <div class="field">
      <TextField
        labelledby="{uid}-args-label"
        bind:value={argsLine}
        bind:input={argsInput}
        autocomplete="off"
        placeholder={msg('modals.propertiesArgsPlaceholder')}
        hint={msg('modals.propertiesArgsHint')}
        error={argsError}
        disabled={argsLoading}
        readonly={argsBusy}
        onkeydown={(e) => e.key === 'Enter' && !composing(e) && argsDirty && onsave()}
      />
    </div>
    <Button size="sm" variant="primary" disabled={argsLoading || (!argsDirty && !argsBusy)} busy={argsBusy} onclick={onsave}>
      {msg('modals.propertiesArgsSave')}
    </Button>
  </div>
  {#if argsLoadError}
    <Notice tone="danger" compact>{argsLoadError}</Notice>
  {/if}
</section>
{/if}

{#if adminShown && !emulated}
  <section class="section">
    <div class="row spread">
      <div class="title">
        <h4 id="{uid}-properties-admin-label">{msg('modals.propertiesAdminTitle')}</h4>
        {#if adminBusy}
          <p class="hint" id="{uid}-admin-held">{held}</p>
        {/if}
      </div>
      <Toggle
        checked={admin}
        labelledby="{uid}-properties-admin-label"
        describedby={adminBusy ? `${uid}-admin-held` : undefined}
        aria-disabled={adminBusy ? 'true' : undefined}
        onchange={onadmin}
      />
    </div>
    <p class="hint">{msg('modals.propertiesAdminHint')}</p>
    {#if adminError}
      <Notice tone="danger" compact>{adminError}</Notice>
    {/if}
  </section>
{/if}

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

  /* The Save button sits by the field, level with it, above the field's hint and error. */
  .row.args {
    align-items: flex-start;
  }

  .field {
    flex: 1;
    min-width: 0;
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
