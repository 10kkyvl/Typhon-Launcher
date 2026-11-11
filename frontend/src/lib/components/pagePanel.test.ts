import { describe, expect, it } from 'vitest';
import { parse } from 'svelte/compiler';
import { frontendSources } from '../testing/sources';

// The window backdrop shows around and between a page's blocks, so no route paints a panel over the
// whole page any more. A route's outermost Card (through {#if}/{#each}/{#key} blocks and through the
// route's own sub-views) is the page itself and takes the transparent `page` surface.

interface Node {
  type: string;
  name?: string;
  attributes?: { type: string; name: string; value: true | { type: string; data?: string }[] }[];
  fragment?: { nodes: Node[] };
  consequent?: { nodes: Node[] } | null;
  alternate?: { nodes: Node[] } | null;
  body?: { nodes: Node[] } | null;
  fallback?: { nodes: Node[] } | null;
  pending?: { nodes: Node[] } | null;
  then?: { nodes: Node[] } | null;
  catch?: { nodes: Node[] } | null;
}

const BLOCK_PARTS = ['consequent', 'alternate', 'body', 'fallback', 'pending', 'then', 'catch'] as const;

function resolve(from: string, relative: string): string {
  const parts = from.split('/').slice(0, -1);
  for (const part of relative.split('/')) {
    if (part === '.') continue;
    if (part === '..') parts.pop();
    else parts.push(part);
  }
  return parts.join('/');
}

function routeImports(path: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const m of frontendSources[path].matchAll(/import (\w+) from '(\.[^']+\.svelte)'/g)) {
    const target = resolve(path, m[2]);
    if (target.startsWith('routes/')) out.set(m[1], target);
  }
  return out;
}

function surface(node: Node): string {
  const attr = node.attributes?.find((a) => a.type === 'Attribute' && a.name === 'surface');
  if (!attr) return 'card';
  if (attr.value === true) return '';
  return attr.value.map((part) => part.data ?? '{expression}').join('');
}

// The outermost Cards a route renders, as `file: surface`.
function outerCards(path: string, seen = new Set<string>()): string[] {
  if (seen.has(path)) return [];
  seen.add(path);
  const imports = routeImports(path);
  const ast = parse(frontendSources[path], { modern: true }) as unknown as { fragment: { nodes: Node[] } };
  const out: string[] = [];
  const walk = (nodes: Node[]) => {
    for (const node of nodes) {
      if (node.type === 'Component' && node.name === 'Card') out.push(`${path}: ${surface(node)}`);
      else if (node.type === 'Component' && node.name && imports.has(node.name)) out.push(...outerCards(imports.get(node.name) as string, seen));
      else if (node.type === 'KeyBlock') walk(node.fragment?.nodes ?? []);
      else if (node.type.endsWith('Block') && node.type !== 'SnippetBlock') {
        for (const part of BLOCK_PARTS) walk(node[part]?.nodes ?? []);
      }
    }
  };
  walk(ast.fragment.nodes);
  return out;
}

function routeRoots(): string[] {
  const app = frontendSources['App.svelte'];
  const shell = app.slice(app.indexOf('<AppShell>'), app.indexOf('</AppShell>'));
  const used = new Set([...shell.matchAll(/<([A-Z]\w*)\b/g)].map((m) => m[1]));
  return [...app.matchAll(/import (\w+) from '\.\/(routes\/[^']+\.svelte)'/g)].filter((m) => used.has(m[1])).map((m) => m[2]);
}

describe('page panel', () => {
  it('finds every route the shell renders', () => {
    expect(routeRoots().length).toBe(16);
  });

  it('no route paints a panel over the whole page', () => {
    const cards = routeRoots().flatMap((path) => outerCards(path));
    expect(cards.length).toBeGreaterThan(0);
    expect(cards.filter((card) => card.endsWith(': panel'))).toEqual([]);
  });

  it('the page surface keeps the panel geometry and paints nothing', () => {
    const card = frontendSources['lib/components/Card.svelte'];
    const rule = /\.card\.page\s*\{([^}]*)\}/.exec(card)?.[1] ?? '';
    expect(rule).toMatch(/background:\s*none/);
    expect(rule).toMatch(/border-color:\s*transparent/);
    expect(rule).not.toMatch(/padding|border-width|border:\s/);
  });
});
