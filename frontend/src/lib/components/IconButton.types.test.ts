import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import type IconButton from './IconButton.svelte';

type Props = ComponentProps<typeof IconButton>;

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('IconButton props', () => {
  it('a count comes with the name that says it', () => {
    const plain: Props = { label: 'Friends' };
    const counted: Props = { label: 'Friends', count: 3, countLabel: 'Friends, 3 requests' };
    // @ts-expect-error: a count a screen reader would not hear
    const silent: Props = { label: 'Friends', count: 3 };
    expect([plain, counted, silent]).toHaveLength(3);
  });
});
