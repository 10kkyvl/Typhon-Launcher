import { render, screen } from '@testing-library/svelte';
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
    render(TagEditor, { target: app, props: { tags: ['coop', 'rpg', 'indie'], placeholder: 'Tag' } });
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
    render(TagEditor, { target: app, props: { tags: ['coop'], placeholder: 'Tag' } });
    const keys = user();
    await keys.click(remove('coop'));
    await keys.type(screen.getByPlaceholderText('Tag'), 'coop{Enter}');
    expect(chips()).toEqual(['coop']);
  });
});
