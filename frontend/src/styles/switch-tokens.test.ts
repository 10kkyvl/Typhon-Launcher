import { describe, expect, it } from 'vitest';
import { frontendSources } from '../lib/testing/sources';

/**
 * The switch, the segmented control and the tabs draw themselves from tokens only, and nobody restyles
 * them from outside: a missing variant is a prop of the component.
 *
 * Limits: only each component's own <style> is read (an inline style or a child component is not seen);
 * the reach-in rule sees `:global(` selectors that name the component's root class.
 */

const COMPONENTS = ['Toggle'] as const;
const ROOTS: Record<(typeof COMPONENTS)[number], string> = { Toggle: 'toggle' };

function style(path: string): string {
  const text = frontendSources[path];
  expect(text, path).toBeTypeOf('string');
  return /<style[^>]*>([\s\S]*?)<\/style>/.exec(text)?.[1] ?? '';
}

function declarations(css: string): { prop: string; value: string }[] {
  return [...css.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([\w-]+)\s*:\s*([^;{}]+);/g)].map((m) => ({ prop: m[1], value: m[2].trim() }));
}

const COLOUR = /#[0-9a-f]{3,8}\b|\b(?:rgb|rgba|hsl|hsla|oklch|oklab)\(/i;
const TIME = /(?:^|[\s,(])\d*\.?\d+m?s\b/;

describe('switch, segmented control and tabs', () => {
  for (const name of COMPONENTS) {
    const path = `lib/components/${name}.svelte`;

    it(`${name} writes no colour`, () => {
      expect(declarations(style(path)).length).toBeGreaterThan(5);
      expect(declarations(style(path)).filter((d) => COLOUR.test(d.value)).map((d) => `${d.prop}: ${d.value}`)).toEqual([]);
    });

    it(`${name} takes every radius and duration from a token`, () => {
      const bad = declarations(style(path)).filter(
        (d) => (/radius$/.test(d.prop) && !/^(?:var\(|calc\(var\(|inherit$|0$)/.test(d.value)) || (/^(?:transition|animation)/.test(d.prop) && TIME.test(d.value)),
      );
      expect(bad.map((d) => `${d.prop}: ${d.value}`)).toEqual([]);
    });

    it(`${name} sets no z-index`, () => {
      expect(declarations(style(path)).filter((d) => d.prop === 'z-index').map((d) => d.value)).toEqual([]);
    });
  }

  it('are restyled by no other component through :global', () => {
    const classes = Object.values(ROOTS).join('|');
    const reach = new RegExp(`:global\\([^)]*\\.(?:${classes})(?![\\w-])`);
    expect(reach.test('.row :global(.toggle) { margin: 0; }')).toBe(true);
    expect(reach.test('.row :global(.toggle-row) { margin: 0; }')).toBe(false);
    const own = new Set<string>(COMPONENTS.map((name) => `lib/components/${name}.svelte`));
    const bad = Object.entries(frontendSources)
      .filter(([path, text]) => path.endsWith('.svelte') && !own.has(path) && reach.test(text))
      .map(([path]) => path);
    expect(bad).toEqual([]);
  });

  // The theme editor's preview draws a switch of its own inside its scoped tokens; it must look like
  // the real one where a theme can change it.
  it('the theme preview draws the switch as the component does', () => {
    const preview = style('routes/settings/ThemePreview.svelte');
    const rule = (css: string, selector: string) => new RegExp(`(?:^|\\})\\s*${selector.replace(/[.\\\\]/g, '\\$&')}\\s*\\{([^}]*)\\}`).exec(css)?.[1] ?? '';
    const value = (body: string, prop: string) => new RegExp(`(?:^|;)\\s*${prop}\\s*:\\s*([^;]+);`).exec(body)?.[1].trim();
    const toggle = style('lib/components/Toggle.svelte');
    expect(value(rule(preview, '.toggle'), 'background')).toBe(value(rule(toggle, '.toggle.on .track'), 'background'));
    expect(value(rule(preview, '.toggle'), 'border-radius')).toBe(value(rule(toggle, '.track'), 'border-radius'));
    expect(value(rule(preview, '.knob'), 'background')).toBe(value(rule(toggle, '.on .knob'), 'background'));
  });
});
