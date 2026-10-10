import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';

function routeNames(): string[] {
  const source = frontendSources['lib/stores/router.ts'];
  const body = /export type RouteName =([\s\S]*?);/.exec(source)?.[1] ?? '';
  return [...body.matchAll(/'([a-z]+)'/g)].map((match) => match[1]);
}

describe('route table', () => {
  const names = routeNames();

  it('reads the route names it is guarding', () => {
    expect(names.length).toBeGreaterThan(10);
  });

  it('has a screen in the app shell for every route', () => {
    const app = frontendSources['App.svelte'];
    const missing = names.filter((name) => !new RegExp(String.raw`\$route\.name === '${name}'`).test(app));
    expect(missing).toEqual([]);
  });

  it('has no screen in the app shell for a route that does not exist', () => {
    const app = frontendSources['App.svelte'];
    const known = new Set(names);
    const stray = [...app.matchAll(/\$route\.name === '([a-z]+)'/g)].map((match) => match[1]).filter((name) => !known.has(name));
    expect(stray).toEqual([]);
  });

  it('only navigates to routes that exist', () => {
    const known = new Set(names);
    const bad: string[] = [];
    for (const [path, source] of Object.entries(frontendSources)) {
      for (const match of source.matchAll(/\bnavigate\(\s*['"]([A-Za-z-]+)['"]/g)) {
        if (!known.has(match[1])) bad.push(`${path}: ${match[1]}`);
      }
    }
    expect(bad).toEqual([]);
  });
});
