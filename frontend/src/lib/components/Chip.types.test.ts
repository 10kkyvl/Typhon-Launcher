import type { ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import type Chip from './Chip.svelte';

type Props = ComponentProps<typeof Chip>;

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('Chip props', () => {
  it('a removable chip names its remove button and takes no hint, which would land on that button', () => {
    const named: Props = { onremove: () => {}, removeLabel: 'Remove the tag rpg' };
    // @ts-expect-error: a remove button without a name
    const unnamed: Props = { onremove: () => {} };
    // @ts-expect-error: a hint on a removable chip
    const hinted: Props = { onremove: () => {}, removeLabel: 'Remove the tag rpg', hint: 'Filters by tag' };
    const plain: Props = { onclick: () => {}, hint: 'Filters by tag' };
    expect([named, unnamed, hinted, plain]).toHaveLength(4);
  });

  it('a chip that goes to an address is a link: it is neither on nor off, removable or disabled', () => {
    const link: Props = { href: 'https://example.com', target: '_blank', rel: 'noopener noreferrer' };
    // @ts-expect-error: a link has no on state
    const pressed: Props = { href: 'https://example.com', selected: true };
    // @ts-expect-error: a link is not a removable tag
    const removable: Props = { href: 'https://example.com', onremove: () => {}, removeLabel: 'Remove' };
    // @ts-expect-error: a link cannot be disabled
    const disabled: Props = { href: 'https://example.com', disabled: true };
    expect([link, pressed, removable, disabled]).toHaveLength(4);
  });
});
