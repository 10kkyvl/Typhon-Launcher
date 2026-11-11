import { describe, expect, it, vi } from 'vitest';
import { renderDom } from '../testing/dom';
import SegmentedHarness from '../testing/SegmentedHarness.svelte';
import type { SegmentOption } from './segmented';

const VIEWS: SegmentOption[] = [
  { id: 'grid', label: 'Grid' },
  { id: 'list', label: 'List' },
  { id: 'table', label: 'Table', disabled: true },
  { id: 'cover', label: 'Covers' },
];

const FILTERS: SegmentOption[] = [
  { id: 'all', label: 'All friends', count: 12 },
  { id: 'away', label: 'Away', count: 3 },
  { id: 'requests', label: 'Requests', count: 0 },
];

const checked = (radio: HTMLElement) => radio.getAttribute('aria-checked');

describe('SegmentedControl', () => {
  it('is a named radio group of radios, one of them checked', () => {
    const { getByRole, getAllByRole } = renderDom(SegmentedHarness, { options: VIEWS, start: 'list' });
    const group = getByRole('radiogroup', { name: 'View' });
    expect(group).toBeTruthy();
    expect(getAllByRole('radio').map((radio) => [radio.textContent?.trim(), checked(radio)])).toEqual([
      ['Grid', 'false'],
      ['List', 'true'],
      ['Table', 'false'],
      ['Covers', 'false'],
    ]);
    expect(getByRole('radio', { name: 'Table' })).toHaveProperty('disabled', true);
  });

  it('takes its name from a visible label next to it', () => {
    const { getByRole } = renderDom(SegmentedHarness, { options: VIEWS, start: 'list', labelled: true });
    expect(getByRole('radiogroup', { name: 'Shown as' })).toBeTruthy();
  });

  it('is one Tab stop: the chosen option, then the control after the group', async () => {
    const { getByRole, user } = renderDom(SegmentedHarness, { options: VIEWS, start: 'list' });
    getByRole('button', { name: 'Before' }).focus();

    await user.tab();
    expect(document.activeElement).toBe(getByRole('radio', { name: 'List' }));
    await user.tab();
    expect(document.activeElement).toBe(getByRole('button', { name: 'After' }));
    await user.tab({ shift: true });
    expect(document.activeElement).toBe(getByRole('radio', { name: 'List' }));
  });

  it('arrows move and choose, skipping a disabled option and wrapping at the ends; Home and End jump', async () => {
    const { getByRole, getByText, user } = renderDom(SegmentedHarness, { options: VIEWS, start: 'list' });
    const radio = (name: string) => getByRole('radio', { name });
    radio('List').focus();

    await user.keyboard('{ArrowRight}');
    expect(document.activeElement).toBe(radio('Covers'));
    expect(checked(radio('Covers'))).toBe('true');
    expect(getByText('cover')).toBeTruthy();

    await user.keyboard('{ArrowDown}');
    expect(document.activeElement).toBe(radio('Grid'));
    await user.keyboard('{ArrowLeft}');
    expect(document.activeElement).toBe(radio('Covers'));
    await user.keyboard('{Home}');
    expect(checked(radio('Grid'))).toBe('true');
    await user.keyboard('{End}');
    expect(checked(radio('Covers'))).toBe('true');
    await user.keyboard('{ArrowUp}');
    expect(checked(radio('List'))).toBe('true');
    expect(document.activeElement).toBe(radio('List'));
  });

  it('a press chooses; a press on a disabled option does not', async () => {
    const { getByRole, getByText, user } = renderDom(SegmentedHarness, { options: VIEWS, start: 'list' });

    await user.click(getByRole('radio', { name: 'Grid' }));
    expect(checked(getByRole('radio', { name: 'Grid' }))).toBe('true');
    expect(checked(getByRole('radio', { name: 'List' }))).toBe('false');

    await user.click(getByRole('radio', { name: 'Table' }));
    expect(getByText('grid')).toBeTruthy();
  });

  it('with nothing chosen yet the first enabled option is the Tab stop', async () => {
    const { getByRole, user } = renderDom(SegmentedHarness, { options: VIEWS, start: 'gone' });
    getByRole('button', { name: 'Before' }).focus();
    await user.tab();
    expect(document.activeElement).toBe(getByRole('radio', { name: 'Grid' }));
  });

  it('a disabled control takes no Tab stop and no choice', async () => {
    const { getByRole, getAllByRole, user } = renderDom(SegmentedHarness, { options: VIEWS, start: 'list', disabled: true });
    expect(getAllByRole('radio').every((radio) => (radio as HTMLButtonElement).disabled)).toBe(true);
    getByRole('button', { name: 'Before' }).focus();
    await user.tab();
    expect(document.activeElement).toBe(getByRole('button', { name: 'After' }));
  });

  it('writes through a function binding, once per choice', async () => {
    const through = vi.fn();
    const { getByRole, user } = renderDom(SegmentedHarness, { options: VIEWS, start: 'list', through });

    await user.click(getByRole('radio', { name: 'Grid' }));
    getByRole('radio', { name: 'Grid' }).focus();
    await user.keyboard('{ArrowRight}');

    expect(through.mock.calls).toEqual([['grid'], ['list']]);
  });

  it('names a segment drawn as an icon and shows that name as a hint', () => {
    const { getByRole } = renderDom(SegmentedHarness, { options: VIEWS, start: 'list', icons: true });
    const grid = getByRole('radio', { name: 'Grid' });
    expect(grid.querySelector('[data-icon="grid"]')).not.toBeNull();
    expect(grid.hasAttribute('title')).toBe(false);
  });

  it('shows a count next to an option and reads it in the name (the filter strips)', () => {
    const { getByRole } = renderDom(SegmentedHarness, { options: FILTERS, start: 'all', variant: 'pill' });
    expect(getByRole('radio', { name: 'All friends 12' })).toBeTruthy();
    expect(getByRole('radio', { name: 'Requests 0' })).toBeTruthy();
    expect(checked(getByRole('radio', { name: 'Away 3' }))).toBe('false');
  });
});
