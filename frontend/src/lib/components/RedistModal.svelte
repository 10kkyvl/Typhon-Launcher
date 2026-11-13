<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import EmptyState from './EmptyState.svelte';
  import Modal from './Modal.svelte';
  import Notice from './Notice.svelte';
  import Spinner from './Spinner.svelte';
  import TextLink from './TextLink.svelte';
  import { msg } from '../i18n';
  import { isUnsupported, kindLabel, redistErrorText, resultText } from '../redist/messages';
  import { missingVcArches } from '../redist/runtimes';
  import {
    installRedist,
    installedRuntimes,
    officialLinks,
    scanRedist,
    type RedistLink,
    type RedistPackage,
    type RedistRuntime,
  } from '../services/redist';
  import { closeRedist, redistTarget } from '../stores/redist';
  import { bytesSize } from '../utils/format';

  interface Outcome {
    text: string;
    failed: boolean;
  }

  const target = $derived($redistTarget);

  let open = $state(false);
  let packages = $state<RedistPackage[]>([]);
  let scanning = $state(false);
  let scanError = $state('');
  let runtimes = $state<RedistRuntime[]>([]);
  let links = $state<RedistLink[]>([]);
  let systemLoading = $state(false);
  let systemError = $state('');
  let linksError = $state('');
  let unsupported = $state(false);
  let installingPath = $state('');
  let outcomes = $state<Record<string, Outcome>>({});

  let seq = 0;
  // One per opening of the dialog: a retry that answers after its opening has passed says so.
  let opening = 0;

  const vcRuntimes = $derived(runtimes.filter((runtime) => runtime.name.includes('Visual C++')));
  const missing = $derived(systemLoading || systemError ? [] : missingVcArches(vcRuntimes));

  $effect(() => {
    open = target !== null;
  });

  $effect(() => {
    const current = $redistTarget;
    untrack(() => {
      // A retry still running when the dialog closes belongs to that opening: the next starts clean.
      opening += 1;
      scanRetrying = false;
      if (current) void load(current.gameId);
      else reset();
    });
  });

  function reset() {
    seq++;
    packages = [];
    scanning = false;
    scanError = '';
    runtimes = [];
    links = [];
    systemLoading = false;
    systemError = '';
    linksError = '';
    unsupported = false;
    installingPath = '';
    outcomes = {};
  }

  // A retry keeps the failed scan on screen, its Retry busy, until the answer replaces it.
  async function load(gameId: string, retrying = false) {
    const failed = retrying ? scanError : '';
    reset();
    scanError = failed;
    const id = seq;
    scanning = true;
    systemLoading = true;
    await Promise.all([loadPackages(gameId, id), loadSystem(id)]);
  }

  async function loadPackages(gameId: string, id: number) {
    try {
      const found = await scanRedist(gameId);
      if (id !== seq) return;
      packages = found;
      scanError = '';
    } catch (err) {
      if (id !== seq) return;
      if (isUnsupported(err)) {
        unsupported = true;
        scanError = '';
      } else scanError = redistErrorText(err);
    } finally {
      if (id === seq) scanning = false;
    }
  }

  async function loadSystem(id: number) {
    try {
      const found = await installedRuntimes();
      if (id !== seq) return;
      runtimes = found;
    } catch (err) {
      if (id !== seq) return;
      if (isUnsupported(err)) unsupported = true;
      else systemError = redistErrorText(err);
      systemLoading = false;
      return;
    }
    try {
      const found = await officialLinks();
      if (id !== seq) return;
      links = found;
    } catch (err) {
      if (id !== seq) return;
      linksError = redistErrorText(err);
    }
    if (id === seq) systemLoading = false;
  }

  async function refreshRuntimes(id: number) {
    try {
      const found = await installedRuntimes();
      if (id === seq) runtimes = found;
    } catch (err) {
      if (id === seq) systemError = redistErrorText(err);
    }
  }

  async function install(pkg: RedistPackage) {
    if (!target || installingPath) return;
    const id = seq;
    installingPath = pkg.path;
    try {
      const result = await installRedist(target.gameId, pkg.path);
      if (id !== seq) return;
      outcomes = { ...outcomes, [pkg.path]: { text: resultText(result, pkg.name), failed: false } };
      await refreshRuntimes(id);
    } catch (err) {
      if (id !== seq) return;
      outcomes = { ...outcomes, [pkg.path]: { text: redistErrorText(err), failed: true } };
    } finally {
      if (id === seq) installingPath = '';
    }
  }

  let scanRetrying = $state(false);
  let bundled = $state<HTMLElement>();

  // The notice's retry scans again; the failure stays on screen until the answer. On success the
  // focus goes to the first package it found, else to the dialog.
  async function retryScan(): Promise<boolean | undefined> {
    if (!target) return false;
    const mine = opening;
    await load(target.gameId, true);
    return mine === opening ? scanError === '' : undefined;
  }

  function meta(pkg: RedistPackage): string {
    return [kindLabel(pkg.kind), pkg.arch, pkg.version, bytesSize(pkg.sizeBytes)].filter(Boolean).join(' · ');
  }

  function linkFor(arch: string): RedistLink | undefined {
    return links.find((link) => link.arch === arch);
  }
