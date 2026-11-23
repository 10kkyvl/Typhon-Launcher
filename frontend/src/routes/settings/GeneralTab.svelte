<script lang="ts">
  import { Eye, RefreshCw, Trash2 } from '@lucide/svelte';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import Select from '../../lib/components/Select.svelte';
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
  let { page, uid }: { page: SettingsPage; uid: string } = $props();

  const current = $derived($settings);
  const scaleValue = $derived(String(Math.round(($settings?.uiScale ?? 1) * 100)));

  const accountReady = $derived($authState === 'authenticated');

  function set(patch: Partial<Settings>) {
    updateSettings(patch);
  }

  const labelId = (name: string) => `${uid}-${name}-label`;
  const subId = (name: string) => `${uid}-${name}-sub`;

  const portable = $derived(page.appInfo?.portable ?? false);

  async function setLaunchOnStartup(value: boolean) {
    await updateSettingsReporting({ launchOnStartup: value }, (err) => {
      const code = errorCode(err);
      toast(code && hasMessage(code) ? msg(code) : msg('state.settingsSaveFailed'), 'danger');
    });
  }

  const gameModeMac = $derived(page.appInfo?.platform === 'darwin');
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
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-launch-on-startup')}>{msg('settings.generalLaunchOnStartupLabel')}</span>
            <span class="row-sub" id={subId('general-launch-on-startup')}>{portable ? msg('settings.generalLaunchOnStartupPortable') : msg('settings.generalLaunchOnStartupSub')}</span>
          </div>
          <Toggle
            checked={!portable && (current?.launchOnStartup ?? false)}
            disabled={portable}
            labelledby={labelId('general-launch-on-startup')}
            describedby={subId('general-launch-on-startup')}
            onchange={(v) => void setLaunchOnStartup(v)}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-minimize-to-tray')}>{msg('settings.generalMinimizeToTrayLabel')}</span>
            <span class="row-sub" id={subId('general-minimize-to-tray')}>{msg('settings.generalMinimizeToTraySub')}</span>
          </div>
          <Toggle
            checked={current?.minimizeToTray ?? true}
            labelledby={labelId('general-minimize-to-tray')}
            describedby={subId('general-minimize-to-tray')}
            onchange={(v) => set({ minimizeToTray: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-discord-rpc')}>{msg('settings.generalDiscordRpcLabel')}</span>
            <span class="row-sub" id={subId('general-discord-rpc')}>{msg('settings.generalDiscordRpcSub')}</span>
          </div>
          <Toggle
            checked={current?.discordRichPresence ?? false}
            labelledby={labelId('general-discord-rpc')}
            describedby={subId('general-discord-rpc')}
            onchange={(v) => set({ discordRichPresence: v })}
          />
        </div>
      </div>
    </Card>

    <Card title={msg('settings.gameModeCardTitle')}>
      <div class="rows">
        <span class="row-sub">{msg('settings.gameModeDuring')}</span>
        {#if !gameModeMac}
          <div class="row">
            <div class="row-text">
              <span class="row-label" id={labelId('game-mode-power')}>{msg('settings.gameModePowerLabel')}</span>
              <span class="row-sub" id={subId('game-mode-power')}>{msg('settings.gameModePowerSub')}</span>
            </div>
            <Toggle
              checked={current?.gameMode?.highPerformance ?? false}
              labelledby={labelId('game-mode-power')}
              describedby={subId('game-mode-power')}
              onchange={(v) => setGameMode({ highPerformance: v })}
            />
          </div>
        {/if}
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('game-mode-hide')}>{msg('settings.gameModeHideLabel')}</span>
            <span class="row-sub" id={subId('game-mode-hide')}>{msg('settings.gameModeHideSub')}</span>
            {#if !trayEnabled}
              <span class="row-sub">{msg('settings.gameModeHideNeedsTray')}</span>
            {/if}
          </div>
          <Toggle
            checked={trayEnabled && (current?.gameMode?.hideLauncher ?? false)}
            disabled={!trayEnabled}
            labelledby={labelId('game-mode-hide')}
            describedby={subId('game-mode-hide')}
            onchange={(v) => setGameMode({ hideLauncher: v })}
          />
        </div>
        {#if !gameModeMac}
          <div class="row">
            <div class="row-text">
              <span class="row-label" id={labelId('game-mode-priority')}>{msg('settings.gameModePriorityLabel')}</span>
              <span class="row-sub" id={subId('game-mode-priority')}>{msg('settings.gameModePrioritySub')}</span>
            </div>
            <Toggle
              checked={current?.gameMode?.raisePriority ?? false}
              labelledby={labelId('game-mode-priority')}
              describedby={subId('game-mode-priority')}
              onchange={(v) => setGameMode({ raisePriority: v })}
            />
          </div>
        {/if}
      </div>
    </Card>

    <Card title={msg('settings.overlayCardTitle')}>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('overlay-enabled')}>{msg('settings.overlayEnabledLabel')}</span>
            <span class="row-sub" id={subId('overlay-enabled')}>{msg('settings.overlayEnabledSub')}</span>
            <span class="row-sub">{msg('settings.overlayExclusiveNote')}</span>
          </div>
          <Toggle
            checked={current?.overlayEnabled ?? true}
            labelledby={labelId('overlay-enabled')}
            describedby={subId('overlay-enabled')}
            onchange={(v) => page.setOverlay({ overlayEnabled: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('overlay-hotkey')}>{msg('settings.overlayHotkeyLabel')}</span>
            <span class="row-sub">{msg('settings.overlayHotkeySub')}</span>
          </div>
          <Select
            labelledby={labelId('overlay-hotkey')}
            value={overlayHotkey}
            width="22rem"
            options={overlayHotkeyOptions}
            onchange={(id) => page.setOverlay({ overlayHotkey: id })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('screenshots-enabled')}>{msg('settings.screenshotsEnabledLabel')}</span>
            <span class="row-sub" id={subId('screenshots-enabled')}>{msg('settings.screenshotsEnabledSub')}</span>
          </div>
          <Toggle
            checked={current?.screenshotsEnabled ?? true}
            labelledby={labelId('screenshots-enabled')}
            describedby={subId('screenshots-enabled')}
            onchange={(v) => setScreenshots({ screenshotsEnabled: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('screenshot-hotkey')}>{msg('settings.screenshotHotkeyLabel')}</span>
            <span class="row-sub">{msg('settings.screenshotHotkeySub')}</span>
          </div>
          <Select
            labelledby={labelId('screenshot-hotkey')}
            value={screenshotHotkey}
            width="22rem"
            options={screenshotHotkeyOptions}
            onchange={(id) => setScreenshots({ screenshotHotkey: id })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.overlayStatusLabel')}</span>
            <!-- The overlay's state, refreshed by events: a change is said politely; a failure sits on a
                 danger plate, which says nothing of its own on opening the page. -->
            <div role="status">
              {#if overlayStatusFailed}
                <div class="message"><Notice tone="danger" compact announce={false}>{overlayStatusText}</Notice></div>
              {:else}
                <span class="row-sub">{overlayStatusText}</span>
              {/if}
            </div>
            {#if page.overlayError}<div class="message"><Notice tone="danger" compact>{page.overlayError}</Notice></div>{/if}
          </div>
        </div>
      </div>
    </Card>

    <LibraryCard {page} />

    <Card title={msg('settings.generalExperimentalCardTitle')}>
      <div class="rows">
        <LanSettingsRow />
      </div>
    </Card>
  </div>

  <div class="settings-column">
    <Card title={msg('settings.generalInterfaceCardTitle')}>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('language')}>{msg('settings.language')}</span>
            <span class="row-sub">{msg('settings.languageSub')}</span>
          </div>
          <Select
            labelledby={labelId('language')}
            value={$settings?.language ?? 'system'}
            width="22rem"
            options={[
              { id: 'system', label: msg('settings.languageSystem') },
              { id: 'ru', label: 'Русский' },
              { id: 'en', label: 'English' },
            ]}
            onchange={(id) => set({ language: id })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('ui-scale')}>{msg('settings.generalUiScaleLabel')}</span>
            <span class="row-sub">{msg('settings.generalUiScaleSub')}</span>
          </div>
          <Select
            labelledby={labelId('ui-scale')}
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
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-animations')}>{msg('settings.generalAnimationsLabel')}</span>
            <span class="row-sub" id={subId('general-animations')}>{msg('settings.generalAnimationsSub')}</span>
          </div>
          <Toggle
            checked={current?.animationsEnabled ?? true}
            labelledby={labelId('general-animations')}
            describedby={subId('general-animations')}
            onchange={(v) => set({ animationsEnabled: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-hw-accel')}>{msg('settings.generalHwAccelLabel')}</span>
            <span class="row-sub" id={subId('general-hw-accel')}>{msg('settings.generalHwAccelSub')}</span>
          </div>
          <Toggle
            checked={current?.hardwareAcceleration ?? true}
            labelledby={labelId('general-hw-accel')}
            describedby={subId('general-hw-accel')}
            onchange={(v) => set({ hardwareAcceleration: v })}
          />
        </div>
      </div>
    </Card>

    <Card title={msg('settings.generalPrivacyCardTitle')}>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-privacy-stats')}>{msg('settings.generalPrivacyStatsLabel')}</span>
            <span class="row-sub" id={subId('general-privacy-stats')}>{msg('settings.generalPrivacyStatsSub')}</span>
          </div>
          <Toggle
            checked={current?.anonymousUsageStats ?? false}
            labelledby={labelId('general-privacy-stats')}
            describedby={subId('general-privacy-stats')}
            onchange={(v) => set({ anonymousUsageStats: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-privacy-diag')}>{msg('settings.generalPrivacyDiagLabel')}</span>
            <span class="row-sub" id={subId('general-privacy-diag')}>{msg('settings.generalPrivacyDiagSub')}</span>
          </div>
          <Toggle
            checked={current?.anonymousDiagnostics ?? false}
            labelledby={labelId('general-privacy-diag')}
            describedby={subId('general-privacy-diag')}
            onchange={(v) => set({ anonymousDiagnostics: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-privacy-heartbeat')}>{msg('settings.generalPrivacyHeartbeatLabel')}</span>
            <span class="row-sub" id={subId('general-privacy-heartbeat')}>{msg('settings.generalPrivacyHeartbeatSub')}</span>
          </div>
          <Toggle
            checked={current?.sessionHeartbeat ?? true}
            labelledby={labelId('general-privacy-heartbeat')}
            describedby={subId('general-privacy-heartbeat')}
            onchange={(v) => set({ sessionHeartbeat: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.generalPrivacyShowSentLabel')}</span>
            <span class="row-sub">{msg('settings.generalPrivacyShowSentSub')}</span>
          </div>
          <Button size="sm" onclick={() => (page.sentDataOpen = true)}>
            <Eye size="1.5rem" strokeWidth={1.8} />
            {msg('settings.generalPrivacyShowSentButton')}
          </Button>
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.generalPrivacySessionStateLabel')}</span>
            <span class="row-sub">{msg('settings.generalPrivacySessionStateSub')}</span>
            <span class="row-sub privacy-link">
              <TextLink onclick={() => page.openLegalDoc({ id: 'privacy', title: msg('settings.generalPrivacyPolicyLinkLabel') })}>
                {msg('settings.generalPrivacyPolicyLinkLabel')}
              </TextLink>
            </span>
          </div>
        </div>
      </div>
    </Card>

    <Card title={msg('settings.generalSyncCardTitle')}>
      <div class="rows">
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-sync-enable')}>{msg('settings.generalSyncEnableLabel')}</span>
            <span class="row-sub" id={subId('general-sync-enable')}>{msg('settings.generalSyncEnableSub')}</span>
            {#if !accountReady}
              <span class="row-sub">{msg('settings.generalSyncNeedsAccountSub')}</span>
            {/if}
          </div>
          <Toggle
            checked={current?.accountSync ?? false}
            labelledby={labelId('general-sync-enable')}
            describedby={subId('general-sync-enable')}
            disabled={!accountReady}
            onchange={(v) => set({ accountSync: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label" id={labelId('general-auto-away')}>{msg('settings.generalAutoAwayLabel')}</span>
            <span class="row-sub" id={subId('general-auto-away')}>{msg('settings.generalAutoAwaySub')}</span>
          </div>
          <Toggle
            checked={current?.presenceAutoAway ?? true}
            labelledby={labelId('general-auto-away')}
            describedby={subId('general-auto-away')}
            disabled={!accountReady || !current?.accountSync}
            onchange={(v) => set({ presenceAutoAway: v })}
          />
        </div>
        <div class="row">
          <div class="row-text">
            <span class="row-label">{msg('settings.generalSyncNowLabel')}</span>
            <span class="row-sub">{msg('settings.generalSyncNowSub')}</span>
          </div>
          <Button size="sm" disabled={!accountReady || !current?.accountSync || page.syncingNow} onclick={page.runSyncNow}>
            <RefreshCw size="1.5rem" strokeWidth={1.8} />
            {page.syncingNow ? msg('settings.generalSyncNowRunning') : msg('settings.generalSyncNowLabel')}
          </Button>
        </div>
        <div class="row danger-zone">
          <div class="row-text">
            <span class="row-label">{msg('settings.generalSyncForgetLabel')}</span>
            <span class="row-sub">{msg('settings.generalSyncForgetSub')}</span>
          </div>
          <Button
            size="sm"
            variant="danger"
            disabled={!accountReady || page.forgettingRemote}
            onclick={page.askForgetRemote}
          >
            <Trash2 size="1.5rem" strokeWidth={1.8} />
            {page.forgettingRemote ? msg('settings.generalSyncForgetRunning') : msg('settings.generalSyncForgetLabel')}
          </Button>
        </div>
      </div>
    </Card>

    {#if accountReady}
      <DiscordCard />
    {/if}
  </div>
</div>

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

  .row.danger-zone {
    margin-top: var(--space-3);
    padding: var(--space-3) var(--space-4);
    border: 1px solid color-mix(in srgb, var(--danger) 30%, transparent);
    border-radius: var(--radius-md);
    background: var(--danger-subtle);
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

  .privacy-link {
    margin-top: var(--space-1);
  }

  .message {
    margin-top: var(--space-2);
  }
</style>
