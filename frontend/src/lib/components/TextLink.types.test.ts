import { createRawSnippet, type ComponentProps } from 'svelte';
import { describe, expect, it } from 'vitest';
import type TextLink from './TextLink.svelte';

type Props = ComponentProps<typeof TextLink>;

const text = createRawSnippet(() => ({ render: () => '<span>guide</span>' }));

// Held by the type checker (npm run check); the run only keeps the file in the suite.
describe('TextLink props', () => {
  it('goes to an address or runs an action, and only an action can be disabled', () => {
    const link: Props = { href: 'https://example.com', children: text };
    const action: Props = { onclick: () => {}, disabled: true, children: text };
    // @ts-expect-error: neither an address nor an action
    const nothing: Props = { children: text };
    // @ts-expect-error: a link cannot be disabled; an action that is not available is a button
    const disabledLink: Props = { href: 'https://example.com', disabled: true, children: text };
    // @ts-expect-error: a native title instead of a hint
    const titled: Props = { onclick: () => {}, title: 'Opens the guide', children: text };
    expect([link, action, nothing, disabledLink, titled]).toHaveLength(5);
  });
});
