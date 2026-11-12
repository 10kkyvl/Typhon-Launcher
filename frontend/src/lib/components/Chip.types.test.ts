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

  // Its text is not a control: only the remove button acts, so what would make the text one is refused.
  it('a removable chip takes no action, button type or end mark of its own', () => {
    const busy: Props = { onremove: () => {}, removeLabel: 'Remove', busy: true, 'data-tag': 'rpg' };
    const refused: Props = { onremove: () => {}, removeLabel: 'Remove', disabled: true, reason: 'Saving the tags' };
    // @ts-expect-error: pressing the text does nothing
    const pressed: Props = { onremove: () => {}, removeLabel: 'Remove', onclick: () => {} };
    // @ts-expect-error: the text is not a submit button
    const submit: Props = { onremove: () => {}, removeLabel: 'Remove', type: 'submit' };
    // @ts-expect-error: the remove button is the end of a removable chip
    const marked: Props = { onremove: () => {}, removeLabel: 'Remove', end: undefined as unknown as import('svelte').Snippet };
    expect([busy, refused, pressed, submit, marked]).toHaveLength(5);
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
