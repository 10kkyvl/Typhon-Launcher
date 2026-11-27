<script lang="ts">
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import ProgressBar from './ProgressBar.svelte';
  import Select from './Select.svelte';
  import Spinner from './Spinner.svelte';
  import Toggle from './Toggle.svelte';
  import { msg } from '../i18n';
  import { WRAPPERS, presetLabel, presetsFor, problemText, wrapperName, type problemActions, type progressView } from '../game/compatMode';
  import type { WrapperId, WrapperInfo } from '../services/wrappers';

  // The compatibility mode (wrappers) section of GamePropertiesModal, which holds its state and does
  // its writes: the switch outlives the dialog.
  let {
    compat,
    loading,
    visible,
    on,
    busy,
    running,
    locked,
    suggested,
    api,
    view,
    actions: compatActions,
    files,
    force,
    wrapper,
    preset = $bindable('default'),
    action: compatAction,
    error,
    keepOffer,
    section = $bindable(),
    onswitch,
    onforce,
    onwrapper,
    onrevert,
    onkeep,
  }: {
    compat: WrapperInfo | null;
    loading: boolean;
    visible: boolean;
    on: boolean;
    busy: boolean;
    running: boolean;
    locked: boolean;
    suggested: string;
    api: string;
    view: ReturnType<typeof progressView> | null;
    actions: ReturnType<typeof problemActions> | null;
    files: string;
    force: boolean;
    wrapper: WrapperId;
    preset?: string;
    action: '' | 'revert' | 'keep';
    error: string;
    keepOffer: boolean;
    section?: HTMLElement;
    onswitch: (on: boolean) => void;
    onforce: () => void;
    onwrapper: (id: string) => void;
    onrevert: () => void;
    onkeep: () => void;
  } = $props();
  const uid = $props.id();
</script>

{#if loading && !compat}
  <section class="section">
    <h4>{msg('modals.compatTitle')}</h4>
    <Spinner size="sm" label={msg('modals.compatLoading')} />
  </section>
{:else if visible && compat}
  <section class="section" bind:this={section}>
    <div class="row spread">
      <h4>{msg('modals.compatTitle')}</h4>
      <Toggle checked={on} label={msg('modals.compatToggle')} disabled={busy || running || (locked && !compat.active)} onchange={onswitch} />
    </div>
    {#if compat.active}
      <p class="path">
        {msg('modals.compatActive', {
          wrapper: wrapperName(compat.active.wrapper),
          version: compat.active.version,
          files,
        })}
      </p>
      <p class="hint">{msg('modals.compatActivePreset', { preset: presetLabel(compat.active.preset) })}</p>
    {:else}
      <p class="path">{api ? msg('modals.compatFound', { api }) : msg('modals.compatFoundUnknown')}</p>
      <p class="hint">
        {suggested ? msg('modals.compatSuggest', { wrapper: wrapperName(suggested) }) : msg('modals.compatNotNeeded')}
      </p>
      {#if suggested === ''}
        <div>
          <Button size="sm" variant="ghost" aria-expanded={force} aria-controls="{uid}-compat-choice" onclick={onforce}>
            {force ? msg('modals.compatForceHide') : msg('modals.compatForce')}
          </Button>
        </div>
      {/if}
      <div class="choice" id="{uid}-compat-choice">
      {#if !locked}
        <div class="row spread">
          <span class="label" id="{uid}-wrapper-label">{msg('modals.compatWrapper')}</span>
          <Select
            labelledby="{uid}-wrapper-label"
            value={wrapper}
            width="26rem"
            disabled={busy || running}
            options={WRAPPERS.map((id) => ({ id, label: wrapperName(id) }))}
            onchange={onwrapper}
          />
        </div>
        {#if presetsFor(wrapper).length > 1}
          <div class="row spread">
            <span class="label" id="{uid}-preset-label">{msg('modals.compatPresetLabel')}</span>
            <Select
              labelledby="{uid}-preset-label"
              bind:value={preset}
              width="26rem"
              disabled={busy || running}
              options={presetsFor(wrapper).map((id) => ({ id, label: presetLabel(id) }))}
            />
          </div>
        {/if}
      {/if}
      </div>
    {/if}
    {#if view}
      <p class="hint">{view.text}</p>
      <ProgressBar value={view.fraction ?? 0} max={1} indeterminate={view.fraction === null} label={msg('modals.compatTitle')} valueText={view.text} />
    {/if}
    {#if running}
      <p class="hint">{msg('modals.compatRunning')}</p>
    {/if}
    {#if compat.problem && compatActions?.dirMissing}
      <p class="hint">{problemText(compat.problem.code)}</p>
    {:else if compat.problem && compatActions}
      {@const actions = compatActions}
      <Notice tone="danger" compact title={msg('modals.compatProblemTitle')}>
        {problemText(compat.problem.code, compat.problem.file)}
        {#if actions.keepChanged}
          <span class="note">{msg('modals.compatKeepChangedHint')}</span>
        {/if}
        {#snippet action()}
          <div class="row">
            <Button size="sm" disabled={(busy && compatAction !== 'revert') || running} busy={compatAction === 'revert'} onclick={onrevert}>
              {msg('modals.compatRevert')}
            </Button>
            {#if actions.keepChanged}
              <Button size="sm" disabled={(busy && compatAction !== 'keep') || running} busy={compatAction === 'keep'} onclick={onkeep}>
                {msg('modals.compatKeepChanged')}
              </Button>
            {/if}
          </div>
        {/snippet}
      </Notice>
    {:else if keepOffer}
      <Notice tone="warning" compact>
        {msg('modals.compatKeepChangedHint')}
        {#snippet action()}
          <Button size="sm" disabled={(busy && compatAction !== 'keep') || running} busy={compatAction === 'keep'} onclick={onkeep}>
            {msg('modals.compatKeepChanged')}
          </Button>
        {/snippet}
      </Notice>
    {/if}
    {#if error}
      <Notice tone="danger" compact>{error}</Notice>
    {/if}
    <p class="hint">{msg('modals.compatHint')}</p>
  </section>
{:else if error}
  <section class="section">
    <h4>{msg('modals.compatTitle')}</h4>
    <Notice tone="danger" compact>{error}</Notice>
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

  .label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .path {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
    word-break: break-all;
  }

  .hint {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .note {
    display: block;
    margin-top: var(--space-1);
  }

  /* Holds the wrapper choice the "force" disclosure shows, as a part of the section's own column. */
  .choice {
    display: contents;
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
