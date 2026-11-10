<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { RefreshCw, AlertTriangle } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Select from '../../lib/components/Select.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import { msg } from '../../lib/i18n';
  import {
    networkChecking,
    networkCodeText,
    networkErrorText,
    networkReasonText,
    networkWarningText,
  } from '../../lib/network/networkText';
  import {
    hasProxyPassword,
    listNetworkInterfaces,
    setProxyPassword,
    testProxy,
    type NetInterface,
  } from '../../lib/services/network';
  import type { Settings } from '../../lib/services/settings';
  import { networkState } from '../../lib/stores/network';
  import { settings, updateSettingsReporting } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';

  const portPattern = /^\d{1,5}$/;

  const saved = $derived({
    mode: $settings?.networkMode ?? 'direct',
    iface: $settings?.networkInterface ?? '',
    proxyType: $settings?.proxyType ?? 'socks5',
    host: $settings?.proxyHost ?? '',
    port: $settings?.proxyPort ? String($settings.proxyPort) : '',
    username: $settings?.proxyUsername ?? '',
  });

  let mode = $state('direct');
  let iface = $state('');
  let proxyType = $state('socks5');
  let host = $state('');
  let port = $state('');
  let username = $state('');
  let password = $state('');

  let hasPassword = $state(false);
  let passwordBusy = $state(false);
  let applying = $state(false);
  let testing = $state(false);
  let failure = $state('');
  let testResult = $state<{ ok: boolean; text: string } | null>(null);

  let interfaces = $state<NetInterface[]>([]);
  let interfacesLoaded = $state(false);
  let interfacesLoading = $state(false);
  let interfacesError = $state('');

  const dirty = $derived(
    mode !== saved.mode ||
      (mode === 'interface' && iface.trim() !== saved.iface) ||
      (mode === 'proxy' &&
        (proxyType !== saved.proxyType ||
          host.trim() !== saved.host ||
          port.trim() !== saved.port ||
          username.trim() !== saved.username ||
          password !== '')),
  );

  $effect(() => {
    const next = saved;
    untrack(() => {
      if (dirty) return;
      mode = next.mode;
      iface = next.iface;
      proxyType = next.proxyType;
      host = next.host;
      port = next.port;
      username = next.username;
    });
  });

  $effect(() => {
    if (mode !== 'interface') return;
    untrack(() => {
      if (!interfacesLoaded && !interfacesLoading) void loadInterfaces();
    });
  });

  onMount(() => {
    void (async () => {
      try {
        hasPassword = await hasProxyPassword();
      } catch (err) {
        toast(networkErrorText(err), 'danger');
      }
    })();
  });

  async function loadInterfaces() {
    if (interfacesLoading) return;
    interfacesLoading = true;
    interfacesError = '';
    try {
      interfaces = await listNetworkInterfaces();
      interfacesLoaded = true;
    } catch (err) {
      interfacesError = networkErrorText(err);
    } finally {
      interfacesLoading = false;
    }
  }

  function interfaceLabel(item: NetInterface): string {
    const parts = [item.name];
    if (item.description && item.description !== item.name) parts.push(item.description);
    if (item.addresses.length > 0) parts.push(item.addresses.join(', '));
    if (item.vpnLike) parts.push(msg('settings.networkInterfaceVpnTag'));
    if (!item.up) parts.push(msg('settings.networkInterfaceDownTag'));
    return parts.join(' · ');
  }

  const modeOptions = $derived([
    { id: 'direct', label: msg('settings.networkModeDirect') },
    { id: 'interface', label: msg('settings.networkModeInterface') },
    { id: 'proxy', label: msg('settings.networkModeProxy') },
  ]);

  const proxyTypeOptions = [
    { id: 'socks5', label: 'SOCKS5' },
    { id: 'http', label: 'HTTP' },
  ];

  const interfaceOptions = $derived.by(() => {
    const options = interfaces.map((item) => ({ id: item.name, label: interfaceLabel(item) }));
    const current = iface.trim();
    if (current === '') return [{ id: '', label: msg('settings.networkInterfacePlaceholder') }, ...options];
    if (interfacesLoaded && !interfaces.some((item) => item.name === current)) {
      return [{ id: current, label: msg('settings.networkInterfaceMissing', { name: current }) }, ...options];
    }
    if (!interfaces.some((item) => item.name === current)) return [{ id: current, label: current }, ...options];
    return options;
  });

  async function apply() {
    if (applying || !dirty) return;
    failure = '';
    testResult = null;
    const rawPort = port.trim();
    if (mode === 'proxy' && rawPort !== '' && !portPattern.test(rawPort)) {
      failure = networkCodeText('settings.proxy_port_invalid');
      return;
    }
    applying = true;
    try {
      const sentPassword = mode === 'proxy' && password !== '';
      if (sentPassword) {
        try {
          await setProxyPassword(username.trim(), password);
        } catch (err) {
          failure = networkErrorText(err);
          return;
        }
      }
      const patch: Partial<Settings> = { networkMode: mode };
      if (mode === 'interface') patch.networkInterface = iface.trim();
      if (mode === 'proxy') {
        patch.proxyType = proxyType;
        patch.proxyHost = host.trim();
        patch.proxyPort = rawPort === '' ? 0 : Number(rawPort);
        patch.proxyUsername = username.trim();
      }
      const ok = await updateSettingsReporting(patch, (err) => {
        failure = networkErrorText(err);
      });
      if (!ok) return;
      if (sentPassword) {
        hasPassword = true;
        password = '';
      }
      toast(msg('settings.networkAppliedToast'), 'success');
    } finally {
      applying = false;
    }
  }

  async function removePassword() {
    if (passwordBusy) return;
    passwordBusy = true;
    failure = '';
    try {
      await setProxyPassword(saved.username, '');
      hasPassword = false;
      password = '';
      toast(msg('settings.networkProxyPasswordDeletedToast'), 'success');
    } catch (err) {
      failure = networkErrorText(err);
    } finally {
      passwordBusy = false;
    }
  }

  async function checkProxy() {
    if (testing || dirty) return;
    testing = true;
    testResult = null;
    try {
      await testProxy();
      testResult = { ok: true, text: msg('settings.networkProxyTestOk') };
    } catch (err) {
      testResult = { ok: false, text: networkErrorText(err) };
    } finally {
      testing = false;
    }
  }

  const status = $derived($networkState);
  const statusChecking = $derived(networkChecking(status));
  const statusDown = $derived(status?.state === 'down' && !statusChecking);
  const statusDetail = $derived(status && !statusChecking ? (statusDown ? networkReasonText(status) : status.address) : '');
  const statusWarning = $derived(networkWarningText(status));
