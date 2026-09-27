<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { Check, Download, MonitorCog, Rocket, Sparkles, Wrench } from '@lucide/svelte';
  import { focusControl } from '../navigation';
  import type { RequestText } from '../contracts';
  import type { Settings } from '../../services/settings';
  import { t } from '../../i18n';
  import { settings } from '../../stores/settings';
  import { BIG_PICTURE_DOWNLOAD_LIMITS, BIG_PICTURE_LANGUAGES, BIG_PICTURE_UI_SCALES, saveBigPicturePreference, type PreferenceSaveState } from '../preferences';

  let { onback }: { onback: () => void; requestText: RequestText } = $props();
  let page: HTMLElement;
  let pendingSaves = $state(0);
  let failedSave = $state(false);
  let saveState = $state<PreferenceSaveState | 'saving' | ''>('');

  const current = $derived($settings);

  async function focusInitial() {
    await tick();
    const target = page?.querySelector<HTMLButtonElement>('[data-bp-default]') ?? page?.querySelector<HTMLButtonElement>('[data-bp-focus]');
    if (target) focusControl(target);
  }

  async function save(patch: Partial<Settings>) {
    pendingSaves += 1;
    failedSave = false;
    saveState = 'saving';
    const result = await saveBigPicturePreference(patch);
    pendingSaves -= 1;
    if (result === 'failed') failedSave = true;
    if (pendingSaves === 0) saveState = failedSave ? 'failed' : 'saved';
  }

  function toggle(key: 'launchOnStartup' | 'minimizeToTray' | 'discordRichPresence' | 'animationsEnabled' | 'autoInstall' | 'installSkipExtras' | 'installSkipShortcuts') {
    if (!current) return;
    void save({ [key]: !current[key] });
  }

  function languageLabel(language: (typeof BIG_PICTURE_LANGUAGES)[number]): string {
    if (language === 'system') return $t('bp.settings.languageSystem');
    if (language === 'ru') return $t('bp.settings.languageRussian');
    return $t('bp.settings.languageEnglish');
  }

  onMount(() => { void focusInitial(); });
</script>

