import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import type IntegerInput from './IntegerInput.svelte';

type Props = ComponentProps<typeof IntegerInput>;

const base = { value: 4, min: 1, max: 16, onchange: () => {} };

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('IntegerInput props', () => {
  it('the field has a name: its own label, or an id for a label to name it', () => {
    const labelled: Props = { ...base, label: 'Threads' };
    const byLabelFor: Props = { ...base, id: 'threads' };
    // @ts-expect-error: a field a screen reader would announce as a bare text box
    const unnamed: Props = { ...base };
    // @ts-expect-error: a hidden name that would take over from the visible label
    const twice: Props = { ...base, id: 'threads', label: 'Threads' };
    expect([labelled, byLabelFor, unnamed, twice]).toHaveLength(4);
  });
});
