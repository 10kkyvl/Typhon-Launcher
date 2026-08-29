<script lang="ts">
  import { themeVars } from '../../lib/theme/apply';
  import { validateCss, validateTokenName, validateTokenValue } from '../../lib/theme/validate';
  import Button from '../../lib/components/Button.svelte';
  import Card from '../../lib/components/Card.svelte';
  import {
    deleteTheme,
    exportTheme,
    importTheme,
    saveTheme,
    selectExportPath,
    selectThemeFile,
    type Theme,
  } from '../../lib/services/theme';
  import { activeTheme, refreshThemes, resetAppearance, selectTheme, themeList, themeMode } from '../../lib/stores/theme';
  import { toast } from '../../lib/stores/toasts';
  import { errorMessage } from '../../lib/utils/errors';

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

  function startEditing(theme: Theme) {
    draft = { ...theme, tokens: { ...theme.tokens } };
    draftName = theme.name;
    cssDraft = theme.css ?? '';
    advancedOpen = false;
    errors = [];
  }

  function pickTheme(theme: Theme) {
    selectTheme(theme.id);
    startEditing(theme);
  }

  function setToken(name: string, value: string) {
    if (!draft) return;
    draft = { ...draft, tokens: { ...draft.tokens, [name]: value } };
  }

  function isColorValue(value: string) {
    return /^#[0-9a-fA-F]{3,8}$/.test(value.trim());
  }

  function validateDraft(): string[] {
    if (!draft) return [];
    const found: string[] = [];
    for (const [name, value] of Object.entries(draft.tokens)) {
      const nameError = validateTokenName(name, allowedTokenNames);
      if (nameError) found.push(nameError);
      const valueError = validateTokenValue(value);
      if (valueError) found.push(`«${name}»: ${valueError}`);
    }
    const cssError = validateCss(cssDraft);
    if (cssError) found.push(cssError);
    return found;
  }

  const previewSource = $derived(draft ?? active);
  const previewVars = $derived(previewSource ? themeVars(previewSource) : {});
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
      toast(`Тема «${saved.name}» сохранена`, 'success');
      await refreshThemes();
    } catch (err) {
      toast(errorMessage(err), 'danger');
    } finally {
      saving = false;
    }
  }

  async function removeDraft() {
    if (!draft || draft.builtIn) return;
    if (!window.confirm(`Удалить тему «${draft.name}»?`)) return;
    deleting = true;
    try {
      await deleteTheme(draft.id);
      toast(`Тема «${draft.name}» удалена`, 'success');
      draft = null;
      await refreshThemes();
    } catch (err) {
      toast(errorMessage(err), 'danger');
    } finally {
      deleting = false;
    }
  }

  async function runImport() {
    importing = true;
    try {
      const path = await selectThemeFile();
      if (!path) return;
      const theme = await importTheme(path);
      toast(`Тема «${theme.name}» импортирована`, 'success');
      await refreshThemes();
      startEditing(theme);
    } catch (err) {
      toast(errorMessage(err), 'danger');
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
      toast(`Тема «${draft.name}» экспортирована`, 'success');
    } catch (err) {
      toast(errorMessage(err), 'danger');
    } finally {
      exporting = false;
    }
  }
</script>

