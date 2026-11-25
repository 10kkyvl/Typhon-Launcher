import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import TextFieldHarness from '../testing/TextFieldHarness.svelte';
import TextField from './TextField.svelte';

afterEach(() => {
  document.body.replaceChildren();
});

const box = () => screen.getByRole('textbox') as HTMLInputElement;

describe('TextField: its name and description', () => {
  it('is named by its own visible label, outside of which nothing about the field is said', () => {
    renderDom(TextField, { label: 'E-mail', hint: 'We send the code here' });
    const field = screen.getByRole('textbox', { name: 'E-mail', description: 'We send the code here' });
    expect(field.classList.contains('input')).toBe(true);
    expect(screen.getByText('E-mail').tagName).toBe('LABEL');
    expect(field.hasAttribute('aria-label')).toBe(false);
  });

  it('is named by a label elsewhere on the page, and draws no label of its own', () => {
    const row = document.createElement('span');
    row.id = 'row-label';
    row.textContent = 'Proxy host';
    document.body.append(row);
    const { container } = renderDom(TextField, { labelledby: 'row-label' });
    expect(screen.getByRole('textbox', { name: 'Proxy host' })).toBeTruthy();
    expect(container.querySelector('label')).toBeNull();
  });

  it('is described by its hint, then the text the page names, then its error', () => {
    const note = document.createElement('p');
    note.id = 'page-note';
    note.textContent = 'Kept on this device';
    document.body.append(note);
    renderDom(TextField, { label: 'Host', hint: 'A name or an address', describedby: 'page-note', error: 'Not a host name' });
    expect(description(box())).toBe('A name or an address Kept on this device Not a host name');
  });

  it('takes the id the page gives it, so the page can find it', () => {
    renderDom(TextField, { label: 'Host', id: 'proxy-host' });
    expect(box().id).toBe('proxy-host');
    expect(screen.getByLabelText('Host')).toBe(box());
  });
});

describe('TextField: an error', () => {
  it('marks the field invalid and is said once, beside the field, not in its name', () => {
    const { container } = renderDom(TextField, { label: 'Username', error: 'Already taken' });
    expect(box().getAttribute('aria-invalid')).toBe('true');
    expect(box().classList.contains('invalid')).toBe(true);
    expect(screen.getByRole('textbox', { name: 'Username' })).toBe(box());
    expect(container.textContent?.match(/Already taken/g)).toHaveLength(1);
    expect(screen.getByText('Already taken').closest('label')).toBeNull();
  });

  it('is read with the field when the field mounts with it, and announced when it comes later', async () => {
    const { rerender } = renderDom(TextField, { label: 'Username', error: 'Already taken' });
    expect(screen.queryByRole('alert')).toBeNull();

    await rerender({ label: 'Username', error: '' });
    expect(box().hasAttribute('aria-invalid')).toBe(false);
    expect(box().classList.contains('invalid')).toBe(false);
    expect(box().hasAttribute('aria-describedby')).toBe(false);

    await rerender({ label: 'Username', error: 'Too short' });
    expect(screen.getByRole('alert').textContent).toBe('Too short');
    expect(description(box())).toBe('Too short');
  });

  it('announces a new error that replaces the one shown', async () => {
    const { rerender } = renderDom(TextField, { label: 'Username', error: '' });
    await rerender({ label: 'Username', error: 'Already taken' });
    const first = screen.getByRole('alert');
    await rerender({ label: 'Username', error: 'Too short' });
    const second = screen.getByRole('alert');
    expect(second.textContent).toBe('Too short');
    expect(second).not.toBe(first);
  });
});

