import { screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import TextAreaHarness from '../testing/TextAreaHarness.svelte';
import TextArea from './TextArea.svelte';
import textAreaSource from './TextArea.svelte?raw';
import type { TextAreaCounter } from './textArea';

afterEach(() => {
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

const box = () => screen.getByRole('textbox') as HTMLTextAreaElement;
const spoken = () => (screen.getByRole('status').textContent ?? '').trim();
const counter: TextAreaCounter = { max: 10, text: (count: number, max: number) => `${count}/${max}`, limitText: (count: number, max: number) => `said ${count} of ${max}` };

describe('TextArea: a multi-line field', () => {
  it('is a textarea drawn by the shared field rule, opening at the rows asked for', () => {
    renderDom(TextArea, { label: 'Details', rows: 6 });
    expect(box().tagName).toBe('TEXTAREA');
    expect(box().classList.contains('input')).toBe(true);
    expect(box().rows).toBe(6);
    expect(box().getAttribute('dir')).toBe('auto');
  });

  // jsdom lays nothing out: the floor is read from the rule and the value the field carries.
  it('cannot be dragged lower than the rows it opens at', () => {
    renderDom(TextArea, { label: 'Details', rows: 6 });
    expect(box().parentElement?.style.getPropertyValue('--rows')).toBe('6');
    const area = ruleBody(styleOf(textAreaSource, 'TextArea.svelte'), '.area');
    expect(area.resize).toBe('vertical');
    expect(area['min-height']).toBe('calc(var(--rows) * 1lh + 1.8rem + 2px)');
    expect(area.padding).toBe('0.9rem 1.2rem');
  });

  it('makes a new line on Enter, and leaves Ctrl/Cmd+Enter to the caller', async () => {
    const { user } = renderDom(TextAreaHarness, {});
    await user.type(box(), 'one{Enter}two');
    expect(box().value).toBe('one\ntwo');
    expect(within(screen.getByRole('list', { name: 'Sent' })).queryAllByRole('listitem')).toEqual([]);

    await user.keyboard('{Control>}{Enter}{/Control}');
    await user.keyboard('{Meta>}{Enter}{/Meta}');

    expect(within(screen.getByRole('list', { name: 'Sent' })).getAllByRole('listitem').map((item) => item.textContent)).toEqual(['one\ntwo', 'one\ntwo']);
    expect(box().value).toBe('one\ntwo');
  });
});

describe('TextArea: its name and description', () => {
  it('is named by its own visible label, outside of which nothing about the field is said', () => {
    renderDom(TextArea, { label: 'Details', hint: 'What you did and what happened' });
    const field = screen.getByRole('textbox', { name: 'Details', description: 'What you did and what happened' });
    expect(screen.getByText('Details').tagName).toBe('LABEL');
    expect(field.hasAttribute('aria-label')).toBe(false);
  });

  it('is named by a label elsewhere on the page, and draws no label of its own', () => {
    const row = document.createElement('span');
    row.id = 'row-label';
    row.textContent = 'Notes';
    document.body.append(row);
    const { container } = renderDom(TextArea, { labelledby: 'row-label' });
    expect(screen.getByRole('textbox', { name: 'Notes' })).toBeTruthy();
    expect(container.querySelector('label')).toBeNull();
  });

  it('is described by its hint, then the text the page names, then its error', () => {
    const note = document.createElement('p');
    note.id = 'page-note';
    note.textContent = 'Support reads it';
    document.body.append(note);
    renderDom(TextArea, { label: 'Details', hint: 'Plain text', describedby: 'page-note', error: 'Too long' });
    expect(description(box())).toBe('Plain text Support reads it Too long');
  });

  it('takes the id the page gives it', () => {
    renderDom(TextArea, { label: 'Details', id: 'reply' });
    expect(box().id).toBe('reply');
    expect(screen.getByLabelText('Details')).toBe(box());
  });
});

describe('TextArea: an error', () => {
  it('marks the field invalid and is said once, beside the field, not in its name', () => {
    const { container } = renderDom(TextArea, { label: 'Details', error: 'Describe the problem.' });
    expect(box().getAttribute('aria-invalid')).toBe('true');
    expect(box().classList.contains('invalid')).toBe(true);
    expect(screen.getByRole('textbox', { name: 'Details' })).toBe(box());
    expect(container.textContent?.match(/Describe the problem\./g)).toHaveLength(1);
  });

  it('is read with the field when the field mounts with it, and announced when it comes later', async () => {
    const { rerender } = renderDom(TextArea, { label: 'Details', error: 'Describe the problem.' });
    expect(screen.queryByRole('alert')).toBeNull();

    await rerender({ label: 'Details', error: '' });
    expect(box().hasAttribute('aria-invalid')).toBe(false);
    expect(box().hasAttribute('aria-describedby')).toBe(false);

    await rerender({ label: 'Details', error: 'Too long' });
    expect(screen.getByRole('alert').textContent).toBe('Too long');
    expect(description(box())).toBe('Too long');
  });

  it('announces a new error that replaces the one shown', async () => {
    const { rerender } = renderDom(TextArea, { label: 'Details', error: '' });
    await rerender({ label: 'Details', error: 'Describe the problem.' });
    const first = screen.getByRole('alert');
    await rerender({ label: 'Details', error: 'Too long' });
    expect(screen.getByRole('alert')).not.toBe(first);
    expect(screen.getByRole('alert').textContent).toBe('Too long');
  });
});

describe('TextArea: what it asks of the browser', () => {
  it('passes the name, placeholder, hard length, direction and required state to the field', () => {
    renderDom(TextArea, { label: 'Details', name: 'body', placeholder: 'What happened', maxlength: 50, dir: 'ltr', required: true });
    expect(box().name).toBe('body');
    expect(box().placeholder).toBe('What happened');
    expect(box().maxLength).toBe(50);
    expect(box().getAttribute('dir')).toBe('ltr');
    expect(box().required).toBe(true);
  });
});

describe('TextArea: when it cannot be changed', () => {
  it('refuses edits while read-only and keeps focus', async () => {
    const { user } = renderDom(TextArea, { label: 'Details', value: 'Sent text', readonly: true });
    await user.click(box());
    await user.keyboard('x');
    expect(box().value).toBe('Sent text');
    expect(document.activeElement).toBe(box());
    expect(box().disabled).toBe(false);
  });

  it('is disabled by its reason, which leads its description on screen', () => {
    renderDom(TextArea, { label: 'Details', hint: 'Plain text', disabledReason: 'The request is closed' });
    expect(box().disabled).toBe(true);
    expect(description(box())).toBe('The request is closed Plain text');
  });

  it('is disabled with no reason where its place says why', () => {
    renderDom(TextArea, { label: 'Details', disabled: true });
    expect(box().disabled).toBe(true);
    expect(box().hasAttribute('aria-describedby')).toBe(false);
  });
});

describe('TextArea: the value and the events', () => {
  it('writes what is typed to the bound value and shows a value the caller sets', async () => {
    const { user } = renderDom(TextAreaHarness, { initial: 'ab' });
    expect(box().value).toBe('ab');
    await user.type(box(), 'c');
    expect(document.querySelector('output')?.textContent).toBe('abc');
    await user.click(screen.getByRole('button', { name: 'Clear' }));
    expect(box().value).toBe('');
  });

  it('hands the page the field itself', async () => {
    const { user } = renderDom(TextAreaHarness, {});
    await user.click(screen.getByRole('button', { name: 'Focus the field' }));
    expect(document.activeElement).toBe(box());
  });

  it('passes input, change, keydown, focus and blur to the caller', async () => {
    const calls: string[] = [];
    const { user } = renderDom(TextArea, {
      label: 'Details',
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

describe('TextArea: the counter', () => {
  it('shows the count against the limit on screen only, outside the name and the description', async () => {
    const { user } = renderDom(TextArea, { label: 'Details', counter });
    expect(screen.getByText('0/10').getAttribute('aria-hidden')).toBe('true');
    await user.type(box(), 'abc');
    expect(screen.getByText('3/10')).toBeTruthy();
    expect(screen.getByRole('textbox', { name: 'Details' })).toBe(box());
    expect(box().hasAttribute('aria-describedby')).toBe(false);
  });

  it('counts characters, an emoji as one, unless the caller counts otherwise', async () => {
    const { user, rerender } = renderDom(TextArea, { label: 'Details', counter });
    await user.type(box(), 'a😀');
    expect(screen.getByText('2/10')).toBeTruthy();
    await rerender({ label: 'Details', counter: { ...counter, measure: (text: string) => text.trim().length } });
    await user.type(box(), '   ');
    expect(screen.getByText('3/10')).toBeTruthy();
  });

  it('says nothing to assistive tech while the text is far from the limit', async () => {
    const { user } = renderDom(TextArea, { label: 'Details', counter });
    await user.type(box(), 'abcdefgh');
    expect(spoken()).toBe('');
  });

  it('says it once near the limit, once at it and once past it, not on every key', async () => {
    const limitText = vi.fn((count: number, max: number) => `said ${count} of ${max}`);
    const { user } = renderDom(TextArea, { label: 'Details', counter: { ...counter, limitText } });
    await user.type(box(), 'abcdefgh');
    expect(limitText).not.toHaveBeenCalled();

    await user.type(box(), 'i');
    expect(spoken()).toBe('said 9 of 10');
    const near = screen.getByRole('status').textContent;

    await user.type(box(), 'j');
    expect(spoken()).toBe('said 10 of 10');
    await user.type(box(), 'k');
    expect(spoken()).toBe('said 11 of 10');
    await user.type(box(), 'lm');
    expect(spoken()).toBe('said 11 of 10');
    expect(limitText).toHaveBeenCalledTimes(3);
    expect(near).toBe('said 9 of 10');
  });

  it('falls silent once the text is far from the limit again', async () => {
    const { user } = renderDom(TextArea, { label: 'Details', counter, value: 'abcdefghi' });
    expect(spoken()).toBe('');
    await user.type(box(), 'jk');
    expect(spoken()).toBe('said 11 of 10');
    await user.clear(box());
    expect(spoken()).toBe('');
  });

  it('marks a count past the limit', async () => {
    const { user } = renderDom(TextArea, { label: 'Details', counter });
    await user.type(box(), 'abcdefghij');
    expect(screen.getByText('10/10').classList.contains('over')).toBe(false);
    await user.type(box(), 'k');
    expect(screen.getByText('11/10').classList.contains('over')).toBe(true);
  });

  it('has no live region without a counter', () => {
    renderDom(TextArea, { label: 'Details' });
    expect(screen.queryByRole('status')).toBeNull();
  });
});

describe('TextArea: growing with its text', () => {
  // jsdom lays nothing out: the field reports the line height and the content height a browser would.
  function layout(field: HTMLTextAreaElement, content: () => number) {
    Object.defineProperty(field, 'scrollHeight', { configurable: true, get: content });
    const real = window.getComputedStyle;
    vi.spyOn(window, 'getComputedStyle').mockImplementation((element, pseudo) => {
      const style = real(element, pseudo);
      if (element !== field) return style;
      const values: Record<string, string> = { 'line-height': '20px', 'padding-top': '9px', 'padding-bottom': '9px', 'border-top-width': '1px', 'border-bottom-width': '1px' };
      return { ...style, getPropertyValue: (name: string) => values[name] ?? style.getPropertyValue(name) } as CSSStyleDeclaration;
    });
  }

  it('follows its text from its rows down to the rows it grows to, then scrolls; the caller clearing it shrinks it back', async () => {
    const { user } = renderDom(TextAreaHarness, { grow: 5 });
    const field = box();
    let lines = 1;
    layout(field, () => lines * 20 + 18);
    await user.type(field, 'a');
    // Four rows at the least (the default), so a single line keeps that height.
    expect(field.style.height).toBe(`${4 * 20 + 20}px`);
    expect(field.style.overflowY).toBe('hidden');

    lines = 5;
    await user.type(field, 'b');
    expect(field.style.height).toBe(`${5 * 20 + 20}px`);

    lines = 9;
    await user.type(field, 'c');
    expect(field.style.height).toBe(`${5 * 20 + 20}px`);
    expect(field.style.overflowY).toBe('auto');

    lines = 1;
    await user.click(screen.getByRole('button', { name: 'Clear' }));
    await tick();
    expect(field.style.height).toBe(`${4 * 20 + 20}px`);
    expect(field.style.overflowY).toBe('hidden');
  });

  it('measures again once it is shown: mounted hidden with text, it does not stay clipped to its rows', async () => {
    // Hidden (display: none), a field has no content height to read; shown, it has its text's.
    let shown = false;
    const resized: (() => void)[] = [];
    vi.stubGlobal(
      'ResizeObserver',
      class {
        private readonly callback: () => void;
        constructor(callback: () => void) {
          this.callback = callback;
        }
        observe() {
          resized.push(this.callback);
        }
        unobserve() {}
        disconnect() {}
      },
    );
    const real = window.getComputedStyle;
    vi.spyOn(window, 'getComputedStyle').mockImplementation((element, pseudo) => {
      const style = real(element, pseudo);
      if (!(element instanceof HTMLTextAreaElement)) return style;
      const values: Record<string, string> = { 'line-height': '20px', 'padding-top': '9px', 'padding-bottom': '9px', 'border-top-width': '1px', 'border-bottom-width': '1px' };
      return { ...style, getPropertyValue: (name: string) => values[name] ?? style.getPropertyValue(name) } as CSSStyleDeclaration;
    });
    Object.defineProperty(HTMLTextAreaElement.prototype, 'scrollHeight', { configurable: true, get: () => (shown ? 9 * 20 + 18 : 0) });
    Object.defineProperty(HTMLTextAreaElement.prototype, 'offsetWidth', { configurable: true, get: () => (shown ? 300 : 0) });
    try {
      renderDom(TextAreaHarness, { grow: 5, initial: 'one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine' });
      await tick();
      expect(box().style.height).toBe(`${4 * 20 + 20}px`);

      shown = true;
      for (const callback of resized) callback();
      await tick();
      expect(box().style.height).toBe(`${5 * 20 + 20}px`);
      expect(box().style.overflowY).toBe('auto');
    } finally {
      Reflect.deleteProperty(HTMLTextAreaElement.prototype, 'scrollHeight');
      Reflect.deleteProperty(HTMLTextAreaElement.prototype, 'offsetWidth');
      vi.unstubAllGlobals();
    }
  });

  it('leaves its height to the player when it does not grow', async () => {
    const { user } = renderDom(TextAreaHarness, {});
    const field = box();
    layout(field, () => 400);
    await user.type(field, 'a');
    expect(field.style.height).toBe('');
    expect(field.classList.contains('grows')).toBe(false);
  });
});

describe('TextArea: no warning', () => {
  it('mounts in every state without a Svelte warning', () => {
    const warn = vi.spyOn(console, 'warn');
    renderDom(TextArea, { label: 'A', error: 'Bad', hint: 'Hint', disabledReason: 'Off', readonly: true, counter, grow: 6 });
    expect(warn).not.toHaveBeenCalled();
  });
});
