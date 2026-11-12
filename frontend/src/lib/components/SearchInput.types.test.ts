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
    // Each case below is refused for one reason only: the others in it are valid.
    // @ts-expect-error: named by its placeholder only
    const unnamed: Props = { placeholder: 'Search the library' };
    // @ts-expect-error: a name passed around the component, beside its own
    const around: Props = { label: 'Search the library', 'aria-label': 'Search' };
    // @ts-expect-error: a label element's id passed around the component, beside its own
    const aroundLabelledby: Props = { labelledby: 'manual-search-heading', 'aria-labelledby': 'other' };
    // @ts-expect-error: two names
    const both: Props = { label: 'Search', labelledby: 'heading' };
    expect([own, visible, unnamed, around, aroundLabelledby, both]).toHaveLength(6);
  });

  it('every search field on the pages has a name', () => {
    const tags = Object.entries(frontendSources)
      .filter(([path]) => path.endsWith('.svelte') && !path.startsWith('lib/testing/'))
      .flatMap(([path, text]) => [...text.matchAll(/<SearchInput\b[^]*?\/>/g)].map((m) => ({ path, tag: m[0] })));
    expect(tags.length).toBeGreaterThanOrEqual(10);
    expect(tags.filter(({ tag }) => !/\b(?:label|labelledby)=/.test(tag)).map(({ path }) => path)).toEqual([]);
  });
});
