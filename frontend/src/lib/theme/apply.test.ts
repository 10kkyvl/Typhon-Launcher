import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Theme } from '../../../bindings/typhon/internal/theme';

function baseTheme(overrides: Partial<Theme> = {}): Theme {
  return {
    id: 'dark',
    name: 'Тёмная',
    base: 'dark',
    tokens: { '--bg': '#0b0f14', '--accent': '#6875e8' },
    css: '',
    builtIn: true,
    updatedAt: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

// Minimal fake `document`: this project has no jsdom/happy-dom installed, so
// tests that touch document.documentElement build just enough of the DOM API
// surface that apply.ts actually calls (style.setProperty/removeProperty,
// getElementById, createElement, head.appendChild) as a plain object.
function createFakeDocument() {
  const rootProps = new Map<string, string>();
  const elements = new Map<string, { id: string; textContent: string }>();

  const documentElement = {
    style: {
      setProperty: (name: string, value: string) => rootProps.set(name, value),
      removeProperty: (name: string) => rootProps.delete(name),
    },
  };

  const head = {
    appendChild: (el: { id: string; textContent: string }) => {
      elements.set(el.id, el);
    },
  };

  const fakeDocument = {
    documentElement,
    head,
    getElementById: (id: string) => elements.get(id) ?? null,
    createElement: () => ({ id: '', textContent: '' }),
  };

  return { fakeDocument, rootProps, elements };
}

beforeEach(() => {
  vi.resetModules();
});

describe('themeVars', () => {
  it('excludes --ui-scale even when the backend sends it', async () => {
    const { themeVars } = await import('./apply');
    const vars = themeVars(baseTheme({ tokens: { '--bg': '#111', '--ui-scale': '1.25' } }));
    expect(vars).toEqual({ '--bg': '#111' });
  });

  it('defends against a null tokens map', async () => {
    const { themeVars } = await import('./apply');
    const vars = themeVars(baseTheme({ tokens: null }));
    expect(vars).toEqual({});
  });

  it('drops keys whose value is undefined', async () => {
    const { themeVars } = await import('./apply');
    const vars = themeVars(baseTheme({ tokens: { '--bg': '#111', '--text': undefined } }));
    expect(vars).toEqual({ '--bg': '#111' });
  });

  it('does not mutate the theme it was given', async () => {
    const { themeVars } = await import('./apply');
    const theme = baseTheme();
    const tokensBefore = { ...theme.tokens };
    themeVars(theme);
    expect(theme.tokens).toEqual(tokensBefore);
  });
});

describe('themeDisplayName', () => {
  it('shows a built-in theme label in the interface language, not the stored Russian name', async () => {
    const { locale } = await import('../i18n');
    locale.set('en');
    const { themeDisplayName } = await import('./apply');
    expect(themeDisplayName(baseTheme({ id: 'dark', name: 'Тёмная', builtIn: true }))).toBe('Dark');
  });

  it('shows the russian label for the same built-in theme in the russian locale', async () => {
    const { locale } = await import('../i18n');
    locale.set('ru');
    const { themeDisplayName } = await import('./apply');
    expect(themeDisplayName(baseTheme({ id: 'dark', name: 'Тёмная', builtIn: true }))).toBe('Тёмная');
  });

  it('translates every built-in preset id, not just dark', async () => {
    const { locale } = await import('../i18n');
    locale.set('en');
    const { themeDisplayName } = await import('./apply');
    expect(themeDisplayName(baseTheme({ id: 'light', name: 'Светлая', builtIn: true }))).toBe('Light');
    expect(themeDisplayName(baseTheme({ id: 'contrast', name: 'Высокий контраст', builtIn: true }))).toBe(
      'High contrast',
    );
  });

  it('leaves an imported theme name untouched in any locale, even one shaped like a preset id', async () => {
    const { locale } = await import('../i18n');
    locale.set('en');
    const { themeDisplayName } = await import('./apply');
    expect(themeDisplayName(baseTheme({ id: 'my-import', name: 'Моя тема', builtIn: false }))).toBe('Моя тема');
    // builtIn: false must win over an id that happens to match a preset —
    // the check has to gate on builtIn first, not look up the id blindly.
    expect(themeDisplayName(baseTheme({ id: 'dark', name: 'Моя тёмная', builtIn: false }))).toBe('Моя тёмная');
  });
});

describe('applyTheme / clearTheme', () => {
  it('creates the style element once and reuses it on repeated applies', async () => {
    const { fakeDocument, elements } = createFakeDocument();
    vi.stubGlobal('document', fakeDocument);

    const { applyTheme } = await import('./apply');
    applyTheme(baseTheme({ css: '.a { color: red; }' }));
    applyTheme(baseTheme({ css: '.b { color: blue; }' }));

    expect(elements.size).toBe(1);
    expect(elements.get('typhon-theme')?.textContent).toBe('.b { color: blue; }');

    vi.unstubAllGlobals();
  });

  it('sets custom-property values on the root element', async () => {
    const { fakeDocument, rootProps } = createFakeDocument();
    vi.stubGlobal('document', fakeDocument);

    const { applyTheme } = await import('./apply');
    applyTheme(baseTheme());

    expect(rootProps.get('--bg')).toBe('#0b0f14');
    expect(rootProps.get('--accent')).toBe('#6875e8');

    vi.unstubAllGlobals();
  });

  it('removes properties that disappear between two applied themes', async () => {
    const { fakeDocument, rootProps } = createFakeDocument();
    vi.stubGlobal('document', fakeDocument);

    const { applyTheme } = await import('./apply');
    applyTheme(baseTheme({ tokens: { '--bg': '#111', '--accent': '#222' } }));
    applyTheme(baseTheme({ tokens: { '--bg': '#333' } }));

    expect(rootProps.get('--bg')).toBe('#333');
    expect(rootProps.has('--accent')).toBe(false);

    vi.unstubAllGlobals();
  });

  it('clearTheme removes every applied property and empties the style element', async () => {
    const { fakeDocument, rootProps, elements } = createFakeDocument();
    vi.stubGlobal('document', fakeDocument);

    const { applyTheme, clearTheme } = await import('./apply');
    applyTheme(baseTheme({ css: '.a { color: red; }' }));
    clearTheme();

    expect(rootProps.size).toBe(0);
    expect(elements.get('typhon-theme')?.textContent).toBe('');

    vi.unstubAllGlobals();
  });
});

it('keeps a personal accent through theme edits, recalculates on switch, and restores original tokens on cancel/reset', async () => {
  const { fakeDocument, rootProps, elements } = createFakeDocument();
  vi.stubGlobal('document', fakeDocument);
  const { applyTheme, applyPersonalAccent } = await import('./apply');
  const dark = baseTheme({ tokens: { '--bg': '#111111', '--accent': '#6875e8', '--danger': '#ff0000' }, css: ':root { --accent: red !important; }' });
  applyTheme(dark);
  applyPersonalAccent('#FFFF00');
  const darkAccent = rootProps.get('--accent');
  applyPersonalAccent('#FF');
  expect(rootProps.get('--accent')).toBe(darkAccent);
  expect(darkAccent).not.toBe('#6875e8');
  expect(rootProps.get('--danger')).toBe('#ff0000');
  applyTheme({ ...dark, name: 'Edited' });
  expect(rootProps.get('--accent')).toBe(darkAccent);
  applyTheme(baseTheme({ base: 'light', tokens: { '--bg': '#ffffff', '--accent': '#1125d8' } }));
  expect(rootProps.get('--accent')).not.toBe(darkAccent);
  applyPersonalAccent('');
  expect(rootProps.get('--accent')).toBe('#1125d8');
  expect(rootProps.has('--accent-on')).toBe(false);
  expect(dark.tokens?.['--accent']).toBe('#6875e8');
  applyTheme(dark);
  expect(elements.get('typhon-theme')?.textContent).toBe(dark.css);
  vi.unstubAllGlobals();
});
