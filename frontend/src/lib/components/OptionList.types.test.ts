import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import type OptionList from './OptionList.svelte';
import type { OptionItem } from './optionList';

type Props = ComponentProps<typeof OptionList<OptionItem>>;

const options: OptionItem[] = [{ id: 'a', title: 'C:/Saves/Hades' }];

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('OptionList props', () => {
  it('the list has one name: its own label, or the id of the words above it', () => {
    const labelled: Props = { label: 'Save folder', options };
    const byWords: Props = { labelledby: 'hint', options };
    // @ts-expect-error: a radio group a screen reader would announce with no name
    const unnamed: Props = { options };
    // @ts-expect-error: two names, one hiding the other
    const twice: Props = { label: 'Save folder', labelledby: 'hint', options };
    expect([labelled, byWords, unnamed, twice]).toHaveLength(4);
  });

  it('every row has an id to choose it by and a title to name it by', () => {
    // @ts-expect-error: a row with no name
    const untitled: Props = { label: 'Save folder', options: [{ id: 'a' }] };
    // @ts-expect-error: a row the value cannot point at
    const unkeyed: Props = { label: 'Save folder', options: [{ title: 'C:/Saves' }] };
    // @ts-expect-error: the choice is an id, not a row
    const byRow: Props = { label: 'Save folder', options, onchange: (row: OptionItem) => row };
    expect([untitled, unkeyed, byRow]).toHaveLength(3);
  });
});
