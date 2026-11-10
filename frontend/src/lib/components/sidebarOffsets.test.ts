import { describe, expect, it } from 'vitest';

/**
 * Floating parts that sit beside the sidebar (the toast/activity corner, the chat, the update card)
 * keep 2.4rem from the island's inner edge on either side, expanded or collapsed to icons: they
 * offset by --sidebar-now, which counts the island's width and the gap outside it.
 */

type Fs = {
  readFileSync(path: URL, encoding: 'utf8'): string;
  readdirSync(path: URL, options: { recursive: true }): string[];
};
const fs = (globalThis as unknown as { process: { getBuiltinModule(id: 'node:fs'): Fs } }).process.getBuiltinModule('node:fs');
const SRC = new URL('../../', import.meta.url);
const read = (path: string) => fs.readFileSync(new URL(path, SRC), 'utf8');

function declared(css: string, name: string): string {
  const value = new RegExp(`${name}\\s*:\\s*([^;]+);`).exec(css)?.[1];
  expect(value, name).toBeTypeOf('string');
  return (value as string).trim();
}

function rule(source: string, selector: string, prop: string, media = false): string {
  const scope = media ? (/@media \(max-width: 1140px\) \{([\s\S]*?)\n {2}\}/.exec(source)?.[1] ?? '') : source;
  const body = new RegExp(`\\n\\s*${selector.replaceAll('.', '\\.')} \\{([^}]*)\\}`).exec(scope)?.[1] ?? '';
  return declared(body, prop);
}

// rem of a calc() made of rem lengths, var() references and `+` / `- ` / `* n`.
function rem(expr: string, vars: Record<string, string>): number {
  const js = expr
    .replace(/var\((--[\w-]+)\)/g, (_, name: string) => {
      expect(vars[name], name).toBeTypeOf('string');
      return `(${rem(vars[name], vars)})`;
    })
    .replace(/calc\(/g, '(')
    .replace(/(\d*\.?\d+)rem/g, '$1');
  expect(js).toMatch(/^[\d.\s()+\-*]+$/);
  return Function(`return ${js}`)() as number;
}

describe('offsets beside the sidebar island', () => {
  const tokens = read('styles/tokens.css');
  const shell = read('styles/shell.css');
  const appShell = read('lib/components/AppShell.svelte');
  const sidebar = read('lib/components/Sidebar.svelte');
  const chat = read('lib/components/ChatPanel.svelte');
  const updateCard = read('lib/components/UpdateOverlay.svelte');

  const base = {
    '--sidebar-w': declared(tokens, '--sidebar-w'),
    '--island-gap': declared(tokens, '--island-gap'),
    '--sidebar-rail-w': declared(shell, '--sidebar-rail-w'),
  };
  const expanded = { ...base, '--sidebar-now': declared(appShell, '--sidebar-now') };
  const collapsedNow = /@media \(max-width: 1140px\) \{[\s\S]*?--sidebar-now:\s*([^;]+);/.exec(appShell)?.[1] ?? '';
  const collapsed = { ...base, '--sidebar-now': collapsedNow };

  // Distance from the window edge to the island's inner edge.
  const islandEdge = (vars: Record<string, string>, width: string) => rem(`calc(var(--island-gap) + ${width})`, vars);

  it('sizes the island from the tokens it offsets by', () => {
    expect(rule(sidebar, '.sidebar', 'width')).toBe('var(--sidebar-w)');
    expect(rule(sidebar, '.sidebar', 'width', true)).toBe('var(--sidebar-rail-w)');
    expect(rule(sidebar, '.sidebar', 'margin')).toBe('var(--island-gap) 0 var(--island-gap) var(--island-gap)');
    expect(rule(sidebar, '.sidebar.right', 'margin')).toBe('var(--island-gap) var(--island-gap) var(--island-gap) 0');
  });

  const cases = [
    { name: 'expanded', vars: expanded, width: 'var(--sidebar-w)', edge: 24 },
    { name: 'collapsed to icons', vars: collapsed, width: 'var(--sidebar-rail-w)', edge: 7.2 },
  ];

  for (const c of cases) {
    it(`keeps 2.4rem from the island ${c.name}, on the left and on the right`, () => {
      expect(islandEdge(c.vars, c.width)).toBeCloseTo(c.edge);
      // Sidebar on the left: the update card starts after it; the corner and the chat sit at the right edge.
      expect(rem(rule(updateCard, '.card', 'left'), c.vars) - islandEdge(c.vars, c.width)).toBeCloseTo(2.4);
      // Sidebar on the right: the corner and the chat stop before it; the update card is at the left edge.
      expect(rem(rule(appShell, '.corner.sidebar-right', 'right'), c.vars) - islandEdge(c.vars, c.width)).toBeCloseTo(2.4);
      expect(rem(rule(chat, '.chat-root.sidebar-right', 'right'), c.vars) - islandEdge(c.vars, c.width)).toBeCloseTo(2.4);
      expect(
        rem(rule(appShell, '.corner.sidebar-right.beside-chat', 'right'), c.vars) - islandEdge(c.vars, c.width) - 38 - 1.2,
      ).toBeCloseTo(2.4);
    });
  }

  it('leaves the far side where it was', () => {
    expect(rule(updateCard, '.card.sidebar-right', 'left')).toBe('2.4rem');
    expect(rule(appShell, '.corner', 'right')).toBe('2.4rem');
    expect(declared(/\.chat-root \{([^}]*)\}/.exec(chat)?.[1] ?? '', 'right')).toBe('2.4rem');
  });

  it('offsets nothing else by the bare sidebar width', () => {
    const files = fs
      .readdirSync(SRC, { recursive: true })
      .map((p) => p.replaceAll('\\', '/'))
      .filter((p) => /\.(svelte|css)$/.test(p) && !['lib/components/Sidebar.svelte', 'lib/components/AppShell.svelte'].includes(p));
    expect(files.filter((p) => read(p).includes('var(--sidebar-w)'))).toEqual([]);
  });
});
