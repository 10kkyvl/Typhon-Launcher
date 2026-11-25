import { screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ruleBody, styleOf } from '../testing/css';
import { renderDom } from '../testing/dom';
import { description } from '../testing/hints';
import PathFieldHarness from '../testing/PathFieldHarness.svelte';
import PathField from './PathField.svelte';
import source from './PathField.svelte?raw';

afterEach(() => {
  document.body.replaceChildren();
});

const LONG = 'D:/Games/A folder whose name runs on for long enough to need a second line/Typhon Library';

function deferred() {
  let resolve!: (value: string) => void;
  let reject!: (err: unknown) => void;
  const promise = new Promise<string>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const browse = () => screen.getByRole('button', { name: 'Browse' });

describe('PathField: a path shown', () => {
  it('is text in a group named by its label, read whole, and describes the button that changes it', () => {
    const { container } = renderDom(PathField, { label: 'Library folder', value: LONG, browseLabel: 'Browse', onbrowse: vi.fn() });
    const group = screen.getByRole('group', { name: 'Library folder' });
    expect(screen.queryByRole('textbox')).toBeNull();
    expect(container.querySelector('input')).toBeNull();
    expect(screen.getByText(LONG).closest('[role="group"]')).toBe(group);
    expect(screen.getByText('Library folder').closest('label')).toBeNull();
    expect(group.contains(browse())).toBe(true);
    expect(description(browse())).toBe(LONG);
  });

  it("shows the caller's words while there is no path, and the button says them", () => {
    renderDom(PathField, { label: 'Library folder', placeholder: 'No folder chosen', browseLabel: 'Browse', onbrowse: vi.fn() });
    expect(screen.getByText('No folder chosen').classList.contains('empty')).toBe(true);
    expect(description(browse())).toBe('No folder chosen');
  });

  it('is named by a label elsewhere on the page, and draws no label of its own', () => {
    const row = document.createElement('span');
    row.id = 'row-label';
    row.textContent = 'Destination';
    document.body.append(row);
    const { container } = renderDom(PathField, { labelledby: 'row-label', value: 'D:/Games', browseLabel: 'Browse', onbrowse: vi.fn() });
    expect(screen.getByRole('group', { name: 'Destination' })).toBeTruthy();
    expect(container.querySelector('label, .label')).toBeNull();
  });

  it('describes the button by the path, what is known about it, the hint, the text the page names and the error, in that order', () => {
    const note = document.createElement('p');
    note.id = 'page-note';
    note.textContent = 'Games already there stay';
    document.body.append(note);
    renderDom(PathFieldHarness, { initial: 'D:/Games', pick: vi.fn() });
    expect(description(browse())).toBe('D:/Games Free: 12 GB');

    document.body.replaceChildren(note);
    renderDom(PathField, { label: 'Library folder', value: 'D:/Games', hint: 'Pick an empty folder', describedby: 'page-note', error: 'Not writable', browseLabel: 'Browse', onbrowse: vi.fn() });
    expect(description(browse())).toBe('D:/Games Pick an empty folder Games already there stay Not writable');
  });

  it('carries no native title anywhere, a long path and a refused button included', () => {
    const { container } = renderDom(PathField, { label: 'Library folder', value: LONG, disabledReason: 'A move is running', browseLabel: 'Browse', onbrowse: vi.fn() });
    expect(container.querySelectorAll('[title]')).toHaveLength(0);
    expect(document.querySelectorAll('[title]')).toHaveLength(0);
  });

  // jsdom lays nothing out: the rule that lets a long path wrap is read from the source.
  it('lets a long path wrap anywhere instead of cutting it', () => {
    const css = styleOf(source, 'PathField.svelte');
    const path = ruleBody(css, '.path');
    expect(path['overflow-wrap']).toBe('anywhere');
    expect(path['min-width']).toBe('0');
    expect(css).not.toMatch(/text-overflow|white-space:\s*nowrap|-webkit-line-clamp/);
  });
});

describe('PathField: Browse', () => {
  it('is busy while the caller picks, takes no second press, keeps focus, and shows what was picked', async () => {
    const answer = deferred();
    const pick = vi.fn(() => answer.promise);
    const { user } = renderDom(PathFieldHarness, { pick });

    await user.click(browse());
    expect(browse().getAttribute('aria-busy')).toBe('true');
    expect(document.activeElement).toBe(browse());
    await user.click(browse());
    await user.keyboard('{Enter}');
    expect(pick).toHaveBeenCalledTimes(1);

    answer.resolve(LONG);
    await vi.waitFor(() => expect(browse().hasAttribute('aria-busy')).toBe(false));
    expect(screen.getByText(LONG, { selector: '.path' })).toBeTruthy();
    expect(document.activeElement).toBe(browse());

    await user.click(browse());
    expect(pick).toHaveBeenCalledTimes(2);
  });

  // Two activations in one task, before Svelte has drawn the busy state: the Button's guard, which
  // stands only while it is given a `reason` (an empty one included), refuses the second.
  it('two presses in one task reach the caller once', () => {
    const onbrowse = vi.fn(() => new Promise<void>(() => {}));
    renderDom(PathField, { label: 'Library folder', browseLabel: 'Browse', onbrowse });
    browse().click();
    browse().click();
    expect(onbrowse).toHaveBeenCalledTimes(1);
  });

  it('takes a caller that answers at once, with no promise', async () => {
    const onbrowse = vi.fn();
    const { user } = renderDom(PathField, { label: 'Library folder', browseLabel: 'Browse', onbrowse });
    await user.click(browse());
    await user.click(browse());
    expect(onbrowse).toHaveBeenCalledTimes(2);
    expect(browse().hasAttribute('aria-busy')).toBe(false);
  });

  it('is ready again after a caller whose picker failed, and logs the failure', async () => {
    const answer = deferred();
    const pick = vi.fn(() => answer.promise);
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {});
    const { user } = renderDom(PathFieldHarness, { pick });

    await user.click(browse());
    answer.reject(new Error('picker gone'));

    await vi.waitFor(() => expect(browse().hasAttribute('aria-busy')).toBe(false));
    expect(logged).toHaveBeenCalledWith('path field: browse', new Error('picker gone'));
    expect(document.activeElement).toBe(browse());
    logged.mockRestore();
  });
});

