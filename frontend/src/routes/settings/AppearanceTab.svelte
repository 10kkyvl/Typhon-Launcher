<script lang="ts">
  import AccentPicker from './AccentPicker.svelte';
  import ThemePreview from './ThemePreview.svelte';
  import { msg } from '../../lib/i18n';
  import { themeDisplayName, personalThemeVars, displayedAccent } from '../../lib/theme/apply';
  import { validateCss, validateTokenName, validateTokenValue } from '../../lib/theme/validate';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import {
    deleteTheme,
    exportTheme,
    importTheme,
    saveTheme,
    selectExportPath,
    selectThemeFile,
    type Theme,
  } from '../../lib/services/theme';
  import { deleteThemePrompt, resetAppearancePrompt, type ConfirmPrompt } from '../../lib/confirm/prompts';
  import { activeTheme, refreshThemes, resetAppearance, selectTheme, themeList, themeMode } from '../../lib/stores/theme';
  import { toast } from '../../lib/stores/toasts';
  import { themeErrorText } from '../../lib/theme/themeErrors';
  import {
    groupedTokenNames,
    groupTitleKey,
    isHexColor,
    tokenGroups,
    tokenHintKey,
    tokenKey,
    tokenLabelKey,
    type TokenGroupId,
  } from '../../lib/theme/tokens';

  const list = $derived($themeList);
  const active = $derived($activeTheme);
  const allowedTokenNames = $derived(
    new Set(list.flatMap((t) => Object.keys(t.tokens)).filter((name) => name !== '--ui-scale')),
  );

  let draft = $state<Theme | null>(null);
  let draftName = $state('');
  let cssDraft = $state('');
  let advancedOpen = $state(false);
  let saving = $state(false);
  let deleting = $state(false);
  let importing = $state(false);
  let exporting = $state(false);
  let errors = $state<string[]>([]);
  let pending = $state<{ prompt: ConfirmPrompt; run: () => Promise<void> } | null>(null);

  function startEditing(theme: Theme) {
    draft = { ...theme, tokens: { ...theme.tokens } };
    draftName = themeDisplayName(theme);
    cssDraft = theme.css ?? '';
    advancedOpen = false;
    errors = [];
  }

  $effect(() => {
    if (draft !== null || $themeMode !== 'theme' || !active) return;
    startEditing(active);
  });

  function pickTheme(theme: Theme) {
    selectTheme(theme.id);
    startEditing(theme);
  }

  function setToken(name: string, value: string) {
    if (!draft) return;
    draft = { ...draft, tokens: { ...draft.tokens, [name]: value } };
  }

  interface TokenSection {
    id: TokenGroupId | 'other';
    colors: boolean;
    collapsed: boolean;
    names: string[];
  }

  const sections = $derived.by<TokenSection[]>(() => {
    if (!draft) return [];
    const present = new Set(Object.keys(draft.tokens));
    const found: TokenSection[] = tokenGroups.map((group) => ({
      id: group.id,
      colors: group.colors,
      collapsed: group.collapsed,
      names: group.tokens.filter((name) => present.has(name)),
    }));
    found.push({
      id: 'other',
      colors: false,
      collapsed: false,
      names: [...present].filter((name) => !groupedTokenNames.has(name)),
    });
    return found.filter((section) => section.names.length > 0);
  });

  const stored = $derived(draft ? (list.find((t) => t.id === draft!.id) ?? null) : null);

  function sameTokens(a: Record<string, string>, b: Record<string, string>) {
    const left = Object.keys(a);
    return left.length === Object.keys(b).length && left.every((name) => a[name] === b[name]);
  }

  const dirty = $derived(
    draft !== null &&
      stored !== null &&
      (draftName !== themeDisplayName(stored) ||
        cssDraft !== (stored.css ?? '') ||
        !sameTokens(draft.tokens, stored.tokens)),
  );

  function revertDraft() {
    if (!stored) return;
    startEditing(stored);
    toast(msg('settings.themeRevertedToast', { name: themeDisplayName(stored) }), 'info');
  }

  function validateDraft(): string[] {
    if (!draft) return [];
    const found: string[] = [];
    for (const [name, value] of Object.entries(draft.tokens)) {
      const nameError = validateTokenName(name, allowedTokenNames);
      if (nameError) found.push(nameError);
      const valueError = validateTokenValue(value);
      if (valueError) found.push(msg('settings.appearanceTokenValueErrorWrap', { name, error: valueError }));
    }
    const cssError = validateCss(cssDraft);
    if (cssError) found.push(cssError);
    return found;
  }

  const previewSource = $derived(draft ?? active);
  const previewVars = $derived.by(() => {
    $displayedAccent;
    return previewSource ? personalThemeVars(previewSource) : {};
  });
  const previewStyle = $derived(
    Object.entries(previewVars)
      .map(([name, value]) => `${name}: ${value}`)
      .join('; '),
  );

  async function saveDraft() {
    if (!draft) return;
    const validation = validateDraft();
    errors = validation;
    if (validation.length > 0) return;
    saving = true;
    try {
      const saved = await saveTheme({ ...draft, name: draftName, css: cssDraft });
      startEditing(saved);
      toast(msg('settings.appearanceSavedToast', { name: saved.name }), 'success');
      await refreshThemes();
    } catch (err) {
      toast(themeErrorText(err), 'danger');
    } finally {
      saving = false;
    }
  }

  function removeDraft() {
    const target = draft;
    if (!target || target.builtIn) return;
    pending = { prompt: deleteThemePrompt(target.name), run: () => runRemove(target) };
  }

  async function runRemove(target: Theme) {
    deleting = true;
    try {
      await deleteTheme(target.id);
      toast(msg('settings.appearanceDeletedToast', { name: target.name }), 'success');
      draft = null;
      await refreshThemes();
    } catch (err) {
      toast(themeErrorText(err), 'danger');
    } finally {
      deleting = false;
    }
  }

  function askReset() {
    pending = { prompt: resetAppearancePrompt(), run: resetAppearance };
  }

  async function runImport() {
    importing = true;
    try {
      const path = await selectThemeFile();
      if (!path) return;
      const theme = await importTheme(path);
      toast(msg('settings.appearanceImportedToast', { name: theme.name }), 'success');
      await refreshThemes();
      startEditing(theme);
    } catch (err) {
      toast(themeErrorText(err), 'danger');
    } finally {
      importing = false;
    }
  }

  async function runExport() {
    if (!draft) return;
    exporting = true;
    try {
      const path = await selectExportPath();
      if (!path) return;
      await exportTheme(draft.id, path);
      toast(msg('settings.appearanceExportedToast', { name: draft.name }), 'success');
    } catch (err) {
      toast(themeErrorText(err), 'danger');
    } finally {
      exporting = false;
    }
  }
