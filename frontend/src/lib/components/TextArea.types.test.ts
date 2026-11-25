import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import type TextArea from './TextArea.svelte';

type Props = ComponentProps<typeof TextArea>;

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('TextArea props', () => {
  it('the field has one name: its own label, or the id of a label beside it', () => {
    const labelled: Props = { label: 'Details' };
    const byRow: Props = { labelledby: 'row-label' };
    // @ts-expect-error: a field a screen reader would announce as a bare text box
    const unnamed: Props = { value: 'x' };
    // @ts-expect-error: two names, one hiding the other
    const twice: Props = { label: 'Details', labelledby: 'row-label' };
    expect([labelled, byRow, unnamed, twice]).toHaveLength(4);
  });

  it('is a multi-line field: no input type and no autocomplete', () => {
    // @ts-expect-error: a single-line field (TextField) has a type
    const typed: Props = { label: 'Details', type: 'text' };
    // @ts-expect-error: a message is not filled in by the browser
    const filled: Props = { label: 'Details', autocomplete: 'off' };
    expect([typed, filled]).toHaveLength(2);
  });

  it('a counter says itself in the caller\'s words', () => {
    const counted: Props = { label: 'Details', counter: { max: 10, text: () => '', limitText: () => '' } };
    // @ts-expect-error: a count with no words to say it in
    const bare: Props = { label: 'Details', counter: { max: 10 } };
    expect([counted, bare]).toHaveLength(2);
  });
});
