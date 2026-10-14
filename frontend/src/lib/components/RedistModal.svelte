<script lang="ts">
  import { untrack } from 'svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';
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

  const vcRuntimes = $derived(runtimes.filter((runtime) => runtime.name.includes('Visual C++')));
  const missing = $derived(systemLoading || systemError ? [] : missingVcArches(vcRuntimes));

  $effect(() => {
    open = target !== null;
  });

  $effect(() => {
    const current = $redistTarget;
    untrack(() => {
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

  async function load(gameId: string) {
    reset();
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
    } catch (err) {
      if (id !== seq) return;
      if (isUnsupported(err)) unsupported = true;
      else scanError = redistErrorText(err);
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

  function retry() {
    if (target) void load(target.gameId);
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
        <p class="hint">{msg('redist.unsupported')}</p>
      {:else}
        <section class="block">
          <h4>{msg('redist.bundledHeading')}</h4>
          {#if scanning}
            <p class="hint">{msg('redist.scanning')}</p>
          {:else if scanError}
            <p class="failure">{scanError}</p>
            <div class="actions">
              <Button size="sm" onclick={retry}>{msg('redist.retry')}</Button>
            </div>
          {:else if packages.length === 0}
            <p class="hint">{msg('redist.bundledEmpty')}</p>
          {:else}
            <ul class="rows">
              {#each packages as pkg (pkg.path)}
                <li class="row">
                  <div class="row-main">
                    <span class="row-name">{pkg.name}</span>
                    <span class="row-meta">{meta(pkg)}</span>
                    {#if outcomes[pkg.path]}
                      <span class:failure={outcomes[pkg.path].failed} class:ok={!outcomes[pkg.path].failed} class="row-result">
                        {outcomes[pkg.path].text}
                      </span>
                    {/if}
                  </div>
                  <Button size="sm" disabled={installingPath !== ''} onclick={() => install(pkg)}>
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
            <p class="hint">{msg('redist.systemLoading')}</p>
          {:else if systemError}
            <p class="failure">{systemError}</p>
          {:else}
            {#if vcRuntimes.length === 0}
              <p class="hint">{msg('redist.systemEmpty')}</p>
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
              <div class="missing">
                <p class="warning">{msg('redist.missing', { arch })}</p>
                {#if link}
                  <a class="download" href={link.url} target="_blank" rel="noopener noreferrer">{msg('redist.download')}</a>
                {:else}
                  <p class="failure">{linksError || msg('redist.noLink')}</p>
                {/if}
              </div>
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

  .hint {
    margin: 0;
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .failure {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .warning {
    margin: 0;
    font-size: var(--font-xs);
    color: var(--warning);
  }

  .ok {
    font-size: var(--font-xs);
    color: var(--success);
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
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

  .missing {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .download {
    font-size: var(--font-sm);
    color: var(--accent);
  }
</style>
