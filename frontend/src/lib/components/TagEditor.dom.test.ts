import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { locale, msg } from '../i18n';
import { user } from '../testing/dom';
import TagEditor from './TagEditor.svelte';

let app: HTMLDivElement;

beforeEach(() => {
  locale.set('en');
  app = document.createElement('div');
  app.id = 'app';
  document.body.append(app);
});

afterEach(() => {
  document.body.replaceChildren();
  locale.set('ru');
});

const remove = (tag: string) => screen.getByRole('button', { name: msg('modals.gameTagsRemove', { tag }) });
const chips = () => [...app.querySelectorAll('.chip')].map((chip) => chip.textContent?.trim());

describe('TagEditor', () => {
  it('removes a tag by its remove button and keeps focus in the list: the next tag, else the previous, else the field', async () => {
    render(TagEditor, { target: app, props: { tags: ['coop', 'rpg', 'indie'], placeholder: 'Tag', label: 'New tag' } });
    const keys = user();

    await keys.click(remove('rpg'));
    await tick();
    expect(chips()).toEqual(['coop', 'indie']);
    expect(document.activeElement).toBe(remove('indie'));

    await keys.keyboard('{Enter}');
    await tick();
    expect(chips()).toEqual(['coop']);
    expect(document.activeElement).toBe(remove('coop'));

    await keys.keyboard(' ');
    await tick();
    expect(chips()).toEqual([]);
    expect(document.activeElement).toBe(screen.getByPlaceholderText('Tag'));
  });

  it('adds a tag typed in the field, and a removed one can be added again', async () => {
    render(TagEditor, { target: app, props: { tags: ['coop'], placeholder: 'Tag', label: 'New tag' } });
    const keys = user();
    await keys.click(remove('coop'));
    await keys.type(screen.getByPlaceholderText('Tag'), 'coop{Enter}');
    expect(chips()).toEqual(['coop']);
  });

  it('names the field by the label or the heading the caller gives', () => {
    render(TagEditor, { target: app, props: { tags: [], placeholder: 'Tag', label: 'New tag' } });
    expect(screen.getByRole('combobox', { name: 'New tag' })).toBeTruthy();
    document.body.replaceChildren(app);
    app.replaceChildren();

    const heading = document.createElement('span');
    heading.id = 'tags-heading';
    heading.textContent = 'Tags (all required)';
    document.body.append(heading);
    render(TagEditor, { target: app, props: { tags: [], placeholder: 'Tag', labelledby: 'tags-heading' } });
    expect(screen.getByRole('combobox', { name: 'Tags (all required)' })).toBeTruthy();
  });

  it('holds Add with the reason while the field is empty, reachable, and a press adds nothing', async () => {
    render(TagEditor, { target: app, props: { tags: [], placeholder: 'Tag', label: 'New tag' } });
    const add = screen.getByRole('button', { name: msg('modals.gameTagsAdd') });

    expect(add).toHaveProperty('disabled', false);
    expect(add.getAttribute('aria-disabled')).toBe('true');
    expect(document.getElementById(add.getAttribute('aria-describedby') ?? '')?.textContent?.trim()).toBe(msg('modals.tagEditorAddEmpty'));
    await user().click(add);
    expect(chips()).toEqual([]);
  });

  it('adds from Add pressed by the keyboard and hands the focus back to the field for the next tag', async () => {
    render(TagEditor, { target: app, props: { tags: [], placeholder: 'Tag', label: 'New tag' } });
    const keys = user();
    const field = screen.getByRole('combobox', { name: 'New tag' });
    await keys.type(field, 'coop');
    await keys.tab();
    const add = screen.getByRole('button', { name: msg('modals.gameTagsAdd') });
    expect(document.activeElement).toBe(add);

    await keys.keyboard('{Enter}');

    expect(chips()).toEqual(['coop']);
    expect(add).toHaveProperty('disabled', false);
    expect(document.activeElement).toBe(field);
    expect((field as HTMLInputElement).value).toBe('');
  });

  it('adds from Add pressed by pointer and puts the focus in the field', async () => {
    render(TagEditor, { target: app, props: { tags: [], placeholder: 'Tag', label: 'New tag' } });
    const keys = user();
    const field = screen.getByRole('combobox', { name: 'New tag' });
    await keys.type(field, 'rpg');

    await keys.click(screen.getByRole('button', { name: msg('modals.gameTagsAdd') }));

    expect(chips()).toEqual(['rpg']);
    expect(document.activeElement).toBe(field);
  });

  it('adds nothing on the Enter that ends an input method composition (key code 229)', async () => {
    render(TagEditor, { target: app, props: { tags: [], placeholder: 'Tag', label: 'New tag' } });
    const field = screen.getByRole('combobox', { name: 'New tag' });
    await fireEvent.input(field, { target: { value: 'タグ' } });

    await fireEvent.keyDown(field, { key: 'Enter', keyCode: 229 });
    expect(chips()).toEqual([]);

    await fireEvent.keyDown(field, { key: 'Enter' });
    expect(chips()).toEqual(['タグ']);
  });
});
