import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import RateLimitInput from './RateLimitInput.svelte';

afterEach(() => {
  document.body.replaceChildren();
});

function setup(value: number) {
  const label = document.createElement('span');
  label.id = 'limit-label';
  label.textContent = 'Download limit';
  document.body.append(label);
  const onchange = vi.fn();
  render(RateLimitInput, { props: { value, presets: [10, 25], labelledby: 'limit-label', onchange } });
  return { onchange, keys: user(), field: () => screen.getByRole('combobox', { name: 'Download limit' }) };
}

describe('RateLimitInput', () => {
  it('choosing a custom value from the list puts focus in the number field', async () => {
    const { onchange, keys, field } = setup(0);

    await keys.click(field());
    await keys.click(screen.getByRole('option', { name: msg('ui.customValue') }));

    const input = await screen.findByRole('textbox', { name: msg('ui.speedMbps') });
    await vi.waitFor(() => expect(document.activeElement).toBe(input));
    expect(onchange).not.toHaveBeenCalled();
  });

  it('choosing a preset from the keyboard reports it in bytes', async () => {
    const { onchange, keys, field } = setup(0);
    field().focus();

    await keys.keyboard('{ArrowDown}{ArrowDown}{Enter}');

    expect(onchange).toHaveBeenCalledExactlyOnceWith(10 * 1024 * 1024);
    expect(document.activeElement).toBe(field());
  });
});
