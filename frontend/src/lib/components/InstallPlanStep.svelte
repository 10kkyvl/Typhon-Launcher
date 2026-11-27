<script lang="ts">
  import { FileSearch, FolderOpen, Gamepad2 } from '@lucide/svelte';
  import type { PlanInfo, SpaceInfo } from '../services/install';
  import type { DestinationHint } from '../install/destinationAdvice';
  import { installTypeLabels } from '../stores/install';
  import { bytesSize } from '../utils/format';
  import Button from './Button.svelte';
  import Notice from './Notice.svelte';
  import PathField from './PathField.svelte';
  import Select from './Select.svelte';
  import SegmentedControl from './SegmentedControl.svelte';
  import Spinner from './Spinner.svelte';
  import SystemRequirements from './SystemRequirements.svelte';
  import { msg } from '../i18n';

  /**
   * InstallModal's plan: what the package is, where it goes, the space it needs and how it is installed.
   * The destination is typed or browsed; a refusal of it is said at the field. While one of the
   * dialog's actions runs (`acting` names it), the rest is held with `held`.
   */
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
    held,
    acting,
    ondestination,
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
    /** Why the plan cannot be changed now ('' while it can). */
    held: string;
    /** The dialog's action that runs, '' while none does. */
    acting: string;
    ondestination: (value: string) => void;
    onbrowse: () => Promise<void> | void;
    onusesuggested: () => void;
    onopensource: () => void;
    onpickinstaller: () => void;
    onpickexecutable: () => void;
  } = $props();
  const uid = $props.id();
  const heldBy = (by: string) => (acting && acting !== by ? held : '');
</script>

  <div class="block">
    <div class="pkg">
      <span class="pkg-name">{info.name}</span>
      <span class="pkg-type">{installTypeLabels(plan.type)}</span>
    </div>

    <div class="rows">
      <div class="row">
        <span class="key">{msg('modals.installSource')}</span>
        <span class="value path">{plan.sourcePath}</span>
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
        <PathField
          label={msg('modals.installDestinationFolder')}
          editable
          bind:value={() => destination, ondestination}
          browseLabel={msg('modals.installBrowse')}
          {onbrowse}
          error={spaceError && !spacePending ? spaceError : ''}
          disabledReason={held || undefined}
        />
        {#if hint && !spacePending && !spaceError}
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
              <Button size="sm" disabled={!!held} reason={held} onclick={onusesuggested}>{msg('modals.installUseSuggested')}</Button>
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
        <Button disabled={!!heldBy('source')} reason={heldBy('source')} busy={acting === 'source'} onclick={onopensource}>
          <FolderOpen size="1.6rem" strokeWidth={1.8} />
          {msg('modals.installOpenDownloadFolder')}
        </Button>
        <Button disabled={!!heldBy('installer')} reason={heldBy('installer')} busy={acting === 'installer'} onclick={onpickinstaller}>
          <FileSearch size="1.6rem" strokeWidth={1.8} />
          {msg('modals.installPickInstallerButton')}
        </Button>
        <Button disabled={!!heldBy('executable')} reason={heldBy('executable')} busy={acting === 'executable'} onclick={onpickexecutable}>
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

  /* A path is read whole: it wraps where it must instead of being cut. */
  .value.path {
    overflow: visible;
    white-space: normal;
    overflow-wrap: anywhere;
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

  .choices {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 0.8rem;
    padding-top: var(--space-2);
    border-top: 1px solid var(--border);
  }
</style>
