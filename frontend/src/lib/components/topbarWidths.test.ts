import { describe, expect, it } from 'vitest';

/**
 * The top bar row never wraps or clips. As the main column narrows, the account chip drops its text
 * first (exactly where the search would otherwise start shrinking below its full width), then the search
 * pill shrinks down to its minimum. The widths are read from the styles, in rem at the default interface
 * scale (1rem = 10px); the window breakpoint of the sidebar rail is in px.
 */

type Fs = { readFileSync(path: URL, encoding: 'utf8'): string };
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const SRC = new URL('../../', import.meta.url);
const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');

function declared(css: string, name: string): string {
  const value = new RegExp(`(?:^|[;{\\s])${name}\\s*:\\s*([^;]+);`).exec(css)?.[1];
  expect(value, name).toBeTypeOf('string');
  return (value as string).trim();
}

function body(source: string, selector: string, scope = source): string {
  const rule = new RegExp(`\\n\\s*${selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')} \\{([^}]*)\\}`).exec(scope)?.[1];
  expect(rule, selector).toBeTypeOf('string');
  return rule as string;
}

// rem of a length made of rem, px (10px to the rem), var() references and calc() with + - *.
function rem(expr: string, vars: Record<string, string>): number {
  const js = expr
    .replace(/var\((--[\w-]+)\)/g, (_, name: string) => {
      expect(vars[name], name).toBeTypeOf('string');
      return `(${rem(vars[name], vars)})`;
    })
    .replace(/calc\(/g, '(')
    .replace(/(\d*\.?\d+)rem/g, '$1')
    .replace(/(\d*\.?\d+)px/g, '($1 / 10)');
  expect(js).toMatch(/^[\d.\s()+\-*/]+$/);
  return Function(`return ${js}`)() as number;
}

function container(source: string, name: string): { at: number; rules: string } {
  const m = new RegExp(`@container ${name} \\(max-width: ([\\d.]+)rem\\) \\{([\\s\\S]*?)\\n {2}\\}`).exec(source);
  expect(m, name).not.toBeNull();
  return { at: Number((m as RegExpExecArray)[1]), rules: (m as RegExpExecArray)[2] };
}

describe('top bar widths', () => {
  const tokens = read('styles/tokens.css');
  const shell = read('styles/shell.css');
  const appShell = read('lib/components/AppShell.svelte');
  const topbar = read('lib/components/Topbar.svelte');
  const search = read('lib/components/TopbarSearch.svelte');
  const account = read('lib/components/AccountMenu.svelte');
  const iconButton = read('lib/components/IconButton.svelte');
  const avatar = read('lib/components/Avatar.svelte');

  const vars: Record<string, string> = {};
  for (const name of ['--space-1', '--space-3', '--page-x', '--sidebar-w', '--island-gap', '--control-md']) vars[name] = declared(tokens, name);
  for (const name of ['--sidebar-rail-w', '--topbar-search-min', '--topbar-search-max', '--topbar-chip-max']) vars[name] = declared(shell, name);
  const len = (expr: string) => rem(expr, vars);

  const bar = body(topbar, '.topbar');
  const island = body(topbar, '.island');
  const gap = len(declared(bar, 'gap'));
  const pad = len(declared(island, 'padding'));
  const edge = len(/^(\S+) solid/.exec(declared(island, 'border'))?.[1] ?? 'none');
  const button = len(declared(body(iconButton, '.icon-btn'), 'width'));
  const actionsGap = len(declared(body(topbar, '.actions'), 'gap'));
  const rightGap = len(declared(body(topbar, '.right'), 'gap'));
  const historyGap = len(declared(body(topbar, '.history'), 'gap'));
  const searchMin = len('var(--topbar-search-min)');
  const searchMax = len('var(--topbar-search-max)');
  const chipFull = len(declared(body(account, '.chip'), 'max-width'));
  const avatarSize = len(declared(body(avatar, '.avatar.sm'), '--size'));
  const leftRule = container(account, 'topbar');
  const centredRule = container(account, 'topbar-centred');
  const compactPad = len(declared(body(account, '.chip', leftRule.rules), 'padding'));
  const chipCompact = avatarSize + 2 * compactPad;

  const history = 2 * pad + 2 * edge + 2 * button + historyGap;
  const right = (chip: number, actions = 3) =>
    2 * pad + 2 * edge + actions * button + Math.max(0, actions - 1) * actionsGap + rightGap + chip;

  // The search's width in a row of the given inner width, or how far it falls short of its minimum.
  function searchWidth(width: number, centred: boolean, chip: number): number {
    const free = centred ? width - 2 * Math.max(history, right(chip)) - 2 * gap : width - history - right(chip) - 2 * gap;
    return Math.min(searchMax, free);
  }

  it('reads a sane geometry', () => {
    expect(history).toBeCloseTo(8);
    expect(right(chipFull)).toBeCloseTo(32.8);
    expect(right(chipCompact)).toBeCloseTo(16.4);
    expect(searchMin).toBeLessThan(searchMax);
    expect(declared(bar, 'padding')).toBe('0 var(--page-x)');
    expect(declared(bar, 'container')).toBe('topbar / inline-size');
    expect(declared(body(topbar, '.topbar.centered'), 'container-name')).toBe('topbar-centred');
  });

  it('sizes the search and the chip from the shell geometry', () => {
    const wrap = body(search, '.search-wrap');
    expect(declared(wrap, 'min-width')).toBe('var(--topbar-search-min)');
    expect(declared(wrap, 'max-width')).toBe('var(--topbar-search-max)');
    expect(declared(body(account, '.chip'), 'max-width')).toBe('var(--topbar-chip-max)');
    expect(declared(body(topbar, '.topbar.centered'), 'grid-template-columns')).toBe(
      '1fr minmax(var(--topbar-search-min), var(--topbar-search-max)) 1fr',
    );
  });

  it('drops the chip text exactly where the search would start to shrink, search on the left', () => {
    expect(leftRule.at).toBeCloseTo(history + right(chipFull) + 2 * gap + searchMax);
  });

  it('drops the chip text exactly where the centred search would start to shrink', () => {
    expect(centredRule.at).toBeCloseTo(2 * right(chipFull) + 2 * gap + searchMax);
  });

  it('hides the chip text from sight only: the button keeps its name for assistive tech', () => {
    for (const rule of [leftRule, centredRule]) {
      const text = body(account, '.chip-text', rule.rules);
      expect(text).not.toMatch(/display:\s*none/);
      expect(text).toMatch(/clip-path: inset\(50%\)/);
      expect(declared(body(account, '.chip', rule.rules), 'padding')).toBe(declared(body(account, '.chip', leftRule.rules), 'padding'));
    }
  });

  const sidebarNow = {
    expanded: len(declared(appShell, '--sidebar-now')),
    rail: rem(/@media \(max-width: 1140px\) \{[\s\S]*?--sidebar-now:\s*([^;]+);/.exec(appShell)?.[1] ?? '', vars),
  };
  const inner = (main: number) => main - 2 * len('var(--page-x)');

  const windows = [1100, 1120, 1140, 1141, 1200, 1280, 1366, 1440, 1920, 2560];
  for (const px of windows) {
    for (const side of ['left', 'right']) {
      for (const centred of [false, true]) {
        it(`fits at ${px}px, sidebar ${side}, search ${centred ? 'centred' : 'left'}`, () => {
          const main = px / 10 - (px <= 1140 ? sidebarNow.rail : sidebarNow.expanded);
          const width = inner(main);
          const compact = width <= (centred ? centredRule.at : leftRule.at);
          const chip = compact ? chipCompact : chipFull;
          const s = searchWidth(width, centred, chip);
          expect(s, `search ${s.toFixed(1)}rem`).toBeGreaterThanOrEqual(searchMin);
          if (!compact) expect(s).toBeCloseTo(searchMax);
        });
      }
    }
  }

  it('fits at the narrowest the shell allows at any interface scale', () => {
    const shellMin = len(declared(body(appShell, '.shell'), 'min-width'));
    const width = inner(shellMin - sidebarNow.expanded);
    for (const centred of [false, true]) {
      expect(width).toBeLessThanOrEqual(centred ? centredRule.at : leftRule.at);
      expect(searchWidth(width, centred, chipCompact)).toBeGreaterThanOrEqual(searchMin);
    }
  });
});
