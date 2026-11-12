<script lang="ts">
  import AccentPicker from './AccentPicker.svelte';
  import LayoutEditor from './LayoutEditor.svelte';
  import ThemePreview from './ThemePreview.svelte';
  import { Trash2 } from '@lucide/svelte';
  import { msg } from '../../lib/i18n';
  import { themeDisplayName, personalThemeVars, displayedAccent, fallbackAccent, fallbackBackground } from '../../lib/theme/apply';
  import { validateCss, validateTokens } from '../../lib/theme/validate';
  import TokenValueInput from './TokenValueInput.svelte';
  import AiSkillButton from '../../lib/components/AiSkillButton.svelte';
  import Button from '../../lib/components/Button.svelte';
  import Tooltip from '../../lib/components/Tooltip.svelte';
  import IconButton from '../../lib/components/IconButton.svelte';
  import Notice from '../../lib/components/Notice.svelte';
  import StatusBadge from '../../lib/components/StatusBadge.svelte';
  import Card from '../../lib/components/Card.svelte';
  import ConfirmModal from '../../lib/components/ConfirmModal.svelte';
  import ThemeTextImportModal from '../../lib/components/ThemeTextImportModal.svelte';
  import ThemeContrast from '../../lib/components/ThemeContrast.svelte';
  import { onDestroy, tick, untrack } from 'svelte';
  import { get } from 'svelte/store';
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
  import {
    activeTheme,
    endTrial,
    forgetTheme,
    refreshThemes,
    resetAppearance,
    selectTheme,
    themeList,
    themeMode,
    themeTrial,
    tryTheme,
  } from '../../lib/stores/theme';
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
  import { nextIndex } from '../../lib/utils/roving';

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
  let textImportOpen = $state(false);
  let exporting = $state(false);
  let errors = $state<string[]>([]);
  // Errors that answer Try or Save are announced, each press anew; those a kept trial updates at every
  // edit stay on screen without being announced while the user types.
  let errorsAnswer = $state(0);
  let errorsQuiet = $state(false);

  function report(found: string[]) {
    errors = found;
    errorsQuiet = false;
    errorsAnswer += 1;
  }
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

  async function pickTheme(theme: Theme) {
    startEditing(theme);
    await selectTheme(theme.id);
  }

  const uid = $props.id();

  // The system choice, then every theme: one choice, a radio group of tiles.
  const presetChecked = $derived([
    $themeMode === 'system',
    ...list.map((theme) => $themeMode === 'theme' && active?.id === theme.id),
  ]);
  const presetStop = $derived(Math.max(0, presetChecked.indexOf(true)));
  let presetGrid = $state<HTMLElement>();

  // While a choice is applied the tiles refuse presses (aria-disabled keeps focus on them).
  let choosing = $state(false);
  // The tile a confirmed choice goes to: focus follows it once the question closes.
  let chosenAfterAsking: number | null = null;
  const presetRadios = () => [...(presetGrid?.querySelectorAll<HTMLElement>('[role="radio"]') ?? [])];

  async function applyPreset(index: number) {
    choosing = true;
    try {
      if (index === 0) await selectTheme('system');
      else await pickTheme(list[index - 1]);
    } finally {
      choosing = false;
    }
  }

  // Choosing a theme replaces the draft: with unsaved edits it asks first. The system choice keeps
  // the draft. Returns whether the choice was made at once.
  function choosePreset(index: number): boolean {
    if (choosing || presetChecked[index]) return false;
    if (index > 0 && dirty && draft) {
      pending = {
        prompt: {
          title: msg('settings.themeDiscardTitle'),
          text: msg('settings.themeDiscardText', { name: draftName }),
          confirm: msg('settings.themeCustomRevert'),
          cancel: msg('settings.themeDiscardStay'),
        },
        run: async () => {
          await applyPreset(index);
          chosenAfterAsking = index;
        },
      };
      return false;
    }
    void applyPreset(index);
    return true;
  }

  async function closePending() {
    const target = chosenAfterAsking;
    chosenAfterAsking = null;
    pending = null;
    if (target === null) return;
    await tick();
    presetRadios()[target]?.focus();
  }

  function presetKeydown(event: KeyboardEvent) {
    if (event.altKey || event.ctrlKey || event.metaKey) return;
    const radios = presetRadios();
    const next = nextIndex(radios.indexOf(event.currentTarget as HTMLElement), radios.length, event.key, { orientation: 'both' });
    if (next === null) return;
    event.preventDefault();
    if (choosePreset(next)) radios[next]?.focus();
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
        JSON.stringify(draft.layout ?? null) !== JSON.stringify(stored.layout ?? null) ||
        !sameTokens(draft.tokens, stored.tokens)),
  );

  function revertDraft() {
    if (!stored) return;
    startEditing(stored);
    toast(msg('settings.themeRevertedToast', { name: themeDisplayName(stored) }), 'info');
  }

  function validateDraft(): string[] {
    if (!draft) return [];
    const found = validateTokens(draft.tokens, allowedTokenNames);
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

  const trialDraft = $derived(draft ? { ...draft, name: draftName, css: cssDraft } : null);
  const trialKept = $derived($themeTrial?.kept ?? false);

  function tryDraft() {
    if (!trialDraft) return;
    const error = tryTheme(trialDraft);
    report(error ? [error] : []);
  }

  // A kept trial follows every edit, so the window shows the draft as it is
  // typed; an edit the CSS check refuses leaves the last accepted one shown.
  $effect(() => {
    if (!trialKept || !trialDraft) return;
    JSON.stringify(trialDraft);
    const next = trialDraft;
    untrack(() => {
      const error = tryTheme(next);
      errors = error ? [error] : [];
      errorsQuiet = true;
    });
  });

  onDestroy(() => {
    if (get(themeTrial)) endTrial();
  });

  async function saveDraft() {
    if (!draft) return;
    const previous = { id: draft.id, builtIn: draft.builtIn };
    const validation = validateDraft();
    report(validation);
    if (validation.length > 0) return;
    saving = true;
    try {
      const saved = await saveTheme({ ...draft, name: draftName, css: cssDraft });
      startEditing(saved);
      if (saved.id !== previous.id || previous.builtIn) await selectTheme(saved.id);
      toast(msg('settings.appearanceSavedToast', { name: saved.name }), 'success');
      await refreshThemes();
    } catch (err) {
      toast(themeErrorText(err), 'danger');
    } finally {
      saving = false;
    }
  }

  function askRemove(target: Theme) {
    if (target.builtIn) return;
    pending = { prompt: deleteThemePrompt(target.name), run: () => runRemove(target) };
  }

  function removeDraft() {
    if (draft) askRemove(draft);
  }

  async function runRemove(target: Theme) {
    deleting = true;
    try {
      await deleteTheme(target.id);
      toast(msg('settings.appearanceDeletedToast', { name: target.name }), 'success');
      await refreshThemes();
      await forgetTheme(target.id);
      if (draft?.id === target.id) draft = null;
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

  async function onTextImported(theme: Theme) {
    toast(msg('settings.appearanceImportedToast', { name: theme.name }), 'success');
    await refreshThemes();
    startEditing(theme);
  }

  async function runExport() {
    if (!draft || dirty) return;
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
      <div class="preset-grid" role="radiogroup" aria-label={msg('settings.appearanceTab')} bind:this={presetGrid}>
        <button
          type="button"
          role="radio"
          class="preset"
          class:selected={presetChecked[0]}
          aria-checked={presetChecked[0]}
          aria-disabled={choosing || undefined}
          tabindex={presetStop === 0 ? 0 : -1}
          onclick={() => choosePreset(0)}
          onkeydown={presetKeydown}
        >
          <span class="preset-swatch system" style={`background: linear-gradient(135deg, ${fallbackBackground.dark} 50%, ${fallbackBackground.light} 50%);`}></span>
          <span class="preset-name">{msg('settings.appearanceSystemPresetLabel')}</span>
        </button>
        {#each list as theme, i (theme.id)}
          <div class="preset-cell">
            <button
              type="button"
              role="radio"
              class="preset"
              class:selected={presetChecked[i + 1]}
              aria-checked={presetChecked[i + 1]}
              aria-disabled={choosing || undefined}
              tabindex={presetStop === i + 1 ? 0 : -1}
              onclick={() => choosePreset(i + 1)}
              onkeydown={presetKeydown}
            >
              <span
                class="preset-swatch"
                style={`background: ${theme.tokens['--bg'] ?? (theme.base === 'light' ? fallbackBackground.light : fallbackBackground.dark)}; border-color: ${theme.tokens['--accent'] ?? fallbackAccent};`}
              ></span>
              <span class="preset-name">{themeDisplayName(theme)}</span>
              {#if !theme.builtIn}
                <span class="preset-tag"><StatusBadge kind="accent" dot={false} label={msg('settings.appearanceCustomThemeTag')} /></span>
              {/if}
            </button>
            {#if !theme.builtIn}
              <span class="preset-delete">
                <IconButton
                  size="sm"
                  label={msg('settings.appearanceDeleteTile', { name: themeDisplayName(theme) })}
                  disabled={deleting}
                  onclick={() => askRemove(theme)}
                >
                  <Trash2 size={14} />
                </IconButton>
              </span>
            {/if}
          </div>
        {/each}
      </div>
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.appearanceImportLabel')}</span>
          <span class="row-sub">{msg('settings.appearanceImportSub')}</span>
        </div>
        <div class="import-actions">
          <Button size="sm" disabled={importing} onclick={() => (textImportOpen = true)}>
            {msg('settings.appearanceImportTextButton')}
          </Button>
          <Button size="sm" disabled={importing} onclick={runImport}>
            {importing ? msg('settings.appearanceImportingEllipsis') : msg('settings.appearanceImportButton')}
          </Button>
        </div>
      </div>
      <div class="row">
        <div class="row-text">
          <span class="row-label">{msg('settings.aiSkillLabel')}</span>
          <span class="row-sub">{msg('settings.aiSkillSub')}</span>
        </div>
        <AiSkillButton label={msg('settings.aiSkillButton')} />
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
        {#if stored?.cssBlocked}
          <div class="blocked">
            <Notice tone="warning" compact announce={false}>{msg('settings.themeCssBlocked')}</Notice>
          </div>
        {/if}
        <div class="editor-layout">
          <div class="editor-preview">
            <Card surface="panel" title={msg('settings.themePreviewTitle')}>
              <ThemePreview vars={previewStyle} />
            </Card>
            <ThemeContrast vars={previewVars} />
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
                          <label class="token-label" for={fieldId}>{label}</label>
                          {#if known}
                            <span class="token-hint" id={`${fieldId}-hint`}>{msg(tokenHintKey(name))}</span>
                          {/if}
                        </div>
                        <div class="token-controls">
                          {#if section.colors}
                            {#if isHexColor(value)}
                              <Tooltip text={name}>
                                <input
                                  class="color-input"
                                  type="color"
                                  aria-label={label}
                                  {value}
                                  oninput={(e) => setToken(name, (e.currentTarget as HTMLInputElement).value)}
                                />
                              </Tooltip>
                            {:else}
                              <span class="swatch" style:background={value} aria-hidden="true"></span>
                            {/if}
                          {/if}
                          <TokenValueInput
                            id={fieldId}
                            {name}
                            {value}
                            describedBy={known ? `${fieldId}-hint` : undefined}
                            onchange={(next) => setToken(name, next)}
                          />
                        </div>
                      </div>
                    {/each}
                  </div>
                </details>
              {/each}
            </div>

            <LayoutEditor bind:layout={draft.layout} />

            <div class="advanced">
              <Button
                size="sm"
                variant="ghost"
                aria-expanded={advancedOpen}
                aria-controls="{uid}-advanced"
                onclick={() => (advancedOpen = !advancedOpen)}
              >
                {advancedOpen ? msg('settings.appearanceHideAdvanced') : msg('settings.appearanceShowAdvanced')}
              </Button>
            </div>
            <div id="{uid}-advanced" class="advanced-body">
              {#if advancedOpen}
                <label class="field">
                  <span class="row-label">{msg('settings.appearanceCustomCssLabel')}</span>
                  <textarea class="textarea" rows="8" bind:value={cssDraft}></textarea>
                </label>
              {/if}
            </div>

            {#snippet errorList()}
              <ul class="errors">
                {#each errors as error}
                  <li>{error}</li>
                {/each}
              </ul>
            {/snippet}
            {#if errors.length > 0 && errorsQuiet}
              <Notice tone="danger" compact announce={false}>{@render errorList()}</Notice>
            {:else if errors.length > 0}
              {#key errorsAnswer}
                <Notice tone="danger" compact>{@render errorList()}</Notice>
              {/key}
            {/if}

            <div class="editor-actions">
              <Button size="sm" disabled={saving} onclick={saveDraft}>
                {saving ? msg('settings.appearanceSavingEllipsis') : msg('common.save')}
              </Button>
              <Button size="sm" variant="secondary" disabled={exporting || dirty} onclick={runExport}>
                {exporting ? msg('settings.appearanceExportingEllipsis') : msg('settings.appearanceExportButton')}
              </Button>
              <Button size="sm" variant="secondary" onclick={tryDraft}>
                {msg('settings.themeTry')}
              </Button>
              {#if dirty}
                <span class="hint export-hint">{msg('settings.appearanceExportDirtyHint')}</span>
              {/if}
              {#if !draft.builtIn}
                <Button size="sm" variant="danger" disabled={deleting} onclick={removeDraft}>
                  {deleting ? msg('settings.appearanceDeletingEllipsis') : msg('common.delete')}
                </Button>
              {/if}
            </div>
            <p class="hint">{msg('settings.themeTryHint')}</p>
          </div>
        </div>
      </Card>
    {:else}
      {@render resetCard()}
    {/if}
  </div>
</div>

<ThemeTextImportModal bind:open={textImportOpen} onimported={onTextImported} />

{#if pending}
  <ConfirmModal prompt={pending.prompt} onconfirm={pending.run} onclose={closePending} errorText={(err) => themeErrorText(err)} />
{/if}

<style>
  .blocked {
    margin-bottom: var(--space-3);
  }
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

  .preset-cell {
    position: relative;
    display: flex;
  }

  .preset-cell > .preset {
    flex: 1 1 auto;
    min-width: 0;
    padding-right: 3.4rem;
  }

  /* The delete button shows on the tile's hover and on keyboard focus. */
  .preset-delete {
    position: absolute;
    top: 50%;
    right: var(--space-2);
    transform: translateY(-50%);
    display: inline-flex;
    opacity: 0;
    transition: opacity var(--dur) var(--ease);
  }

  .preset-cell:hover .preset-delete,
  .preset-delete:focus-within {
    opacity: 1;
  }

  @media (hover: none) {
    .preset-delete {
      opacity: 1;
    }
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

  .preset[aria-disabled='true'] {
    cursor: progress;
  }

  .preset.selected {
    border-color: var(--accent-bright);
    background: var(--accent-subtle);
  }

  .preset-swatch {
    width: 2.4rem;
    height: 2.4rem;
    border-radius: var(--radius-sm);
    border: 2px solid var(--border-strong);
    flex-shrink: 0;
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
    display: inline-flex;
    margin-left: auto;
    flex-shrink: 0;
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-6);
    padding: 1.3rem 0;
  }

  .import-actions {
    display: flex;
    gap: var(--space-3);
    flex-shrink: 0;
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


  .advanced {
    align-self: flex-start;
  }

  .advanced-body:empty {
    display: none;
  }

  .textarea {
    width: 100%;
    min-height: 14rem;
    padding: var(--space-2);
    background: var(--surface-2);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    font-size: var(--font-xs);
    font-family: var(--font-mono);
    font-variant-ligatures: none;
    color: var(--text);
    resize: vertical;
  }

  .errors {
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: 0;
  }

  .export-hint {
    margin-bottom: 0;
    align-self: center;
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
