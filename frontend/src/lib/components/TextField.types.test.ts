import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import type TextField from './TextField.svelte';

type Props = ComponentProps<typeof TextField>;

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('TextField props', () => {
  it('the field has one name: its own label, or the id of a label beside it', () => {
    const labelled: Props = { label: 'E-mail' };
    const byRow: Props = { labelledby: 'row-label' };
    // @ts-expect-error: a field a screen reader would announce as a bare text box
    const unnamed: Props = { value: 'x' };
    // @ts-expect-error: two names, one hiding the other
    const twice: Props = { label: 'E-mail', labelledby: 'row-label' };
    expect([labelled, byRow, unnamed, twice]).toHaveLength(4);
  });

  it('is a single-line text field: no number, date or checkbox type', () => {
    const password: Props = { label: 'Password', type: 'password' };
    // @ts-expect-error: a number has its own field (IntegerInput)
    const number: Props = { label: 'Count', type: 'number' };
    // @ts-expect-error: not a text field
    const box: Props = { label: 'Agree', type: 'checkbox' };
    expect([password, number, box]).toHaveLength(3);
  });
});
