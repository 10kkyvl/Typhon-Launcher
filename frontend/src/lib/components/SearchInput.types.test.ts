import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import { frontendSources } from '../testing/sources';
import type SearchInput from './SearchInput.svelte';

type Props = ComponentProps<typeof SearchInput>;

// The placeholder is an example of what to type and goes away with the first letter: it names
// nothing. Held by the type checker (npm run check); the run keeps the file in the suite and reads
// the call sites.
describe('SearchInput props', () => {
  it('a search field is named by a visible label elsewhere on the page or a label of its own', () => {
    const own: Props = { label: 'Search the library' };
    const visible: Props = { labelledby: 'manual-search-heading' };
    // @ts-expect-error: named by its placeholder only
    const unnamed: Props = { placeholder: 'Search the library' };
    // @ts-expect-error: the name passed around the component
    const around: Props = { 'aria-label': 'Search the library' };
    // @ts-expect-error: two names
    const both: Props = { label: 'Search', labelledby: 'heading' };
    expect([own, visible, unnamed, around, both]).toHaveLength(5);
  });

  it('every search field on the pages has a name', () => {
    const tags = Object.entries(frontendSources)
      .filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'))
      .flatMap(([path, text]) => [...text.matchAll(/<SearchInput\b[^]*?\/>/g)].map((m) => ({ path, tag: m[0] })));
    expect(tags.length).toBeGreaterThanOrEqual(10);
    expect(tags.filter(({ tag }) => !/\b(?:label|labelledby)=/.test(tag)).map(({ path }) => path)).toEqual([]);
  });
});
