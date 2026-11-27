<script lang="ts">
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import Toggle from './Toggle.svelte';
  import { msg } from '../i18n';

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
</script>

{#if !emulated}
<section class="section">
  <h4 id="{uid}-args-label">{msg('modals.propertiesArgsTitle')}</h4>
  <div class="row">
    <input
      class="input sm"
      type="text"
      aria-labelledby="{uid}-args-label"
      bind:this={argsInput}
      spellcheck="false"
      autocomplete="off"
      placeholder={msg('modals.propertiesArgsPlaceholder')}
      disabled={argsLoading}
      bind:value={argsLine}
      onkeydown={(e) => e.key === 'Enter' && argsDirty && onsave()}
    />
    <Button size="sm" variant="primary" disabled={argsLoading || (!argsDirty && !argsBusy)} busy={argsBusy} onclick={onsave}>
      {msg('modals.propertiesArgsSave')}
    </Button>
  </div>
  {#if argsLoadError}
    <Notice tone="danger" compact>{argsLoadError}</Notice>
  {:else if argsError}
    <Notice tone="danger" compact announce={false}>{argsError}</Notice>
  {/if}
  <p class="hint">{msg('modals.propertiesArgsHint')}</p>
</section>
{/if}

{#if adminShown && !emulated}
  <section class="section">
    <div class="row spread">
      <h4 id="{uid}-properties-admin-label">{msg('modals.propertiesAdminTitle')}</h4>
      <Toggle checked={admin} labelledby="{uid}-properties-admin-label" disabled={adminBusy} onchange={onadmin} />
    </div>
    <p class="hint">{msg('modals.propertiesAdminHint')}</p>
    {#if adminError}
      <Notice tone="danger" compact announce={false}>{adminError}</Notice>
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

  .row .input {
    flex: 1;
  }

  .spread {
    justify-content: space-between;
  }
</style>
