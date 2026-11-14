import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');

// The bleed area spans the scroller; a bleed child lines its content up with the page through --page-inset,
// which must hold wherever it is used (padding, an absolutely placed child, vertical use), so it is
// expressed in the bleed container's own width, not in percent of whatever box uses it.

const page = frontendSources['lib/components/Page.svelte'];
const rule = (source: string, selector: string) => new RegExp(`${selector.replace(/\./g, '\\.')}\\s*\\{([^}]*)\\}`).exec(source)?.[1] ?? '';
const decl = (body: string, property: string) => new RegExp(`(?:^|;|\\n)\\s*${property}\\s*:\\s*([^;]+);`).exec(body)?.[1].trim();

function rootPx(name: string): number {
  const css = fs.readFileSync(new URL('../../styles/tokens.css', import.meta.url), 'utf8');
  const value = new RegExp(`${name}:\\s*([\\d.]+)rem;`).exec(css)?.[1];
  expect(value, name).toBeTypeOf('string');
  return Number(value) * 10;
}

// Evaluates a declaration made of var(), calc(), max(), arithmetic and 100cqi, all in px.
function evaluate(expr: string, vars: Record<string, number>, cqi: number): number {
  const js = expr
    .replace(/var\((--[\w-]+)\)/g, (_, name: string) => {
      expect(vars[name], name).toBeTypeOf('number');
      return String(vars[name]);
    })
    .replace(/100cqi/g, String(cqi))
    .replace(/\bcalc\(/g, '(')
    .replace(/\bmax\(/g, 'Math.max(')
    .replace(/\bmin\(/g, 'Math.min(');
  expect(js).toMatch(/^(?:[\d\s.+\-*/(),]|Math\.max|Math\.min)+$/);
  return Function(`"use strict"; return (${js});`)() as number;
}

describe('the page bleed', () => {
  const bleed = rule(page, '.bleed');
  const content = rule(page, '.content');

  it('is an inline-size container and states the inset in its own width', () => {
    expect(decl(bleed, 'container-type')).toBe('inline-size');
    expect(decl(bleed, '--page-inset')).toBe('max(var(--page-x), calc((100cqi - var(--page-max)) / 2))');
    expect(decl(bleed, '--page-inset')).not.toMatch(/%/);
  });

  it('lines the game hero up with the page content at 1100, 1440 and 2560 px', () => {
    const hero = rule(frontendSources['routes/game/GamePageHero.svelte'], '  .hero');
    expect(decl(hero, 'padding')).toMatch(/var\(--page-inset\)/);
    expect(decl(hero, 'margin-inline')).toBeUndefined();
    const maxWidth = decl(content, 'max-width') as string;
    const padding = (decl(content, 'padding') as string).split(/\s+/)[1];
    for (const width of [1100, 1440, 2560]) {
      // The shell widens the page inset on very wide windows (AppShell, min-width 2200px).
      const vars = { '--page-x': width >= 2200 ? rootPx('--space-12') : rootPx('--page-x'), '--page-max': rootPx('--page-max') };
      const box = Math.min(width, evaluate(maxWidth, vars, width));
      const contentX = (width - box) / 2 + evaluate(padding, vars, width);
      const heroX = evaluate(decl(bleed, '--page-inset') as string, vars, width);
      expect(heroX, `${width}px`).toBeCloseTo(contentX, 6);
    }
  });
});
