import { describe, expect, it } from 'vitest';

/**
 * Accent roles: --accent is a fill behind a label (button, badge, count) and is under 3:1 on the
 * raised surfaces of a dark theme. Text and icons take --accent-text, focus outlines and focus
 * borders --accent-ring, every other indicator (progress, switch, selection border, marker,
 * data bar) --accent-bright. A decorative tint mixed from --accent stays allowed.
 */

type Fs = {
  readFileSync(path: URL, encoding: 'utf8'): string;
  readdirSync(path: URL, options: { recursive: true }): string[];
};
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

const SRC = new URL('../', import.meta.url);
const files = fs
  .readdirSync(SRC, { recursive: true })
  .map((path) => path.replaceAll('\\', '/'))
  .filter((path) => /\.(svelte|css|ts)$/.test(path) && !/\.(test|d)\.ts$/.test(path) && !path.startsWith('lib/i18n/catalog/'));
const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');

const ACCENT = 'var(--accent)';
// Properties that never paint a fill: any --accent in them is text, an icon, an outline or a ring.
const NEVER = /^(?:color|caret-color|fill|stroke|outline(?:-color)?|box-shadow|text-decoration(?:-color)?)$/;
// A border drawn in the accent itself marks a state; a tint mixed from it is decoration.
const BORDER = /^border(?:-(?:top|right|bottom|left|block|inline)(?:-(?:start|end))?)?(?:-color)?$/;
// Selectors of indicators whose background is the indicator, never a fill behind a label.
const INDICATOR = /progress|slider|thumb|track|marker|indicator|[.-]dot\b|fill\b|marquee|\.toggle\b|switch-knob|tone-\d/;

interface Use {
  where: string;
  selector: string;
  prop: string;
  value: string;
}

function uses(path: string): Use[] {
  const text = read(path);
  const out: Use[] = [];
  for (const m of text.matchAll(/([-\w]+)\s*:\s*([^;{}]*?var\(--accent\)[^;{}]*)/g)) {
    const at = m.index ?? 0;
    const open = text.lastIndexOf('{', at);
    const selector = text.slice(Math.max(text.lastIndexOf('}', open), text.lastIndexOf(';', open)) + 1, open).trim();
    out.push({ where: `${path}:${text.slice(0, at).split('\n').length}`, selector, prop: m[1], value: m[2].trim() });
  }
  return out;
}

const all = files.filter((path) => !path.endsWith('.ts')).flatMap(uses);
const direct = (value: string) => value.includes(ACCENT) && !/color-mix\([^()]*var\(--accent\)/.test(value);

describe('accent roles', () => {
  it('has files to scan', () => {
    expect(files.length).toBeGreaterThan(100);
    expect(all.length).toBeGreaterThan(10);
  });

  it('paints no text, icon, outline or ring with the fill accent', () => {
    expect(all.filter((u) => NEVER.test(u.prop)).map((u) => `${u.where} ${u.prop}: ${u.value}`)).toEqual([]);
  });

  it('draws no state border in the fill accent', () => {
    expect(all.filter((u) => BORDER.test(u.prop) && direct(u.value)).map((u) => `${u.where} ${u.prop}: ${u.value}`)).toEqual([]);
  });

  it('fills no indicator with the fill accent', () => {
    const bad = all.filter((u) => /^background(?:-color)?$/.test(u.prop) && INDICATOR.test(u.selector) && direct(u.value));
    expect(bad.map((u) => `${u.where} ${u.selector} { ${u.prop}: ${u.value} }`)).toEqual([]);
  });

  it('passes no fill accent as a colour value to a component', () => {
    const bad = files.flatMap((path) =>
      [...read(path).matchAll(/(['"`])var\(--accent\)\1/g)].map((m) => `${path}:${read(path).slice(0, m.index).split('\n').length}`),
    );
    expect(bad).toEqual([]);
  });
});
