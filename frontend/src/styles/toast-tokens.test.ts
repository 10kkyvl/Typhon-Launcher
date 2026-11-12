import { describe, expect, it } from 'vitest';
import { TOAST_FIXED_REM, TOAST_MEASURE_REM, TOAST_MIN_WIDTH_REM } from '../lib/components/toastSpot';

/**
 * Tripwires for the toast: its styles take every colour, radius and duration from a token, its
 * buttons are the base ones, and the theme editor's preview draws the same surface. Behaviour is
 * held by Toasts.dom.test.ts and stores/toasts.test.ts.
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const SRC = new URL('../', import.meta.url);
const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');

const style = (source: string) => /<style>([\s\S]*)<\/style>/.exec(source)?.[1] ?? '';
const markup = (source: string) => source.replace(/<script[\s\S]*?<\/script>/g, '').replace(/<style>[\s\S]*<\/style>/, '');

function rule(css: string, selector: string): Record<string, string> {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const body = new RegExp(`(?:^|\\})\\s*${escaped}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';
  return Object.fromEntries(
    body
      .split(';')
      .map((line) => line.split(':'))
      .filter((pair) => pair.length >= 2)
      .map(([name, ...value]) => [name.trim(), value.join(':').trim()]),
  );
}

function literals(css: string): string[] {
  const found: string[] = [];
  for (const line of css.split('\n')) {
    const declaration = /^\s*([a-z-]+)\s*:\s*(.+?);?\s*$/.exec(line);
    if (!declaration) continue;
    const [, name, value] = declaration;
    const colour = /#[0-9a-f]{3,8}\b|\b(?:rgb|rgba|hsl|hsla|oklch)\(/i.test(value);
    const radius = /radius/.test(name) && !/^var\(--radius-[a-z0-9-]+\)$/.test(value);
    const duration = /^(?:animation|transition)/.test(name) && /(?:^|[\s,])[\d.]+m?s\b/.test(value);
    if (colour || radius || duration) found.push(line.trim());
  }
  return found;
}

const toasts = read('lib/components/Toasts.svelte');
const preview = read('routes/settings/ThemePreview.svelte');

describe('the literal scan', () => {
  it('catches a colour, a radius and a duration written as a number', () => {
    expect(literals('  color: #fff;\n  background: rgba(0, 0, 0, 0.2);')).toHaveLength(2);
    expect(literals('  border-radius: 6px;')).toHaveLength(1);
    expect(literals('  animation: in 200ms ease;\n  transition: opacity 0.2s;')).toHaveLength(2);
    expect(literals('  color: var(--text);\n  border-radius: var(--radius-md);\n  animation: in var(--dur-panel) var(--ease);')).toEqual([]);
  });
});

describe('Toasts.svelte', () => {
  it('is read', () => {
    expect(style(toasts)).toContain('.toast');
  });

  it('writes no colour, radius or duration of its own', () => {
    expect(literals(style(toasts))).toEqual([]);
  });

  it('has no button of its own: the action and the close button are the base components', () => {
    expect(markup(toasts)).not.toMatch(/<button\b/);
    expect(markup(toasts)).toMatch(/<Button\b/);
    expect(markup(toasts)).toMatch(/<IconButton\b/);
    expect(style(toasts)).not.toMatch(/\.action\b|\.dismiss\b/);
  });

  it('keeps no z-index and no placement: where the stack stands is its host', () => {
    expect(style(toasts)).not.toMatch(/z-index|position\s*:/);
  });
});

// A toast lays itself out for the width it has, in CSS alone: jsdom lays nothing out, so these hold
// the rules; what they give at a width is seen in the running app.
describe('a toast in a narrow place', () => {
  const css = style(toasts).replace(/\/\*[\s\S]*?\*\//g, '');
  const tokensCss = read('styles/tokens.css');
  const rem = (value: string): number => {
    const token = /^var\((--[\w-]+)\)$/.exec(value.trim());
    if (token) return rem(new RegExp(`${token[1]}:\\s*([^;]+);`).exec(tokensCss)?.[1] ?? '');
    const number = /^([\d.]+)(rem|px)$/.exec(value.trim());
    if (!number) throw new Error(`not a length: ${value}`);
    return number[2] === 'px' ? Number(number[1]) / 10 : Number(number[1]);
  };

  it('names the least width a message is read in, and the script that narrows the stack uses the same', () => {
    expect(rule(css, '.toast')['--toast-measure']).toBe(`${TOAST_MEASURE_REM}rem`);
  });

  it('the action wraps under the message when the message would get less than that, and stands at the right', () => {
    const content = rule(css, '.content');
    expect(content.display).toBe('flex');
    expect(content['flex-wrap']).toBe('wrap');
    expect(content['justify-content']).toBe('flex-end');
    expect(content.flex).toBe('1');
    expect(content['min-width']).toBe('0');
    // Beside an action the message asks for the measure; alone it takes what there is.
    expect(rule(css, '.message:not(:only-child)')['min-width']).toBe('var(--toast-measure)');
    expect(rule(css, '.message').flex).toBe('1');
    expect(rule(css, '.message')['min-width']).toBe('0');
  });

  it('the close button stays at the top right, level with the first line of the message', () => {
    const close = rule(css, '.close');
    expect(close['align-self']).toBe('flex-start');
    expect(close['min-height']).toBe('calc(1.4em + 2 * var(--space-2))');
    expect(rule(css, '.message')['line-height']).toBe('1.4');
    expect(rule(css, '.message').padding).toBe('var(--space-2) 0');
  });

  it('is laid out by its styles, not measured in script', () => {
    expect(toasts).not.toMatch(/getBoundingClientRect|ResizeObserver|clientWidth|offsetWidth|matchMedia/);
  });

  it('the least width of the stack is what a toast takes besides its message, plus the measure', () => {
    const toast = rule(css, '.toast');
    const [top, right, , left] = toast.padding.split(/\s+/);
    expect(top).toBe('var(--space-1)');
    const border = /^([\d.]+px)/.exec(toast.border)?.[1] ?? '';
    const icon = /<Icon size="([\d.]+rem)"/.exec(toasts)?.[1] ?? '';
    const close = rule(style(read('lib/components/IconButton.svelte')), '.sm').width;
    expect(markup(toasts)).toMatch(/<IconButton\s+size="sm"/);
    const fixed = rem(left) + rem(icon) + rem(toast.gap) + rem(toast.gap) + rem(close) + rem(right) + 2 * rem(border);

    expect(fixed).toBeCloseTo(TOAST_FIXED_REM, 5);
    expect(TOAST_MIN_WIDTH_REM).toBeCloseTo(TOAST_FIXED_REM + TOAST_MEASURE_REM, 5);
  });
});

describe('the theme editor preview', () => {
  it('draws its toast on the same surface as the real one', () => {
    const real = rule(style(toasts), '.toast');
    const clone = rule(style(preview), '.toast');

    for (const name of ['background', 'border', 'border-radius', 'box-shadow']) {
      expect(real[name], name).toBeTruthy();
      expect(clone[name], name).toBe(real[name]);
    }
  });
});
