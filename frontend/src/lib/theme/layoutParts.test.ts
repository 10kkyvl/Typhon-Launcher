import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';
import { LAYOUT_PARTS, PARTS_WITH_ID } from './layout';

describe('layout parts', () => {
  const sources = Object.entries(frontendSources).filter(([path]) => !path.endsWith('theme/layout.ts'));

  it.each(LAYOUT_PARTS)('%s has an element in the sources', (name) => {
    const literal = new RegExp(`data-part=["']${name}["']|'data-part':\\s*'${name}'`);
    expect(sources.some(([, text]) => literal.test(text))).toBe(true);
  });

  // A part that takes an id carries `data-id` on the same element: in markup right after `data-part`
  // (other attributes between them aside), or in an attribute object next to it.
  it.each(PARTS_WITH_ID)('%s carries a data-id next to its data-part', (name) => {
    const tag = new RegExp(`data-part=["']${name}["'](\\s+[\\w:-]+(=("[^"]*"|'[^']*'|\\{[^}]*\\}))?)*?\\s+data-id=`);
    const object = new RegExp(`'data-part':\\s*'${name}',\\s*'data-id':`);
    expect(sources.some(([, text]) => tag.test(text) || object.test(text))).toBe(true);
  });

  // A data-part name is theme API: a theme may select it, and it is kept or converted from then on.
  // A node that is only a test or screen-reader hook does not take one.
  it('every data-part in the sources is a registered part', () => {
    const named = new Set(sources.flatMap(([, text]) => [...text.matchAll(/data-part=["']([a-z0-9-]+)["']|'data-part':\s*'([a-z0-9-]+)'/g)].map((m) => m[1] ?? m[2])));
    expect(named.size).toBeGreaterThan(20);
    expect([...named].filter((name) => !(LAYOUT_PARTS as readonly string[]).includes(name)).sort()).toEqual([]);
  });
});
