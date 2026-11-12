import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import { renderDom } from '../../lib/testing/dom';
import TokenValueInput from './TokenValueInput.svelte';

afterEach(() => {
  document.body.replaceChildren();
});

const props = (value: string) => ({ id: 'f', name: '--ambient-dim', value, onchange: () => {} });

// Chromium reports a number field that holds a half-typed number ("0.", "-", "1e") or nothing as an
// empty value, with an input event: the theme hears '' while the user is still typing.
describe('the theme token value field while it is typed', () => {
  it('says nothing about an empty value until the user leaves the field', async () => {
    const view = renderDom(TokenValueInput, props('0.5'));
    const field = screen.getByRole('spinbutton');
    field.focus();

    await view.rerender(props(''));

    expect(screen.queryByRole('alert')).toBeNull();
    expect(field.hasAttribute('aria-invalid')).toBe(false);

    await view.user.tab();

    expect(screen.getByRole('alert')).toBeTruthy();
    expect(field.getAttribute('aria-invalid')).toBe('true');
  });

  it('says at once that a typed number is outside the range', async () => {
    const view = renderDom(TokenValueInput, props('0.5'));
    const field = screen.getByRole('spinbutton');
    field.focus();

    await view.rerender(props('2'));

    expect(screen.getByRole('alert')).toBeTruthy();
    expect(field.getAttribute('aria-invalid')).toBe('true');
  });
});
