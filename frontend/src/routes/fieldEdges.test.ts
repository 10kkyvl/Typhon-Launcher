import { describe, expect, it } from 'vitest';
import { frontendSources } from '../lib/testing/sources';

// The selectors of a component's own style rules, each with its declarations.
function rules(path: string): [selector: string, body: string][] {
  const style = /<style>([\s\S]*?)<\/style>/.exec(frontendSources[path] ?? '')?.[1] ?? '';
  return [...style.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^{}]*)\}/g)].map((m) => [m[1].trim(), m[2]]);
}

// A source tripwire: the sign-in and support forms leave their fields' edge, background and locked
// look to the shared `.input` rule (global.css), which draws an edge that can be seen on any surface.
// A scoped `.input` rule of their own would win over it and bring back the faint one.
describe('the sign-in and support forms\' fields', () => {
  it.each(['routes/auth/AuthScreen.svelte', 'routes/support/NewTicket.svelte', 'routes/support/TicketView.svelte'])(
    '%s draws no field edge of its own',
    (path) => {
      const own = rules(path).filter(([selector, body]) => /(^|,\s*)\.input(:disabled)?$/.test(selector) || (/\.input\b/.test(selector) && /\b(border|background)\s*:/.test(body) && !/invalid/.test(selector)));
      expect(rules(path).length, path).toBeGreaterThan(0);
      expect(own.map(([selector]) => selector), path).toEqual([]);
    },
  );
});
