<script lang="ts">
  import { FileSearch, FolderOpen, Gamepad2 } from '@lucide/svelte';
  import type { PlanInfo, SpaceInfo } from '../services/install';
  import type { DestinationHint } from '../install/destinationAdvice';
  import { installTypeLabels } from '../stores/install';
  import { bytesSize, truncateMiddle } from '../utils/format';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import Select from './Select.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Spinner from './Spinner.svelte';
  import SystemRequirements from './SystemRequirements.svelte';
  import { msg } from '../i18n';

  /** InstallModal's plan: what the package is, where it goes, the space it needs and how it is installed. */
  let {
    info,
    plan,
    space,
    spacePending,
    spaceError,
    notEnoughSpace,
    requirementsGameId,
    destination,
    hint,
    roots,
    rootOptions,
    rootChoice,
    pickRoot,
    controlled,
    silent,
    portable,
    external,
    modeOptions,
    mode,
    setMode,
    movesInPlace,
    ondestinationinput,
    onbrowse,
    onusesuggested,
    onopensource,
    onpickinstaller,
    onpickexecutable,
  }: {
    info: PlanInfo;
    plan: PlanInfo['plan'];
    space: SpaceInfo | null;
    spacePending: boolean;
    spaceError: string;
    notEnoughSpace: boolean;
    requirementsGameId: string;
    destination: string;
    hint: DestinationHint | null;
    roots: string[];
    rootOptions: { id: string; label: string }[];
    rootChoice: string;
    pickRoot: (id: string) => void;
    controlled: boolean;
    silent: boolean;
    portable: boolean;
    external: boolean;
    modeOptions: { id: string; label: string }[];
    mode: string;
    setMode: (next: string) => void;
    movesInPlace: boolean;
    ondestinationinput: (event: Event & { currentTarget: HTMLInputElement }) => void;
    onbrowse: () => void;
    onusesuggested: () => void;
    onopensource: () => void;
    onpickinstaller: () => void;
    onpickexecutable: () => void;
  } = $props();
  const uid = $props.id();
</script>

  <div class="block">
    <div class="pkg">
      <span class="pkg-name">{info.name}</span>
      <span class="pkg-type">{installTypeLabels(plan.type)}</span>
    </div>

    <div class="rows">
      <div class="row">
        <span class="key">{msg('modals.installSource')}</span>
        <span class="value">{truncateMiddle(plan.sourcePath, 48)}</span>
      </div>
      {#if space}
        <div class="row">
          <span class="key">{msg('modals.installRequiredSpace')}</span>
          <span class="value" class:danger={notEnoughSpace} class:stale={spacePending}>{bytesSize(space.requiredBytes)}</span>
        </div>
        <div class="row">
          <span class="key">{msg('modals.installFreeSpace')}</span>
          <span class="value" class:danger={notEnoughSpace} class:stale={spacePending}>{bytesSize(space.freeBytes)}</span>
        </div>
      {/if}
    </div>

    {#if spacePending}
      <Spinner size="sm" label={msg('modals.installRecalculating')} />
    {:else if notEnoughSpace}
      <Notice tone="danger" compact>{msg('modals.installNotEnoughSpace')}</Notice>
    {/if}

    {#if requirementsGameId}
      <SystemRequirements gameId={requirementsGameId} installPath={destination} heading={false} />
    {/if}

    {#if (controlled || silent) && roots.length > 1}
      <div class="field">
        <span class="field-label" id="{uid}-root-label">{msg('modals.installRootLabel')}</span>
        <Select width="100%" options={rootOptions} labelledby="{uid}-root-label" bind:value={() => rootChoice, pickRoot} />
      </div>
    {/if}

    {#if controlled || silent}
      <div class="field">
        <span class="field-label" id="{uid}-destination-label">{msg('modals.installDestinationFolder')}</span>
        <div class="field-controls">
          <input
            class="input sm"
            type="text"
            aria-labelledby="{uid}-destination-label"
            aria-invalid={spaceError && !spacePending ? 'true' : undefined}
            aria-describedby={spaceError && !spacePending ? `${uid}-destination-error` : undefined}
            value={destination}
            oninput={ondestinationinput}
          />
          <Button size="sm" onclick={onbrowse}>{msg('modals.installBrowse')}</Button>
        </div>
        {#if spaceError && !spacePending}
          <p class="note danger" id="{uid}-destination-error" role="alert">{spaceError}</p>
        {:else if hint && !spacePending}
          <div class="hint">
            <p class="note">
              {#if hint.kind === 'adjusted'}
                {msg('modals.installDestinationAdjusted', { path: hint.path })}
              {:else if hint.kind === 'suggest'}
                {msg('modals.installDestinationBusy', { path: hint.path })}
              {:else}
                {msg('modals.installDestinationReusable')}
              {/if}
            </p>
            {#if hint.kind === 'suggest'}
              <Button size="sm" onclick={onusesuggested}>{msg('modals.installUseSuggested')}</Button>
            {/if}
          </div>
        {/if}
      </div>
    {/if}

    {#if portable}
      {#if info.seeding}
        <p class="note">{msg('modals.installSeedingCopyNote')}</p>
      {:else}
        <div class="field">
          <span class="field-label" id="{uid}-mode-label">{msg('modals.installModeLabel')}</span>
          <SegmentedControl options={modeOptions} labelledby="{uid}-mode-label" bind:value={() => mode, setMode} />
        </div>
      {/if}
      {#if movesInPlace}
        <p class="note">{msg('modals.installSameVolumeNote')}</p>
      {/if}
    {/if}

    {#if external}
      {#if silent}
        <p class="note">{msg('modals.installSilentNote')}</p>
      {:else}
        <p class="note">{msg('modals.installExternalWindowNote')}</p>
      {/if}
    {/if}

    {#if plan.type === 'unknown'}
      <div class="choices">
        <p class="note">{msg('modals.installUnknownTypeNote')}</p>
        <Button onclick={onopensource}>
          <FolderOpen size="1.6rem" strokeWidth={1.8} />
          {msg('modals.installOpenDownloadFolder')}
        </Button>
        <Button onclick={onpickinstaller}>
          <FileSearch size="1.6rem" strokeWidth={1.8} />
          {msg('modals.installPickInstallerButton')}
        </Button>
        <Button onclick={onpickexecutable}>
          <Gamepad2 size="1.6rem" strokeWidth={1.8} />
          {msg('modals.installPickExecutableButton')}
        </Button>
      </div>
    {/if}
  </div>

<style>
  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .pkg {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .pkg-name {
    font-size: var(--font-lg);
    font-weight: 600;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .pkg-type {
    font-size: var(--font-xs);
    color: var(--text-3);
    flex-shrink: 0;
  }

  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: 0.8rem 0;
    font-size: var(--font-sm);
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .key {
    color: var(--text-3);
    flex-shrink: 0;
  }

  .value {
    min-width: 0;
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-variant-numeric: tabular-nums;
  }

  .value.danger {
    color: var(--danger);
  }

  .value.stale {
    opacity: 0.6;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 0.8rem;
    align-items: flex-start;
  }

  .field-label {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .field-controls {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    width: 100%;
  }

  .field-controls .input {
    flex: 1;
  }

  .note {
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-3);
  }

  .hint {
    display: flex;
    align-items: center;
    gap: 0.8rem;
  }

  .note.danger {
    color: var(--danger);
  }

  .choices {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 0.8rem;
    padding-top: var(--space-2);
    border-top: 1px solid var(--border);
  }
</style>
