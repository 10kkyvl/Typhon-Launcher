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
