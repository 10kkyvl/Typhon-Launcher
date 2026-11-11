import { describe, expect, it } from 'vitest';
import { parse } from 'svelte/compiler';
import { frontendSources } from '../testing/sources';

// Every main-window route is laid out by one page container, so every route has the same content inset
// and width, and the window backdrop shows around and between its blocks. What this guard can assert from
// the sources: which component a route is rooted in, that nothing else in the routes nests a second one,
// that the container itself paints nothing, and which page-level rules paint the page or stick over the
// scrolled content. What it cannot: fills set inline or from script, and how large a block renders.

interface Node {
  type: string;
  name?: string;
  data?: string;
}

// Snippets are declarations, and Svelte requires its special elements at the top level of a component.
const SKIP = new Set(['Comment', 'SnippetBlock', 'SvelteWindow', 'SvelteDocument', 'SvelteBody', 'SvelteHead']);

function routeRoots(): string[] {
  const app = frontendSources['App.svelte'];
  const shell = app.slice(app.indexOf('<AppShell>'), app.indexOf('</AppShell>'));
  const used = new Set([...shell.matchAll(/<([A-Z]\w*)\b/g)].map((m) => m[1]));
  return [...app.matchAll(/import (\w+) from '\.\/(routes\/[^']+\.svelte)'/g)].filter((m) => used.has(m[1])).map((m) => m[2]);
}

function topLevel(source: string): Node[] {
  const ast = parse(source, { modern: true }) as unknown as { fragment: { nodes: Node[] } };
  return ast.fragment.nodes.filter((node) => !SKIP.has(node.type) && !(node.type === 'Text' && !node.data?.trim()));
}

function rooted(source: string): boolean {
  const nodes = topLevel(source);
  return nodes.length === 1 && nodes[0].type === 'Component' && nodes[0].name === 'Page';
}

function styleRules(source: string): { selector: string; body: string }[] {
  const style = /<style[^>]*>([\s\S]*?)<\/style>/.exec(source)?.[1] ?? '';
  return [...style.matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((m) => ({ selector: m[1].trim(), body: m[2] }));
}

const routeFiles = () => Object.keys(frontendSources).filter((path) => path.startsWith('routes/') && path.endsWith('.svelte'));

describe('the page container', () => {
  it('finds every route the shell renders', () => {
    expect(routeRoots()).toHaveLength(16);
  });

  it('roots every route the shell renders, and nothing else of the route sits outside it', () => {
    const outside = routeRoots().flatMap((path) =>
      rooted(frontendSources[path]) ? [] : [`${path}: ${topLevel(frontendSources[path]).map((n) => n.name ?? n.type).join(', ')}`],
    );
    expect(outside).toEqual([]);
  });

  it('lets a route keep the elements Svelte requires at the top level next to its page', () => {
    for (const special of ['<svelte:window onkeydown={key} />', '<svelte:document onvisibilitychange={key} />', '<svelte:body onclick={key} />', '<svelte:head><title>x</title></svelte:head>']) {
      expect(rooted(`<script>let key = () => {};</script>\n${special}\n<Page><p>x</p></Page>`), special).toBe(true);
    }
    expect(rooted('<Page></Page>\n<div>outside</div>')).toBe(false);
  });

  it('is never nested: route sub-views and components render inside the route page', () => {
    const roots = new Set(routeRoots());
    const nested = Object.entries(frontendSources)
      .filter(([path]) => !roots.has(path) && path.endsWith('.svelte'))
      .filter(([, source]) => /<Page\b/.test(source.replace(/<script[\s\S]*?<\/script>/g, '')))
      .map(([path]) => path);
    expect(nested).toEqual([]);
  });

  it('paints nothing and takes its inset and width from the page tokens', () => {
    const page = frontendSources['lib/components/Page.svelte'];
    expect(page).toBeTypeOf('string');
    const rules = styleRules(page);
    expect(rules.filter((rule) => /background|border\s*:/.test(rule.body)).map((rule) => rule.selector)).toEqual([]);
    expect(page).toMatch(/var\(--page-x\)/);
    expect(page).toMatch(/var\(--page-max\)/);
  });

  it('leaves Card a card: no page surface any more', () => {
    expect(frontendSources['lib/components/Card.svelte']).not.toMatch(/'page'|\.card\.page/);
    expect(routeFiles().filter((path) => /surface="page"/.test(frontendSources[path]))).toEqual([]);
  });

  it('no route paints the page colour over the backdrop', () => {
    // The theme preview draws a miniature window, page colour included, inside its own bordered card.
    const allowed = new Set(['routes/settings/ThemePreview.svelte']);
    const painted = routeFiles()
      .filter((path) => !allowed.has(path))
      .flatMap((path) =>
        styleRules(frontendSources[path])
          .filter((rule) => /background(?:-color)?\s*:\s*var\(--bg\)/.test(rule.body))
          .map((rule) => `${path}: ${rule.selector}`),
      );
    expect(painted).toEqual([]);
  });

  // A sticky bar over the scrolled page sits on the wash, so it is an island. A bar that sticks over
  // something that sets its own colours says so in a `/* sticks over: <surface> */` comment on its rule, and
  // then wears a colour that surface sets. The check sees the fill of the sticky rule itself only: a fill
  // on a child of a sticky wrapper (the catalog toolbar row in its dock) is not tied to the sticky rule
  // by the stylesheet alone and is left to review.
  const STICKS_OVER: Record<string, { file: string; why: string }> = {
    'profile canvas': {
      file: 'lib/components/ProfileCanvas.svelte',
      why: 'the profile theme, not the window theme, sets the surfaces and ink inside the canvas',
    },
  };

  it('a bar that sticks over the scrolled page is an island; one over a themed surface wears that surface (fill of the sticky rule itself only)', () => {
    const wrong = routeFiles().flatMap((path) =>
      styleRules(frontendSources[path])
        .filter((rule) => /position\s*:\s*sticky/.test(rule.body) && /background/.test(rule.body))
        .flatMap((rule) => {
          const over = /\/\*\s*sticks over:\s*([a-z ]+?)\s*[.*]/.exec(rule.selector)?.[1];
          const fill = /background\s*:\s*var\((--[\w-]+)\)\s*;/.exec(rule.body)?.[1];
          const where = `${path}: ${rule.selector.replace(/\/\*[\s\S]*?\*\//g, '').trim()}`;
          if (!over) return fill === '--island-fill' ? [] : [`${where} paints ${fill ?? 'a raw fill'} over the wash`];
          const surface = STICKS_OVER[over];
          if (!surface) return [`${where} sticks over an unknown surface: ${over}`];
          const sets = [...frontendSources[surface.file].matchAll(/style:(--[\w-]+)=/g)].map((m) => m[1]);
          return fill && sets.includes(fill) ? [] : [`${where} paints ${fill ?? 'a raw fill'}, which the ${over} does not set`];
        }),
    );
    expect(wrong).toEqual([]);
  });
});