</script>

<Modal bind:open title={msg('redist.title', { title: target?.title ?? '' })} onclose={closeRedist}>
  {#if target}
    <div class="redist">
      {#if unsupported}
        <Notice compact announce={false}>{msg('redist.unsupported')}</Notice>
      {:else}
        <section class="block" bind:this={bundled}>
          <h4>{msg('redist.bundledHeading')}</h4>
          {#if scanError || scanRetrying}
            <Notice tone="danger" compact bind:retrying={scanRetrying} retry={{ label: msg('redist.retry'), run: retryScan, within: () => bundled }}>
              {scanError}
            </Notice>
          {:else if scanning}
            <Spinner size="sm" label={msg('redist.scanning')} />
          {:else if packages.length === 0}
            <EmptyState size="inline" title={msg('redist.bundledEmpty')} />
          {:else}
            <ul class="rows">
              {#each packages as pkg (pkg.path)}
                <li class="row">
                  <div class="row-main">
                    <span class="row-name">{pkg.name}</span>
                    <span class="row-meta">{meta(pkg)}</span>
                    {#if outcomes[pkg.path]}
                      <span
                        class:failure={outcomes[pkg.path].failed}
                        class:ok={!outcomes[pkg.path].failed}
                        class="row-result"
                        role={outcomes[pkg.path].failed ? 'alert' : 'status'}
                      >
                        {outcomes[pkg.path].text}
                      </span>
                    {/if}
                  </div>
                  <Button
                    size="sm"
                    disabled={installingPath !== '' && installingPath !== pkg.path}
                    busy={installingPath === pkg.path}
                    onclick={() => install(pkg)}
                  >
                    {installingPath === pkg.path ? msg('redist.installing') : msg('redist.install')}
                  </Button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <section class="block">
          <h4>{msg('redist.systemHeading')}</h4>
          {#if systemLoading}
            <Spinner size="sm" label={msg('redist.systemLoading')} />
          {:else if systemError}
            <Notice tone="danger" compact>{systemError}</Notice>
          {:else}
            {#if vcRuntimes.length === 0}
              <EmptyState size="inline" title={msg('redist.systemEmpty')} />
            {:else}
              <ul class="rows">
                {#each vcRuntimes as runtime (runtime.name + runtime.arch + runtime.version)}
                  <li class="row">
                    <div class="row-main">
                      <span class="row-name">{runtime.name}</span>
                      <span class="row-meta">{[runtime.arch, runtime.version].filter(Boolean).join(' · ')}</span>
                    </div>
                  </li>
                {/each}
              </ul>
            {/if}
            {#each missing as arch (arch)}
              {@const link = linkFor(arch)}
              {#if link}
                <Notice tone="warning" compact announce={false}>
                  {msg('redist.missing', { arch })}
                  {#snippet action()}
                    <TextLink href={link.url} target="_blank" rel="noopener noreferrer">{msg('redist.download')}</TextLink>
                  {/snippet}
                </Notice>
              {:else}
                <Notice tone="warning" compact announce={false}>
                  {msg('redist.missing', { arch })}
                  <span class="no-link">{linksError || msg('redist.noLink')}</span>
                </Notice>
              {/if}
            {/each}
          {/if}
        </section>
      {/if}
    </div>
  {/if}

  {#snippet footer()}
    <Button onclick={closeRedist}>{msg('common.close')}</Button>
  {/snippet}
</Modal>

<style>
  .redist {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  h4 {
    margin: 0;
    font-size: var(--font-xs);
    font-weight: 600;
    color: var(--text-3);
  }

  .failure {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .ok {
    font-size: var(--font-xs);
    color: var(--success);
  }

  .rows {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .row-main {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .row-name {
    font-size: var(--font-sm);
    color: var(--text);
    word-break: break-word;
  }

  .row-meta {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .no-link {
    display: block;
  }
</style>
