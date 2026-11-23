import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { msg } from '../i18n';
import { user } from '../testing/dom';
import { description } from '../testing/hints';
import RateLimitInput from './RateLimitInput.svelte';

afterEach(() => {
  document.body.replaceChildren();
});

function setup(value: number, more: { describedby?: string; disabled?: boolean } = {}) {
  const label = document.createElement('span');
  label.id = 'limit-label';
  label.textContent = 'Download limit';
  const hint = document.createElement('span');
  hint.id = 'limit-hint';
  hint.textContent = 'Maximum incoming speed';
  document.body.append(label, hint);
  const onchange = vi.fn();
  render(RateLimitInput, { props: { value, presets: [10, 25], labelledby: 'limit-label', onchange, ...more } });
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

  it('without a description or disabled, has neither on the list or the number field', () => {
    const { field } = setup(3 * 1024 * 1024);
    const input = screen.getByRole('textbox', { name: msg('ui.speedMbps') });
    for (const element of [field(), input]) {
      expect(element.hasAttribute('aria-describedby')).toBe(false);
      expect(element.hasAttribute('disabled')).toBe(false);
    }
  });

  it('is described by the text its caller names, the list and the number field both', () => {
    const { field } = setup(3 * 1024 * 1024, { describedby: 'limit-hint' });
    expect(description(field())).toBe('Maximum incoming speed');
    expect(description(screen.getByRole('textbox', { name: msg('ui.speedMbps') }))).toBe('Maximum incoming speed');
  });

  it('turns off the list and the number field together, the value still shown', async () => {
    const { onchange, keys, field } = setup(3 * 1024 * 1024, { disabled: true });
    const input = screen.getByRole('textbox', { name: msg('ui.speedMbps') }) as HTMLInputElement;
    expect(field().hasAttribute('disabled')).toBe(true);
    expect(input.disabled).toBe(true);
    expect(input.value).toBe('3');

    await keys.click(field());
    await keys.type(input, '7{Enter}');

    expect(screen.queryByRole('option')).toBeNull();
    expect(onchange).not.toHaveBeenCalled();
  });
});