describe('TextField: what it asks of the browser', () => {
  it('passes the type, autocomplete, input mode, name, placeholder, length and required state to the input', () => {
    renderDom(TextField, {
      label: 'Login',
      type: 'email',
      autocomplete: 'username',
      inputmode: 'email',
      name: 'login',
      placeholder: 'name@example.test',
      maxlength: 24,
      required: true,
    });
    const field = box();
    expect(field.type).toBe('email');
    expect(field.getAttribute('autocomplete')).toBe('username');
    expect(field.getAttribute('inputmode')).toBe('email');
    expect(field.name).toBe('login');
    expect(field.placeholder).toBe('name@example.test');
    expect(field.maxLength).toBe(24);
    expect(field.required).toBe(true);
  });

  it('is a password field when asked, which no role names as a text box', () => {
    const { container } = renderDom(TextField, { label: 'Password', type: 'password', autocomplete: 'current-password' });
    const field = container.querySelector('input') as HTMLInputElement;
    expect(field.type).toBe('password');
    expect(screen.getByLabelText('Password')).toBe(field);
  });
});

describe('TextField: when it cannot be changed', () => {
  it('refuses edits while read-only and keeps focus', async () => {
    const { user } = renderDom(TextField, { label: 'Login', value: 'player_one', readonly: true });
    await user.click(box());
    await user.keyboard('x');
    expect(box().value).toBe('player_one');
    expect(document.activeElement).toBe(box());
    expect(box().disabled).toBe(false);
  });

  it('is disabled with no reason where its place says why', () => {
    renderDom(TextField, { label: 'Host', disabled: true });
    expect(box().disabled).toBe(true);
    expect(box().hasAttribute('aria-describedby')).toBe(false);
  });

  it('is disabled by its reason, which leads its description on screen', () => {
    renderDom(TextField, { label: 'Host', hint: 'A name or an address', disabledReason: 'The proxy is off' });
    expect(box().disabled).toBe(true);
    expect(description(box())).toBe('The proxy is off A name or an address');
    expect(screen.getByText('The proxy is off')).toBeTruthy();
  });
});

describe('TextField: the value and the events', () => {
  it('writes what is typed to the bound value and shows a value the caller sets', async () => {
    const { user } = renderDom(TextFieldHarness, { initial: 'ab' });
    expect(box().value).toBe('ab');
    await user.type(box(), 'c');
    expect(screen.getByRole('status').textContent).toBe('abc');
    await user.click(screen.getByRole('button', { name: 'Set' }));
    expect(box().value).toBe('set from outside');
  });

  it('hands the page the input itself', async () => {
    const { user } = renderDom(TextFieldHarness);
    await user.click(screen.getByRole('button', { name: 'Focus the field' }));
    expect(document.activeElement).toBe(box());
  });

  it('passes input, change, keydown, focus and blur to the caller', async () => {
    const calls: string[] = [];
    const { user } = renderDom(TextField, {
      label: 'Name',
      oninput: () => calls.push('input'),
      onchange: () => calls.push('change'),
      onkeydown: (event: KeyboardEvent) => calls.push(`keydown ${event.key}`),
      onfocus: () => calls.push('focus'),
      onblur: () => calls.push('blur'),
    });
    await user.click(box());
    await user.keyboard('a');
    await user.tab();
    expect(calls).toEqual(['focus', 'keydown a', 'input', 'keydown Tab', 'change', 'blur']);
  });
});

describe('TextField: one control at its end', () => {
  it('puts the control after the field in the Tab order, inside the field box', async () => {
    const { user } = renderDom(TextFieldHarness, { initial: 'abc' });
    const clear = screen.getByRole('button', { name: 'Clear' });
    expect(clear.closest('.box')).toBe(box().closest('.box'));
    expect(box().classList.contains('trailed')).toBe(true);

    box().focus();
    await user.tab();
    expect(document.activeElement).toBe(clear);
    await user.keyboard('{Enter}');
    expect(box().value).toBe('');
  });

  it('leaves the field its full width when there is no control', () => {
    renderDom(TextField, { label: 'Name' });
    expect(box().classList.contains('trailed')).toBe(false);
    expect(document.querySelector('.trailing')).toBeNull();
  });
});

describe('TextField: no warning', () => {
  it('mounts in every state without a Svelte warning', () => {
    const warn = vi.spyOn(console, 'warn');
    renderDom(TextField, { label: 'A', error: 'Bad', hint: 'Hint', disabledReason: 'Off', readonly: true, type: 'password' });
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});
