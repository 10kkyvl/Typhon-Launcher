<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { RefreshCw } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import SegmentedControl from '../../lib/components/SegmentedControl.svelte';
  import Select from '../../lib/components/Select.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
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
  import { focusHandOff } from '../../lib/utils/focusHandOff';

  const portPattern = /^\d{1,5}$/;
  const uid = $props.id();

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
  let portError = $state('');
  // Each refusal puts the message up anew, so the same bad port applied again is said again.
  let portRefusals = $state(0);
  const portBad = $derived(port.trim() !== '' && !portPattern.test(port.trim()));
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

  type Saved = typeof saved;
  // What the fields were last filled from. While none of them has been changed since, all of them
  // follow a save made elsewhere; once the user has changed any one, none is refilled, so an edit in
  // progress is kept whole. A password is never filled in, so it does not count as an edit. While Apply
  // runs the store shows the patch at once and takes it back if the save fails: neither is followed,
  // so a failed Apply leaves the typed fields as they were, and a good one makes the card untouched.
  let shown: Saved | null = null;
  const editedFrom = (from: Saved) =>
    mode !== from.mode ||
    iface.trim() !== from.iface ||
    proxyType !== from.proxyType ||
    host.trim() !== from.host ||
    port.trim() !== from.port ||
    username.trim() !== from.username;

  $effect(() => {
    const next = saved;
    if (applying) return;
    untrack(() => {
      if (shown && editedFrom(shown) && editedFrom(next)) return;
      shown = next;
      mode = next.mode;
      iface = next.iface;
      proxyType = next.proxyType;
      host = next.host;
      port = next.port;
      username = next.username;
    });
  });

  // The mark goes as soon as the port is a valid one again.
  $effect(() => {
    if (!portBad) untrack(() => (portError = ''));
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
    portError = '';
    testResult = null;
    const rawPort = port.trim();
    if (mode === 'proxy' && rawPort !== '' && !portPattern.test(rawPort)) {
      portError = networkCodeText('settings.proxy_port_invalid');
      portRefusals += 1;
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

  // The delete button goes once the password is gone: focus held on it moves to the password field.
  async function removePassword() {
    if (passwordBusy) return;
    const handOff = focusHandOff();
    passwordBusy = true;
    failure = '';
    portError = '';
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
    await handOff(() => document.getElementById(`${uid}-password`));
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
  <SettingsRow label={msg('settings.networkModeLabel')} hint={msg('settings.networkModeSub')}>
    {#snippet children({ labelId, hintId })}
      <SegmentedControl options={modeOptions} labelledby={labelId} describedby={hintId} bind:value={mode} disabled={applying} />
    {/snippet}
  </SettingsRow>

  {#if mode === 'interface'}
    <SettingsRow label={msg('settings.networkInterfaceLabel')} hint={msg('settings.networkInterfaceSub')}>
      {#snippet children({ labelId, hintId })}
        <Select labelledby={labelId} describedby={hintId} value={iface} width="32rem" options={interfaceOptions} onchange={(id) => (iface = id)} />
        <Button size="sm" busy={interfacesLoading} onclick={loadInterfaces}>
          {#if !interfacesLoading}<RefreshCw size="1.4rem" strokeWidth={1.8} />{/if}
          {msg('settings.networkInterfaceRefresh')}
        </Button>
      {/snippet}
      {#snippet below()}
        {#if interfacesError}
          <Notice tone="danger" compact>{interfacesError}</Notice>
        {:else if interfacesLoaded && interfaces.length === 0}
          <Notice tone="warning" compact>{msg('settings.networkInterfaceEmpty')}</Notice>
        {/if}
      {/snippet}
    </SettingsRow>
  {/if}

  {#if mode === 'proxy'}
    <SettingsRow label={msg('settings.networkProxyTypeLabel')}>
      {#snippet children({ labelId })}
        <Select labelledby={labelId} value={proxyType} width="20rem" options={proxyTypeOptions} onchange={(id) => (proxyType = id)} />
      {/snippet}
    </SettingsRow>
    <SettingsRow label={msg('settings.networkProxyHostLabel')} hint={msg('settings.networkProxyHostSub')} labelFor="{uid}-host">
      {#snippet children({ hintId })}
        <input id="{uid}-host" class="input field" type="text" autocomplete="off" spellcheck="false" aria-describedby={hintId} placeholder="proxy.example.com" bind:value={host} />
      {/snippet}
    </SettingsRow>
    <SettingsRow label={msg('settings.networkProxyPortLabel')} labelFor="{uid}-port">
      {#snippet children()}
        <input
          id="{uid}-port"
          class="input field port"
          type="text"
          inputmode="numeric"
          autocomplete="off"
          aria-invalid={portError ? 'true' : undefined}
          aria-describedby={portError ? `${uid}-port-error` : undefined}
          placeholder="1080"
          bind:value={port}
        />
      {/snippet}
      {#snippet below()}
        {#if portError}
          {#key portRefusals}
            <span class="field-error" id="{uid}-port-error" role="alert">{portError}</span>
          {/key}
        {/if}
      {/snippet}
    </SettingsRow>
    <SettingsRow label={msg('settings.networkProxyUsernameLabel')} hint={msg('settings.networkProxyUsernameSub')} labelFor="{uid}-username">
      {#snippet children({ hintId })}
        <input id="{uid}-username" class="input field" type="text" autocomplete="off" spellcheck="false" aria-describedby={hintId} bind:value={username} />
      {/snippet}
    </SettingsRow>
    <SettingsRow label={msg('settings.networkProxyPasswordLabel')} hint={msg('settings.networkProxyPasswordSub')} labelFor="{uid}-password">
      {#snippet children({ hintId })}
        <input
          id="{uid}-password"
          class="input field"
          type="password"
          autocomplete="new-password"
          aria-describedby={hintId}
          placeholder={hasPassword ? msg('settings.networkProxyPasswordSaved') : msg('settings.networkProxyPasswordLabel')}
          bind:value={password}
        />
        {#if hasPassword}
          <Button size="sm" variant="danger" busy={passwordBusy} onclick={removePassword}>
            {msg('settings.networkProxyPasswordDelete')}
          </Button>
        {/if}
      {/snippet}
    </SettingsRow>
    <div class="part">
      <div class="wide"><Notice tone="warning" compact announce={false}>{msg('settings.networkProxyWarning')}</Notice></div>
    </div>
    <div class="part test">
      <div class="result">
        <!-- The button's description already says this to a screen reader; this line is for the eye. -->
        {#if dirty}
          <span class="needs-apply">{msg('settings.networkProxyTestNeedsApply')}</span>
        {:else if testResult}
          <Notice tone={testResult.ok ? 'success' : 'danger'} compact>{testResult.text}</Notice>
        {/if}
      </div>
      <Button size="sm" disabled={dirty} reason={dirty ? msg('settings.networkProxyTestNeedsApply') : ''} busy={testing} onclick={checkProxy}>
        {msg('settings.networkProxyTest')}
      </Button>
    </div>
  {/if}

  {#if failure}
    <div class="part">
      <div class="wide"><Notice tone="danger" compact>{failure}</Notice></div>
    </div>
  {/if}

  <div class="part actions">
    <Button variant="primary" disabled={!dirty} reason={dirty ? '' : msg('settings.networkApplyNothing')} busy={applying} onclick={apply}>
      {msg('settings.networkApply')}
    </Button>
  </div>

  {#if status}
    <div class="part status">
      <div class="status-text">
        <span class="status-label">{msg('settings.networkStatusLabel')}</span>
        {#if statusDetail}
          <span class="status-sub">{statusDetail}</span>
        {/if}
        {#if statusWarning}
          <span class="status-sub">{statusWarning}</span>
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
</Card>

<style>
  .part {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3) var(--space-6);
    padding: var(--space-3) 0;
    border-top: 1px solid var(--border);
  }

  .wide {
    flex: 1;
    min-width: 0;
  }

  .result {
    flex: 1 1 22rem;
    min-width: 0;
  }

  .needs-apply {
    font-size: var(--font-xs);
    line-height: 1.5;
    color: var(--text-2);
  }

  .actions {
    justify-content: flex-end;
  }

  .status-text {
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
    min-width: 0;
  }

  .status-label {
    font-size: var(--font-md);
    font-weight: 500;
  }

  .status-sub {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .field {
    width: 26rem;
    max-width: 100%;
  }

  .field.port {
    width: 12rem;
  }

  .field-error {
    font-size: var(--font-xs);
    color: var(--danger);
  }

  .port[aria-invalid='true'] {
    border-color: var(--danger);
  }
</style>
