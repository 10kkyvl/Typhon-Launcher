import type { ComponentProps } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import IntegerInput from './IntegerInput.svelte';
import Select from './Select.svelte';
import Slider from './Slider.svelte';
import Toggle from './Toggle.svelte';

// A settings row names its control with its label and describes it with its hint, by id: every base
// control a row holds takes the hint's id, and a field takes a <label for> through its own id.

beforeEach(() => {
  const hint = document.createElement('p');
  hint.id = 'row-hint';
  hint.textContent = 'What the setting does';
  const label = document.createElement('span');
  label.id = 'row-label';
  label.textContent = 'The setting';
  document.body.append(hint, label);
});

afterEach(() => {
  document.body.replaceChildren();
});

describe('a base control described by the text beside it', () => {
  it('the switch', () => {
    const { getByRole } = renderDom(Toggle, { labelledby: 'row-label', describedby: 'row-hint' });
    expect(getByRole('switch', { name: 'The setting', description: 'What the setting does' })).toBeTruthy();
  });

  it('the select', () => {
    const { getByRole } = renderDom(Select, {
      options: [
        { id: 'a', label: 'First' },
        { id: 'b', label: 'Second' },
      ],
      value: 'a',
      labelledby: 'row-label',
      describedby: 'row-hint',
    });
    expect(getByRole('combobox', { name: 'The setting', description: 'What the setting does' })).toBeTruthy();
  });

  it('the slider', () => {
    const { getByRole } = renderDom(Slider, { position: 1, max: 4, label: 'The setting', hideLabel: true, valueText: '1', describedby: 'row-hint', onchange: vi.fn() });
    expect(getByRole('slider', { name: 'The setting', description: 'What the setting does' })).toBeTruthy();
  });

  it('the number field, named by a label for its id', () => {
    const label = document.createElement('label');
    label.htmlFor = 'threads';
    label.textContent = 'Threads';
    document.body.append(label);
    const { getByRole } = renderDom(IntegerInput, { id: 'threads', value: 4, min: 1, max: 16, describedby: 'row-hint', onchange: vi.fn() });
    expect(getByRole('textbox', { name: 'Threads', description: 'What the setting does' })).toBeTruthy();
  });

  // A row's `labelFor` names the field by its visible label: an `aria-label` beside it would take over
  // and the screen reader would say a name nobody sees.
  it('the number field named by a label for its id keeps that name when a label is passed too', () => {
    const label = document.createElement('label');
    label.htmlFor = 'threads';
    label.textContent = 'Threads';
    document.body.append(label);
    const props = { id: 'threads', label: 'Hidden copy', value: 4, min: 1, max: 16, onchange: vi.fn() } as unknown as ComponentProps<typeof IntegerInput>;
    const { getByRole } = renderDom(IntegerInput, props);
    expect(getByRole('textbox', { name: 'Threads' }).hasAttribute('aria-label')).toBe(false);
  });
});

// The callers written before these props pass none of them: their controls carry no new attribute.
describe('a base control given no hint and no id', () => {
  it('the number field keeps its own name, and has no id and no description', () => {
    const { getByRole } = renderDom(IntegerInput, { value: 4, min: 1, max: 16, label: 'Threads', onchange: vi.fn() });
    const field = getByRole('textbox', { name: 'Threads' });
    expect(field.getAttributeNames().sort()).toEqual(['aria-invalid', 'aria-label', 'class', 'inputmode', 'type']);
  });

  it('the select has no description', () => {
    const { getByRole } = renderDom(Select, { options: [{ id: 'a', label: 'First' }], value: 'a', label: 'Order' });
    expect(getByRole('combobox', { name: 'Order' }).hasAttribute('aria-describedby')).toBe(false);
  });

  it('the slider has no description', () => {
    const { getByRole } = renderDom(Slider, { position: 1, max: 4, label: 'Speed', valueText: '1', onchange: vi.fn() });
    expect(getByRole('slider', { name: 'Speed' }).hasAttribute('aria-describedby')).toBe(false);
  });
});