</script>

<Card title={msg('settings.networkCardTitle')}>
  <div class="rows">
    <div class="row">
      <div class="row-text">
        <span class="row-label">{msg('settings.networkModeLabel')}</span>
        <span class="row-sub">{msg('settings.networkModeSub')}</span>
      </div>
      <SegmentedControl options={modeOptions} bind:value={mode} disabled={applying} />
    </div>

    {#if mode === 'interface'}
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.networkInterfaceLabel')}</span>
          <span class="row-sub">{msg('settings.networkInterfaceSub')}</span>
          {#if interfacesError}
            <span class="failure">{interfacesError}</span>
          {:else if interfacesLoaded && interfaces.length === 0}
            <span class="failure">{msg('settings.networkInterfaceEmpty')}</span>
          {/if}
        </div>
        <div class="control">
          <Select
            value={iface}
            width="32rem"
            options={interfaceOptions}
            onchange={(id) => (iface = id)}
          />
          <Button size="sm" disabled={interfacesLoading} onclick={loadInterfaces}>
            <RefreshCw size="1.4rem" strokeWidth={1.8} class={interfacesLoading ? 'spin' : ''} />
            {msg('settings.networkInterfaceRefresh')}
          </Button>
        </div>
      </div>
    {/if}

    {#if mode === 'proxy'}
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.networkProxyTypeLabel')}</span>
        </div>
        <Select value={proxyType} width="20rem" options={proxyTypeOptions} onchange={(id) => (proxyType = id)} />
      </div>
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.networkProxyHostLabel')}</span>
          <span class="row-sub">{msg('settings.networkProxyHostSub')}</span>
        </div>
        <input
          class="input field"
          type="text"
          autocomplete="off"
          spellcheck="false"
          aria-label={msg('settings.networkProxyHostLabel')}
          placeholder="proxy.example.com"
          bind:value={host}
        />
      </div>
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.networkProxyPortLabel')}</span>
        </div>
        <input
          class="input field port"
          type="text"
          inputmode="numeric"
          autocomplete="off"
          aria-label={msg('settings.networkProxyPortLabel')}
          placeholder="1080"
          bind:value={port}
        />
      </div>
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.networkProxyUsernameLabel')}</span>
          <span class="row-sub">{msg('settings.networkProxyUsernameSub')}</span>
        </div>
        <input
          class="input field"
          type="text"
          autocomplete="off"
          spellcheck="false"
          aria-label={msg('settings.networkProxyUsernameLabel')}
          bind:value={username}
        />
      </div>
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.networkProxyPasswordLabel')}</span>
          <span class="row-sub">{msg('settings.networkProxyPasswordSub')}</span>
        </div>
        <div class="control">
          <input
            class="input field"
            type="password"
            autocomplete="new-password"
            aria-label={msg('settings.networkProxyPasswordLabel')}
            placeholder={hasPassword
              ? msg('settings.networkProxyPasswordSaved')
              : msg('settings.networkProxyPasswordLabel')}
            bind:value={password}
          />
          {#if hasPassword}
            <Button size="sm" variant="danger" disabled={passwordBusy} onclick={removePassword}>
              {msg('settings.networkProxyPasswordDelete')}
            </Button>
          {/if}
        </div>
      </div>
      <div class="row">
        <div class="note">
          <AlertTriangle size="1.6rem" strokeWidth={1.8} />
          <span>{msg('settings.networkProxyWarning')}</span>
        </div>
      </div>
      <div class="row">
        <div class="row-text">
          {#if dirty}
            <span class="row-sub">{msg('settings.networkProxyTestNeedsApply')}</span>
          {:else if testResult}
            <span class={testResult.ok ? 'success' : 'failure'}>{testResult.text}</span>
          {/if}
        </div>
        <Button size="sm" disabled={dirty || testing} onclick={checkProxy}>
          {testing ? msg('settings.networkProxyTesting') : msg('settings.networkProxyTest')}
        </Button>
      </div>
    {/if}

    {#if failure}
      <div class="row">
        <span class="failure">{failure}</span>
      </div>
    {/if}

    <div class="row actions">
      <Button variant="primary" disabled={!dirty || applying} onclick={apply}>
        {applying ? msg('settings.networkApplying') : msg('settings.networkApply')}
      </Button>
    </div>

    {#if status}
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.networkStatusLabel')}</span>
          {#if statusDetail}
            <span class="row-sub">{statusDetail}</span>
          {/if}
          {#if statusWarning}
            <span class="row-sub">{statusWarning}</span>
          {/if}
        </div>
        <StatusBadge
          kind={statusChecking ? 'accent' : statusDown ? 'danger' : statusWarning ? 'warning' : 'success'}
          label={statusChecking
            ? msg('settings.networkStatusChecking')
            : statusDown
              ? msg('settings.networkStatusDown')
              : msg('settings.networkStatusOk')}
        />
      </div>
    {/if}
  </div>
</Card>

<style>
  .rows {
    display: flex;
    flex-direction: column;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
  }

  .row + .row {
    border-top: 1px solid var(--border);
  }

  .row-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .row-label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .row-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .control {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .field {
    width: 26rem;
  }

  .field.port {
    width: 12rem;
  }

  .control .field {
    width: 22rem;
  }

  .actions {
    justify-content: flex-end;
  }

  .note {
    flex: 1;
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-3);
    background: var(--warning-subtle);
    border-radius: var(--radius-md);
    font-size: var(--font-xs);
    color: var(--text-2);
    line-height: 1.5;
  }

  .note :global(svg) {
    flex-shrink: 0;
    color: var(--warning);
  }

  .failure {
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .success {
    font-size: var(--font-xs);
    color: var(--success);
  }

  .control :global(.spin) {
    animation: spin 900ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