<div class="single-column">
  <section class="group">
    <h3>Оформление</h3>
    <div class="preset-grid">
      <button type="button" class="preset" class:selected={$themeMode === 'system'} onclick={() => selectTheme('system')}>
        <span class="preset-swatch system"></span>
        <span class="preset-name">Системная</span>
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
          <span class="preset-name">{theme.name}</span>
          {#if !theme.builtIn}
            <span class="preset-tag">своя</span>
          {/if}
        </button>
      {/each}
    </div>
    <div class="row">
      <div class="row-text">
        <span class="row-label">Импорт темы</span>
        <span class="row-sub">Загрузить файл темы с диска</span>
      </div>
      <Button size="sm" disabled={importing} onclick={runImport}>
        {importing ? 'Импорт…' : 'Импорт'}
      </Button>
    </div>
  </section>

  {#if draft}
    <section class="group">
      <h3>Редактирование: {draft.name}</h3>
      {#if draft.builtIn}
        <p class="hint">Это встроенная тема — изменения сохранятся как новая тема.</p>
      {/if}
      <div class="editor-layout">
        <div class="editor-fields">
          <label class="field">
            <span class="row-label">Название</span>
            <input class="input" type="text" bind:value={draftName} />
          </label>

          <div class="token-rows">
            {#each Object.keys(draft.tokens) as name (name)}
              <div class="token-row">
                <span class="token-name">{name}</span>
                {#if isColorValue(draft.tokens[name] ?? '')}
                  <input
                    class="color-input"
                    type="color"
                    value={draft.tokens[name]}
                    oninput={(e) => setToken(name, (e.currentTarget as HTMLInputElement).value)}
                  />
                {/if}
                <input
                  class="input sm token-value"
                  type="text"
                  value={draft.tokens[name] ?? ''}
                  oninput={(e) => setToken(name, (e.currentTarget as HTMLInputElement).value)}
                />
              </div>
            {/each}
          </div>

          <button type="button" class="advanced-toggle" onclick={() => (advancedOpen = !advancedOpen)}>
            {advancedOpen ? 'Скрыть дополнительно' : 'Дополнительно'}
          </button>
          {#if advancedOpen}
            <label class="field">
              <span class="row-label">Пользовательский CSS</span>
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
              {saving ? 'Сохранение…' : 'Сохранить'}
            </Button>
            <Button size="sm" variant="secondary" disabled={exporting} onclick={runExport}>
              {exporting ? 'Экспорт…' : 'Экспорт'}
            </Button>
            {#if !draft.builtIn}
              <Button size="sm" variant="danger" disabled={deleting} onclick={removeDraft}>
                {deleting ? 'Удаление…' : 'Удалить'}
              </Button>
            {/if}
          </div>
        </div>

        <div class="editor-preview">
          <Card>
            <div class="preview" style={previewStyle}>
              <span class="preview-title">Typhon</span>
              <span class="preview-sub">Живое превью темы</span>
              <button type="button" class="preview-btn">Кнопка</button>
            </div>
          </Card>
        </div>
      </div>
    </section>
  {/if}

  <section class="group">
    <h3>Сброс</h3>
    <div class="row">
      <div class="row-text">
        <span class="row-label">Вернуть оформление по умолчанию</span>
        <span class="row-sub">Отменяет пользовательские темы и возвращает встроенную тёмную тему. Также доступно по Ctrl+Shift+Alt+T</span>
      </div>
      <Button size="sm" variant="danger" onclick={resetAppearance}>Сбросить</Button>
    </div>
  </section>
</div>

<style>
  .single-column {
    max-width: 96rem;
  }

  .group {
    margin-bottom: var(--space-10);
  }

  .group h3 {
    font-size: var(--font-xl);
    font-weight: 600;
    letter-spacing: var(--tracking-heading);
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
    grid-template-columns: 1fr;
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

  .token-rows {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .token-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .token-name {
    font-size: var(--font-xs);
    color: var(--text-2);
    font-family: monospace;
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
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
    flex: 1 1 18rem;
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
    gap: var(--space-2);
  }

  .editor-preview {
    min-width: 0;
  }

  .preview {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-3);
    padding: var(--space-4);
    background: var(--bg);
    border-radius: var(--radius-md);
    color: var(--text);
  }

  .preview-title {
    font-size: var(--font-lg);
    font-weight: 600;
    color: var(--text);
  }

  .preview-sub {
    font-size: var(--font-sm);
    color: var(--text-2);
  }

  .preview-btn {
    padding: 0 1.5rem;
    height: var(--control-md);
    background: var(--accent);
    color: #fff;
    border-radius: var(--radius-md);
    font-size: var(--font-sm);
    font-weight: 500;
  }

  @media (min-width: 1200px) {
    .editor-layout {
      grid-template-columns: 1fr 32rem;
    }
  }
</style>