describe('PathField: an error', () => {
  it('is read with the path when the field mounts with it, and announced when it comes later', async () => {
    const props = { label: 'Library folder', value: 'D:/Games', browseLabel: 'Browse', onbrowse: vi.fn() };
    const { rerender, container } = renderDom(PathField, { ...props, error: 'Not writable' });
    expect(screen.queryByRole('alert')).toBeNull();
    expect(container.textContent?.match(/Not writable/g)).toHaveLength(1);

    await rerender({ ...props, error: '' });
    expect(description(browse())).toBe('D:/Games');

    await rerender({ ...props, error: 'No room' });
    expect(screen.getByRole('alert').textContent).toBe('No room');
    expect(description(browse())).toBe('D:/Games No room');
  });

  it('a changed error is announced again once the field was seen without one', async () => {
    const props = { label: 'Library folder', value: 'D:/Games', browseLabel: 'Browse', onbrowse: vi.fn() };
    const { rerender } = renderDom(PathField, { ...props, error: '' });
    await rerender({ ...props, error: 'Not writable' });
    const first = screen.getByRole('alert');
    await rerender({ ...props, error: 'No room' });
    const second = screen.getByRole('alert');
    expect(second.textContent).toBe('No room');
    expect(second).not.toBe(first);
  });

  it('mounted with an error, a changed error that never cleared is read, not announced', async () => {
    const props = { label: 'Library folder', value: 'D:/Games', browseLabel: 'Browse', onbrowse: vi.fn() };
    const { rerender } = renderDom(PathField, { ...props, error: 'Not writable' });
    await rerender({ ...props, error: 'No room' });
    expect(screen.queryByRole('alert')).toBeNull();
    expect(description(browse())).toBe('D:/Games No room');
  });
});

