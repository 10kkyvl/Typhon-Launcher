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
});