</script>

{#snippet resetCard()}
  <Card title={msg('settings.appearanceResetCardTitle')}>
    <div class="row">
      <div class="row-text">
        <span class="row-label">{msg('settings.appearanceResetLabel')}</span>
        <span class="row-sub">{msg('settings.appearanceResetSub')}</span>
      </div>
      <Button size="sm" variant="danger" onclick={askReset}>{msg('settings.appearanceResetButton')}</Button>
    </div>
  </Card>
{/snippet}

<div class="settings-grid">
  <div class="settings-column">
    <AccentPicker />
    <Card title={msg('settings.appearanceTab')}>
      <div class="preset-grid">
        <button type="button" class="preset" class:selected={$themeMode === 'system'} onclick={() => selectTheme('system')}>
          <span class="preset-swatch system"></span>
          <span class="preset-name">{msg('settings.appearanceSystemPresetLabel')}</span>
        </button>
        {#each list as theme (theme.id)}
          <button
            type="button"
            class="preset"
            class:selected={$themeMode === 'theme' && active?.id === theme.id}
            onclick={() => pickTheme(theme)}
          >
            <span
              class="preset-swatch"
              style={`background: ${theme.tokens['--bg'] ?? (theme.base === 'light' ? '#f4f6f8' : '#0b0f14')}; border-color: ${theme.tokens['--accent'] ?? '#6875e8'};`}
            ></span>
            <span class="preset-name">{themeDisplayName(theme)}</span>
            {#if !theme.builtIn}
              <span class="preset-tag">{msg('settings.appearanceCustomThemeTag')}</span>
            {/if}
          </button>
        {/each}
      </div>
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.appearanceImportLabel')}</span>
          <span class="row-sub">{msg('settings.appearanceImportSub')}</span>
        </div>
        <Button size="sm" disabled={importing} onclick={runImport}>
          {importing ? msg('settings.appearanceImportingEllipsis') : msg('settings.appearanceImportButton')}
        </Button>
      </div>
      {#if draft}
        <div class="row">
          <div class="row-text">
            <span class="row-label">
              {draft.builtIn ? msg('settings.themeUnsavedRevert') : msg('settings.themeCustomRevert')}
            </span>
            <span class="row-sub">
              {draft.builtIn ? msg('settings.themeUnsavedRevertSub') : msg('settings.themeCustomRevertSub')}
            </span>
          </div>
          <Button size="sm" variant="secondary" disabled={!dirty} onclick={revertDraft}>
            {msg('settings.themeRevertButton')}
          </Button>
        </div>
      {/if}
    </Card>

    {#if draft}
      {@render resetCard()}
    {/if}
  </div>
  <div class="settings-column">
    {#if draft}
      <Card title={msg('settings.appearanceEditingCardTitle', { name: themeDisplayName(draft) })}>
        {#if draft.builtIn}
          <p class="hint">{msg('settings.appearanceBuiltinHint')}</p>
        {/if}
        <div class="editor-layout">
          <div class="editor-preview">
            <Card surface="panel" title={msg('settings.themePreviewTitle')}>
              <ThemePreview vars={previewStyle} />
            </Card>
          </div>

          <div class="editor-fields">
            <label class="field">
              <span class="row-label">{msg('settings.appearanceNameFieldLabel')}</span>
              <input class="input" type="text" bind:value={draftName} />
            </label>

            <p class="hint">{msg('settings.themeTokensIntro')}</p>
            <div class="token-groups">
              {#each sections as section (section.id)}
                <details class="token-group" open={!section.collapsed}>
                  <summary>{msg(groupTitleKey(section.id))}</summary>
                  <div class="token-rows">
                    {#each section.names as name (name)}
                      {@const value = draft.tokens[name] ?? ''}
                      {@const known = groupedTokenNames.has(name)}
                      {@const label = known ? msg(tokenLabelKey(name)) : name}
                      {@const fieldId = `theme-token-${tokenKey(name)}`}
                      <div class="token-row">
                        <div class="token-text">
                          <label class="token-label" for={fieldId} title={name}>{label}</label>
                          {#if known}
                            <span class="token-hint" id={`${fieldId}-hint`}>{msg(tokenHintKey(name))}</span>
                          {/if}
                        </div>
                        <div class="token-controls">
                          {#if section.colors}
                            {#if isHexColor(value)}
                              <input
                                class="color-input"
                                type="color"
                                aria-label={label}
                                title={name}
                                {value}
                                oninput={(e) => setToken(name, (e.currentTarget as HTMLInputElement).value)}
                              />
                            {:else}
                              <span class="swatch" style:background={value} aria-hidden="true"></span>
                            {/if}
                          {/if}
                          <input
                            id={fieldId}
                            class="input sm token-value"
                            type="text"
                            title={name}
                            aria-describedby={known ? `${fieldId}-hint` : undefined}
                            {value}
                            oninput={(e) => setToken(name, (e.currentTarget as HTMLInputElement).value)}
                          />
                        </div>
                      </div>
                    {/each}
                  </div>
                </details>
              {/each}
            </div>

            <button type="button" class="advanced-toggle" onclick={() => (advancedOpen = !advancedOpen)}>
              {advancedOpen ? msg('settings.appearanceHideAdvanced') : msg('settings.appearanceShowAdvanced')}
            </button>
            {#if advancedOpen}
              <label class="field">
                <span class="row-label">{msg('settings.appearanceCustomCssLabel')}</span>
                <textarea class="textarea" rows="8" bind:value={cssDraft}></textarea>
              </label>
            {/if}

            {#if errors.length > 0}
              <ul class="errors">
                {#each errors as error}
                  <li>{error}</li>
                {/each}
              </ul>
            {/if}

            <div class="editor-actions">
              <Button size="sm" disabled={saving} onclick={saveDraft}>
                {saving ? msg('settings.appearanceSavingEllipsis') : msg('common.save')}
              </Button>
              <Button size="sm" variant="secondary" disabled={exporting} onclick={runExport}>
                {exporting ? msg('settings.appearanceExportingEllipsis') : msg('settings.appearanceExportButton')}
              </Button>
              {#if !draft.builtIn}
                <Button size="sm" variant="danger" disabled={deleting} onclick={removeDraft}>
                  {deleting ? msg('settings.appearanceDeletingEllipsis') : msg('common.delete')}
                </Button>
              {/if}
            </div>
          </div>
        </div>
      </Card>
    {:else}
      {@render resetCard()}
    {/if}
  </div>
</div>

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={() => (pending = null)} />
{/if}

<style>
  .hint {
    font-size: var(--font-xs);
    color: var(--text-3);
    margin-bottom: var(--space-3);
  }

  .preset-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .preset {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-3);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    text-align: left;
    transition:
      border-color var(--dur) var(--ease),
      background var(--dur) var(--ease);
  }

  .preset:hover {
    background: var(--surface-3);
  }

  .preset.selected {
    border-color: var(--accent);
    background: var(--accent-subtle);
  }

  .preset-swatch {
    width: 2.4rem;
    height: 2.4rem;
    border-radius: var(--radius-sm);
    border: 2px solid var(--border-strong);
    flex-shrink: 0;
  }

  .preset-swatch.system {
    background: linear-gradient(135deg, #0b0f14 50%, #f4f6f8 50%);
  }

  .preset-name {
    font-size: var(--font-sm);
    font-weight: 500;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .preset-tag {
    margin-left: auto;
    font-size: var(--font-xs);
    color: var(--accent-text);
    flex-shrink: 0;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
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

  .editor-layout {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-6);
    align-items: start;
  }

  .editor-fields {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    min-width: 0;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .token-groups {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .token-group {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
  }

  .token-group > summary {
    padding: var(--space-3) var(--space-4);
    font-size: var(--font-sm);
    font-weight: 600;
    cursor: pointer;
  }

  .token-group > summary:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .token-rows {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: 0 var(--space-4) var(--space-4);
  }

  .token-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-2) var(--space-4);
  }

  .token-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex: 1 1 16rem;
    min-width: 0;
  }

  .token-label {
    font-size: var(--font-sm);
    font-weight: 500;
    color: var(--text);
  }

  .token-hint {
    font-size: var(--font-xs);
    color: var(--text-3);
  }

  .token-controls {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 1 22rem;
    min-width: 0;
  }

  .swatch {
    width: 3.4rem;
    height: var(--control-sm);
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-sm);
    flex-shrink: 0;
  }

  .color-input {
    width: 3.4rem;
    height: var(--control-sm);
    padding: 0.2rem;
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    flex-shrink: 0;
  }

  .token-value {
    flex: 1 1 auto;
    min-width: 0;
  }

  .advanced-toggle {
    align-self: flex-start;
    font-size: var(--font-xs);
    font-weight: 500;
    color: var(--accent-text);
    padding: 0.4rem 0;
  }

  .advanced-toggle:hover {
    color: var(--text);
  }

  .textarea {
    width: 100%;
    min-height: 14rem;
    padding: var(--space-2);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: var(--font-xs);
    font-family: monospace;
    color: var(--text);
    resize: vertical;
  }

  .errors {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-3);
    background: var(--danger-subtle);
    border-radius: var(--radius-md);
    color: var(--danger);
    font-size: var(--font-xs);
  }

  .editor-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .editor-preview {
    min-width: 0;
  }
</style>
