<script lang="ts">
  import { Eye, RefreshCw, Trash2 } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Select from '../../lib/components/Select.svelte';
  import SettingsRow from '../../lib/components/SettingsRow.svelte';
  import TextLink from '../../lib/components/TextLink.svelte';
  import Toggle from '../../lib/components/Toggle.svelte';
  import DiscordCard from './DiscordCard.svelte';
  import LanSettingsRow from './LanSettingsRow.svelte';
  import LibraryCard from './LibraryCard.svelte';
  import { errorCode, hasMessage, msg } from '../../lib/i18n';
  import { SCREENSHOT_HOTKEYS } from '../../lib/services/screenshots';
  import { OVERLAY_HOTKEYS } from '../../lib/services/overlay';
  import type { GameMode, Settings } from '../../lib/services/settings';
  import { settings, updateSettings, updateSettingsReporting } from '../../lib/stores/settings';
  import { toast } from '../../lib/stores/toasts';
  import { authState } from '../../lib/stores/user';
  import { overlayReason, type SettingsPage } from './settingsPage.svelte';

  // The general tab: startup, game mode, the overlay and screenshots, the library, the interface,
  // privacy, account sync and Discord. The overlay status, the sync actions and the dialogs these
  // rows open are the page's: they outlive a switch of tabs.
  let { page }: { page: SettingsPage } = $props();

  const current = $derived($settings);
  const scaleValue = $derived(String(Math.round(($settings?.uiScale ?? 1) * 100)));

  const accountReady = $derived($authState === 'authenticated');
  const syncOn = $derived(current?.accountSync ?? false);
  const needsAccount = $derived(accountReady ? undefined : msg('settings.generalSyncNeedsAccountSub'));
  // Auto-away and Sync now act on the synced account: off without one, then off while sync is.
  const needsSync = $derived(needsAccount ?? (syncOn ? undefined : msg('settings.generalSyncOffReason')));

  function set(patch: Partial<Settings>) {
    updateSettings(patch);
  }

  const portable = $derived(page.appInfo?.portable ?? false);

  async function setLaunchOnStartup(value: boolean) {
    await updateSettingsReporting({ launchOnStartup: value }, (err) => {
      const code = errorCode(err);
      toast(code && hasMessage(code) ? msg(code) : msg('state.settingsSaveFailed'), 'danger');
    });
  }

  // Windows-only rows wait for the platform: drawn on a guess, a Mac would show them while the app
  // facts are pending or after their read failed.
  const gameModePlatformRows = $derived(page.appInfo !== null && page.appInfo.platform !== 'darwin');
  const trayEnabled = $derived(current?.minimizeToTray ?? true);

  function setGameMode(change: Partial<GameMode>) {
    const base = current?.gameMode ?? { highPerformance: false, hideLauncher: false, raisePriority: false };
    updateSettings({ gameMode: { ...base, ...change } });
  }

  const overlayHotkeyOptions = OVERLAY_HOTKEYS.map((key) => ({ id: key, label: key }));
  const overlayHotkey = $derived.by(() => {
    const id = current?.overlayHotkey ?? OVERLAY_HOTKEYS[0];
    return overlayHotkeyOptions.some((o) => o.id === id) ? id : OVERLAY_HOTKEYS[0];
  });
  const overlayStatusFailed = $derived(page.overlayInfoFailed || (!!page.overlayInfo?.supported && !!page.overlayInfo.error));
  const overlayStatusText = $derived.by(() => {
    if (page.overlayInfoFailed) return msg('settings.overlayStatusUnknown');
    if (!page.overlayInfo) return '';
    if (!page.overlayInfo.supported) return msg('settings.overlayStatusUnsupported');
    if (page.overlayInfo.error) return overlayReason(page.overlayInfo.error) || msg('settings.overlaySaveErrorPlain');
    if (page.overlayInfo.enabled) return msg('settings.overlayStatusOn', { hotkey: page.overlayInfo.hotkey });
    return msg('settings.overlayStatusOff');
  });

  const screenshotHotkeyOptions = SCREENSHOT_HOTKEYS.map((key) => ({ id: key, label: key }));
  const screenshotHotkey = $derived.by(() => {
    const id = current?.screenshotHotkey ?? SCREENSHOT_HOTKEYS[0];
    return screenshotHotkeyOptions.some((o) => o.id === id) ? id : SCREENSHOT_HOTKEYS[0];
  });

  async function setScreenshots(patch: Partial<Settings>) {
    await updateSettingsReporting(patch, (err) => {
      const reason = overlayReason(err);
      toast(
        reason ? msg('settings.screenshotsSaveError', { reason }) : msg('settings.screenshotsSaveErrorPlain'),
        'danger',
      );
    });
  }
