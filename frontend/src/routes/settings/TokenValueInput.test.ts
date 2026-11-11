import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import TokenValueInput from './TokenValueInput.svelte';
import { tokenRangeText } from '../../lib/theme/tokenRanges';

const field = (name: string, value: string) => render(TokenValueInput, { props: { id: 'f', name, value, onchange: () => {} } }).body;

describe('the theme token value field', () => {
  it('is a number input with the range and step of a ranged token', () => {
    const html = field('--ambient-dim', '0.6');
    expect(html).toMatch(/type="number"/);
    expect(html).toMatch(/min="0"/);
    expect(html).toMatch(/max="1"/);
    expect(html).toMatch(/step="0.01"/);
    expect(html).not.toMatch(/aria-invalid="true"/);
  });

  it('keeps free text for the other kinds', () => {
    const html = field('--radius-md', '1.2rem');
    expect(html).toMatch(/type="text"/);
    expect(html).not.toMatch(/min=|max=|step=/);
  });

  it('says what is wrong with a value outside the range, on the field', () => {
    const html = field('--ambient-dim', '2');
    expect(html).toMatch(/aria-invalid="true"/);
    expect(html).toContain(tokenRangeText({ name: '--ambient-dim', min: 0, max: 1 }));
    expect(html).toMatch(/aria-describedby="[^"]*f-range/);
  });
});
