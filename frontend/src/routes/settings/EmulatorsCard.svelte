<script lang="ts">
  import { onMount } from 'svelte';
  import { ExternalLink, FolderOpen, Search } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Checkbox from '../../lib/components/Checkbox.svelte';
  import Modal from '../../lib/components/Modal.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
  import Spinner from '../../lib/components/Spinner.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { hasMessage, msg } from '../../lib/i18n';
  import { libraryErrorText } from '../../lib/library/errors';
  import { inWails } from '../../lib/services/backend';
  import {
    detectEmulators,
    listPlatforms,
    openOfficialPage,
    selectEmulatorFile,
    setEmulatorPath,
    type DetectResult,
    type PlatformStatus,
  } from '../../lib/services/emulators';
  import { toast } from '../../lib/stores/toasts';


  let rows = $state<PlatformStatus[]>([]);
  let loading = $state(true);
  let loadFailed = $state(false);
  let busy = $state('');
  let detecting = $state(false);
  let retrying = $state(false);
  let list = $state<HTMLElement>();

  let found = $state<DetectResult | null>(null);
  let foundOpen = $state(false);
  let picked = $state<Record<string, boolean>>({});
  let applying = $state(false);
  // The ticked paths the last Apply could not save, in the launcher's words.
  let applyFailures = $state<string[]>([]);
  const foundUid = $props.id();
  const applyFailureId = `${foundUid}-apply-failure`;

  async function fetchRows() {
    try {
      rows = await listPlatforms();
      loadFailed = false;
    } catch (err) {
      console.error('list emulators', err);
      loadFailed = true;
    }
  }

  async function load() {
    loading = true;
    loadFailed = false;
    try {
      await fetchRows();
    } finally {
      loading = false;
    }
  }

  async function retry(): Promise<boolean> {
    await fetchRows();
    return !loadFailed;
  }

  onMount(() => {
    void load();
  });

  function replace(next: PlatformStatus) {
    rows = rows.map((row) => (row.id === next.id ? next : row));
  }

  function statusKind(row: PlatformStatus): 'success' | 'warning' | 'danger' | 'neutral' {
    switch (row.status) {
      case 'ready':
        return 'success';
      case 'missing':
        return 'danger';
      case 'unavailable':
        return 'warning';
      default:
        return 'neutral';
    }
  }

  function statusLabel(row: PlatformStatus): string {
    switch (row.status) {
      case 'ready':
        return msg('emul.statusReady');
      case 'missing':
        return msg('emul.statusMissing');
      case 'unavailable':
        return msg('emul.statusUnavailable');
      default:
        return msg('emul.statusNotSet');
    }
  }

  function reasonText(row: PlatformStatus): string {
    if (row.status !== 'unavailable' || !row.reason) return '';
    return hasMessage(row.reason) ? msg(row.reason) : '';
  }

  function nameOf(platform: string): string {
    return rows.find((row) => row.id === platform)?.emulator ?? platform;
  }

  async function openSite(row: PlatformStatus) {
    try {
      await openOfficialPage(row.id);
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.openSiteFailed')), 'danger');
    }
  }

  async function choose(row: PlatformStatus) {
    if (busy) return;
    let path = '';
    try {
      path = await selectEmulatorFile(msg('emul.pickDialog', { emulator: row.emulator }));
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.openDialogFailed')), 'danger');
      return;
    }
    if (!path) return;
    await save(row, path, msg('emul.pathSaved', { emulator: row.emulator }));
  }

  async function reset(row: PlatformStatus) {
    if (busy) return;
    await save(row, '', msg('emul.pathReset', { emulator: row.emulator }));
  }

  async function save(row: PlatformStatus, path: string, done: string) {
    busy = row.id;
    try {
      replace(await setEmulatorPath(row.id, path));
      toast(done, 'success');
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.saveFailed')), 'danger');
    } finally {
      busy = '';
    }
  }

  async function detect() {
    if (detecting) return;
    if (!inWails) {
      toast(msg('emul.desktopOnly'));
      return;
    }
    detecting = true;
    try {
      const result = await detectEmulators();
      if (result.found.length === 0) {
        toast(result.unreadable > 0 ? msg('emul.detectNoneUnreadable') : msg('emul.detectNone'));
        return;
      }
      found = result;
      picked = Object.fromEntries(result.found.map((item) => [item.platform, true]));
      applyFailures = [];
      foundOpen = true;
    } catch (err) {
      toast(libraryErrorText(err, msg('emul.detectFailed')), 'danger');
    } finally {
      detecting = false;
    }
  }

  // Saves each ticked path once. A path that is saved leaves the list; one that is not stays ticked
  // and is said in the dialog, which stays open for another Apply.
  async function applyFound() {
    if (!found || applying || !anyPicked) return;
    applying = true;
    applyFailures = [];
    const failures: string[] = [];
    const saved = new Set<string>();
    try {
      for (const item of found.found) {
        if (!picked[item.platform]) continue;
        try {
          replace(await setEmulatorPath(item.platform, item.path));
          saved.add(item.platform);
        } catch (err) {
          failures.push(
            msg('emul.detectApplyFailed', {
              emulator: nameOf(item.platform),
              reason: libraryErrorText(err, msg('emul.saveFailed')),
            }),
          );
        }
      }
    } finally {
      applying = false;
    }
    if (failures.length === 0) {
      toast(msg('emul.detectApplied'), 'success');
      foundOpen = false;
      return;
    }
    found = { ...found, found: found.found.filter((item) => !saved.has(item.platform)) };
    applyFailures = failures;
  }

  // Only the rows still listed: a saved path leaves the list but keeps its tick in `picked`.
  const anyPicked = $derived(found?.found.some((item) => picked[item.platform]) ?? false);
  const heldReason = $derived(applying ? msg('emul.detectApplying') : '');
