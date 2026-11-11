import { describe, expect, it } from 'vitest';
import { renderDom } from '../testing/dom';
import TabsHarness from '../testing/TabsHarness.svelte';
import type { TabItem } from './tabs';

const TABS: TabItem[] = [
  { id: 'general', label: 'General' },
  { id: 'downloads', label: 'Downloads', count: 4 },
  { id: 'beta', label: 'Beta', disabled: true },
  { id: 'about', label: 'About' },
];

const selected = (tab: HTMLElement) => tab.getAttribute('aria-selected');

describe('Tabs', () => {
  it('is a named tab list whose chosen tab and its panel name each other', () => {
    const { getByRole, getAllByRole } = renderDom(TabsHarness, { tabs: TABS, start: 'downloads' });
    expect(getByRole('tablist', { name: 'Sections' })).toBeTruthy();
    expect(getAllByRole('tab').map((tab) => selected(tab))).toEqual(['false', 'true', 'false', 'false']);

    const tab = getByRole('tab', { name: 'Downloads 4' });
    const panel = getByRole('tabpanel', { name: 'Downloads 4' });
    expect(tab.getAttribute('aria-controls')).toBe(panel.id);
    expect(panel.getAttribute('aria-labelledby')).toBe(tab.id);
    expect(panel.textContent).toContain('Content of downloads');
    // A tab whose panel is not on the page points at nothing.
    expect(getByRole('tab', { name: 'General' }).hasAttribute('aria-controls')).toBe(false);
    expect(getByRole('tab', { name: 'Beta' })).toHaveProperty('disabled', true);
  });

  it('is one Tab stop, and Tab goes from the chosen tab into its panel', async () => {
    const { getByRole, user } = renderDom(TabsHarness, { tabs: TABS, start: 'downloads' });
    getByRole('button', { name: 'Before' }).focus();

    await user.tab();
    expect(document.activeElement).toBe(getByRole('tab', { name: 'Downloads 4' }));
    await user.tab();
    expect(document.activeElement).toBe(getByRole('tabpanel'));
    await user.tab();
    expect(document.activeElement).toBe(getByRole('button', { name: 'After' }));
  });

  it('arrows move to the next tab and show it, skipping a disabled tab and wrapping; Home and End jump', async () => {
    const { getByRole, user } = renderDom(TabsHarness, { tabs: TABS, start: 'downloads' });
    const tab = (name: string) => getByRole('tab', { name });
    tab('Downloads 4').focus();

    await user.keyboard('{ArrowRight}');
    expect(document.activeElement).toBe(tab('About'));
    expect(selected(tab('About'))).toBe('true');
    expect(getByRole('tabpanel', { name: 'About' }).textContent).toContain('Content of about');

    await user.keyboard('{ArrowRight}');
    expect(selected(tab('General'))).toBe('true');
    await user.keyboard('{ArrowLeft}');
    expect(selected(tab('About'))).toBe('true');
    await user.keyboard('{Home}');
    expect(selected(tab('General'))).toBe('true');
    await user.keyboard('{End}');
    expect(selected(tab('About'))).toBe('true');
    expect(document.activeElement).toBe(tab('About'));
  });

  it('up and down arrows are left to the page', async () => {
    const { getByRole, user } = renderDom(TabsHarness, { tabs: TABS, start: 'downloads' });
    getByRole('tab', { name: 'Downloads 4' }).focus();
    await user.keyboard('{ArrowDown}{ArrowUp}');
    expect(selected(getByRole('tab', { name: 'Downloads 4' }))).toBe('true');
  });

  it('a press shows a tab; a press on a disabled tab does not', async () => {
    const { getByRole, user } = renderDom(TabsHarness, { tabs: TABS, start: 'downloads' });

    await user.click(getByRole('tab', { name: 'General' }));
    expect(getByRole('tabpanel', { name: 'General' })).toBeTruthy();

    await user.click(getByRole('tab', { name: 'Beta' }));
    expect(getByRole('tabpanel', { name: 'General' })).toBeTruthy();
  });

  it('scrolls a strip wider than its room just enough to show the chosen tab', async () => {
    const { getByRole, user } = renderDom(TabsHarness, { tabs: TABS, start: 'general' });
    const list = getByRole('tablist');
    Object.defineProperty(list, 'clientWidth', { configurable: true, value: 200 });
    const lefts: Record<string, number> = { General: 0, 'Downloads 4': 100, Beta: 220, About: 300 };
    for (const [name, left] of Object.entries(lefts)) {
      const tab = getByRole('tab', { name });
      Object.defineProperty(tab, 'offsetLeft', { configurable: true, value: left });
      Object.defineProperty(tab, 'offsetWidth', { configurable: true, value: 90 });
    }

    await user.click(getByRole('tab', { name: 'About' }));
    expect(list.scrollLeft).toBe(190);
    await user.click(getByRole('tab', { name: 'Downloads 4' }));
    expect(list.scrollLeft).toBe(100);
  });
});
