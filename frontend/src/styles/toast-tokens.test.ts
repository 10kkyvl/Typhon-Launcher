import { describe, expect, it } from 'vitest';

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