</script>

<Card title={msg('emul.cardTitle')}>
  {#snippet action()}
    <Button size="sm" disabled={detecting || loading || retrying} onclick={detect}>
      <Search size="1.4rem" strokeWidth={1.8} />
      {detecting ? msg('emul.detecting') : msg('emul.detect')}
    </Button>
  {/snippet}
  <p class="hint">{msg('emul.cardHint')}</p>
  {#if loading}
    <Spinner label={msg('emul.loading')} />
  {:else if loadFailed}
    <Notice tone="danger" compact bind:retrying retry={{ label: msg('common.retry'), run: retry, within: () => list }}>
      {msg('emul.loadFailed')}
    </Notice>
  {:else}
    <div bind:this={list}>
      {#each rows as row (row.id)}
        <SettingsRow label={msg('settings.emulatorsRowLabel', { platform: row.name, emulator: row.emulator })} hint={row.path || undefined} note={reasonText(row) || undefined}>
          {#snippet children()}
            <StatusBadge kind={statusKind(row)} label={statusLabel(row)} />
            <Button size="sm" aria-label={msg('settings.emulatorsOfficialFor', { platform: row.name })} onclick={() => openSite(row)}>
              <ExternalLink size="1.3rem" strokeWidth={1.8} />
              {msg('emul.official')}
            </Button>
            <Button size="sm" aria-label={msg('settings.emulatorsChooseFor', { platform: row.name })} disabled={busy !== ''} onclick={() => choose(row)}>
              <FolderOpen size="1.3rem" strokeWidth={1.8} />
              {msg('emul.choose')}
            </Button>
            {#if row.status !== 'not_set'}
              <Button size="sm" variant="ghost" aria-label={msg('settings.emulatorsResetFor', { platform: row.name })} disabled={busy !== ''} onclick={() => reset(row)}>
                {msg('emul.reset')}
              </Button>
            {/if}
          {/snippet}
          {#snippet below()}
            {#if row.warnings.includes('discord_exit_hang')}
              <Notice tone="warning" compact announce={false}>{msg('emul.discordExitHang')}</Notice>
            {/if}
          {/snippet}
        </SettingsRow>
      {/each}
    </div>
  {/if}
</Card>

<Modal dismissible={!applying} busy={applying} bind:open={foundOpen} title={msg('emul.detectTitle')}>
  {#if found}
    <p class="hint">{msg('emul.detectIntro')}</p>
    <div class="found">
      {#each found.found as item (item.platform)}
        <div class="found-row">
          <Checkbox bind:checked={picked[item.platform]} label={nameOf(item.platform)} description={item.path} disabled={applying} reason={heldReason} />
        </div>
      {/each}
    </div>
    {#if found.unreadable > 0}
      <p class="hint">{msg('emul.detectPartial')}</p>
    {/if}
    {#if applyFailures.length > 0}
      <Notice tone="danger" compact id={applyFailureId} title={msg('emul.detectApplyPartly')}>
        {#each applyFailures as failure (failure)}<p class="failure">{failure}</p>{/each}
      </Notice>
    {/if}
  {/if}
  {#snippet footer()}
    <Button disabled={applying} reason={heldReason} onclick={() => (foundOpen = false)}>{msg('common.cancel')}</Button>
    <Button
      variant="primary"
      disabled={!applying && !anyPicked}
      reason={!applying && !anyPicked ? msg('emul.detectPickOne') : ''}
      busy={applying}
      aria-describedby={applyFailures.length > 0 ? applyFailureId : undefined}
      onclick={applyFound}
    >
      {msg('emul.detectApply')}
    </Button>
  {/snippet}
</Modal>

<style>
  .hint {
    margin: 0 0 var(--space-3);
    font-size: var(--font-xs);
    color: var(--text-3);
    line-height: 1.5;
  }

  .found {
    display: flex;
    flex-direction: column;
  }

  .found-row {
    padding: 1.1rem 0;
  }

  .found-row + .found-row {
    border-top: 1px solid var(--border);
  }

  .failure {
    margin: 0;
  }
</style>
