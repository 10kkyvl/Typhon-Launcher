import { render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import Tabs from './Tabs.svelte';
import type { TabItem } from './tabs';

const TABS: TabItem[] = [
  { id: 'overview', label: 'Overview' },
  { id: 'screenshots', label: 'Screenshots' },
];

afterEach(() => {
  document.body.replaceChildren();
});

const strip = () => screen.getByRole('tablist').parentElement as HTMLElement;

// The marker's colour by default is the theme's --accent-bright (styles/game-tone.test.ts measures the
// rule); a page may hand it another, as the game page hands its game's accent.
describe("Tabs: the chosen tab's marker", () => {
  it('sets no colour of its own when the page names none', () => {
    render(Tabs, { props: { tabs: TABS, value: 'overview', idPrefix: 'a', label: 'Sections' } });
    expect(strip().style.getPropertyValue('--tab-marker')).toBe('');
  });

  it('takes the colour the page hands it', () => {
    render(Tabs, { props: { tabs: TABS, value: 'overview', idPrefix: 'b', label: 'Sections', marker: 'var(--game-accent)' } });
    expect(strip().style.getPropertyValue('--tab-marker')).toBe('var(--game-accent)');
  });
});
