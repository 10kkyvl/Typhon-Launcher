import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';
import { LAYOUT_PARTS } from './layout';

describe('layout parts', () => {
  const sources = Object.entries(frontendSources).filter(([path]) => !path.endsWith('theme/layout.ts'));

  it.each(LAYOUT_PARTS)('%s has an element in the sources', (name) => {
    const literal = new RegExp(`data-part=["']${name}["']|'data-part':\\s*'${name}'`);
    expect(sources.some(([, text]) => literal.test(text))).toBe(true);
  });
});