<div class="bp-page settings-page" bind:this={page}>
  <header class="bp-header">
    <div class="title-copy">
      <span class="eyebrow">TYPHON · BIG PICTURE</span>
      <h1>{$t('bp.settings.title')}</h1>
      <p>{$t('bp.settings.subtitle')}</p>
    </div>
    <div class="bp-actions">
      <button class="bp-button back-button" data-bp-focus="settings-back" data-bp-default onclick={onback}>
        <span aria-hidden="true">←</span>{$t('bp.settings.back')}
      </button>
    </div>
  </header>

  {#if current}
    <div class="bp-grid settings-grid">
      <section class="bp-card">
        <h2><Rocket size="1.45em" />{$t('bp.settings.sectionBehavior')}</h2>
        <div class="setting-list">
          <div class="setting-row">
            <div class="setting-copy"><h3>{$t('bp.settings.launchOnStartup')}</h3><p>{$t('bp.settings.launchOnStartupHint')}</p></div>
            <button class="bp-button switch" class:on={current.launchOnStartup} role="switch" aria-checked={current.launchOnStartup} aria-label={$t('bp.settings.launchOnStartup')} data-bp-focus="settings-launch-startup" onclick={() => toggle('launchOnStartup')}>
              <span class="switch-state">{current.launchOnStartup ? $t('bp.settings.enabled') : $t('bp.settings.disabled')}</span><span class="switch-knob"></span>
            </button>
          </div>
          <div class="setting-row">
            <div class="setting-copy"><h3>{$t('bp.settings.minimizeToTray')}</h3><p>{$t('bp.settings.minimizeToTrayHint')}</p></div>
            <button class="bp-button switch" class:on={current.minimizeToTray} role="switch" aria-checked={current.minimizeToTray} aria-label={$t('bp.settings.minimizeToTray')} data-bp-focus="settings-minimize-tray" onclick={() => toggle('minimizeToTray')}>
              <span class="switch-state">{current.minimizeToTray ? $t('bp.settings.enabled') : $t('bp.settings.disabled')}</span><span class="switch-knob"></span>
            </button>
          </div>
          <div class="setting-row">
            <div class="setting-copy"><h3>{$t('bp.settings.discordPresence')}</h3><p>{$t('bp.settings.discordPresenceHint')}</p></div>
            <button class="bp-button switch" class:on={current.discordRichPresence} role="switch" aria-checked={current.discordRichPresence} aria-label={$t('bp.settings.discordPresence')} data-bp-focus="settings-discord-presence" onclick={() => toggle('discordRichPresence')}>
              <span class="switch-state">{current.discordRichPresence ? $t('bp.settings.enabled') : $t('bp.settings.disabled')}</span><span class="switch-knob"></span>
            </button>
          </div>
        </div>
      </section>

      <section class="bp-card">
        <h2><MonitorCog size="1.45em" />{$t('bp.settings.sectionInterface')}</h2>
        <div class="setting-list">
          <div class="setting-row">
            <div class="setting-copy"><h3>{$t('bp.settings.animations')}</h3><p>{$t('bp.settings.animationsHint')}</p></div>
            <button class="bp-button switch" class:on={current.animationsEnabled} role="switch" aria-checked={current.animationsEnabled} aria-label={$t('bp.settings.animations')} data-bp-focus="settings-animations" onclick={() => toggle('animationsEnabled')}>
              <span class="switch-state">{current.animationsEnabled ? $t('bp.settings.enabled') : $t('bp.settings.disabled')}</span><span class="switch-knob"></span>
            </button>
          </div>
          <div class="choice-block">
            <div class="setting-copy"><h3>{$t('bp.settings.scale')}</h3><p>{$t('bp.settings.scaleHint')}</p></div>
            <div class="choices" aria-label={$t('bp.settings.scale')}>
              {#each BIG_PICTURE_UI_SCALES as scale (scale)}
                {@const value = Math.round(scale * 100)}
                <button class="bp-button choice" class:selected={Math.round(current.uiScale * 100) === value} aria-pressed={Math.round(current.uiScale * 100) === value} data-bp-focus={`settings-scale-${value}`} onclick={() => void save({ uiScale: scale })}>
                  {$t('bp.settings.scaleValue', { value })}
                </button>
              {/each}
            </div>
          </div>
        </div>
      </section>

      <section class="bp-card">
        <h2><Download size="1.45em" />{$t('bp.settings.sectionDownloads')}</h2>
        <div class="setting-copy"><p>{$t('bp.settings.downloadsHint')}</p></div>
        <div class="choices download-choices" aria-label={$t('bp.settings.sectionDownloads')}>
          {#each BIG_PICTURE_DOWNLOAD_LIMITS as count (count)}
            <button class="bp-button choice" class:selected={current.maxActiveDownloads === count} aria-pressed={current.maxActiveDownloads === count} data-bp-focus={`settings-downloads-${count}`} onclick={() => void save({ maxActiveDownloads: count })}>
              {$t('bp.settings.downloadsValue', { count })}
            </button>
          {/each}
        </div>
      </section>

      <section class="bp-card">
        <h2><Wrench size="1.45em" />{$t('bp.settings.sectionInstall')}</h2>
        <div class="setting-list">
          <div class="setting-row">
            <div class="setting-copy"><h3>{$t('bp.settings.autoInstall')}</h3><p>{$t('bp.settings.autoInstallHint')}</p></div>
            <button class="bp-button switch" class:on={current.autoInstall} role="switch" aria-checked={current.autoInstall} aria-label={$t('bp.settings.autoInstall')} data-bp-focus="settings-auto-install" onclick={() => toggle('autoInstall')}>
              <span class="switch-state">{current.autoInstall ? $t('bp.settings.enabled') : $t('bp.settings.disabled')}</span><span class="switch-knob"></span>
            </button>
          </div>
          <div class="setting-row">
            <div class="setting-copy"><h3>{$t('bp.settings.skipExtras')}</h3><p>{$t('bp.settings.skipExtrasHint')}</p></div>
            <button class="bp-button switch" class:on={current.installSkipExtras} role="switch" aria-checked={current.installSkipExtras} aria-label={$t('bp.settings.skipExtras')} data-bp-focus="settings-skip-extras" onclick={() => toggle('installSkipExtras')}>
              <span class="switch-state">{current.installSkipExtras ? $t('bp.settings.enabled') : $t('bp.settings.disabled')}</span><span class="switch-knob"></span>
            </button>
          </div>
          <div class="setting-row">
            <div class="setting-copy"><h3>{$t('bp.settings.skipShortcuts')}</h3><p>{$t('bp.settings.skipShortcutsHint')}</p></div>
            <button class="bp-button switch" class:on={current.installSkipShortcuts} role="switch" aria-checked={current.installSkipShortcuts} aria-label={$t('bp.settings.skipShortcuts')} data-bp-focus="settings-skip-shortcuts" onclick={() => toggle('installSkipShortcuts')}>
              <span class="switch-state">{current.installSkipShortcuts ? $t('bp.settings.enabled') : $t('bp.settings.disabled')}</span><span class="switch-knob"></span>
            </button>
          </div>
        </div>
      </section>

      <section class="bp-card">
        <h2><Sparkles size="1.45em" />{$t('bp.settings.language')}</h2>
        <div class="setting-copy"><p>{$t('bp.settings.languageHint')}</p></div>
        <div class="choices language-choices" aria-label={$t('bp.settings.language')}>
          {#each BIG_PICTURE_LANGUAGES as language (language)}
            <button class="bp-button choice" class:selected={current.language === language} aria-pressed={current.language === language} data-bp-focus={`settings-language-${language}`} onclick={() => void save({ language })}>
              {languageLabel(language)}
            </button>
          {/each}
        </div>
      </section>
    </div>
  {:else}
    <div class="bp-empty"><Sparkles size="2.5rem" /><p>{$t('bp.settings.unavailable')}</p></div>
  {/if}

  <div class="save-status" aria-live="polite" role={saveState === 'failed' ? 'alert' : 'status'}>
    {#if saveState === 'saving'}<span>{$t('bp.settings.saving')}</span>
    {:else if saveState === 'saved'}<span class="success"><Check size="1.15em" />{$t('bp.settings.saved')}</span>
    {:else if saveState === 'failed'}<span class="error">{$t('bp.settings.saveFailed')}</span>{/if}
  </div>
</div>

<style>
  .settings-page { --setting-pad: clamp(18px, 2.2vw, 42px); padding-bottom: 2rem; }
  .title-copy { min-width: 0; }
  .eyebrow { display: block; margin-bottom: .35rem; color: var(--text-3); font-size: .72em; font-weight: 700; letter-spacing: .16em; }
  .bp-header h1 { margin: 0; font-size: clamp(2rem, 4vw, 4.6rem); line-height: 1.08; }
  .bp-header p { margin: .55rem 0 0; color: var(--text-2); font-size: 1.05em; }
  .back-button { min-height: 3.4rem; padding-inline: 1.2rem; }
  .back-button > span { font-size: 1.35em; line-height: .8; }
  .settings-grid { align-items: stretch; margin-top: clamp(1.2rem, 2.2vh, 2.4rem); }
  .settings-page .settings-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .bp-card { padding: var(--setting-pad); }
  .bp-card h2 { display: flex; align-items: center; gap: .75rem; margin: 0 0 1.35rem; font-size: clamp(1.25rem, 1.6vw, 1.8rem); }
  .bp-card h2 :global(svg) { flex: none; color: var(--accent); }
  .setting-list { display: flex; flex-direction: column; gap: 1.3rem; }
  .setting-row { display: flex; align-items: center; gap: 1.4rem; justify-content: space-between; min-width: 0; }
  .setting-copy { min-width: 0; }
  .setting-copy h3 { margin: 0 0 .22rem; font-size: 1.08em; line-height: 1.25; }
  .setting-copy p { margin: 0; max-width: 42rem; color: #b8c5d7; font-size: .88em; line-height: 1.45; }
  .switch { position: relative; flex: none; display: inline-flex; justify-content: space-between; align-items: center; gap: .8rem; min-width: 6.5rem; min-height: 3.25rem; padding: .45rem .55rem .45rem .95rem; border-radius: 99px; background: var(--surface-2); border: 1px solid var(--border); }
  .switch.on { background: color-mix(in srgb, var(--accent) 34%, var(--surface-2)); border-color: var(--accent); }
  .switch-state { min-width: 1.8em; font-size: .8em; font-weight: 700; text-transform: uppercase; }
  .switch-knob { width: 1.9rem; height: 1.9rem; border-radius: 50%; background: var(--text-3); transition: transform var(--dur) var(--ease), background var(--dur) var(--ease); }
  .switch.on .switch-knob { background: var(--accent); }
  .choice-block { display: flex; flex-direction: column; gap: .8rem; }
  .choices { display: flex; flex-wrap: wrap; gap: .55rem; }
  .choice { min-width: 5rem; min-height: 3.1rem; padding: .6rem 1rem; border: 1px solid var(--border); border-radius: var(--radius-md); background: var(--surface-2); font-weight: 650; }
  .choice.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 25%, var(--surface-2)); color: var(--text); }
  .download-choices { margin-top: 1rem; }
  .save-status { min-height: 1.6em; margin: 1rem var(--setting-pad) 0; color: var(--text-3); font-size: .9em; }
  .save-status span { display: inline-flex; align-items: center; gap: .4rem; }
  .save-status .success { color: var(--success); }
  .save-status .error { color: var(--danger); }
  .bp-empty { min-height: 12rem; }
  @media (max-width: 1000px) {
    .setting-row { align-items: flex-start; }
    .settings-page .settings-grid { grid-template-columns: 1fr; }
  }
  @media (min-width: 851px) and (max-width: 1380px) {
    .setting-row { gap: .8rem; }
    .switch { min-width: 5.7rem; }
    .switch-knob { width: 1.65rem; height: 1.65rem; }
  }
</style>