describe('PathField: editable', () => {
  it('is a text field named by its label and described as TextField is', () => {
    const note = document.createElement('p');
    note.id = 'page-note';
    note.textContent = 'Games already there stay';
    document.body.append(note);
    renderDom(PathField, { label: 'Install to', editable: true, value: 'D:/Games', hint: 'A folder on any drive', describedby: 'page-note', error: 'Not writable', browseLabel: 'Browse', onbrowse: vi.fn() });
    const field = screen.getByRole('textbox', { name: 'Install to' }) as HTMLInputElement;
    expect(field.value).toBe('D:/Games');
    expect(field.classList.contains('input')).toBe(true);
    expect(field.getAttribute('aria-invalid')).toBe('true');
    expect(field.classList.contains('invalid')).toBe(true);
    expect(field.getAttribute('spellcheck')).toBe('false');
    expect(description(field)).toBe('A folder on any drive Games already there stay Not writable');
    expect(screen.getByText('Install to').tagName).toBe('LABEL');
    expect(screen.queryByRole('group')).toBeNull();
  });

  it('is described by what is known about the path; Browse beside it is described by nothing', () => {
    renderDom(PathFieldHarness, { editable: true, initial: 'D:/Games', pick: vi.fn() });
    expect(screen.getByText('Free: 12 GB')).toBeTruthy();
    expect(description(screen.getByRole('textbox', { name: 'Install to' }))).toBe('Free: 12 GB');
    expect(browse().hasAttribute('aria-describedby')).toBe(false);
  });

  it('reads the reason, what is known about the path, the hint, the page text and the error, in that order', () => {
    const note = document.createElement('p');
    note.id = 'page-note';
    note.textContent = 'Games already there stay';
    document.body.append(note);
    const words = { hint: 'A folder on any drive', describedby: 'page-note', error: 'Not writable' };
    renderDom(PathFieldHarness, { editable: true, initial: 'D:/Games', pick: vi.fn(), ...words, disabledReason: 'A move is running' });
    expect(description(screen.getByRole('textbox', { name: 'Install to' }))).toBe('A move is running Free: 12 GB A folder on any drive Games already there stay Not writable');

    document.body.replaceChildren(note);
    renderDom(PathFieldHarness, { initial: 'D:/Games', pick: vi.fn(), ...words });
    expect(description(browse())).toBe('D:/Games Free: 12 GB A folder on any drive Games already there stay Not writable');
  });

  it('writes what is typed to the bound value, shows a value the caller sets, and takes what Browse picked', async () => {
    const answer = deferred();
    const { user } = renderDom(PathFieldHarness, { editable: true, initial: 'D:/Gam', pick: () => answer.promise });
    const field = screen.getByRole('textbox', { name: 'Install to' }) as HTMLInputElement;

    await user.type(field, 'es');
    expect(screen.getByRole('status').textContent).toBe('D:/Games');
    await user.click(screen.getByRole('button', { name: 'Set' }));
    expect(field.value).toBe('set from outside');

    await user.click(browse());
    answer.resolve('E:/Library');
    await vi.waitFor(() => expect(field.value).toBe('E:/Library'));
    expect(screen.getByRole('status').textContent).toBe('E:/Library');
  });

  it('hands the page the text field itself', async () => {
    const { user } = renderDom(PathFieldHarness, { editable: true, pick: vi.fn() });
    await user.click(screen.getByRole('button', { name: 'Focus the field' }));
    expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Install to' }));
  });
});

describe('PathField: when it cannot be changed', () => {
  it('is refused by its reason: the field is disabled, the reason leads its description, Browse keeps focus and does nothing', async () => {
    const onbrowse = vi.fn();
    const { user } = renderDom(PathField, { label: 'Install to', editable: true, hint: 'A folder on any drive', disabledReason: 'A move is running', browseLabel: 'Browse', onbrowse });
    const field = screen.getByRole('textbox', { name: 'Install to' }) as HTMLInputElement;
    expect(field.disabled).toBe(true);
    expect(description(field)).toBe('A move is running A folder on any drive');
    expect(screen.getByText('A move is running', { selector: '.reason' })).toBeTruthy();

    expect(browse().getAttribute('aria-disabled')).toBe('true');
    browse().focus();
    expect(document.activeElement).toBe(browse());
    await user.keyboard('{Enter}');
    await user.click(browse());
    expect(onbrowse).not.toHaveBeenCalled();
    expect(description(browse())).toBe('A move is running');
  });

  it('a shown path refused by its reason: Browse is described by the path and the reason, once each', () => {
    renderDom(PathField, { label: 'Library folder', value: 'D:/Games', disabledReason: 'A move is running', browseLabel: 'Browse', onbrowse: vi.fn() });
    expect(description(browse())).toBe('D:/Games A move is running');
  });

  it('is disabled with no reason where its place says why', async () => {
    const onbrowse = vi.fn();
    const { user } = renderDom(PathField, { label: 'Library folder', value: 'D:/Games', disabled: true, browseLabel: 'Browse', onbrowse });
    expect(browse()).toHaveProperty('disabled', true);
    await user.click(browse());
    expect(onbrowse).not.toHaveBeenCalled();
  });
});

describe('PathField: no warning', () => {
  it('mounts in every state without a Svelte warning', () => {
    const warn = vi.spyOn(console, 'warn');
    renderDom(PathField, { label: 'A', value: 'D:/x', error: 'Bad', hint: 'Hint', disabledReason: 'Off', browseLabel: 'Browse', onbrowse: vi.fn() });
    renderDom(PathField, { labelledby: 'x', editable: true, error: 'Bad', hint: 'Hint', disabledReason: 'Off', browseLabel: 'Browse', onbrowse: vi.fn() });
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });
});
