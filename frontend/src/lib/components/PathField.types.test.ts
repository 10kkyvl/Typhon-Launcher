import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import type PathField from './PathField.svelte';

type Props = ComponentProps<typeof PathField>;

const browse = { browseLabel: 'Browse', onbrowse: () => {} };

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('PathField props', () => {
  it('the path has one name: its own label, or the id of a label beside it', () => {
    const labelled: Props = { label: 'Library folder', ...browse };
    const byRow: Props = { labelledby: 'row-label', ...browse };
    // @ts-expect-error: a group and a button a screen reader would announce with no name
    const unnamed: Props = { value: 'D:/Games', ...browse };
    // @ts-expect-error: two names, one hiding the other
    const twice: Props = { label: 'Library folder', labelledby: 'row-label', ...browse };
    expect([labelled, byRow, unnamed, twice]).toHaveLength(4);
  });

  it('always offers to choose another path, in the words and with the picker of its caller', () => {
    const later: Props = { label: 'Library folder', browseLabel: 'Browse', onbrowse: async () => {} };
    // @ts-expect-error: a Browse button with no words
    const wordless: Props = { label: 'Library folder', onbrowse: () => {} };
    // @ts-expect-error: a Browse button that does nothing
    const idle: Props = { label: 'Library folder', browseLabel: 'Browse' };
    // @ts-expect-error: the part waits for a promise of nothing; the caller writes what was picked
    const answers: Props = { label: 'Library folder', browseLabel: 'Browse', onbrowse: async () => 'D:/Games' };
    expect([later, wordless, idle, answers]).toHaveLength(4);
  });
});