</script>

<div class="settings-grid">
  <div class="settings-column">
    <Card title={msg('settings.generalStartupCardTitle')}>
      <SettingsRow
        label={msg('settings.generalLaunchOnStartupLabel')}
        hint={msg('settings.generalLaunchOnStartupSub')}
        disabledReason={portable ? msg('settings.generalLaunchOnStartupPortable') : undefined}
      >
        {#snippet children({ labelId, hintId, disabled })}
          <Toggle
            checked={!portable && (current?.launchOnStartup ?? false)}
            {disabled}
            labelledby={labelId}
            describedby={hintId}
            onchange={(v) => void setLaunchOnStartup(v)}
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalMinimizeToTrayLabel')} hint={msg('settings.generalMinimizeToTraySub')}>
        {#snippet children({ labelId, hintId })}
          <Toggle checked={current?.minimizeToTray ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ minimizeToTray: v })} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalDiscordRpcLabel')} hint={msg('settings.generalDiscordRpcSub')}>
        {#snippet children({ labelId, hintId })}
          <Toggle checked={current?.discordRichPresence ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ discordRichPresence: v })} />
        {/snippet}
      </SettingsRow>
    </Card>

    <Card title={msg('settings.gameModeCardTitle')}>
      <p class="intro">{msg('settings.gameModeDuring')}</p>
      {#if gameModePlatformRows}
        <SettingsRow label={msg('settings.gameModePowerLabel')} hint={msg('settings.gameModePowerSub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle
              checked={current?.gameMode?.highPerformance ?? false}
              labelledby={labelId}
              describedby={hintId}
              onchange={(v) => setGameMode({ highPerformance: v })}
            />
          {/snippet}
        </SettingsRow>
      {/if}
      <SettingsRow
        label={msg('settings.gameModeHideLabel')}
        hint={msg('settings.gameModeHideSub')}
        disabledReason={trayEnabled ? undefined : msg('settings.gameModeHideNeedsTray')}
      >
        {#snippet children({ labelId, hintId, disabled })}
          <Toggle
            checked={trayEnabled && (current?.gameMode?.hideLauncher ?? false)}
            {disabled}
            labelledby={labelId}
            describedby={hintId}
            onchange={(v) => setGameMode({ hideLauncher: v })}
          />
        {/snippet}
      </SettingsRow>
      {#if gameModePlatformRows}
        <SettingsRow label={msg('settings.gameModePriorityLabel')} hint={msg('settings.gameModePrioritySub')}>
          {#snippet children({ labelId, hintId })}
            <Toggle
              checked={current?.gameMode?.raisePriority ?? false}
              labelledby={labelId}
              describedby={hintId}
              onchange={(v) => setGameMode({ raisePriority: v })}
            />
          {/snippet}
        </SettingsRow>
      {/if}
    </Card>

    <Card title={msg('settings.overlayCardTitle')}>
      <SettingsRow label={msg('settings.overlayEnabledLabel')} hint={msg('settings.overlayEnabledSub')} note={msg('settings.overlayExclusiveNote')}>
        {#snippet children({ labelId, hintId })}
          <Toggle
            checked={current?.overlayEnabled ?? true}
            labelledby={labelId}
            describedby={hintId}
            onchange={(v) => page.setOverlay({ overlayEnabled: v })}
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.overlayHotkeyLabel')} hint={msg('settings.overlayHotkeySub')}>
        {#snippet children({ labelId, hintId })}
          <Select
            labelledby={labelId}
            describedby={hintId}
            value={overlayHotkey}
            width="22rem"
            options={overlayHotkeyOptions}
            onchange={(id) => page.setOverlay({ overlayHotkey: id })}
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.screenshotsEnabledLabel')} hint={msg('settings.screenshotsEnabledSub')}>
        {#snippet children({ labelId, hintId })}
          <Toggle
            checked={current?.screenshotsEnabled ?? true}
            labelledby={labelId}
            describedby={hintId}
            onchange={(v) => setScreenshots({ screenshotsEnabled: v })}
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.screenshotHotkeyLabel')} hint={msg('settings.screenshotHotkeySub')}>
        {#snippet children({ labelId, hintId })}
          <Select
            labelledby={labelId}
            describedby={hintId}
            value={screenshotHotkey}
            width="22rem"
            options={screenshotHotkeyOptions}
            onchange={(id) => setScreenshots({ screenshotHotkey: id })}
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.overlayStatusLabel')}>
        {#snippet below({ labelId })}
          <!-- The overlay's state, refreshed by events: a change is said politely; a failure sits on a
               danger plate, which says nothing of its own on opening the page. -->
          <div role="status" aria-labelledby={labelId}>
            {#if overlayStatusFailed}
              <Notice tone="danger" compact announce={false}>{overlayStatusText}</Notice>
            {:else}
              <span class="status-text">{overlayStatusText}</span>
            {/if}
          </div>
          {#if page.overlayError}<div class="message"><Notice tone="danger" compact>{page.overlayError}</Notice></div>{/if}
        {/snippet}
      </SettingsRow>
    </Card>

    <LibraryCard {page} />

    <Card title={msg('settings.generalExperimentalCardTitle')}>
      <LanSettingsRow />
    </Card>
  </div>

  <div class="settings-column">
    <Card title={msg('settings.generalInterfaceCardTitle')}>
      <SettingsRow label={msg('settings.language')} hint={msg('settings.languageSub')}>
        {#snippet children({ labelId, hintId })}
          <!-- Each language by its own name, read the same whatever the interface's language. -->
          <Select
            labelledby={labelId}
            describedby={hintId}
            value={$settings?.language ?? 'system'}
            width="22rem"
            options={[
              { id: 'system', label: msg('settings.languageSystem') },
              { id: 'ru', label: 'Русский' },
              { id: 'en', label: 'English' },
            ]}
            onchange={(id) => set({ language: id })}
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalUiScaleLabel')} hint={msg('settings.generalUiScaleSub')}>
        {#snippet children({ labelId, hintId })}
          <Select
            labelledby={labelId}
            describedby={hintId}
            value={scaleValue}
            width="22rem"
            options={[
              { id: '90', label: '90%' },
              { id: '100', label: msg('settings.generalUiScaleDefaultOption') },
              { id: '110', label: '110%' },
              { id: '125', label: '125%' },
            ]}
            onchange={(id) => set({ uiScale: Number(id) / 100 })}
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalAnimationsLabel')} hint={msg('settings.generalAnimationsSub')}>
        {#snippet children({ labelId, hintId })}
          <Toggle checked={current?.animationsEnabled ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ animationsEnabled: v })} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalHwAccelLabel')} hint={msg('settings.generalHwAccelSub')}>
        {#snippet children({ labelId, hintId })}
          <Toggle checked={current?.hardwareAcceleration ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ hardwareAcceleration: v })} />
        {/snippet}
      </SettingsRow>
    </Card>

    <Card title={msg('settings.generalPrivacyCardTitle')}>
      <SettingsRow label={msg('settings.generalPrivacyStatsLabel')} hint={msg('settings.generalPrivacyStatsSub')}>
        {#snippet children({ labelId, hintId })}
          <Toggle checked={current?.anonymousUsageStats ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ anonymousUsageStats: v })} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalPrivacyDiagLabel')} hint={msg('settings.generalPrivacyDiagSub')}>
        {#snippet children({ labelId, hintId })}
          <Toggle checked={current?.anonymousDiagnostics ?? false} labelledby={labelId} describedby={hintId} onchange={(v) => set({ anonymousDiagnostics: v })} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalPrivacyHeartbeatLabel')} hint={msg('settings.generalPrivacyHeartbeatSub')}>
        {#snippet children({ labelId, hintId })}
          <Toggle checked={current?.sessionHeartbeat ?? true} labelledby={labelId} describedby={hintId} onchange={(v) => set({ sessionHeartbeat: v })} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalPrivacyShowSentLabel')} hint={msg('settings.generalPrivacyShowSentSub')}>
        {#snippet children({ hintId })}
          <Button size="sm" aria-describedby={hintId} onclick={() => (page.sentDataOpen = true)}>
            <Eye size="1.5rem" strokeWidth={1.8} />
            {msg('settings.generalPrivacyShowSentButton')}
          </Button>
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalPrivacySessionStateLabel')} hint={msg('settings.generalPrivacySessionStateSub')}>
        {#snippet below()}
          <TextLink onclick={() => page.openLegalDoc({ id: 'privacy', title: msg('settings.generalPrivacyPolicyLinkLabel') })}>
            {msg('settings.generalPrivacyPolicyLinkLabel')}
          </TextLink>
        {/snippet}
      </SettingsRow>
    </Card>

    <Card title={msg('settings.generalSyncCardTitle')}>
      <SettingsRow label={msg('settings.generalSyncEnableLabel')} hint={msg('settings.generalSyncEnableSub')} disabledReason={needsAccount}>
        {#snippet children({ labelId, hintId, disabled })}
          <Toggle checked={syncOn} labelledby={labelId} describedby={hintId} {disabled} onchange={(v) => set({ accountSync: v })} />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalAutoAwayLabel')} hint={msg('settings.generalAutoAwaySub')} disabledReason={needsSync}>
        {#snippet children({ labelId, hintId, disabled })}
          <Toggle
            checked={current?.presenceAutoAway ?? true}
            labelledby={labelId}
            describedby={hintId}
            {disabled}
            onchange={(v) => set({ presenceAutoAway: v })}
          />
        {/snippet}
      </SettingsRow>
      <SettingsRow label={msg('settings.generalSyncNowLabel')} hint={msg('settings.generalSyncNowSub')} disabledReason={needsSync}>
        {#snippet children({ hintId, disabled })}
          <Button size="sm" aria-describedby={hintId} {disabled} busy={page.syncingNow} onclick={page.runSyncNow}>
            <RefreshCw size="1.5rem" strokeWidth={1.8} />
            {page.syncingNow ? msg('settings.generalSyncNowRunning') : msg('settings.generalSyncNowLabel')}
          </Button>
        {/snippet}
      </SettingsRow>
      <div class="danger-zone">
        <SettingsRow label={msg('settings.generalSyncForgetLabel')} hint={msg('settings.generalSyncForgetSub')} disabledReason={needsAccount}>
          {#snippet children({ hintId, disabled })}
            <Button size="sm" variant="danger" aria-describedby={hintId} {disabled} busy={page.forgettingRemote} onclick={page.askForgetRemote}>
              <Trash2 size="1.5rem" strokeWidth={1.8} />
              {page.forgettingRemote ? msg('settings.generalSyncForgetRunning') : msg('settings.generalSyncForgetLabel')}
            </Button>
          {/snippet}
        </SettingsRow>
      </div>
    </Card>

    {#if accountReady}
      <DiscordCard />
    {/if}
  </div>
</div>

<style>
  .intro {
    margin: 0 0 var(--space-1);
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .status-text {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .message {
    margin-top: var(--space-2);
  }

  /* An action that cannot be undone stands apart, on its own danger plate. */
  .danger-zone {
    margin-top: var(--space-3);
    padding: 0 var(--space-4);
    border: 1px solid color-mix(in srgb, var(--danger) 30%, transparent);
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
  }
</style>
